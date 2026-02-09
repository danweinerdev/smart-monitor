package main

import (
	"context"
	"flag"
	"fmt"
	"log"
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
		log.Fatalf("FATAL: %v", err)
	}
	if cfg == nil {
		cfg = &Config{Interval: 30 * time.Minute}
		if h, err := os.Hostname(); err == nil {
			cfg.Hostname = h
		}
	}

	discoverer := NewDevFSDiscoverer("")
	runner := NewExecSmartctl()
	collector := NewCollector(discoverer, runner)

	var writer MetricWriter
	if *echo {
		writer = &echoWriter{}
	} else {
		w := NewInfluxWriter(cfg.InfluxDB)
		defer w.Close()
		writer = w
	}

	cfg.RunOnce = *runOnce

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	run(ctx, cfg, collector, writer)
}

// run is the main loop, extracted for testability.
func run(ctx context.Context, cfg *Config, collector *Collector, writer MetricWriter) {
	log.Printf("Starting synology-monitor (interval=%s, hostname=%s)", cfg.Interval, cfg.Hostname)

	collect(ctx, cfg, collector, writer)

	if cfg.RunOnce {
		return
	}

	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Shutting down...")
			writer.Close()
			return
		case <-ticker.C:
			collect(ctx, cfg, collector, writer)
		}
	}
}

func collect(ctx context.Context, cfg *Config, collector *Collector, writer MetricWriter) {
	metrics, errs := collector.CollectAll()
	for _, err := range errs {
		log.Printf("ERROR: %v", err)
	}

	if len(metrics) == 0 {
		log.Println("WARNING: no metrics collected")
		return
	}

	if err := writer.Write(ctx, cfg.Hostname, metrics); err != nil {
		log.Printf("ERROR: writing to InfluxDB: %v", err)
		return
	}

	log.Printf("Wrote %d drive metrics to InfluxDB", len(metrics))
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
