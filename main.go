package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"
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

	// Set up structured logging.
	levelVar := &slog.LevelVar{}
	levelVar.Set(ParseLogLevel(cfg.LogLevel))
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: levelVar}))
	slog.SetDefault(logger)

	discoverer := NewDevFSDiscoverer("")
	runner := NewExecSmartctl()
	collector := NewCollector(discoverer, runner, logger)

	var writer MetricWriter
	if *echo {
		writer = &echoWriter{}
	} else {
		w := NewInfluxWriter(cfg.InfluxDB, logger)
		defer w.Close()
		writer = w
	}

	cfg.RunOnce = *runOnce

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// SIGHUP for config reload.
	sighupCh := make(chan os.Signal, 1)
	signal.Notify(sighupCh, syscall.SIGHUP)
	defer signal.Stop(sighupCh)

	run(ctx, cfg, collector, writer, &runOptions{
		Logger:     logger,
		LevelVar:   levelVar,
		ConfigPath: *configPath,
		ReloadCh:   sighupCh,
	})
}

// runOptions holds optional parameters for the run loop.
type runOptions struct {
	Logger     *slog.Logger
	LevelVar   *slog.LevelVar
	ConfigPath string
	ReloadCh   <-chan os.Signal
}

// run is the main loop, extracted for testability.
func run(ctx context.Context, cfg *Config, collector *Collector, writer MetricWriter, opts *runOptions) {
	logger := slog.Default()
	if opts != nil && opts.Logger != nil {
		logger = opts.Logger
	}

	logger.Info("starting synology-monitor",
		"interval", cfg.Interval,
		"hostname", cfg.Hostname,
	)

	collect(ctx, cfg, collector, writer, logger)

	if cfg.RunOnce {
		return
	}

	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	var reloadCh <-chan os.Signal
	if opts != nil {
		reloadCh = opts.ReloadCh
	}

	for {
		select {
		case <-ctx.Done():
			logger.Info("shutting down, performing final collection...")
			finalCtx, finalCancel := context.WithTimeout(context.Background(), 30*time.Second)
			collect(finalCtx, cfg, collector, writer, logger)
			finalCancel()
			writer.Close()
			logger.Info("shutdown complete")
			return

		case <-reloadCh:
			if opts == nil || opts.ConfigPath == "" {
				continue
			}
			logger.Info("received SIGHUP, reloading config", "path", opts.ConfigPath)
			newCfg, err := LoadConfig(opts.ConfigPath)
			if err != nil {
				logger.Error("config reload failed, keeping current config", "error", err)
				continue
			}
			if newCfg.Interval != cfg.Interval {
				ticker.Reset(newCfg.Interval)
				logger.Info("updated poll interval", "interval", newCfg.Interval)
			}
			if opts.LevelVar != nil && newCfg.LogLevel != cfg.LogLevel {
				opts.LevelVar.Set(ParseLogLevel(newCfg.LogLevel))
				logger.Info("updated log level", "level", newCfg.LogLevel)
			}
			cfg.Interval = newCfg.Interval
			cfg.Hostname = newCfg.Hostname
			cfg.LogLevel = newCfg.LogLevel

		case <-ticker.C:
			collect(ctx, cfg, collector, writer, logger)
		}
	}
}

func collect(ctx context.Context, cfg *Config, collector *Collector, writer MetricWriter, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}

	metrics, errs := collector.CollectAll()
	for _, err := range errs {
		logger.Error("collection error", "error", err)
	}

	if len(metrics) == 0 {
		logger.Warn("no metrics collected")
		return
	}

	if err := writer.Write(ctx, cfg.Hostname, metrics); err != nil {
		logger.Error("writing to InfluxDB", "error", err)
		return
	}

	logger.Info("wrote metrics", "count", len(metrics))
}

// echoWriter prints metrics to stdout.
type echoWriter struct{}

func (e *echoWriter) Write(_ context.Context, hostname string, metrics []DriveMetrics) error {
	for _, m := range metrics {
		fmt.Printf("drive=%s type=%s model=%q serial=%q hostname=%s\n",
			m.Drive.Name, m.Drive.Type, m.Model, m.Serial, hostname)

		keys := make([]string, 0, len(m.Fields))
		for k := range m.Fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			fmt.Printf("  %s = %v\n", k, m.Fields[k])
		}
	}
	return nil
}

func (e *echoWriter) Close() {}
