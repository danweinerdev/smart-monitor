package main

import (
	"bytes"
	"context"
	"testing"

	monitor "github.com/danweinerdev/go-monitor"
)

func TestIntegration_RunOnceEcho(t *testing.T) {
	drives := []Drive{
		{Path: "/dev/sata1", Name: "sata1", Type: DriveTypeSATA},
		{Path: "/dev/nvme0n1", Name: "nvme0n1", Type: DriveTypeNVMe},
	}
	runner := &mockRunner{
		outputs: map[string]*SmartctlOutput{
			"/dev/sata1":   makeSATAOutput("WD Red", "WD001"),
			"/dev/nvme0n1": makeNVMeOutput("Samsung 970", "SAM001"),
		},
		errs: map[string]error{},
	}

	collector := NewCollector(&mockDiscoverer{drives: drives}, runner, nil)
	collectFn := makeCollectFunc(collector, "test-nas", nil)

	var buf bytes.Buffer
	echoBackend := monitor.NewEcho(&buf, nil)

	cfg := monitor.DefaultConfig()
	cfg.Global.PollInterval = monitor.Duration{Duration: 50_000_000} // 50ms

	m, err := monitor.New("test-synology",
		collectFn,
		monitor.WithConfig(cfg),
		monitor.WithRunOnce(true),
		monitor.WithBackend(echoBackend),
	)
	if err != nil {
		t.Fatalf("monitor.New failed: %v", err)
	}

	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("monitor.Run failed: %v", err)
	}

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("smart")) {
		t.Error("output missing 'smart' measurement")
	}
	if !bytes.Contains([]byte(output), []byte("sata1")) {
		t.Error("output missing sata1")
	}
	if !bytes.Contains([]byte(output), []byte("nvme0n1")) {
		t.Error("output missing nvme0n1")
	}
}

func TestIntegration_RunOnceNoMetrics(t *testing.T) {
	collector := NewCollector(&mockDiscoverer{drives: nil}, &mockRunner{}, nil)
	collectFn := makeCollectFunc(collector, "test-nas", nil)

	var buf bytes.Buffer
	echoBackend := monitor.NewEcho(&buf, nil)

	cfg := monitor.DefaultConfig()

	m, err := monitor.New("test-synology",
		collectFn,
		monitor.WithConfig(cfg),
		monitor.WithRunOnce(true),
		monitor.WithBackend(echoBackend),
	)
	if err != nil {
		t.Fatalf("monitor.New failed: %v", err)
	}

	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("monitor.Run failed: %v", err)
	}

	if buf.Len() != 0 {
		t.Errorf("expected no output for empty discovery, got: %s", buf.String())
	}
}

func TestIntegration_MetricFieldsPreserved(t *testing.T) {
	drives := []Drive{
		{Path: "/dev/sata1", Name: "sata1", Type: DriveTypeSATA},
	}
	runner := &mockRunner{
		outputs: map[string]*SmartctlOutput{
			"/dev/sata1": makeSATAOutput("WD Red", "WD001"),
		},
		errs: map[string]error{},
	}

	collector := NewCollector(&mockDiscoverer{drives: drives}, runner, nil)
	collectFn := makeCollectFunc(collector, "test-nas", nil)

	var buf bytes.Buffer
	echoBackend := monitor.NewEcho(&buf, nil)

	cfg := monitor.DefaultConfig()

	m, err := monitor.New("test-synology",
		collectFn,
		monitor.WithConfig(cfg),
		monitor.WithRunOnce(true),
		monitor.WithBackend(echoBackend),
	)
	if err != nil {
		t.Fatalf("monitor.New failed: %v", err)
	}

	if err := m.Run(context.Background()); err != nil {
		t.Fatalf("monitor.Run failed: %v", err)
	}

	output := buf.String()
	// The echo backend outputs in line protocol format.
	// Verify key fields are present.
	if !bytes.Contains([]byte(output), []byte("power_on_hours")) {
		t.Error("output missing power_on_hours field")
	}
	if !bytes.Contains([]byte(output), []byte("temperature")) {
		t.Error("output missing temperature field")
	}
}
