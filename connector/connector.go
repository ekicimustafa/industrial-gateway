package connector

import (
	"context"
)

// RPCRequest is an inbound command from the platform targeting a device.
type RPCRequest struct {
	ID       string
	DeviceID string
	Method   string
	Params   map[string]any
}

// RPCResponse is the result sent back to the platform.
type RPCResponse struct {
	ID      string
	Success bool
	Payload map[string]any
	Error   string
}

// Connector is the interface every protocol connector must implement.
//
// Lifecycle:
//
//	Start(ctx, out) — connect and begin polling; send DataPoints to out
//	Stop()          — graceful shutdown; must return after ctx is cancelled
//	Status()        — current state
//	HandleRPC()     — write a value to the field device, return result
type Connector interface {
	Start(ctx context.Context, out chan<- DataPoint) error
	Stop()
	Status() Status
	HandleRPC(ctx context.Context, req RPCRequest) RPCResponse
}
