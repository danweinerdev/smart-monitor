package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"

	monitor "github.com/danweinerdev/go-monitor"
	"github.com/danweinerdev/go-monitor/influxdb"
	"github.com/danweinerdev/smart-monitor/internal/collector"
	"github.com/danweinerdev/smart-monitor/internal/config"
	"github.com/danweinerdev/smart-monitor/internal/discovery"
	"github.com/danweinerdev/smart-monitor/internal/smartctl"
)

func main() {
	configPath := flag.String("config", "/etc/synology-monitor/config.yaml", "path to config file")
	runOnce := flag.Bool("run-once", false, "collect once and exit")
	echo := flag.Bool("echo", false, "print metrics to stdout instead of writing to InfluxDB")
	flag.Parse()

	cfg, err := config.LoadConfig(*configPath)
	if err != nil && !*echo {
		slog.Error("fatal configuration error", "error", err)
		os.Exit(1)
	}
	if cfg == nil {
		cfg = &config.Config{Interval: 30 * time.Minute, LogLevel: "info"}
		if h, err := os.Hostname(); err == nil {
			cfg.Hostname = h
		}
	}

	logger, levelVar := monitor.NewLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	disc := discovery.NewDevFSDiscoverer("")
	runner := smartctl.NewExecSmartctl()
	coll := collector.NewCollector(disc, runner, logger)
	collectFn := collector.MakeCollectFunc(coll, cfg.Hostname, logger)

	opts := []monitor.Option{
		monitor.WithConfig(cfg.MonitorConfig()),
		monitor.WithLogger(logger),
		monitor.WithRunOnce(*runOnce),
		monitor.WithReloadFunc(func(path string) (*monitor.Config, error) {
			newCfg, err := config.LoadConfig(*configPath)
			if err != nil {
				return nil, err
			}
			// Update log level on reload.
			if newCfg.LogLevel != cfg.LogLevel {
				levelVar.Set(monitor.ParseLogLevel(newCfg.LogLevel))
			}
			cfg = newCfg
			return newCfg.MonitorConfig(), nil
		}),
	}

	if *echo {
		opts = append(opts, monitor.WithEcho(true))
	} else {
		influxBackend := influxdb.New(cfg.InfluxBackendConfig(), logger)
		opts = append(opts, monitor.WithBackend(influxBackend))
	}

	m, err := monitor.New("smart-monitor", collectFn, opts...)
	if err != nil {
		slog.Error("failed to initialize monitor", "error", err)
		os.Exit(1)
	}

	if err := m.Run(context.Background()); err != nil {
		slog.Error("monitor exited with error", "error", err)
		os.Exit(1)
	}
}
