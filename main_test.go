package main

import (
	"bytes"
	"context"
	"log"
	"os"
	"testing"
	"time"
)

func TestRunLoop_CollectsAndShutdown(t *testing.T) {
	drives := []Drive{
		{Path: "/dev/sata1", Name: "sata1", Type: DriveTypeSATA},
	}
	runner := &mockRunner{
		outputs: map[string]*SmartctlOutput{
			"/dev/sata1": makeSATAOutput("WD Red", "WD001"),
		},
		errs: map[string]error{},
	}

	collector := NewCollector(&mockDiscoverer{drives: drives}, runner)
	writer := newMockWriter(nil)

	cfg := &Config{
		Interval: 50 * time.Millisecond,
		Hostname: "test-nas",
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		run(ctx, cfg, collector, writer)
		close(done)
	}()

	// Let a few ticks happen.
	time.Sleep(180 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("run loop did not shut down in time")
	}

	// Should have at least 2 writes: immediate + ticker ticks.
	if len(writer.written) < 2 {
		t.Errorf("got %d writes, want at least 2", len(writer.written))
	}
}

func TestRunLoop_GracefulShutdown(t *testing.T) {
	collector := NewCollector(&mockDiscoverer{drives: nil}, &mockRunner{})
	writer := newMockWriter(nil)

	cfg := &Config{
		Interval: 1 * time.Hour,
		Hostname: "test-nas",
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		run(ctx, cfg, collector, writer)
		close(done)
	}()

	// Cancel immediately after startup collection.
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("run loop did not shut down in time")
	}

	if !writer.closed {
		t.Error("writer.Close() was not called")
	}
}

func TestRunLoop_WriterErrorDoesNotCrash(t *testing.T) {
	drives := []Drive{
		{Path: "/dev/sata1", Name: "sata1", Type: DriveTypeSATA},
	}
	runner := &mockRunner{
		outputs: map[string]*SmartctlOutput{
			"/dev/sata1": makeSATAOutput("WD Red", "WD001"),
		},
		errs: map[string]error{},
	}

	collector := NewCollector(&mockDiscoverer{drives: drives}, runner)
	writer := newMockWriter(context.DeadlineExceeded)

	cfg := &Config{
		Interval: 50 * time.Millisecond,
		Hostname: "test-nas",
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		run(ctx, cfg, collector, writer)
		close(done)
	}()

	time.Sleep(120 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("run loop did not shut down in time")
	}
	// If we get here, the loop survived write errors.
}

func TestRunOnce(t *testing.T) {
	drives := []Drive{
		{Path: "/dev/sata1", Name: "sata1", Type: DriveTypeSATA},
	}
	runner := &mockRunner{
		outputs: map[string]*SmartctlOutput{
			"/dev/sata1": makeSATAOutput("WD Red", "WD001"),
		},
		errs: map[string]error{},
	}

	collector := NewCollector(&mockDiscoverer{drives: drives}, runner)
	writer := newMockWriter(nil)

	cfg := &Config{
		Interval: 1 * time.Hour,
		Hostname: "test-nas",
		RunOnce:  true,
	}

	ctx := context.Background()
	run(ctx, cfg, collector, writer)

	if len(writer.written) != 1 {
		t.Errorf("got %d writes, want exactly 1", len(writer.written))
	}
}

func TestEchoWriter(t *testing.T) {
	w := &echoWriter{}

	metrics := []DriveMetrics{
		{
			Drive:  Drive{Path: "/dev/sata1", Name: "sata1", Type: DriveTypeSATA},
			Model:  "WD Red",
			Serial: "WD001",
			Fields: map[string]interface{}{
				"temperature":    int64(34),
				"power_on_hours": int64(1000),
			},
		},
		{
			Drive:  Drive{Path: "/dev/nvme0n1", Name: "nvme0n1", Type: DriveTypeNVMe},
			Model:  "Samsung 970",
			Serial: "SAM001",
			Fields: map[string]interface{}{
				"temperature":     int64(38),
				"percentage_used": int64(5),
			},
		},
	}

	// Capture stdout.
	r, wPipe, _ := os.Pipe()
	origStdout := os.Stdout
	os.Stdout = wPipe

	err := w.Write(context.Background(), "test-nas", metrics)

	wPipe.Close()
	os.Stdout = origStdout

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out bytes.Buffer
	out.ReadFrom(r)
	output := out.String()

	// Verify both drives appear.
	if !bytes.Contains([]byte(output), []byte("drive=sata1")) {
		t.Error("output missing sata1 drive")
	}
	if !bytes.Contains([]byte(output), []byte("drive=nvme0n1")) {
		t.Error("output missing nvme0n1 drive")
	}
	if !bytes.Contains([]byte(output), []byte("temperature = 34")) {
		t.Error("output missing temperature field for sata1")
	}
	if !bytes.Contains([]byte(output), []byte("percentage_used = 5")) {
		t.Error("output missing percentage_used field for nvme0n1")
	}
}

func TestRunOnceWithEcho(t *testing.T) {
	drives := []Drive{
		{Path: "/dev/sata1", Name: "sata1", Type: DriveTypeSATA},
	}
	runner := &mockRunner{
		outputs: map[string]*SmartctlOutput{
			"/dev/sata1": makeSATAOutput("WD Red", "WD001"),
		},
		errs: map[string]error{},
	}

	collector := NewCollector(&mockDiscoverer{drives: drives}, runner)
	writer := &echoWriter{}

	cfg := &Config{
		Interval: 1 * time.Hour,
		Hostname: "test-nas",
		RunOnce:  true,
	}

	// Redirect stdout to capture echo output.
	r, wPipe, _ := os.Pipe()
	origStdout := os.Stdout
	os.Stdout = wPipe

	// Redirect log output to stderr so it doesn't go to our pipe.
	log.SetOutput(os.Stderr)
	defer log.SetOutput(os.Stderr)

	ctx := context.Background()
	run(ctx, cfg, collector, writer)

	wPipe.Close()
	os.Stdout = origStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)

	if !bytes.Contains(buf.Bytes(), []byte("drive=sata1")) {
		t.Error("echo output missing sata1")
	}
}
