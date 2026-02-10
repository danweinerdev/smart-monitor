package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/influxdata/influxdb-client-go/v2/api/write"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// mockBlockingAPI implements api.WriteAPIBlocking for testing retry logic.
type mockBlockingAPI struct {
	callCount int
	failUntil int
	failErr   error
	points    []*write.Point
}

func (m *mockBlockingAPI) WriteRecord(_ context.Context, _ ...string) error { return nil }
func (m *mockBlockingAPI) EnableBatching()                                   {}
func (m *mockBlockingAPI) Flush(_ context.Context) error                     { return nil }

func (m *mockBlockingAPI) WritePoint(_ context.Context, points ...*write.Point) error {
	m.callCount++
	if m.callCount <= m.failUntil {
		return m.failErr
	}
	m.points = append(m.points, points...)
	return nil
}

func newTestInfluxWriterWithAPI(api *mockBlockingAPI, retryAttempts int, retryDelay time.Duration) *InfluxWriter {
	return &InfluxWriter{
		writeAPI:      api,
		retryAttempts: retryAttempts,
		retryDelay:    retryDelay,
		logger:        discardLogger(),
		healthy:       true,
	}
}

func TestInfluxWriter_SATAPointConstruction(t *testing.T) {
	w := newTestableInfluxWriter(nil)

	metrics := []DriveMetrics{
		{
			Drive:  Drive{Path: "/dev/sata1", Name: "sata1", Type: DriveTypeSATA},
			Model:  "WDC WD40EFAX",
			Serial: "WD-ABC123",
			Fields: map[string]interface{}{
				"temperature":              int64(34),
				"power_on_hours":           int64(28456),
				"reallocated_sector_count": int64(0),
				"current_pending_sector":   int64(0),
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
		"temperature":              34,
		"power_on_hours":           28456,
		"reallocated_sector_count": 0,
		"current_pending_sector":   0,
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

func TestInfluxWriter_RetrySuccess(t *testing.T) {
	// Fails twice, succeeds on third attempt.
	mock := &mockBlockingAPI{
		failUntil: 2,
		failErr:   fmt.Errorf("connection refused"),
	}
	w := newTestInfluxWriterWithAPI(mock, 3, time.Millisecond)

	metrics := []DriveMetrics{
		{
			Drive:  Drive{Path: "/dev/sata1", Name: "sata1", Type: DriveTypeSATA},
			Model:  "WD",
			Serial: "WD001",
			Fields: map[string]interface{}{"temperature": int64(34)},
		},
	}

	err := w.Write(context.Background(), "nas01", metrics)
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	if mock.callCount != 3 {
		t.Errorf("call count = %d, want 3", mock.callCount)
	}
	if !w.Healthy() {
		t.Error("writer should be healthy after successful write")
	}
}

func TestInfluxWriter_RetryExhausted(t *testing.T) {
	// All attempts fail.
	mock := &mockBlockingAPI{
		failUntil: 100,
		failErr:   fmt.Errorf("connection refused"),
	}
	w := newTestInfluxWriterWithAPI(mock, 2, time.Millisecond)

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
		t.Fatal("expected error after exhausted retries")
	}
	// 1 initial + 2 retries = 3 calls.
	if mock.callCount != 3 {
		t.Errorf("call count = %d, want 3", mock.callCount)
	}
}

func TestInfluxWriter_RetryContextCanceled(t *testing.T) {
	mock := &mockBlockingAPI{
		failUntil: 100,
		failErr:   fmt.Errorf("connection refused"),
	}
	w := newTestInfluxWriterWithAPI(mock, 5, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())

	metrics := []DriveMetrics{
		{
			Drive:  Drive{Path: "/dev/sata1", Name: "sata1", Type: DriveTypeSATA},
			Model:  "WD",
			Serial: "WD001",
			Fields: map[string]interface{}{"temperature": int64(34)},
		},
	}

	// Cancel after first failure during retry delay.
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	err := w.Write(ctx, "nas01", metrics)
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}

func TestInfluxWriter_HealthTracking(t *testing.T) {
	mock := &mockBlockingAPI{
		failUntil: 100,
		failErr:   fmt.Errorf("connection refused"),
	}
	w := newTestInfluxWriterWithAPI(mock, 0, time.Millisecond) // No retries.

	metrics := []DriveMetrics{
		{
			Drive:  Drive{Path: "/dev/sata1", Name: "sata1", Type: DriveTypeSATA},
			Model:  "WD",
			Serial: "WD001",
			Fields: map[string]interface{}{"temperature": int64(34)},
		},
	}

	if !w.Healthy() {
		t.Fatal("should start healthy")
	}

	// First two failures: still healthy.
	w.Write(context.Background(), "nas01", metrics)
	if !w.Healthy() {
		t.Error("should be healthy after 1 failure")
	}
	w.Write(context.Background(), "nas01", metrics)
	if !w.Healthy() {
		t.Error("should be healthy after 2 failures")
	}

	// Third consecutive failure: unhealthy.
	w.Write(context.Background(), "nas01", metrics)
	if w.Healthy() {
		t.Error("should be unhealthy after 3 consecutive failures")
	}
}

func TestInfluxWriter_HealthRecovery(t *testing.T) {
	mock := &mockBlockingAPI{
		failUntil: 2,
		failErr:   fmt.Errorf("connection refused"),
	}
	w := newTestInfluxWriterWithAPI(mock, 0, time.Millisecond) // No retries.

	metrics := []DriveMetrics{
		{
			Drive:  Drive{Path: "/dev/sata1", Name: "sata1", Type: DriveTypeSATA},
			Model:  "WD",
			Serial: "WD001",
			Fields: map[string]interface{}{"temperature": int64(34)},
		},
	}

	// Fail twice, then succeed — verify health resets.
	w.Write(context.Background(), "nas01", metrics)
	w.Write(context.Background(), "nas01", metrics)

	// Third call succeeds (mock.failUntil=2, so call 3 succeeds).
	err := w.Write(context.Background(), "nas01", metrics)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if !w.Healthy() {
		t.Error("should be healthy after successful write")
	}
}
