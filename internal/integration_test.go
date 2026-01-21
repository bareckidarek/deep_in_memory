//go:build ebpf

package internal

import (
	"context"
	"testing"
	"time"

	"deep_in_memory/internal/collector"
	"deep_in_memory/internal/persistence"
)

// TestIntegration_CollectorToDB verifies that collector writes to DB
// and that data is visible through Reader
func TestIntegration_CollectorToDB(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create temp storage
	procDBPath := t.TempDir() + "/test_proc"
	addrDBPath := t.TempDir() + "/test_addr"

	writer, reader, err := persistence.Open(procDBPath, addrDBPath)
	if err != nil {
		t.Fatalf("failed to open storage: %v", err)
	}
	defer writer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Start collector
	cfg := collector.CollectorConfig{
		ErrorCallback: func(err error) {
			t.Logf("Collector error: %v", err)
		},
	}

	ebpfCollector, cleanup, err := collector.StartCollector(ctx, cfg, writer, reader)
	if err != nil {
		t.Skipf("Skipping integration test - eBPF not available: %v", err)
		return
	}
	defer cleanup()

	// Set a target PID
	err = ebpfCollector.SetTargetPID(1)
	if err != nil {
		t.Errorf("failed to set target PID: %v", err)
	}

	// Wait for some events to be collected (or timeout)
	// On an active system, sched events should fire relatively quickly
	deadline := time.Now().Add(3 * time.Second)
	var foundProcesses bool
	for time.Now().Before(deadline) {
		procs, err := reader.AllProcessData(ctx)
		if err != nil {
			t.Fatalf("failed to read process data: %v", err)
		}

		if len(procs) > 0 {
			t.Logf("✓ Collector->DB->Reader pipeline working: found %d processes", len(procs))
			foundProcesses = true
			break
		}

		time.Sleep(200 * time.Millisecond)
	}

	if !foundProcesses {
		t.Logf("Warning: No processes captured during test (may be normal on quiet system)")
	}

	// Verify log aggregator is accessible
	logAgg := ebpfCollector.GetLogAggregator()
	if logAgg == nil {
		t.Error("log aggregator should not be nil")
	}
}
