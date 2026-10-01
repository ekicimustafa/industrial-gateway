package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/ekicimustafa/industrial-gateway/config"
	"github.com/ekicimustafa/industrial-gateway/connector/modbus"
	"github.com/ekicimustafa/industrial-gateway/connector"
	gw_mqtt "github.com/ekicimustafa/industrial-gateway/mqtt"
	"github.com/ekicimustafa/industrial-gateway/mqtt/buffer"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	cfgPath := flag.String("config", "config.json", "path to config.json")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}
	slog.Info("config loaded", "connectors", len(cfg.Connectors))

	buf, err := buffer.Open(cfg.Buffer.Path)
	if err != nil {
		slog.Error("buffer open failed", "err", err)
		os.Exit(1)
	}
	defer buf.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	dataCh := make(chan connector.DataPoint, 512)

	mqttCfg := cfg.MQTT.ToMQTTConfig("industrial-gateway")
	pub := gw_mqtt.NewPublisher(mqttCfg, buf)
	go pub.Run(ctx, dataCh)
	defer pub.Disconnect()

	var wg sync.WaitGroup
	for _, entry := range cfg.Connectors {
		switch entry.Type {
		case "modbus_tcp":
			conn := modbus.NewTCP(entry.ModbusTCP.ToModbusConfig())
			if err := conn.Start(ctx, dataCh); err != nil {
				slog.Error("connector start failed", "err", err)
				os.Exit(1)
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-ctx.Done()
				conn.Stop()
			}()
		}
	}

	slog.Info("gateway started — press Ctrl+C to stop")
	<-ctx.Done()
	slog.Info("shutting down...")
	wg.Wait()
	slog.Info("gateway stopped")
}
