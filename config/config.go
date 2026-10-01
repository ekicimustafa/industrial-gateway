// Package config loads and validates the gateway configuration from a JSON file.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/ekicimustafa/industrial-gateway/connector/modbus"
	gw_mqtt "github.com/ekicimustafa/industrial-gateway/mqtt"
)

// -------------------------------------------------------------------
// Top-level config shape
// -------------------------------------------------------------------

// Gateway is the root configuration object.
type Gateway struct {
	MQTT       MQTTConfig       `json:"mqtt"`
	Buffer     BufferConfig     `json:"buffer"`
	Connectors []ConnectorEntry `json:"connectors"`
}

// MQTTConfig maps to the "mqtt" block in config.json.
type MQTTConfig struct {
	BrokerURL string `json:"broker_url"`
	ClientID  string `json:"client_id"`
	Username  string `json:"username"` // ThingsBoard device access token
	Password  string `json:"password"`
}

// BufferConfig controls the SQLite offline queue.
type BufferConfig struct {
	Path    string `json:"path"`     // default: "data/telemetry.db"
	MaxRows int64  `json:"max_rows"` // default: 100000
}

// ConnectorEntry is one entry in the "connectors" array.
// "type" determines which connector struct to build.
type ConnectorEntry struct {
	Type string `json:"type"` // "modbus_tcp"

	// Modbus TCP fields — present only when type == "modbus_tcp"
	ModbusTCP *ModbusTCPConfig `json:"modbus_tcp,omitempty"`
}

// -------------------------------------------------------------------
// Modbus TCP config shapes (JSON → modbus.ConnectorConfig)
// -------------------------------------------------------------------

type ModbusTCPConfig struct {
	ID        string        `json:"id"`
	Host      string        `json:"host"`
	Port      int           `json:"port"`       // default: 502
	TimeoutMs int           `json:"timeout_ms"` // default: 5000
	Slaves    []SlaveConfig `json:"slaves"`
}

type SlaveConfig struct {
	DeviceID     string        `json:"device_id"`
	UnitID       uint8         `json:"unit_id"`
	PollPeriodMs int           `json:"poll_period_ms"` // default: 5000
	Points       []PointConfig `json:"points"`
}

type PointConfig struct {
	Key          string  `json:"key"`
	Address      uint16  `json:"address"`
	Length       uint16  `json:"length"`        // default: 1
	DataType     string  `json:"data_type"`     // "uint16", "int16", "float32", ...
	Scale        float64 `json:"scale"`         // default: 1.0
	RegisterType string  `json:"register_type"` // "holding", "input", "coil" — default: "holding"
}

// -------------------------------------------------------------------
// Load reads and validates config.json
// -------------------------------------------------------------------

// Load reads the JSON file at path, validates required fields, and
// returns a ready-to-use Gateway config.
func Load(path string) (*Gateway, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}

	var cfg Gateway
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}

	applyDefaults(&cfg)

	if err := validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func applyDefaults(cfg *Gateway) {
	if cfg.Buffer.Path == "" {
		cfg.Buffer.Path = "data/telemetry.db"
	}
	if cfg.Buffer.MaxRows == 0 {
		cfg.Buffer.MaxRows = 100_000
	}
	for i := range cfg.Connectors {
		if cfg.Connectors[i].Type == "modbus_tcp" && cfg.Connectors[i].ModbusTCP != nil {
			m := cfg.Connectors[i].ModbusTCP
			if m.Port == 0 {
				m.Port = 502
			}
			if m.TimeoutMs == 0 {
				m.TimeoutMs = 5000
			}
			for j := range m.Slaves {
				if m.Slaves[j].PollPeriodMs == 0 {
					m.Slaves[j].PollPeriodMs = 5000
				}
				for k := range m.Slaves[j].Points {
					pt := &m.Slaves[j].Points[k]
					if pt.Length == 0 {
						pt.Length = 1
					}
					if pt.Scale == 0 {
						pt.Scale = 1.0
					}
					if pt.RegisterType == "" {
						pt.RegisterType = "holding"
					}
					if pt.DataType == "" {
						pt.DataType = "uint16"
					}
				}
			}
		}
	}
}

func validate(cfg *Gateway) error {
	if cfg.MQTT.BrokerURL == "" {
		return fmt.Errorf("config: mqtt.broker_url is required")
	}
	if len(cfg.Connectors) == 0 {
		return fmt.Errorf("config: at least one connector is required")
	}
	for i, c := range cfg.Connectors {
		switch c.Type {
		case "modbus_tcp":
			if c.ModbusTCP == nil {
				return fmt.Errorf("config: connector[%d] type=modbus_tcp but modbus_tcp block is missing", i)
			}
			if c.ModbusTCP.Host == "" {
				return fmt.Errorf("config: connector[%d].modbus_tcp.host is required", i)
			}
		default:
			return fmt.Errorf("config: connector[%d] unknown type %q", i, c.Type)
		}
	}
	return nil
}

// -------------------------------------------------------------------
// Converters: config types → connector types
// -------------------------------------------------------------------

// ToMQTTConfig converts MQTTConfig to the mqtt package's Config type.
func (c MQTTConfig) ToMQTTConfig(clientID string) gw_mqtt.Config {
	id := c.ClientID
	if id == "" {
		id = clientID
	}
	return gw_mqtt.Config{
		BrokerURL:   c.BrokerURL,
		ClientID:    id,
		Username:    c.Username,
		Password:    c.Password,
		TopicPrefix: "v1/devices/me",
	}
}

// ToModbusConfig converts a ModbusTCPConfig to modbus.ConnectorConfig.
func (m *ModbusTCPConfig) ToModbusConfig() modbus.ConnectorConfig {
	slaves := make([]modbus.SlaveConfig, len(m.Slaves))
	for i, s := range m.Slaves {
		points := make([]modbus.Point, len(s.Points))
		for j, p := range s.Points {
			points[j] = modbus.Point{
				Key:          p.Key,
				Address:      p.Address,
				Length:       p.Length,
				DataType:     modbus.DataType(p.DataType),
				Scale:        p.Scale,
				RegisterType: modbus.RegisterType(p.RegisterType),
			}
		}
		slaves[i] = modbus.SlaveConfig{
			DeviceID:   s.DeviceID,
			UnitID:     s.UnitID,
			PollPeriod: time.Duration(s.PollPeriodMs) * time.Millisecond,
			Points:     points,
		}
	}
	return modbus.ConnectorConfig{
		ID:      m.ID,
		Host:    m.Host,
		Port:    m.Port,
		Timeout: time.Duration(m.TimeoutMs) * time.Millisecond,
		Slaves:  slaves,
	}
}
