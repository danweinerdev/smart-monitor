package main

import (
	"fmt"
	"log"
)

// Collector orchestrates drive discovery and SMART data collection.
type Collector struct {
	discoverer DriveDiscoverer
	runner     SmartctlRunner
}

func NewCollector(discoverer DriveDiscoverer, runner SmartctlRunner) *Collector {
	return &Collector{
		discoverer: discoverer,
		runner:     runner,
	}
}

// CollectAll discovers drives and collects SMART metrics from each.
// It returns all successfully collected metrics and any errors encountered.
// Individual drive failures do not stop collection of other drives.
func (c *Collector) CollectAll() ([]DriveMetrics, []error) {
	drives, err := c.discoverer.Discover()
	if err != nil {
		return nil, []error{fmt.Errorf("drive discovery failed: %w", err)}
	}

	if len(drives) == 0 {
		log.Println("WARNING: no drives discovered")
		return nil, nil
	}

	var metrics []DriveMetrics
	var errs []error

	for _, drive := range drives {
		output, err := c.runner.Run(drive)
		if err != nil {
			errs = append(errs, fmt.Errorf("smartctl failed for %s: %w", drive.Path, err))
			continue
		}

		m, err := ParseMetrics(drive, output)
		if err != nil {
			errs = append(errs, fmt.Errorf("parsing metrics for %s: %w", drive.Path, err))
			continue
		}

		metrics = append(metrics, *m)
	}

	return metrics, errs
}
