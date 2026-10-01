package connector

import "time"

// Type identifies a connector protocol. Values match the "type" field the
// SolarTools platform sends in config_full (Python: ConnectorType).
type Type string

const (
	TypeModbusTCP       Type = "modbus_tcp"
	TypeModbusRTU       Type = "modbus_rtu"
	TypeModbusRTUBridge Type = "modbus_rtu_bridge"
	TypeOPCUA           Type = "opcua"
	TypeSocket          Type = "socket"
	TypeS7              Type = "s7"
	TypeREST            Type = "rest"
	TypeSNMP            Type = "snmp"
	TypeBACnet          Type = "bacnet"
	TypeMQTTSub         Type = "mqtt_sub"
	TypeEnerjisaAPI     Type = "enerjisa_api"
)

// Status represents the lifecycle state of a connector (Python: ConnectorStatus).
type Status string

const (
	StatusStopped    Status = "stopped"
	StatusConnecting Status = "connecting"
	StatusActive     Status = "active"
	StatusError      Status = "error"
)

// DataPoint is a single telemetry reading from a field device.
// Connectors produce DataPoints; the publisher consumes them.
type DataPoint struct {
	ConnectorID string
	DeviceID    string
	Key         string
	Value       any
	Ts          int64 // Unix milliseconds
}

// NewDataPoint creates a DataPoint stamped with the current time.
func NewDataPoint(connectorID, deviceID, key string, value any) DataPoint {
	return DataPoint{
		ConnectorID: connectorID,
		DeviceID:    deviceID,
		Key:         key,
		Value:       value,
		Ts:          time.Now().UnixMilli(),
	}
}
