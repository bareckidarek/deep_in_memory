package app

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/timshannon/bolthold"

	"deep_in_memory/internal/constants"
	"deep_in_memory/internal/data"
	storedb "deep_in_memory/internal/thirdparty/db"
	uimsg "deep_in_memory/internal/ui/msg"
)

// DBHelper provides consistent database operations with error handling.
// Thread-safe: errorCallback is protected by mutex for concurrent access.
type DBHelper struct {
	db            storedb.Db[data.ProcessData]
	addressDB     storedb.Db[data.AddressData]
	mu            sync.RWMutex // Protects errorCallback
	errorCallback func(error)  // Callback to report non-critical errors to UI
}

// NewDBHelper creates a new database helper
func NewDBHelper(db storedb.Db[data.ProcessData], addressDB storedb.Db[data.AddressData], errorCallback func(error)) *DBHelper {
	return &DBHelper{
		db:            db,
		addressDB:     addressDB,
		errorCallback: errorCallback,
	}
}

// SetErrorCallback sets the callback function for reporting non-critical errors.
// Thread-safe: can be called while reportError is executing.
func (h *DBHelper) SetErrorCallback(callback func(error)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.errorCallback = callback
}

// reportError sends a non-critical error to the UI if callback is set.
// Thread-safe: reads callback under lock to prevent race with SetErrorCallback.
func (h *DBHelper) reportError(err error) {
	h.mu.RLock()
	cb := h.errorCallback
	h.mu.RUnlock()
	if cb != nil {
		cb(err)
	}
}

// pidKey returns the string key used for database operations.
func pidKey(pid int) string {
	return fmt.Sprintf("%d", pid)
}

// GetOrCreateProcess retrieves a process or creates a new one if it doesn't exist
// Returns error if the process has already exited (prevents recreation from stale events)
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

// getProcess retrieves a process from database without creating it
// Returns nil if process doesn't exist or has exited
func (h *DBHelper) getProcess(pid int) (*data.ProcessData, error) {

	// Try to find existing process
	records, err := h.db.Query(bolthold.Where("Proc.PID").Eq(pid))
	if err != nil {
		return nil, fmt.Errorf("failed to query process %d: %w", pid, err)
	}

	if len(records) > 0 {
		return &records[0], nil
	}

	// Process not found
	return nil, nil
}

// getOrCreateAddressData retrieves AddressData for a process or creates new one
func (h *DBHelper) getOrCreateAddressData(pid int) (*data.AddressData, error) {
	records, err := h.addressDB.Query(bolthold.Where("PID").Eq(pid))
	if err != nil {
		return nil, fmt.Errorf("failed to query address data for %d: %w", pid, err)
	}

	if len(records) > 0 {
		return &records[0], nil
	}

	// Create new AddressData
	return &data.AddressData{
		PID:     pid,
		Threads: []data.ThreadAddressData{},
	}, nil
}

// UpsertProcess safely stores a process record to main database
// Note: Address database is updated separately by HandlePageFaultEvent and HandleSwapEvent
func (h *DBHelper) UpsertProcess(pid int, record *data.ProcessData) error {
	if record == nil {
		return fmt.Errorf("cannot upsert nil process record for PID %d", pid)
	}

	// Save record to main database (only Count fields, no Value)
	if err := h.db.Upsert(pidKey(pid), record); err != nil {
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
	key := pidKey(ev.PID)

	// Delete from both databases - errors are silently ignored as this can
	// occur for any process in the OS, including those not being tracked
	h.db.Delete(key)
	h.addressDB.Delete(key)

	return nil
}

// HandleMmapEvent handles mmap events for a process
// Only processes existing processes - does not create new ones
func (h *DBHelper) HandleMmapEvent(ev uimsg.ProcessMmap) error {
	record, err := h.getProcess(ev.PID)
	if err != nil {
		return fmt.Errorf("failed to get process for mmap %d: %w", ev.PID, err)
	}
	if record == nil {
		// Process not found or has exited - ignore stale event
		return nil
	}

	// Update ProcessData - add new range
	record.AddMmapRange(ev.TID, ev.Addr, ev.Len)

	// Save ProcessData (no Value, just structure)
	if err := h.db.Upsert(pidKey(ev.PID), record); err != nil {
		return fmt.Errorf("failed to upsert process: %w", err)
	}

	// AddressData will be populated when first page fault/swap occurs for this range
	return nil
}

// HandleBrkEvent handles brk events for a process
// Only processes existing processes - does not create new ones
func (h *DBHelper) HandleBrkEvent(ev uimsg.ProcessBrk) error {
	record, err := h.getProcess(ev.PID)
	if err != nil {
		return fmt.Errorf("failed to get process for brk %d: %w", ev.PID, err)
	}
	if record == nil {
		// Process not found or has exited - ignore stale event
		return nil
	}

	// Update ProcessData - extend brk range
	record.ExtendBrkRange(ev.TID, ev.Brk, ev.Ret)

	// Save ProcessData (no Value, just structure)
	if err := h.db.Upsert(pidKey(ev.PID), record); err != nil {
		return fmt.Errorf("failed to upsert process: %w", err)
	}

	// AddressData will be populated when first page fault/swap occurs for this range
	return nil
}

// HandleMunmapEvent handles munmap events for a process
func (h *DBHelper) HandleMunmapEvent(ev uimsg.ProcessMunmap) error {
	record, err := h.getProcess(ev.PID)
	if err != nil {
		return fmt.Errorf("failed to query process for munmap %d: %w", ev.PID, err)
	}

	if record == nil {
		// Process not found - ignore stale event
		return nil
	}

	// Update ProcessData - remove range
	record.RemoveMmapRange(ev.TID, ev.Addr, ev.Len)

	// Save ProcessData
	if err := h.db.Upsert(pidKey(ev.PID), record); err != nil {
		return fmt.Errorf("failed to upsert process: %w", err)
	}

	// Note: We don't remove from AddressData - it will just have orphaned ranges
	// which is fine since they won't be accessed anymore
	return nil
}

// HandleSwapEvent handles swap events for a process
func (h *DBHelper) HandleSwapEvent(ev uimsg.ProcessInitSwapUnswap) error {
	// Filter swap events by error code
	if ev.ErrorCode != constants.SwapEventSuccessCode {
		return nil
	}

	status := ""
	switch ev.Type {
	case 0:
		status = "INIT"
	case 1:
		status = "SWAP"
	case 2:
		status = "UNSWAP"
	}

	log.Println("Updated page status for PID", ev.PID, "TID", ev.TID, "Address", fmt.Sprintf("0x%X", ev.Address), "Status", status)

	err := h.updatePageStatus(ev.PID, ev.TID, ev.Address, status)
	if err != nil {
		// Provide additional context for swap events on unknown processes
		record, _ := h.getProcess(ev.PID)
		if record == nil {
			h.reportError(fmt.Errorf("swap event for unknown process PID %d (process may have exited)", ev.PID))
			return nil
		}
	}
	return err
}

// updatePageStatus handles the shared logic for page fault and swap events:
// finds the process and range, updates the address database, syncs counts, and saves.
func (h *DBHelper) updatePageStatus(pid, tid int, address uint64, status string) error {
	record, err := h.getProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to get process %d: %w", pid, err)
	}
	if record == nil {
		// Process not found — it may have exited. Silently ignore.
		return nil
	}

	// Calculate page address and offset
	pageAddr := (address / data.PageSizeBytes) * data.PageSizeBytes
	r := record.FindRangeForAddr(tid, pageAddr)
	if r == nil {
		// Range not found — mmap may not have been processed yet, or the range
		// was munmapped. Silently ignore rather than error.
		return nil
	}
	pageOffset := (pageAddr - r.StartVal) / data.PageSizeBytes

	// --- UPDATE ADDRESS DATABASE ---
	addrData, err := h.getOrCreateAddressData(pid)
	if err != nil {
		return fmt.Errorf("failed to get address data: %w", err)
	}

	addrData.AddPageOffset(tid, r.StartVal, pageOffset, status)

	if err := h.addressDB.Upsert(pidKey(pid), addrData); err != nil {
		return fmt.Errorf("failed to upsert address data: %w", err)
	}

	// --- UPDATE MAIN DATABASE ---
	counts := addrData.GetStatusCounts(tid, r.StartVal)
	if r.Statuses == nil {
		r.Statuses = make(map[string]*data.StatusCount)
	}
	for s, count := range counts {
		if r.Statuses[s] == nil {
			r.Statuses[s] = &data.StatusCount{}
		}
		r.Statuses[s].Count = count
	}

	if err := h.db.Upsert(pidKey(pid), record); err != nil {
		return fmt.Errorf("failed to upsert process: %w", err)
	}
	return nil
}
