package main

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	monitor "github.com/danweinerdev/go-monitor"
	"github.com/danweinerdev/smart-monitor/internal/collector"
	"github.com/danweinerdev/smart-monitor/internal/discovery"
	"github.com/danweinerdev/smart-monitor/internal/smartctl"
)

// mockDiscoverer implements discovery.DriveDiscoverer for testing.
type mockDiscoverer struct {
	drives []discovery.Drive
	err    error
}

func (m *mockDiscoverer) Discover() ([]discovery.Drive, error) {
	return m.drives, m.err
}

// mockRunner implements smartctl.SmartctlRunner for testing.
type mockRunner struct {
	outputs map[string]*smartctl.SmartctlOutput
	errs    map[string]error
}

func (m *mockRunner) Run(drive discovery.Drive) (*smartctl.SmartctlOutput, error) {
	if err, ok := m.errs[drive.Path]; ok {
		return nil, err
	}
	if out, ok := m.outputs[drive.Path]; ok {
		return out, nil
	}
	return nil, fmt.Errorf("no mock output for %s", drive.Path)
}

func makeSATAOutput(model, serial string) *smartctl.SmartctlOutput {
	return &smartctl.SmartctlOutput{
		ModelName: model,
		SerialNum: serial,
		ATASmart: &smartctl.ATASmartAttributes{
			Table: []smartctl.ATASmartAttribute{
				{ID: 9, Name: "Power_On_Hours", Raw: smartctl.ATASmartAttributeValue{Value: 1000}},
				{ID: 194, Name: "Temperature_Celsius", Raw: smartctl.ATASmartAttributeValue{Value: 35}},
			},
		},
	}
}

func makeNVMeOutput(model, serial string) *smartctl.SmartctlOutput {
	return &smartctl.SmartctlOutput{
		ModelName: model,
		SerialNum: serial,
		NVMESmart: &smartctl.NVMESmartHealth{
			Temperature:    38,
			PowerOnHours:   5000,
			PercentageUsed: 5,
			AvailableSpare: 100,
		},
	}
}

func TestIntegration_RunOnceEcho(t *testing.T) {
	drives := []discovery.Drive{
		{Path: "/dev/sata1", Name: "sata1", Type: discovery.DriveTypeSATA},
		{Path: "/dev/nvme0n1", Name: "nvme0n1", Type: discovery.DriveTypeNVMe},
	}
	runner := &mockRunner{
		outputs: map[string]*smartctl.SmartctlOutput{
			"/dev/sata1":   makeSATAOutput("WD Red", "WD001"),
			"/dev/nvme0n1": makeNVMeOutput("Samsung 970", "SAM001"),
		},
		errs: map[string]error{},
	}

	coll := collector.NewCollector(&mockDiscoverer{drives: drives}, runner, nil)
	collectFn := collector.MakeCollectFunc(coll, "test-nas", nil)

	var buf bytes.Buffer
	echoBackend := monitor.NewEcho(&buf, nil)

	cfg := monitor.DefaultConfig()
	cfg.Global.PollInterval = monitor.Duration{Duration: 50_000_000} // 50ms

	m, err := monitor.New("test-smart",
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
	coll := collector.NewCollector(&mockDiscoverer{drives: nil}, &mockRunner{}, nil)
	collectFn := collector.MakeCollectFunc(coll, "test-nas", nil)

	var buf bytes.Buffer
	echoBackend := monitor.NewEcho(&buf, nil)

	cfg := monitor.DefaultConfig()

	m, err := monitor.New("test-smart",
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
	drives := []discovery.Drive{
		{Path: "/dev/sata1", Name: "sata1", Type: discovery.DriveTypeSATA},
	}
	runner := &mockRunner{
		outputs: map[string]*smartctl.SmartctlOutput{
			"/dev/sata1": makeSATAOutput("WD Red", "WD001"),
		},
		errs: map[string]error{},
	}

	coll := collector.NewCollector(&mockDiscoverer{drives: drives}, runner, nil)
	collectFn := collector.MakeCollectFunc(coll, "test-nas", nil)

	var buf bytes.Buffer
	echoBackend := monitor.NewEcho(&buf, nil)

	cfg := monitor.DefaultConfig()

	m, err := monitor.New("test-smart",
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
