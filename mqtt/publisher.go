// Package mqtt handles MQTT connectivity and telemetry publishing.
package mqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/ekicimustafa/industrial-gateway/internal"
	"github.com/ekicimustafa/industrial-gateway/mqtt/buffer"
)

// Config holds the MQTT broker connection parameters.
type Config struct {
	BrokerURL   string // e.g. "tcp://broker:1883"
	ClientID    string
	Username    string // ThingsBoard device access token goes here
	Password    string
	TopicPrefix string // e.g. "v1/devices/me" — ThingsBoard telemetry topic
}

// Publisher buffers DataPoints to SQLite and flushes them to an MQTT broker.
// It reconnects automatically on disconnect.
type Publisher struct {
	cfg    Config
	buf    *buffer.Buffer
	client mqtt.Client
	mu     sync.Mutex // guards client

	flushInterval time.Duration
	batchSize     int
}

// NewPublisher creates a Publisher backed by the given SQLite buffer.
func NewPublisher(cfg Config, buf *buffer.Buffer) *Publisher {
	return &Publisher{
		cfg:           cfg,
		buf:           buf,
		flushInterval: 500 * time.Millisecond,
		batchSize:     100,
	}
}

// Run starts two goroutines: one writes incoming DataPoints to the buffer,
// the other flushes the buffer to MQTT on a ticker.
// Run blocks until ctx is cancelled.
func (p *Publisher) Run(ctx context.Context, in <-chan internal.DataPoint) {
	var wg sync.WaitGroup

	// Writer: channel → buffer
	wg.Add(1)
	go func() {
		defer wg.Done()
		p.writeLoop(ctx, in)
	}()

	// Flusher: buffer → MQTT
	wg.Add(1)
	go func() {
		defer wg.Done()
		p.flushLoop(ctx)
	}()

	wg.Wait()
}

func (p *Publisher) writeLoop(ctx context.Context, in <-chan internal.DataPoint) {
	// Collect up to batchSize points or wait 50ms before writing to SQLite
	// to reduce write amplification.
	const collectTimeout = 50 * time.Millisecond

	batch := make([]internal.DataPoint, 0, p.batchSize)
	timer := time.NewTimer(collectTimeout)
	defer timer.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := p.buf.Write(batch); err != nil {
			slog.Error("buffer write failed", "err", err)
		}
		batch = batch[:0]
	}

	for {
		select {
		case <-ctx.Done():
			flush() // drain whatever is left
			return
		case dp, ok := <-in:
			if !ok {
				flush()
				return
			}
			batch = append(batch, dp)
			if len(batch) >= p.batchSize {
				flush()
				if !timer.Stop() {
					<-timer.C
				}
				timer.Reset(collectTimeout)
			}
		case <-timer.C:
			flush()
			timer.Reset(collectTimeout)
		}
	}
}

func (p *Publisher) flushLoop(ctx context.Context) {
	ticker := time.NewTicker(p.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.flush(); err != nil {
				slog.Warn("mqtt flush failed", "err", err)
			}
		}
	}
}

func (p *Publisher) flush() error {
	rows, err := p.buf.ReadBatch(p.batchSize)
	if err != nil || len(rows) == 0 {
		return err
	}

	client := p.getClient()
	if client == nil || !client.IsConnected() {
		if err := p.connect(); err != nil {
			return nil // not connected yet — data stays in buffer
		}
		client = p.getClient()
	}

	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		payload, err := json.Marshal(row.Values)
		if err != nil {
			slog.Error("json marshal failed", "err", err)
			continue
		}

		// ThingsBoard telemetry topic with explicit timestamp:
		// v1/devices/me/telemetry  (platform resolves device by MQTT username)
		topic := p.cfg.TopicPrefix + "/telemetry"
		msg := fmt.Sprintf(`{"ts":%d,"values":%s}`, row.Ts, payload)

		token := client.Publish(topic, 1, false, msg)
		token.Wait()
		if token.Error() != nil {
			slog.Warn("mqtt publish failed", "device", row.DeviceID, "err", token.Error())
			return token.Error()
		}
		ids = append(ids, row.ID)
	}

	if len(ids) > 0 {
		return p.buf.Delete(ids)
	}
	return nil
}

func (p *Publisher) connect() error {
	opts := mqtt.NewClientOptions().
		AddBroker(p.cfg.BrokerURL).
		SetClientID(p.cfg.ClientID).
		SetUsername(p.cfg.Username).
		SetPassword(p.cfg.Password).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetOnConnectHandler(func(_ mqtt.Client) {
			slog.Info("mqtt connected", "broker", p.cfg.BrokerURL)
		}).
		SetConnectionLostHandler(func(_ mqtt.Client, err error) {
			slog.Warn("mqtt connection lost", "err", err)
		})

	client := mqtt.NewClient(opts)
	token := client.Connect()
	token.WaitTimeout(10 * time.Second)
	if err := token.Error(); err != nil {
		return fmt.Errorf("mqtt connect: %w", err)
	}

	p.mu.Lock()
	p.client = client
	p.mu.Unlock()
	return nil
}

func (p *Publisher) getClient() mqtt.Client {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.client
}

// Disconnect cleanly closes the MQTT connection.
func (p *Publisher) Disconnect() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.client != nil && p.client.IsConnected() {
		p.client.Disconnect(500)
	}
}
