// Command gateway is the main entry point for industrial-gateway.
//
// Usage:
//
//	gateway -config config.json
//
// It reads a JSON config, starts the Modbus TCP connector(s), and
// routes DataPoints through the SQLite buffer to the MQTT broker.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ekicimustafa/industrial-gateway/connector/modbus"
	"github.com/ekicimustafa/industrial-gateway/internal"
	gw_mqtt "github.com/ekicimustafa/industrial-gateway/mqtt"
	"github.com/ekicimustafa/industrial-gateway/mqtt/buffer"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	_ = flag.String("config", "config.json", "path to config.json")
	flag.Parse()

	// Hardcoded example config — replace with JSON loading in the next step.
	modbusCfg := modbus.ConnectorConfig{
		ID:      "conn-modbus-1",
		Host:    "127.0.0.1",
		Port:    5020, // matches the pymodbus simulator from on-prem docs
		Timeout: 5 * time.Second,
		Slaves: []modbus.SlaveConfig{
			{
				DeviceID:   "device-uuid-example",
				UnitID:     1,
				PollPeriod: 5 * time.Second,
				Points: []modbus.Point{
					{Key: "voltage", Address: 0, Length: 1, DataType: modbus.DataUint16, Scale: 0.1, RegisterType: modbus.RegisterHolding},
					{Key: "current", Address: 1, Length: 1, DataType: modbus.DataUint16, Scale: 0.01, RegisterType: modbus.RegisterHolding},
					{Key: "power", Address: 2, Length: 2, DataType: modbus.DataUint32, Scale: 0.001, RegisterType: modbus.RegisterHolding},
				},
			},
		},
	}

	mqttCfg := gw_mqtt.Config{
		BrokerURL:   "tcp://localhost:1883",
		ClientID:    "industrial-gateway-dev",
		Username:    "", // ThingsBoard access token
		Password:    "",
		TopicPrefix: "v1/devices/me",
	}

	// Open SQLite buffer
	buf, err := buffer.Open("data/telemetry.db")
	if err != nil {
		slog.Error("buffer open failed", "err", err)
		os.Exit(1)
	}
	defer buf.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// DataPoint channel: connectors → publisher
	dataCh := make(chan internal.DataPoint, 512)

	// Start MQTT publisher
	pub := gw_mqtt.NewPublisher(mqttCfg, buf)
	go pub.Run(ctx, dataCh)
	defer pub.Disconnect()

	// Start Modbus TCP connector
	conn := modbus.NewTCP(modbusCfg)
	if err := conn.Start(ctx, dataCh); err != nil {
		slog.Error("modbus connector failed to start", "err", err)
		os.Exit(1)
	}
	defer conn.Stop()

	slog.Info("gateway started — waiting for shutdown signal")
	<-ctx.Done()
	slog.Info("gateway shutting down")
}
