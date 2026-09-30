// Package modbus provides a Modbus TCP connector.
// Each slave polls on its own ticker (fixes SB-388).
// TCP client is protected by a mutex (fixes SB-386).
package modbus

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"math"
	"net"
	"sync"
	"time"

	"github.com/ekicimustafa/industrial-gateway/connector"
	"github.com/ekicimustafa/industrial-gateway/internal"
)

// TCPConnector reads Modbus TCP holding/input registers and coils.
// It implements connector.Connector.
type TCPConnector struct {
	cfg    ConnectorConfig
	status connector.Status
	mu     sync.Mutex // guards conn AND status

	conn    net.Conn
	txID    uint16 // Modbus transaction ID counter
	cancel  context.CancelFunc
}

// NewTCP creates a TCPConnector from the given config.
func NewTCP(cfg ConnectorConfig) *TCPConnector {
	if cfg.Port == 0 {
		cfg.Port = 502
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 5 * time.Second
	}
	return &TCPConnector{cfg: cfg, status: connector.StatusStopped}
}

// Start connects to the TCP host and begins polling all slaves.
// Each slave runs in its own goroutine with its own ticker — no slave
// blocks another, and each honours its own PollPeriod (SB-388 fix).
func (c *TCPConnector) Start(ctx context.Context, out chan<- internal.DataPoint) error {
	c.setStatus(connector.StatusConnecting)
	if err := c.dial(); err != nil {
		c.setStatus(connector.StatusError)
		return fmt.Errorf("modbus tcp connect %s:%d: %w", c.cfg.Host, c.cfg.Port, err)
	}
	c.setStatus(connector.StatusActive)

	childCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel

	var wg sync.WaitGroup
	for _, slave := range c.cfg.Slaves {
		wg.Add(1)
		go func(s SlaveConfig) {
			defer wg.Done()
			c.pollSlave(childCtx, s, out)
		}(slave)
	}

	go func() {
		wg.Wait()
		c.setStatus(connector.StatusStopped)
	}()

	return nil
}

func (c *TCPConnector) pollSlave(ctx context.Context, s SlaveConfig, out chan<- internal.DataPoint) {
	period := s.PollPeriod
	if period <= 0 {
		period = 10 * time.Second
	}
	ticker := time.NewTicker(period)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, pt := range s.Points {
				value, err := c.readPoint(s.UnitID, pt)
				if err != nil {
					slog.Warn("modbus read failed",
						"device", s.DeviceID,
						"key", pt.Key,
						"err", err,
					)
					c.reconnect()
					break // retry on next tick
				}
				out <- internal.NewDataPoint(c.cfg.ID, s.DeviceID, pt.Key, value)
			}
		}
	}
}

// readPoint sends a Modbus request for one Point and decodes the response.
// The TCP connection is protected by c.mu (SB-386 fix).
func (c *TCPConnector) readPoint(unit byte, pt Point) (any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return nil, fmt.Errorf("not connected")
	}

	var fc byte
	switch pt.RegisterType {
	case RegisterHolding:
		fc = 0x03
	case RegisterInput:
		fc = 0x04
	case RegisterCoil:
		fc = 0x01
	default:
		fc = 0x03
	}

	length := pt.Length
	if length == 0 {
		length = 1
	}

	req := c.buildADU(unit, fc, pt.Address, length)
	c.conn.SetDeadline(time.Now().Add(c.cfg.Timeout)) //nolint:errcheck

	if _, err := c.conn.Write(req); err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}

	// Read MBAP header (7 bytes) then PDU
	header := make([]byte, 7)
	if _, err := readFull(c.conn, header); err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	pduLen := int(binary.BigEndian.Uint16(header[4:6])) - 1 // minus unit byte
	pdu := make([]byte, pduLen)
	if _, err := readFull(c.conn, pdu); err != nil {
		return nil, fmt.Errorf("read pdu: %w", err)
	}

	// pdu[0] = function code, pdu[1] = byte count, pdu[2:] = data
	if len(pdu) < 2 {
		return nil, fmt.Errorf("short response")
	}
	if pdu[0]&0x80 != 0 {
		return nil, fmt.Errorf("modbus exception 0x%02X", pdu[1])
	}
	data := pdu[2:]
	return decode(data, pt.DataType, pt.Scale)
}

func (c *TCPConnector) buildADU(unit, fc byte, addr, qty uint16) []byte {
	c.txID++
	adu := make([]byte, 12)
	binary.BigEndian.PutUint16(adu[0:], c.txID) // transaction ID
	binary.BigEndian.PutUint16(adu[2:], 0)      // protocol ID
	binary.BigEndian.PutUint16(adu[4:], 6)      // length of remaining
	adu[6] = unit                                // unit ID
	adu[7] = fc                                  // function code
	binary.BigEndian.PutUint16(adu[8:], addr)
	binary.BigEndian.PutUint16(adu[10:], qty)
	return adu
}

func decode(data []byte, dt DataType, scale float64) (any, error) {
	if scale == 0 {
		scale = 1
	}
	switch dt {
	case DataBool:
		if len(data) < 1 {
			return nil, fmt.Errorf("not enough bytes for bool")
		}
		return data[0] != 0, nil
	case DataUint16:
		if len(data) < 2 {
			return nil, fmt.Errorf("not enough bytes for uint16")
		}
		return float64(binary.BigEndian.Uint16(data)) * scale, nil
	case DataInt16:
		if len(data) < 2 {
			return nil, fmt.Errorf("not enough bytes for int16")
		}
		return float64(int16(binary.BigEndian.Uint16(data))) * scale, nil
	case DataUint32:
		if len(data) < 4 {
			return nil, fmt.Errorf("not enough bytes for uint32")
		}
		return float64(binary.BigEndian.Uint32(data)) * scale, nil
	case DataInt32:
		if len(data) < 4 {
			return nil, fmt.Errorf("not enough bytes for int32")
		}
		return float64(int32(binary.BigEndian.Uint32(data))) * scale, nil
	case DataFloat32:
		if len(data) < 4 {
			return nil, fmt.Errorf("not enough bytes for float32")
		}
		bits := binary.BigEndian.Uint32(data)
		return float64(math.Float32frombits(bits)) * scale, nil
	default:
		return nil, fmt.Errorf("unknown data type %q", dt)
	}
}

// Stop cancels all slave goroutines and closes the TCP connection.
func (c *TCPConnector) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
}

// Status returns the current connector state.
func (c *TCPConnector) Status() connector.Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status
}

// HandleRPC writes a single register value (FC06) to the device.
func (c *TCPConnector) HandleRPC(_ context.Context, req connector.RPCRequest) connector.RPCResponse {
	addr, ok1 := req.Params["address"].(float64)
	value, ok2 := req.Params["value"].(float64)
	if !ok1 || !ok2 {
		return connector.RPCResponse{ID: req.ID, Error: "params must have address and value (number)"}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return connector.RPCResponse{ID: req.ID, Error: "not connected"}
	}

	// FC06: write single register
	adu := make([]byte, 12)
	c.txID++
	binary.BigEndian.PutUint16(adu[0:], c.txID)
	binary.BigEndian.PutUint16(adu[2:], 0)
	binary.BigEndian.PutUint16(adu[4:], 6)
	adu[6] = 0xFF // broadcast unit — caller should pass target unit if needed
	adu[7] = 0x06
	binary.BigEndian.PutUint16(adu[8:], uint16(addr))
	binary.BigEndian.PutUint16(adu[10:], uint16(value))

	c.conn.SetDeadline(time.Now().Add(c.cfg.Timeout)) //nolint:errcheck
	if _, err := c.conn.Write(adu); err != nil {
		return connector.RPCResponse{ID: req.ID, Error: err.Error()}
	}

	// Read echo back (12 bytes)
	resp := make([]byte, 12)
	if _, err := readFull(c.conn, resp); err != nil {
		return connector.RPCResponse{ID: req.ID, Error: err.Error()}
	}
	return connector.RPCResponse{ID: req.ID, Success: true}
}

func (c *TCPConnector) dial() error {
	conn, err := net.DialTimeout("tcp",
		fmt.Sprintf("%s:%d", c.cfg.Host, c.cfg.Port),
		c.cfg.Timeout,
	)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
	return nil
}

func (c *TCPConnector) reconnect() {
	c.mu.Lock()
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	c.mu.Unlock()

	slog.Info("modbus tcp reconnecting", "host", c.cfg.Host)
	for i := 0; i < 5; i++ {
		time.Sleep(time.Duration(i+1) * 2 * time.Second)
		if err := c.dial(); err == nil {
			slog.Info("modbus tcp reconnected")
			return
		}
	}
	slog.Error("modbus tcp reconnect failed, giving up")
	c.setStatus(connector.StatusError)
}

func (c *TCPConnector) setStatus(s connector.Status) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status = s
}

// readFull reads exactly len(buf) bytes from conn.
func readFull(conn net.Conn, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}
