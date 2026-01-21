package persistence

import (
	"context"
	"os"
	"testing"
	"time"

	"deep_in_memory/internal/data"
)

func TestPersistence(t *testing.T) {
	// Create temp files for test databases
	procDBPath := t.TempDir() + "/test_proc"
	addrDBPath := t.TempDir() + "/test_addr"

	writer, reader, err := Open(procDBPath, addrDBPath)
	if err != nil {
		t.Fatalf("failed to open storage: %v", err)
	}
	defer writer.Close()

	ctx := context.Background()

	// Test process data write and read
	testProc := &data.ProcessData{
		Proc: data.Process{
			PID:       1234,
			Name:      "test_process",
			StartTime: time.Now().UTC(),
		},
		Threads: []data.ThreadData{},
	}

	err = writer.WriteProcessData(ctx, "1234", testProc)
	if err != nil {
		t.Fatalf("failed to write process data: %v", err)
	}

	// Read back
	procs, err := reader.AllProcessData(ctx)
	if err != nil {
		t.Fatalf("failed to read process data: %v", err)
	}

	if len(procs) != 1 {
		t.Fatalf("expected 1 process, got %d", len(procs))
	}

	if procs[0].Proc.PID != 1234 {
		t.Errorf("expected PID 1234, got %d", procs[0].Proc.PID)
	}

	if procs[0].Proc.Name != "test_process" {
		t.Errorf("expected name 'test_process', got '%s'", procs[0].Proc.Name)
	}

	// Test address data
	testAddr := &data.AddressData{
		PID: 1234,
	}
	// add a mmap/address range using provided helper
	testAddr.AddMmapAddressRange(1, 0x1000, 4096)

	err = writer.WriteAddressData(ctx, "1234", testAddr)
	if err != nil {
		t.Fatalf("failed to write address data: %v", err)
	}

	addrs, err := reader.AllAddressData(ctx)
	if err != nil {
		t.Fatalf("failed to read address data: %v", err)
	}

	if len(addrs) != 1 {
		t.Fatalf("expected 1 address entry, got %d", len(addrs))
	}

	if addrs[0].PID != 1234 {
		t.Errorf("expected PID 1234, got %d", addrs[0].PID)
	}

	// Test delete
	err = writer.DeleteProcessData(ctx, "1234")
	if err != nil {
		t.Fatalf("failed to delete process data: %v", err)
	}

	procs, err = reader.AllProcessData(ctx)
	if err != nil {
		t.Fatalf("failed to read after delete: %v", err)
	}

	if len(procs) != 0 {
		t.Errorf("expected 0 processes after delete, got %d", len(procs))
	}
}

func TestOpen_InvalidPath(t *testing.T) {
	_, _, err := Open("/invalid/path/that/does/not/exist", "/another/invalid")
	if err == nil {
		t.Fatal("expected error when opening invalid path, got nil")
	}
}

func TestClose(t *testing.T) {
	procDBPath := t.TempDir() + "/test_proc"
	addrDBPath := t.TempDir() + "/test_addr"

	writer, _, err := Open(procDBPath, addrDBPath)
	if err != nil {
		t.Fatalf("failed to open storage: %v", err)
	}

	err = writer.Close()
	if err != nil {
		t.Errorf("failed to close storage: %v", err)
	}

	// Verify files were created
	if _, err := os.Stat(procDBPath + ".db"); os.IsNotExist(err) {
		t.Error("process DB file was not created")
	}
	if _, err := os.Stat(addrDBPath + ".db"); os.IsNotExist(err) {
		t.Error("address DB file was not created")
	}
}
