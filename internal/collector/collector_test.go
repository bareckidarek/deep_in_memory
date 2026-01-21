//go:build ebpf

package collector

import (
	"context"
	"testing"
	"time"

	"deep_in_memory/internal/persistence"
)

// TestCollectorBasic tests basic collector functionality with temp storage
func TestCollectorBasic(t *testing.T) {
	// Create temp storage
	procDBPath := t.TempDir() + "/test_proc"
	addrDBPath := t.TempDir() + "/test_addr"

	writer, reader, err := persistence.Open(procDBPath, addrDBPath)
	if err != nil {
		t.Fatalf("failed to open storage: %v", err)
	}
	defer writer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := CollectorConfig{
		ErrorCallback: func(err error) {
			t.Logf("Collector error: %v", err)
		},
	}

	// Note: This test will fail on systems without eBPF support
	// In CI/CD, we should use build tags to skip eBPF tests
	_, cleanup, err := StartCollector(ctx, cfg, writer, reader)
	if err != nil {
		// Expected on systems without eBPF or without root privileges
		t.Skipf("Skipping test - eBPF collector failed to start (expected without root/eBPF): %v", err)
		return
	}
	defer cleanup()

	// If we got here, eBPF is available
	// Wait a bit to see if any events come through
	time.Sleep(100 * time.Millisecond)

	// Read from storage to verify collector is writing
	procs, err := reader.AllProcessData(ctx)
	if err != nil {
		t.Fatalf("failed to read process data: %v", err)
	}

	// We may or may not have processes depending on system activity
	t.Logf("Found %d processes in storage", len(procs))
}

// TestCollectorSetTargetPID tests target PID setting
func TestCollectorSetTargetPID(t *testing.T) {
	procDBPath := t.TempDir() + "/test_proc"
	addrDBPath := t.TempDir() + "/test_addr"

	writer, reader, err := persistence.Open(procDBPath, addrDBPath)
	if err != nil {
		t.Fatalf("failed to open storage: %v", err)
	}
	defer writer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := CollectorConfig{}
	collector, cleanup, err := StartCollector(ctx, cfg, writer, reader)
	if err != nil {
		t.Skipf("Skipping test - eBPF collector failed to start: %v", err)
		return
	}
	defer cleanup()

	// Test setting target PID
	err = collector.SetTargetPID(1234)
	if err != nil {
		t.Errorf("failed to set target PID: %v", err)
	}

	if collector.targetPID != 1234 {
		t.Errorf("expected target PID 1234, got %d", collector.targetPID)
	}
}

// TestCollectorLogAggregator tests log aggregator access
func TestCollectorLogAggregator(t *testing.T) {
	procDBPath := t.TempDir() + "/test_proc"
	addrDBPath := t.TempDir() + "/test_addr"

	writer, reader, err := persistence.Open(procDBPath, addrDBPath)
	if err != nil {
		t.Fatalf("failed to open storage: %v", err)
	}
	defer writer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := CollectorConfig{}
	collector, cleanup, err := StartCollector(ctx, cfg, writer, reader)
	if err != nil {
		t.Skipf("Skipping test - eBPF collector failed to start: %v", err)
		return
	}
	defer cleanup()

	// Test log aggregator access
	logAgg := collector.GetLogAggregator()
	if logAgg == nil {
		t.Error("expected non-nil log aggregator")
	}

	// Verify it returns formatted logs (should be empty initially)
	logs := logAgg.GetFormattedLogs()
	if logs == nil {
		// Empty is ok, should return nil or empty slice
		t.Logf("No logs initially (expected)")
	}
}
