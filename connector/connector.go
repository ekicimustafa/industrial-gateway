// Package connector defines the contract every protocol connector fulfils
// and Base, the shared run loop they embed (Python: connectors/base.py).
package connector

import (
	"context"
	"encoding/json"
)

// RPCRequest is a platform → field device command
// (TB-style {"method": ..., "params": ..., "id": ...}).
type RPCRequest struct {
	ID     string
	Method string
	Params json.RawMessage
}

// Connector is what the gateway service manages. Concrete connectors get
// most of it by embedding *Base and only override what they support.
type Connector interface {
	ID() string
	Type() Type
	Name() string
	Status() Status

	// Running reports whether the run loop goroutine is alive.
	Running() bool

	// Start launches the run loop in the background and returns at once.
	Start(ctx context.Context)

	// Stop cancels the run loop, waits for it, then disconnects.
	// It always returns within the configured stop/disconnect timeouts.
	Stop()

	// ManagementProbe is a best-effort reachability check
	// (mgmt_connector_probe RPC).
	ManagementProbe(ctx context.Context, deviceName string) map[string]any

	// DeviceConnectionStates maps device id → "connected" | "unreachable" |
	// "unknown" for heartbeat reporting.
	DeviceConnectionStates() map[string]string

	// HandleRPC executes a device command; the result is sent back to the
	// platform as the RPC ack.
	HandleRPC(ctx context.Context, deviceName string, req RPCRequest) map[string]any
}

// Protocol is the part each connector implements and Base drives
// (Python: the abstract _connect / _run / _disconnect methods).
type Protocol interface {
	// Connect establishes the field connection. Base calls it again after
	// Run fails, so it must reuse a healthy connection or open a new one.
	Connect(ctx context.Context) error

	// Run polls devices until ctx is cancelled. Returning nil before that
	// is treated as unexpected and retried.
	Run(ctx context.Context) error

	// Disconnect releases the field connection. It should honour ctx;
	// Base abandons it after the disconnect timeout either way.
	Disconnect(ctx context.Context) error
}

// Factory builds a connector from its config. Telemetry goes to out.
type Factory func(cfg Config, out chan<- DataPoint) (Connector, error)
