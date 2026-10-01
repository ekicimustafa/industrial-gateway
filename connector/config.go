package connector

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Config is one entry of the "connectors" array in the platform's
// config_full payload and in the local conf/gateway.json snapshot
// (Python: ConnectorConfig).
//
// The protocol-specific document under "config" (TB-style
// {"master": {"slaves": [...]}}) is kept raw; each connector parses it
// into its own typed structs.
type Config struct {
	ID   string `json:"id"`
	Type Type   `json:"type"`
	Name string `json:"name"`

	// Enabled is a pointer so a missing field can be told apart from
	// "enabled": false — the platform omits it for enabled connectors.
	Enabled *bool `json:"enabled,omitempty"`

	Config  json.RawMessage  `json:"config,omitempty"`  // TB-style protocol document
	Params  map[string]any   `json:"params,omitempty"`  // legacy flat params
	Devices []map[string]any `json:"devices,omitempty"` // legacy device list
}

// ParseConfig decodes and validates one connector entry.
func ParseConfig(data []byte) (Config, error) {
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("connector config: %w", err)
	}
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// Validate checks the fields every connector needs.
// Python raised KeyError only when a key was absent; an empty id is
// rejected here too, since connectors are keyed by id.
func (c Config) Validate() error {
	var missing []string
	if c.ID == "" {
		missing = append(missing, "id")
	}
	if c.Type == "" {
		missing = append(missing, "type")
	}
	if c.Name == "" {
		missing = append(missing, "name")
	}
	if len(missing) > 0 {
		return fmt.Errorf("connector config %q: missing %s", c.ID, strings.Join(missing, ", "))
	}
	return nil
}

// IsEnabled reports whether the connector should run; absent means enabled.
func (c Config) IsEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}

// LogLevel returns the per-connector "logLevel" override, looked up in
// params first and then in the protocol document. Empty when unset.
func (c Config) LogLevel() string {
	if lvl, ok := c.Params["logLevel"].(string); ok && lvl != "" {
		return lvl
	}
	var doc struct {
		LogLevel string `json:"logLevel"`
	}
	if c.hasDocument() && json.Unmarshal(c.Config, &doc) == nil {
		return doc.LogLevel
	}
	return ""
}

// DeviceNames lists the sub-device names this connector serves, used to
// route device RPC commands. It reads master.slaves[] and master.devices[]
// (S7 uses the latter) plus the legacy devices[] list.
func (c Config) DeviceNames() ([]string, error) {
	var names []string
	add := func(name string) {
		if name != "" {
			names = append(names, name)
		}
	}

	if c.hasDocument() {
		type device struct {
			DeviceName string `json:"deviceName"`
		}
		var doc struct {
			Master struct {
				Slaves  []device `json:"slaves"`
				Devices []device `json:"devices"`
			} `json:"master"`
		}
		if err := json.Unmarshal(c.Config, &doc); err != nil {
			return nil, fmt.Errorf("connector %q: parse config document: %w", c.ID, err)
		}
		for _, s := range doc.Master.Slaves {
			add(s.DeviceName)
		}
		for _, d := range doc.Master.Devices {
			add(d.DeviceName)
		}
	}

	for _, d := range c.Devices {
		name, _ := d["name"].(string)
		if name == "" {
			name, _ = d["deviceName"].(string)
		}
		add(name)
	}
	return names, nil
}

// hasDocument reports whether a non-null "config" document is present.
func (c Config) hasDocument() bool {
	return len(c.Config) > 0 && string(c.Config) != "null"
}
