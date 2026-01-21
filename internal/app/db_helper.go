package app

import (
	"fmt"
	"time"

	"github.com/timshannon/bolthold"

	"deep_in_memory/internal/constants"
	"deep_in_memory/internal/data"
	storedb "deep_in_memory/internal/thirdparty/db"
	uimsg "deep_in_memory/internal/ui/msg"
)

// DBHelper provides consistent database operations with error handling
type DBHelper struct {
	db            storedb.Db[data.ProcessData]
	errorCallback func(error) // Callback to report non-critical errors to UI
}

// NewDBHelper creates a new database helper
func NewDBHelper(db storedb.Db[data.ProcessData], errorCallback func(error)) *DBHelper {
	return &DBHelper{
		db:            db,
		errorCallback: errorCallback,
	}
}

// SetErrorCallback sets the callback function for reporting non-critical errors
func (h *DBHelper) SetErrorCallback(callback func(error)) {
	h.errorCallback = callback
}

// reportError sends a non-critical error to the UI if callback is set
func (h *DBHelper) reportError(err error) {
	if h.errorCallback != nil {
		h.errorCallback(err)
	}
}

// GetOrCreateProcess retrieves a process or creates a new one if it doesn't exist
func (h *DBHelper) GetOrCreateProcess(pid int, name string) (*data.ProcessData, bool, error) {

	// Try to find existing process
	records, err := h.db.Query(bolthold.Where("Proc.PID").Eq(pid))
	if err != nil {
		return nil, false, fmt.Errorf("failed to query process %d: %w", pid, err)
	}

	if len(records) > 0 {
		return &records[0], false, nil // Found existing process
	}

	// Create new process
	record := &data.ProcessData{
		Proc: data.Process{
			PID:       pid,
			Name:      name,
			StartTime: time.Now().UTC(),
		},
		Threads: []data.ThreadData{},
	}

	return record, true, nil // Created new process, isNew = true
}

// UpsertProcess safely stores a process record with logging
func (h *DBHelper) UpsertProcess(pid int, record *data.ProcessData) error {
	if record == nil {
		return fmt.Errorf("cannot upsert nil process record for PID %d", pid)
	}

	key := fmt.Sprintf("%d", pid)
	if err := h.db.Upsert(key, record); err != nil {
		return fmt.Errorf("failed to upsert process %d: %w", pid, err)
	}

	return nil
}

// HandleProcessExec handles process execution events
func (h *DBHelper) HandleProcessExec(ev uimsg.ProcessExec) error {
	record := &data.ProcessData{
		Proc: data.Process{
			PID:       ev.PID,
			Name:      ev.Comm,
			StartTime: ev.Timestamp.UTC(),
		},
		Threads: []data.ThreadData{},
	}

	return h.UpsertProcess(ev.PID, record)
}

// HandleProcessExit handles process exit events
func (h *DBHelper) HandleProcessExit(ev uimsg.ProcessExit) error {
	key := fmt.Sprintf("%d", ev.PID)
	if err := h.db.Delete(key); err != nil {
		return fmt.Errorf("failed to delete process %d: %w", ev.PID, err)
	}
	return nil
}

// HandleMmapEvent handles mmap events for a process
func (h *DBHelper) HandleMmapEvent(ev uimsg.ProcessMmap) error {
	record, _, err := h.GetOrCreateProcess(ev.PID, ev.Comm)
	if err != nil {
		return fmt.Errorf("failed to get/create process for mmap %d: %w", ev.PID, err)
	}

	record.AddMmapRange(ev.TID, ev.Addr, ev.Len)
	return h.UpsertProcess(ev.PID, record)
}

// HandleBrkEvent handles brk events for a process
func (h *DBHelper) HandleBrkEvent(ev uimsg.ProcessBrk) error {
	record, _, err := h.GetOrCreateProcess(ev.PID, ev.Comm)
	if err != nil {
		return fmt.Errorf("failed to get/create process for brk %d: %w", ev.PID, err)
	}

	record.ExtendBrkRange(ev.TID, ev.Brk, ev.Ret)
	return h.UpsertProcess(ev.PID, record)
}

// HandleMunmapEvent handles munmap events for a process
func (h *DBHelper) HandleMunmapEvent(ev uimsg.ProcessMunmap) error {
	records, err := h.db.Query(bolthold.Where("Proc.PID").Eq(ev.PID))
	if err != nil {
		return fmt.Errorf("failed to query process for munmap %d: %w", ev.PID, err)
	}

	if len(records) == 0 {
		// Process not found, nothing to unmap - report but don't fail
		h.reportError(fmt.Errorf("munmap event for unknown process PID %d (process may have exited)", ev.PID))
		return nil
	}

	record := &records[0]
	record.RemoveMmapRange(ev.TID, ev.Addr, ev.Len)
	return h.UpsertProcess(ev.PID, record)
}

// HandlePageFaultEvent handles page fault events for a process
func (h *DBHelper) HandlePageFaultEvent(ev uimsg.ProcessPageFault) error {
	// Filter page faults by error code (only handle user write faults)
	if ev.ErrorCode != constants.PageFaultErrorCode {
		return nil
	}

	records, err := h.db.Query(bolthold.Where("Proc.PID").Eq(ev.PID))
	if err != nil {
		return fmt.Errorf("failed to query process for page fault %d: %w", ev.PID, err)
	}

	if len(records) == 0 {
		// Process not found for page fault - report but don't fail
		h.reportError(fmt.Errorf("page fault event for unknown process PID %d (process may have exited)", ev.PID))
		return nil
	}

	record := &records[0]
	record.AddPageFault(ev.TID, ev.Address)
	return h.UpsertProcess(ev.PID, record)
}

// HandleSwapEvent handles swap events for a process
func (h *DBHelper) HandleSwapEvent(ev uimsg.ProcessSwapUnswap) error {
	// Filter swap events by error code
	if ev.ErrorCode != constants.SwapEventSuccessCode {
		return nil
	}

	records, err := h.db.Query(bolthold.Where("Proc.PID").Eq(ev.PID))
	if err != nil {
		return fmt.Errorf("failed to query process for swap %d: %w", ev.PID, err)
	}

	if len(records) == 0 {
		// Process not found for swap event - report but don't fail
		h.reportError(fmt.Errorf("swap event for unknown process PID %d (process may have exited)", ev.PID))
		return nil
	}

	record := &records[0]
	record.AddSwapStatus(ev.TID, ev.Address, ev.IsSwap)
	return h.UpsertProcess(ev.PID, record)
}

// LoadSmapsForProcess loads memory ranges from /proc/PID/smaps for a process
func (h *DBHelper) LoadSmapsForProcess(pid int) error {
	threads, err := data.LoadSmapsRanges(pid)
	if err != nil {
		// Failed to load smaps - report but don't fail
		h.reportError(fmt.Errorf("failed to load smaps for process %d: %w", pid, err))
		return nil
	}

	records, err := h.db.Query(bolthold.Where("Proc.PID").Eq(pid))
	if err != nil {
		return fmt.Errorf("failed to query process for smaps %d: %w", pid, err)
	}

	if len(records) == 0 {
		// Process not found for smaps loading - report but don't fail
		h.reportError(fmt.Errorf("process %d not found in database for smaps loading", pid))
		return nil
	}

	record := &records[0]
	if len(threads) > 0 {
		// Replace or merge threads - for simplicity, replace all ranges
		record.Threads = threads
	}

	return h.UpsertProcess(pid, record)
}
