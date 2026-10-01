package connector

import (
	"slices"
	"testing"
)

// Trimmed from solartools-gateway conf/gateway.json.example.
const modbusEntry = `{
  "id": "connector-uuid-1",
  "name": "Solar Inverters — Line 1",
  "type": "modbus_tcp",
  "config": {
    "logLevel": "debug",
    "master": {
      "slaves": [
        {"host": "192.168.1.10", "unitId": 1, "deviceName": "Inverter-1", "pollPeriod": 5000},
        {"host": "192.168.1.11", "unitId": 2, "deviceName": "Inverter-2", "pollPeriod": 5000}
      ]
    }
  }
}`

func TestParseConfigPlatformEntry(t *testing.T) {
	c, err := ParseConfig([]byte(modbusEntry))
	if err != nil {
		t.Fatal(err)
	}
	if c.ID != "connector-uuid-1" || c.Type != TypeModbusTCP || c.Name != "Solar Inverters — Line 1" {
		t.Fatalf("unexpected identity: %+v", c)
	}
	if !c.IsEnabled() {
		t.Error("missing enabled must mean enabled")
	}
	if got := c.LogLevel(); got != "debug" {
		t.Errorf("LogLevel() = %q, want debug", got)
	}
	names, err := c.DeviceNames()
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"Inverter-1", "Inverter-2"}; !slices.Equal(names, want) {
		t.Errorf("DeviceNames() = %v, want %v", names, want)
	}
}

func TestParseConfigMissingFields(t *testing.T) {
	_, err := ParseConfig([]byte(`{"id": "c1", "config": {}}`))
	if err == nil {
		t.Fatal("expected error for missing type and name")
	}
	if got, want := err.Error(), `connector config "c1": missing type, name`; got != want {
		t.Errorf("error = %q, want %q", got, want)
	}
}

func TestConfigEnabled(t *testing.T) {
	c, err := ParseConfig([]byte(`{"id": "c1", "type": "s7", "name": "PLC", "enabled": false}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.IsEnabled() {
		t.Error(`"enabled": false must disable the connector`)
	}
}

func TestDeviceNamesAllSources(t *testing.T) {
	c, err := ParseConfig([]byte(`{
	  "id": "c1", "type": "s7", "name": "PLC",
	  "params": {"logLevel": "warning"},
	  "config": {"master": {"devices": [{"deviceName": "Press-1"}, {"deviceName": ""}]}},
	  "devices": [{"name": "Legacy-A"}, {"deviceName": "Legacy-B"}, {}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	names, err := c.DeviceNames()
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"Press-1", "Legacy-A", "Legacy-B"}; !slices.Equal(names, want) {
		t.Errorf("DeviceNames() = %v, want %v", names, want)
	}
	if got := c.LogLevel(); got != "warning" {
		t.Errorf("params logLevel must win: got %q", got)
	}
}

func TestDeviceNamesNullOrMalformedDocument(t *testing.T) {
	c, err := ParseConfig([]byte(`{"id": "c1", "type": "rest", "name": "API", "config": null}`))
	if err != nil {
		t.Fatal(err)
	}
	if names, err := c.DeviceNames(); err != nil || len(names) != 0 {
		t.Errorf("null document: got (%v, %v), want no names and no error", names, err)
	}

	c.Config = []byte(`{"master": {"slaves": "not-a-list"}}`)
	if _, err := c.DeviceNames(); err == nil {
		t.Error("malformed document must return an error")
	}
}
