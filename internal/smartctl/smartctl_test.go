package smartctl

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/danweinerdev/smart-monitor/internal/discovery"
)

func loadFixture(t *testing.T, path string) *SmartctlOutput {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading fixture %s: %v", path, err)
	}
	var output SmartctlOutput
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatalf("parsing fixture %s: %v", path, err)
	}
	return &output
}

func TestParseSATAHealthy(t *testing.T) {
	output := loadFixture(t, "testdata/sata_healthy.json")
	drive := discovery.Drive{Path: "/dev/sata1", Name: "sata1", Type: discovery.DriveTypeSATA}

	metrics, err := ParseMetrics(drive, output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if metrics.Model != "WDC WD40EFAX-68JH4N1" {
		t.Errorf("model = %q", metrics.Model)
	}
	if metrics.Serial != "WD-WXB2D81JKTP2" {
		t.Errorf("serial = %q", metrics.Serial)
	}

	expectedFields := map[string]int64{
		"raw_read_error_rate":      0,
		"reallocated_sector_count": 0,
		"seek_error_rate":          0,
		"power_on_hours":           28456,
		"spin_retry_count":         0,
		"temperature":              34,
		"current_pending_sector":   0,
		"offline_uncorrectable":    0,
	}

	for name, want := range expectedFields {
		got, ok := metrics.Fields[name]
		if !ok {
			t.Errorf("missing field %q", name)
			continue
		}
		if got.(int64) != want {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
}

func TestParseSATADegraded(t *testing.T) {
	output := loadFixture(t, "testdata/sata_degraded.json")
	drive := discovery.Drive{Path: "/dev/sata2", Name: "sata2", Type: discovery.DriveTypeSATA}

	metrics, err := ParseMetrics(drive, output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if metrics.Fields["reallocated_sector_count"].(int64) != 16 {
		t.Errorf("reallocated_sector_count = %v, want 16", metrics.Fields["reallocated_sector_count"])
	}
	if metrics.Fields["current_pending_sector"].(int64) != 8 {
		t.Errorf("current_pending_sector = %v, want 8", metrics.Fields["current_pending_sector"])
	}
	if metrics.Fields["offline_uncorrectable"].(int64) != 4 {
		t.Errorf("offline_uncorrectable = %v, want 4", metrics.Fields["offline_uncorrectable"])
	}
	if metrics.Fields["temperature"].(int64) != 42 {
		t.Errorf("temperature = %v, want 42", metrics.Fields["temperature"])
	}
}

func TestParseNVMeHealthy(t *testing.T) {
	output := loadFixture(t, "testdata/nvme_healthy.json")
	drive := discovery.Drive{Path: "/dev/nvme0n1", Name: "nvme0n1", Type: discovery.DriveTypeNVMe}

	metrics, err := ParseMetrics(drive, output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if metrics.Model != "Samsung SSD 970 EVO Plus 500GB" {
		t.Errorf("model = %q", metrics.Model)
	}
	if metrics.Serial != "S4EVNF0M812345" {
		t.Errorf("serial = %q", metrics.Serial)
	}

	expectedFields := map[string]int64{
		"temperature":        38,
		"power_on_hours":     12345,
		"percentage_used":    3,
		"available_spare":    100,
		"media_errors":       0,
		"unsafe_shutdowns":   12,
		"data_units_read":    52468135,
		"data_units_written": 38912456,
	}

	for name, want := range expectedFields {
		got, ok := metrics.Fields[name]
		if !ok {
			t.Errorf("missing field %q", name)
			continue
		}
		if got.(int64) != want {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
}

func TestParseNVMeWorn(t *testing.T) {
	output := loadFixture(t, "testdata/nvme_worn.json")
	drive := discovery.Drive{Path: "/dev/nvme1n1", Name: "nvme1n1", Type: discovery.DriveTypeNVMe}

	metrics, err := ParseMetrics(drive, output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if metrics.Fields["percentage_used"].(int64) != 87 {
		t.Errorf("percentage_used = %v, want 87", metrics.Fields["percentage_used"])
	}
	if metrics.Fields["available_spare"].(int64) != 15 {
		t.Errorf("available_spare = %v, want 15", metrics.Fields["available_spare"])
	}
	if metrics.Fields["media_errors"].(int64) != 3 {
		t.Errorf("media_errors = %v, want 3", metrics.Fields["media_errors"])
	}
	if metrics.Fields["unsafe_shutdowns"].(int64) != 45 {
		t.Errorf("unsafe_shutdowns = %v, want 45", metrics.Fields["unsafe_shutdowns"])
	}
}

func TestParseSATA_MissingAttributes(t *testing.T) {
	// Output with no ata_smart_attributes at all.
	output := &SmartctlOutput{
		ModelName: "Test Drive",
		SerialNum: "TEST123",
	}
	drive := discovery.Drive{Path: "/dev/sata1", Name: "sata1", Type: discovery.DriveTypeSATA}

	_, err := ParseMetrics(drive, output)
	if err == nil {
		t.Fatal("expected error for missing ata_smart_attributes")
	}
}

func TestParseNVMe_MissingHealthLog(t *testing.T) {
	output := &SmartctlOutput{
		ModelName: "Test NVMe",
		SerialNum: "TEST456",
	}
	drive := discovery.Drive{Path: "/dev/nvme0n1", Name: "nvme0n1", Type: discovery.DriveTypeNVMe}

	_, err := ParseMetrics(drive, output)
	if err == nil {
		t.Fatal("expected error for missing nvme health log")
	}
}

func TestParseSATA_MissingIndividualAttribute(t *testing.T) {
	// Output with only a subset of attributes — missing ones should be omitted, not zero-filled.
	output := &SmartctlOutput{
		ModelName: "Minimal Drive",
		SerialNum: "MIN123",
		ATASmart: &ATASmartAttributes{
			Table: []ATASmartAttribute{
				{ID: 9, Name: "Power_On_Hours", Raw: ATASmartAttributeValue{Value: 1000}},
			},
		},
	}
	drive := discovery.Drive{Path: "/dev/sata1", Name: "sata1", Type: discovery.DriveTypeSATA}

	metrics, err := ParseMetrics(drive, output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := metrics.Fields["power_on_hours"]; !ok {
		t.Error("power_on_hours should be present")
	}
	if _, ok := metrics.Fields["reallocated_sector_count"]; ok {
		t.Error("reallocated_sector_count should be omitted when not in output")
	}
}

func TestParseSATA_TemperatureFallback(t *testing.T) {
	// No attribute 194, but top-level temperature is present.
	output := &SmartctlOutput{
		ModelName:   "Fallback Drive",
		SerialNum:   "FB123",
		Temperature: &SmartctlTemperature{Current: 37},
		ATASmart: &ATASmartAttributes{
			Table: []ATASmartAttribute{
				{ID: 9, Name: "Power_On_Hours", Raw: ATASmartAttributeValue{Value: 500}},
			},
		},
	}
	drive := discovery.Drive{Path: "/dev/sata1", Name: "sata1", Type: discovery.DriveTypeSATA}

	metrics, err := ParseMetrics(drive, output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	temp, ok := metrics.Fields["temperature"]
	if !ok {
		t.Fatal("temperature should be present via fallback")
	}
	if temp.(int64) != 37 {
		t.Errorf("temperature = %v, want 37", temp)
	}
}

func TestParseMalformedJSON(t *testing.T) {
	var output SmartctlOutput
	err := json.Unmarshal([]byte(`{{{not json`), &output)
	if err == nil {
		t.Fatal("expected parse error for malformed JSON")
	}
}

func TestExecSmartctl_CommandConstruction(t *testing.T) {
	// We can't run smartctl in tests, but we can verify the ExecSmartctl struct.
	e := NewExecSmartctl()
	if e.Path != "smartctl" {
		t.Errorf("path = %q, want smartctl", e.Path)
	}
}
