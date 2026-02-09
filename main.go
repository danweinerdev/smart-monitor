package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	configPath := flag.String("config", "/etc/synology-monitor/config.yaml", "path to config file")
	flag.Parse()

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("FATAL: %v", err)
	}

	discoverer := NewDevFSDiscoverer("")
	runner := NewExecSmartctl()
	collector := NewCollector(discoverer, runner)
	writer := NewInfluxWriter(cfg.InfluxDB)
	defer writer.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	run(ctx, cfg, collector, writer)
}

// run is the main loop, extracted for testability.
func run(ctx context.Context, cfg *Config, collector *Collector, writer MetricWriter) {
	log.Printf("Starting synology-monitor (interval=%s, hostname=%s)", cfg.Interval, cfg.Hostname)

	collect(ctx, cfg, collector, writer)

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
