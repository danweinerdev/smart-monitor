package main

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestInfluxWriter_SATAPointConstruction(t *testing.T) {
	w := newTestableInfluxWriter(nil)

	metrics := []DriveMetrics{
		{
			Drive:  Drive{Path: "/dev/sata1", Name: "sata1", Type: DriveTypeSATA},
			Model:  "WDC WD40EFAX",
			Serial: "WD-ABC123",
			Fields: map[string]interface{}{
				"temperature":            int64(34),
				"power_on_hours":         int64(28456),
				"reallocated_sector_count": int64(0),
				"current_pending_sector": int64(0),
			},
		},
	}

	err := w.Write(context.Background(), "nas01", metrics)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	points := w.Points()
	if len(points) != 1 {
		t.Fatalf("got %d points, want 1", len(points))
	}

	p := points[0]

	// Check tags.
	tagChecks := map[string]string{
		"hostname": "nas01",
		"device":   "sata1",
		"model":    "WDC WD40EFAX",
		"serial":   "WD-ABC123",
		"type":     "sata",
	}
	for k, want := range tagChecks {
		if err := verifyPointTag(p, k, want); err != nil {
			t.Error(err)
		}
	}

	// Check fields.
	fieldChecks := map[string]int64{
		"temperature":            34,
		"power_on_hours":         28456,
		"reallocated_sector_count": 0,
		"current_pending_sector": 0,
	}
	for k, want := range fieldChecks {
		v, err := verifyPointField(p, k)
		if err != nil {
			t.Error(err)
			continue
		}
		if v.(int64) != want {
			t.Errorf("field %s = %v, want %v", k, v, want)
		}
	}
}

func TestInfluxWriter_NVMePointConstruction(t *testing.T) {
	w := newTestableInfluxWriter(nil)

	metrics := []DriveMetrics{
		{
			Drive:  Drive{Path: "/dev/nvme0n1", Name: "nvme0n1", Type: DriveTypeNVMe},
			Model:  "Samsung 970 EVO",
			Serial: "SAM-XYZ",
			Fields: map[string]interface{}{
				"temperature":        int64(38),
				"percentage_used":    int64(3),
				"available_spare":    int64(100),
				"media_errors":       int64(0),
				"data_units_written": int64(38912456),
			},
		},
	}

	err := w.Write(context.Background(), "nas01", metrics)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	points := w.Points()
	if len(points) != 1 {
		t.Fatalf("got %d points, want 1", len(points))
	}

	p := points[0]
	if err := verifyPointTag(p, "type", "nvme"); err != nil {
		t.Error(err)
	}

	nvmeFields := []string{"temperature", "percentage_used", "available_spare", "media_errors", "data_units_written"}
	for _, f := range nvmeFields {
		if _, err := verifyPointField(p, f); err != nil {
			t.Error(err)
		}
	}
}

func TestInfluxWriter_MultipleMetrics(t *testing.T) {
	w := newTestableInfluxWriter(nil)

	metrics := []DriveMetrics{
		{
			Drive:  Drive{Path: "/dev/sata1", Name: "sata1", Type: DriveTypeSATA},
			Model:  "WD Red",
			Serial: "WD001",
			Fields: map[string]interface{}{"temperature": int64(34)},
		},
		{
			Drive:  Drive{Path: "/dev/sata2", Name: "sata2", Type: DriveTypeSATA},
			Model:  "WD Red",
			Serial: "WD002",
			Fields: map[string]interface{}{"temperature": int64(36)},
		},
	}

	err := w.Write(context.Background(), "nas01", metrics)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(w.Points()) != 2 {
		t.Errorf("got %d points, want 2", len(w.Points()))
	}
}

func TestInfluxWriter_WriteError(t *testing.T) {
	w := newTestableInfluxWriter(fmt.Errorf("connection refused"))

	metrics := []DriveMetrics{
		{
			Drive:  Drive{Path: "/dev/sata1", Name: "sata1", Type: DriveTypeSATA},
			Model:  "WD",
			Serial: "WD001",
			Fields: map[string]interface{}{"temperature": int64(34)},
		},
	}

	err := w.Write(context.Background(), "nas01", metrics)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildPoints_Timestamp(t *testing.T) {
	ts := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	metrics := []DriveMetrics{
		{
			Drive:  Drive{Path: "/dev/sata1", Name: "sata1", Type: DriveTypeSATA},
			Model:  "WD",
			Serial: "WD001",
			Fields: map[string]interface{}{"temperature": int64(34)},
		},
	}

	points := buildPoints("nas01", metrics, ts)
	if len(points) != 1 {
		t.Fatalf("got %d points, want 1", len(points))
	}

	if !points[0].Time().Equal(ts) {
		t.Errorf("timestamp = %v, want %v", points[0].Time(), ts)
	}
}

func TestBuildPoints_MeasurementName(t *testing.T) {
	metrics := []DriveMetrics{
		{
			Drive:  Drive{Path: "/dev/sata1", Name: "sata1", Type: DriveTypeSATA},
			Model:  "WD",
			Serial: "WD001",
			Fields: map[string]interface{}{"temperature": int64(34)},
		},
	}

	points := buildPoints("nas01", metrics, time.Now())
	if points[0].Name() != "smart" {
		t.Errorf("measurement = %q, want %q", points[0].Name(), "smart")
	}
}
