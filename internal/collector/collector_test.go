package collector

import (
	"context"
	"fmt"
	"testing"

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

func TestCollector_AllSucceed(t *testing.T) {
	drives := []discovery.Drive{
		{Path: "/dev/sata1", Name: "sata1", Type: discovery.DriveTypeSATA},
		{Path: "/dev/sata2", Name: "sata2", Type: discovery.DriveTypeSATA},
		{Path: "/dev/nvme0n1", Name: "nvme0n1", Type: discovery.DriveTypeNVMe},
	}

	runner := &mockRunner{
		outputs: map[string]*smartctl.SmartctlOutput{
			"/dev/sata1":   makeSATAOutput("WD Red", "WD001"),
			"/dev/sata2":   makeSATAOutput("WD Red", "WD002"),
			"/dev/nvme0n1": makeNVMeOutput("Samsung 970", "SAM001"),
		},
		errs: map[string]error{},
	}

	c := NewCollector(&mockDiscoverer{drives: drives}, runner, nil)
	metrics, errs := c.CollectAll()

	if len(errs) != 0 {
		t.Errorf("unexpected errors: %v", errs)
	}
	if len(metrics) != 3 {
		t.Fatalf("got %d metrics, want 3", len(metrics))
	}
}

func TestCollector_PartialFailure(t *testing.T) {
	drives := []discovery.Drive{
		{Path: "/dev/sata1", Name: "sata1", Type: discovery.DriveTypeSATA},
		{Path: "/dev/sata2", Name: "sata2", Type: discovery.DriveTypeSATA},
		{Path: "/dev/sata3", Name: "sata3", Type: discovery.DriveTypeSATA},
	}

	runner := &mockRunner{
		outputs: map[string]*smartctl.SmartctlOutput{
			"/dev/sata1": makeSATAOutput("WD Red", "WD001"),
			"/dev/sata3": makeSATAOutput("WD Red", "WD003"),
		},
		errs: map[string]error{
			"/dev/sata2": fmt.Errorf("device busy"),
		},
	}

	c := NewCollector(&mockDiscoverer{drives: drives}, runner, nil)
	metrics, errs := c.CollectAll()

	if len(metrics) != 2 {
		t.Errorf("got %d metrics, want 2", len(metrics))
	}
	if len(errs) != 1 {
		t.Errorf("got %d errors, want 1", len(errs))
	}
}

func TestCollector_EmptyDiscovery(t *testing.T) {
	c := NewCollector(&mockDiscoverer{drives: nil}, &mockRunner{}, nil)
	metrics, errs := c.CollectAll()

	if len(metrics) != 0 {
		t.Errorf("got %d metrics, want 0", len(metrics))
	}
	if len(errs) != 0 {
		t.Errorf("got %d errors, want 0", len(errs))
	}
}

func TestCollector_DiscoveryError(t *testing.T) {
	c := NewCollector(
		&mockDiscoverer{err: fmt.Errorf("permission denied")},
		&mockRunner{},
		nil,
	)
	metrics, errs := c.CollectAll()

	if metrics != nil {
		t.Errorf("expected nil metrics, got %v", metrics)
	}
	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1", len(errs))
	}
}

func TestCollector_ConcurrentExecution(t *testing.T) {
	// Verify that all drives are polled even with concurrent execution.
	drives := make([]discovery.Drive, 10)
	outputs := make(map[string]*smartctl.SmartctlOutput)
	for i := range drives {
		path := fmt.Sprintf("/dev/sata%d", i+1)
		name := fmt.Sprintf("sata%d", i+1)
		drives[i] = discovery.Drive{Path: path, Name: name, Type: discovery.DriveTypeSATA}
		outputs[path] = makeSATAOutput("WD Red", fmt.Sprintf("WD%03d", i+1))
	}

	runner := &mockRunner{outputs: outputs, errs: map[string]error{}}
	c := NewCollector(&mockDiscoverer{drives: drives}, runner, nil)
	metrics, errs := c.CollectAll()

	if len(errs) != 0 {
		t.Errorf("unexpected errors: %v", errs)
	}
	if len(metrics) != 10 {
		t.Errorf("got %d metrics, want 10", len(metrics))
	}
}

func TestMakeCollectFunc_ConvertsMetrics(t *testing.T) {
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

	c := NewCollector(&mockDiscoverer{drives: drives}, runner, nil)
	collectFn := MakeCollectFunc(c, "test-nas", nil)

	metrics, err := collectFn(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(metrics) != 2 {
		t.Fatalf("got %d metrics, want 2", len(metrics))
	}

	for _, m := range metrics {
		if m.Measurement != "smart" {
			t.Errorf("measurement = %q, want %q", m.Measurement, "smart")
		}
		if m.Tags["hostname"] != "test-nas" {
			t.Errorf("hostname tag = %q, want %q", m.Tags["hostname"], "test-nas")
		}
		if len(m.Fields) == 0 {
			t.Error("fields should not be empty")
		}
	}

	// Verify SATA metric tags.
	sataMetric := metrics[0]
	if sataMetric.Tags["device"] != "sata1" {
		t.Errorf("device tag = %q, want %q", sataMetric.Tags["device"], "sata1")
	}
	if sataMetric.Tags["type"] != "sata" {
		t.Errorf("type tag = %q, want %q", sataMetric.Tags["type"], "sata")
	}
	if sataMetric.Tags["model"] != "WD Red" {
		t.Errorf("model tag = %q, want %q", sataMetric.Tags["model"], "WD Red")
	}
	if sataMetric.Tags["serial"] != "WD001" {
		t.Errorf("serial tag = %q, want %q", sataMetric.Tags["serial"], "WD001")
	}

	// Verify NVMe metric tags.
	nvmeMetric := metrics[1]
	if nvmeMetric.Tags["device"] != "nvme0n1" {
		t.Errorf("device tag = %q, want %q", nvmeMetric.Tags["device"], "nvme0n1")
	}
	if nvmeMetric.Tags["type"] != "nvme" {
		t.Errorf("type tag = %q, want %q", nvmeMetric.Tags["type"], "nvme")
	}
}

func TestMakeCollectFunc_PartialFailure(t *testing.T) {
	drives := []discovery.Drive{
		{Path: "/dev/sata1", Name: "sata1", Type: discovery.DriveTypeSATA},
		{Path: "/dev/sata2", Name: "sata2", Type: discovery.DriveTypeSATA},
	}

	runner := &mockRunner{
		outputs: map[string]*smartctl.SmartctlOutput{
			"/dev/sata1": makeSATAOutput("WD Red", "WD001"),
		},
		errs: map[string]error{
			"/dev/sata2": fmt.Errorf("device busy"),
		},
	}

	c := NewCollector(&mockDiscoverer{drives: drives}, runner, nil)
	collectFn := MakeCollectFunc(c, "test-nas", nil)

	metrics, err := collectFn(context.Background())
	if err != nil {
		t.Fatalf("unexpected error on partial failure: %v", err)
	}

	if len(metrics) != 1 {
		t.Errorf("got %d metrics, want 1", len(metrics))
	}
}

func TestMakeCollectFunc_AllFail(t *testing.T) {
	drives := []discovery.Drive{
		{Path: "/dev/sata1", Name: "sata1", Type: discovery.DriveTypeSATA},
	}

	runner := &mockRunner{
		outputs: map[string]*smartctl.SmartctlOutput{},
		errs: map[string]error{
			"/dev/sata1": fmt.Errorf("device busy"),
		},
	}

	c := NewCollector(&mockDiscoverer{drives: drives}, runner, nil)
	collectFn := MakeCollectFunc(c, "test-nas", nil)

	metrics, err := collectFn(context.Background())
	if err == nil {
		t.Fatal("expected error when all drives fail")
	}
	if metrics != nil {
		t.Errorf("expected nil metrics, got %v", metrics)
	}
}
