# industrial-gateway

![CI](https://github.com/ekicimustafa/industrial-gateway/actions/workflows/ci.yml/badge.svg)

A production-grade IoT gateway written in Go for industrial automation environments.
Reads data from field devices over Modbus TCP and forwards it to an MQTT broker with a durable offline buffer — no data is lost during network outages.

```
Field Device ──[Modbus TCP]──► Connector ──► chan DataPoint
                                                    │
                                             SQLite Buffer  ◄── survives power loss
                                                    │
                                             MQTT Publisher ──[MQTT]──► Broker
```

## Features

- **Offline-first** — all telemetry is written to a local SQLite buffer before any network interaction; data is never lost on disconnect
- **Per-device poll rate** — each Modbus slave polls on its own ticker at its own interval; a fast device never blocks a slow one
- **Safe concurrent RPC** — write commands and poll loops share the TCP connection through a mutex; no interleaved responses
- **Automatic reconnect** — both the Modbus TCP client and MQTT publisher reconnect with exponential back-off
- **JSON config** — a single `config.json` describes all connectors, devices and register mappings
- **Zero CGO** — uses [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite), a pure-Go SQLite driver; cross-compiles without a C toolchain

## Supported protocols

| Protocol | Status | Notes |
|----------|--------|-------|
| Modbus TCP | ✅ | FC01 / FC03 / FC04 read, FC06 write |
| Modbus RTU | 🔜 planned | |
| Siemens S7 | 🔜 planned | |

## Getting started

### Prerequisites

- Go 1.23+

### Build

```bash
git clone https://github.com/ekicimustafa/industrial-gateway.git
cd industrial-gateway
go build -o gateway ./cmd/gateway
```

### Configure

Copy the example config and edit it:

```bash
cp config.example.json config.json
```

Minimum required fields:

```json
{
  "mqtt": {
    "broker_url": "tcp://your-broker:1883",
    "username": "YOUR_DEVICE_ACCESS_TOKEN"
  },
  "connectors": [
    {
      "type": "modbus_tcp",
      "modbus_tcp": {
        "id": "inverter-01",
        "host": "192.168.1.10",
        "slaves": [
          {
            "device_id": "your-platform-device-uuid",
            "unit_id": 1,
            "poll_period_ms": 5000,
            "points": [
              { "key": "voltage", "address": 0, "data_type": "uint16", "scale": 0.1 },
              { "key": "power_w", "address": 2, "length": 2, "data_type": "uint32" }
            ]
          }
        ]
      }
    }
  ]
}
```

### Run

```bash
./gateway -config config.json
```

## Configuration reference

### `mqtt`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `broker_url` | string | — | MQTT broker address, e.g. `tcp://host:1883` |
| `client_id` | string | `"industrial-gateway"` | MQTT client identifier |
| `username` | string | — | Access token / username |
| `password` | string | `""` | Password (leave empty for token-only auth) |

### `buffer`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `path` | string | `"data/telemetry.db"` | SQLite file path |
| `max_rows` | int | `100000` | Max buffered rows; oldest are trimmed when exceeded |

### `connectors[].modbus_tcp`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `host` | string | — | Device IP address |
| `port` | int | `502` | Modbus TCP port |
| `timeout_ms` | int | `5000` | Per-request timeout |

### `slaves[]`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `unit_id` | int | — | Modbus slave address (1–247) |
| `poll_period_ms` | int | `5000` | Poll interval for this slave |

### `points[]`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `key` | string | — | Telemetry key sent to the platform |
| `address` | int | — | Modbus register address (0-based) |
| `length` | int | `1` | Number of registers (use `2` for 32-bit types) |
| `data_type` | string | `"uint16"` | `uint16` · `int16` · `uint32` · `int32` · `float32` · `bool` |
| `scale` | float | `1.0` | Multiplied against the decoded value |
| `register_type` | string | `"holding"` | `holding` (FC03) · `input` (FC04) · `coil` (FC01) |

## Architecture

The gateway is built around three composable pieces:

```
┌─────────────────────────────────────────────────────────────────┐
│ connector.Connector (interface)                                  │
│   Start(ctx, out chan<- DataPoint) error                         │
│   Stop()                                                        │
│   HandleRPC(ctx, RPCRequest) RPCResponse                        │
└─────────────────────────────────────────────────────────────────┘
          │ implemented by
          ▼
┌──────────────────────┐     ┌──────────────────────────────────┐
│ modbus.TCPConnector  │     │ mqtt.Publisher                   │
│                      │     │                                  │
│  per-slave goroutine │     │  writeLoop: chan → SQLite        │
│  per-slave ticker    │──►──│  flushLoop: SQLite → MQTT        │
│  sync.Mutex on conn  │     │  auto-reconnect with back-off    │
└──────────────────────┘     └──────────────────────────────────┘
```

**Why Go?**
The original Python implementation had several concurrency issues that are hard to express safely in `asyncio`:
concurrent TCP access without a lock, a shared poll interval that ignored per-device configuration, and no
graceful handling of corrupt config files. Go's goroutine-per-slave model and `sync.Mutex` make these
constraints explicit and compiler-enforced.

## License

MIT
