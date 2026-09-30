package modbus

import "time"

// RegisterType identifies which Modbus register bank to read.
type RegisterType string

const (
	RegisterHolding RegisterType = "holding" // FC03
	RegisterInput   RegisterType = "input"   // FC04
	RegisterCoil    RegisterType = "coil"    // FC01
)

// DataType controls how raw register bytes are decoded.
type DataType string

const (
	DataUint16  DataType = "uint16"
	DataInt16   DataType = "int16"
	DataUint32  DataType = "uint32"
	DataInt32   DataType = "int32"
	DataFloat32 DataType = "float32"
	DataBool    DataType = "bool" // for coils
)

// Point is a single register-to-key mapping on a slave device.
type Point struct {
	Key        string       // telemetry key sent to platform
	Address    uint16       // Modbus register address (0-based)
	Length     uint16       // number of registers (1 for 16-bit, 2 for 32-bit)
	DataType   DataType     // how to decode raw bytes
	Scale      float64      // multiply decoded value (0 → treated as 1)
	RegisterType RegisterType
}

// SlaveConfig describes one Modbus slave (field device).
type SlaveConfig struct {
	DeviceID   string        // platform device UUID
	UnitID     byte          // Modbus slave address (1–247)
	PollPeriod time.Duration // how often to poll this slave
	Points     []Point
}

// ConnectorConfig holds the TCP connector parameters.
type ConnectorConfig struct {
	ID      string        // connector UUID
	Host    string        // e.g. "192.168.1.10"
	Port    int           // default 502
	Timeout time.Duration // per-request timeout
	Slaves  []SlaveConfig
}
