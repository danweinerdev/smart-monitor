package collector

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	monitor "github.com/danweinerdev/go-monitor"
	"github.com/danweinerdev/smart-monitor/internal/discovery"
	"github.com/danweinerdev/smart-monitor/internal/smartctl"
)

// Collector orchestrates drive discovery and SMART data collection.
type Collector struct {
	discoverer discovery.DriveDiscoverer
	runner     smartctl.SmartctlRunner
	logger     *slog.Logger
}

func NewCollector(discoverer discovery.DriveDiscoverer, runner smartctl.SmartctlRunner, logger *slog.Logger) *Collector {
	if logger == nil {
		logger = slog.Default()
	}
	return &Collector{
		discoverer: discoverer,
		runner:     runner,
		logger:     logger,
	}
}

// driveResult holds the result of polling a single drive.
type driveResult struct {
	metrics *smartctl.DriveMetrics
	err     error
}

// CollectAll discovers drives and collects SMART metrics from each concurrently.
// Individual drive failures do not stop collection of other drives.
func (c *Collector) CollectAll() ([]smartctl.DriveMetrics, []error) {
	drives, err := c.discoverer.Discover()
	if err != nil {
		return nil, []error{fmt.Errorf("drive discovery failed: %w", err)}
	}

	if len(drives) == 0 {
		c.logger.Warn("no drives discovered")
		return nil, nil
	}

	results := make([]driveResult, len(drives))
	var wg sync.WaitGroup

	for i, drive := range drives {
		wg.Add(1)
		go func(idx int, d discovery.Drive) {
			defer wg.Done()

			output, err := c.runner.Run(d)
			if err != nil {
				results[idx] = driveResult{err: fmt.Errorf("smartctl failed for %s: %w", d.Path, err)}
				return
			}

			m, err := smartctl.ParseMetrics(d, output)
			if err != nil {
				results[idx] = driveResult{err: fmt.Errorf("parsing metrics for %s: %w", d.Path, err)}
				return
			}

			results[idx] = driveResult{metrics: m}
		}(i, drive)
	}

	wg.Wait()

	var metrics []smartctl.DriveMetrics
	var errs []error
	for _, r := range results {
		if r.err != nil {
			errs = append(errs, r.err)
		} else if r.metrics != nil {
			metrics = append(metrics, *r.metrics)
		}
	}

	return metrics, errs
}

// MakeCollectFunc wraps a Collector into a monitor.CollectFunc,
// converting []DriveMetrics to []*monitor.Metric.
func MakeCollectFunc(collector *Collector, hostname string, logger *slog.Logger) monitor.CollectFunc {
	if logger == nil {
		logger = slog.Default()
	}
	return func(ctx context.Context) ([]*monitor.Metric, error) {
		metrics, errs := collector.CollectAll()
		for _, err := range errs {
			logger.Error("collection error", "error", err)
		}

		if len(metrics) == 0 && len(errs) > 0 {
			return nil, fmt.Errorf("all drives failed: %d errors", len(errs))
		}

		result := make([]*monitor.Metric, 0, len(metrics))
		for _, m := range metrics {
			metric := monitor.NewMetric("smart").
				WithTag("hostname", hostname).
				WithTag("device", m.Drive.Name).
				WithTag("model", m.Model).
				WithTag("serial", m.Serial).
				WithTag("type", string(m.Drive.Type)).
				WithFields(m.Fields)
			result = append(result, metric)
		}

		return result, nil
	}
}
