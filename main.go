package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"

	monitor "github.com/danweinerdev/go-monitor"
	"github.com/danweinerdev/go-monitor/influxdb"
)

func main() {
	configPath := flag.String("config", "/etc/synology-monitor/config.yaml", "path to config file")
	runOnce := flag.Bool("run-once", false, "collect once and exit")
	echo := flag.Bool("echo", false, "print metrics to stdout instead of writing to InfluxDB")
	flag.Parse()

	cfg, err := LoadConfig(*configPath)
	if err != nil && !*echo {
		slog.Error("fatal configuration error", "error", err)
		os.Exit(1)
	}
	if cfg == nil {
		cfg = &Config{Interval: 30 * time.Minute, LogLevel: "info"}
		if h, err := os.Hostname(); err == nil {
			cfg.Hostname = h
		}
	}

	logger, levelVar := monitor.NewLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	discoverer := NewDevFSDiscoverer("")
	runner := NewExecSmartctl()
	collector := NewCollector(discoverer, runner, logger)
	collectFn := makeCollectFunc(collector, cfg.Hostname, logger)

	opts := []monitor.Option{
		monitor.WithConfig(cfg.monitorConfig()),
		monitor.WithLogger(logger),
		monitor.WithRunOnce(*runOnce),
		monitor.WithReloadFunc(func(path string) (*monitor.Config, error) {
			newCfg, err := LoadConfig(*configPath)
			if err != nil {
				return nil, err
			}
			// Update log level on reload.
			if newCfg.LogLevel != cfg.LogLevel {
				levelVar.Set(monitor.ParseLogLevel(newCfg.LogLevel))
			}
			cfg = newCfg
			return newCfg.monitorConfig(), nil
		}),
	}

	if *echo {
		opts = append(opts, monitor.WithEcho(true))
	} else {
		influxBackend := influxdb.New(cfg.influxDBConfig(), logger)
		opts = append(opts, monitor.WithBackend(influxBackend))
	}

	m, err := monitor.New("synology-monitor", collectFn, opts...)
	if err != nil {
		slog.Error("failed to initialize monitor", "error", err)
		os.Exit(1)
	}

	if err := m.Run(context.Background()); err != nil {
		slog.Error("monitor exited with error", "error", err)
		os.Exit(1)
	}
}
