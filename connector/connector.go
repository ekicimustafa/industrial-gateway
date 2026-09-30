package connector

import (
	"context"

	"github.com/ekicimustafa/industrial-gateway/internal"
)

// Status represents the lifecycle state of a connector.
type Status string

const (
	StatusStopped    Status = "stopped"
	StatusConnecting Status = "connecting"
	StatusActive     Status = "active"
	StatusError      Status = "error"
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
	Start(ctx context.Context, out chan<- internal.DataPoint) error
	Stop()
	Status() Status
	HandleRPC(ctx context.Context, req RPCRequest) RPCResponse
}
