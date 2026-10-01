package modbus

// Prototype RPC types, kept only so the pre-port TCP connector compiles.
// Step 5 of docs/PORTING.md replaces this connector with the port of
// modbus_connector.py / slave.py, which uses connector.RPCRequest.

// RPCRequest is an inbound write command for the prototype connector.
type RPCRequest struct {
	ID       string
	DeviceID string
	Method   string
	Params   map[string]any
}

// RPCResponse is the prototype connector's write result.
type RPCResponse struct {
	ID      string
	Success bool
	Payload map[string]any
	Error   string
}
