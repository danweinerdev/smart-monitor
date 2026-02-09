package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// countingCollector wraps a Collector with a call counter for testing.
type countingCollector struct {
	collector *Collector
	count     atomic.Int32
}

func (c *countingCollector) collectAll() ([]DriveMetrics, []error) {
	c.count.Add(1)
	return c.collector.CollectAll()
}

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
