package main

import (
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// PollStats holds statistics for a single polling cycle.
type PollStats struct {
	TotalPolls      int64
	SuccessfulPolls int64
	FailedPolls     int64
	TotalMetrics    int64
	LastDuration    time.Duration
}

// Collector orchestrates drive discovery and SMART data collection.
type Collector struct {
	discoverer DriveDiscoverer
	runner     SmartctlRunner
	logger     *slog.Logger

	mu    sync.Mutex
	stats PollStats
}

func NewCollector(discoverer DriveDiscoverer, runner SmartctlRunner, logger *slog.Logger) *Collector {
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
	metrics *DriveMetrics
	err     error
}

// CollectAll discovers drives and collects SMART metrics from each concurrently.
// Individual drive failures do not stop collection of other drives.
func (c *Collector) CollectAll() ([]DriveMetrics, []error) {
	start := time.Now()

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
		go func(idx int, d Drive) {
			defer wg.Done()

			output, err := c.runner.Run(d)
			if err != nil {
				results[idx] = driveResult{err: fmt.Errorf("smartctl failed for %s: %w", d.Path, err)}
				return
			}

			m, err := ParseMetrics(d, output)
			if err != nil {
				results[idx] = driveResult{err: fmt.Errorf("parsing metrics for %s: %w", d.Path, err)}
				return
			}

			results[idx] = driveResult{metrics: m}
		}(i, drive)
	}

	wg.Wait()

	var metrics []DriveMetrics
	var errs []error
	for _, r := range results {
		if r.err != nil {
			errs = append(errs, r.err)
		} else if r.metrics != nil {
			metrics = append(metrics, *r.metrics)
		}
	}

	duration := time.Since(start)

	c.mu.Lock()
	c.stats.TotalPolls++
	c.stats.SuccessfulPolls += int64(len(metrics))
	c.stats.FailedPolls += int64(len(errs))
	c.stats.TotalMetrics += int64(len(metrics))
	c.stats.LastDuration = duration
	c.mu.Unlock()

	c.logger.Info("poll completed",
		"duration", duration,
		"success", len(metrics),
		"failed", len(errs),
		"drives", len(drives),
	)

	return metrics, errs
}

// Stats returns a snapshot of polling statistics.
func (c *Collector) Stats() PollStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stats
}
