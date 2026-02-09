package main

import (
	"context"
	"fmt"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
)

// MetricWriter abstracts InfluxDB writes.
type MetricWriter interface {
	Write(ctx context.Context, hostname string, metrics []DriveMetrics) error
	Close()
}

// InfluxWriter writes SMART metrics to InfluxDB v2.
type InfluxWriter struct {
	client   influxdb2.Client
	writeAPI api.WriteAPIBlocking
}

func NewInfluxWriter(cfg InfluxDBConfig) *InfluxWriter {
	client := influxdb2.NewClient(cfg.URL, cfg.Token)
	writeAPI := client.WriteAPIBlocking(cfg.Org, cfg.Bucket)
	return &InfluxWriter{
		client:   client,
		writeAPI: writeAPI,
	}
}

func (w *InfluxWriter) Write(ctx context.Context, hostname string, metrics []DriveMetrics) error {
	points := buildPoints(hostname, metrics, time.Now())
	return w.writeAPI.WritePoint(ctx, points...)
}

func (w *InfluxWriter) Close() {
	w.client.Close()
}

func buildPoints(hostname string, metrics []DriveMetrics, ts time.Time) []*write.Point {
	points := make([]*write.Point, 0, len(metrics))
	for _, m := range metrics {
		p := influxdb2.NewPointWithMeasurement("smart").
			AddTag("hostname", hostname).
			AddTag("device", m.Drive.Name).
			AddTag("model", m.Model).
			AddTag("serial", m.Serial).
			AddTag("type", string(m.Drive.Type)).
			SetTime(ts)

		for k, v := range m.Fields {
			p.AddField(k, v)
		}

		points = append(points, p)
	}
	return points
}

// pointRecorder captures points for testing without a real InfluxDB.
type pointRecorder struct {
	points []*write.Point
	err    error
}

func (r *pointRecorder) WritePoint(ctx context.Context, points ...*write.Point) error {
	if r.err != nil {
		return r.err
	}
	r.points = append(r.points, points...)
	return nil
}

// mockWriter implements MetricWriter for testing.
type mockWriter struct {
	written  [][]DriveMetrics
	hostname string
	closed   bool
	err      error
}

func (m *mockWriter) Write(ctx context.Context, hostname string, metrics []DriveMetrics) error {
	if m.err != nil {
		return m.err
	}
	m.hostname = hostname
	m.written = append(m.written, metrics)
	return nil
}

func (m *mockWriter) Close() {
	m.closed = true
}

func newMockWriter(err error) *mockWriter {
	return &mockWriter{err: err}
}

// testableInfluxWriter wraps a pointRecorder for unit testing point construction.
type testableInfluxWriter struct {
	recorder *pointRecorder
}

func newTestableInfluxWriter(writeErr error) *testableInfluxWriter {
	return &testableInfluxWriter{
		recorder: &pointRecorder{err: writeErr},
	}
}

func (w *testableInfluxWriter) Write(ctx context.Context, hostname string, metrics []DriveMetrics) error {
	points := buildPoints(hostname, metrics, time.Now())
	return w.recorder.WritePoint(ctx, points...)
}

func (w *testableInfluxWriter) Close() {}

func (w *testableInfluxWriter) Points() []*write.Point {
	return w.recorder.points
}

// verifyPointTag checks a tag value on a write.Point using the TagList method.
func verifyPointTag(p *write.Point, key, want string) error {
	for _, tag := range p.TagList() {
		if tag.Key == key {
			if tag.Value != want {
				return fmt.Errorf("tag %q = %q, want %q", key, tag.Value, want)
			}
			return nil
		}
	}
	return fmt.Errorf("tag %q not found", key)
}

// verifyPointField checks a field value on a write.Point using the FieldList method.
func verifyPointField(p *write.Point, key string) (interface{}, error) {
	for _, field := range p.FieldList() {
		if field.Key == key {
			return field.Value, nil
		}
	}
	return nil, fmt.Errorf("field %q not found", key)
}
