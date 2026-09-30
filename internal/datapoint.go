package internal

import "time"

// DataPoint is a single telemetry reading from a field device.
// Connectors produce DataPoints; the publisher consumes them.
type DataPoint struct {
	ConnectorID string
	DeviceID    string
	Key         string
	Value       any
	Ts          int64 // Unix milliseconds
}

// NewDataPoint creates a DataPoint with the current timestamp.
func NewDataPoint(connectorID, deviceID, key string, value any) DataPoint {
	return DataPoint{
		ConnectorID: connectorID,
		DeviceID:    deviceID,
		Key:         key,
		Value:       value,
		Ts:          time.Now().UnixMilli(),
	}
}
