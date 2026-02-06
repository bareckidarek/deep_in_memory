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
	addressDB     storedb.Db[data.AddressData]
	errorCallback func(error) // Callback to report non-critical errors to UI
}

// NewDBHelper creates a new database helper
func NewDBHelper(db storedb.Db[data.ProcessData], addressDB storedb.Db[data.AddressData], errorCallback func(error)) *DBHelper {
	return &DBHelper{
		db:            db,
		addressDB:     addressDB,
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

	// Delete from both databases
	if err := h.db.Delete(key); err != nil {
		return fmt.Errorf("failed to delete process %d: %w", ev.PID, err)
	}

	// Also delete from address database (ignore errors if not found)
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
	key := fmt.Sprintf("%d", ev.PID)
	if err := h.db.Upsert(key, record); err != nil {
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
	key := fmt.Sprintf("%d", ev.PID)
	if err := h.db.Upsert(key, record); err != nil {
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
	key := fmt.Sprintf("%d", ev.PID)
	if err := h.db.Upsert(key, record); err != nil {
		return fmt.Errorf("failed to upsert process: %w", err)
	}

	// Note: We don't remove from AddressData - it will just have orphaned ranges
	// which is fine since they won't be accessed anymore
	return nil
}

// HandlePageFaultEvent handles page fault events for a process
func (h *DBHelper) HandlePageFaultEvent(ev uimsg.ProcessPageFault) error {
	// Filter page faults by error code (only handle user write faults)
	if ev.ErrorCode != constants.PageFaultErrorCode {
		return nil
	}

	record, err := h.getProcess(ev.PID)
	if err != nil {
		return fmt.Errorf("failed to get process for page fault %d: %w", ev.PID, err)
	}

	if record == nil {
		// Process not found - ignore stale event
		return nil
	}

	// Calculate page address and offset
	pageAddr := (ev.Address / data.PageSizeBytes) * data.PageSizeBytes

	// Find the range containing this address
	r := record.FindRangeForAddr(ev.TID, pageAddr)
	if r == nil {
		// Range not found - this shouldn't happen but handle gracefully
		return nil
	}

	pageOffset := (pageAddr - r.StartVal) / data.PageSizeBytes

	// --- UPDATE ADDRESS DATABASE ---
	// Get or create address data
	addrData, err := h.getOrCreateAddressData(ev.PID)
	if err != nil {
		return fmt.Errorf("failed to get address data: %w", err)
	}

	// Add page offset directly to AddressData
	addrData.AddPageOffset(ev.TID, r.StartVal, pageOffset, "INIT")

	// Save AddressData
	addrKey := fmt.Sprintf("%d", ev.PID)
	if err := h.addressDB.Upsert(addrKey, addrData); err != nil {
		return fmt.Errorf("failed to upsert address data: %w", err)
	}

	// --- UPDATE MAIN DATABASE ---
	// Get counts from AddressData and update ProcessData
	counts := addrData.GetStatusCounts(ev.TID, r.StartVal)

	// Update counts in ProcessData
	if r.Statuses == nil {
		r.Statuses = make(map[string]*data.StatusCount)
	}
	for status, count := range counts {
		if r.Statuses[status] == nil {
			r.Statuses[status] = &data.StatusCount{}
		}
		r.Statuses[status].Count = count
	}

	// Save ProcessData
	key := fmt.Sprintf("%d", ev.PID)
	if err := h.db.Upsert(key, record); err != nil {
		return fmt.Errorf("failed to upsert process: %w", err)
	}

	return nil
}

// HandleSwapEvent handles swap events for a process
func (h *DBHelper) HandleSwapEvent(ev uimsg.ProcessSwapUnswap) error {
	// Filter swap events by error code
	if ev.ErrorCode != constants.SwapEventSuccessCode {
		return nil
	}

	record, err := h.getProcess(ev.PID)
	if err != nil {
		return fmt.Errorf("failed to get process for swap %d: %w", ev.PID, err)
	}

	if record == nil {
		// Process not found for swap event - report but don't fail
		h.reportError(fmt.Errorf("swap event for unknown process PID %d (process may have exited)", ev.PID))
		return nil
	}

	// Calculate page address and offset
	pageAddr := (ev.Address / data.PageSizeBytes) * data.PageSizeBytes

	// Find the range containing this address
	r := record.FindRangeForAddr(ev.TID, pageAddr)
	if r == nil {
		// Range not found - this shouldn't happen but handle gracefully
		return nil
	}

	pageOffset := (pageAddr - r.StartVal) / data.PageSizeBytes

	// --- UPDATE ADDRESS DATABASE ---
	// Get or create address data
	addrData, err := h.getOrCreateAddressData(ev.PID)
	if err != nil {
		return fmt.Errorf("failed to get address data: %w", err)
	}

	// Add page offset directly to AddressData with appropriate status
	if ev.IsSwap {
		addrData.AddPageOffset(ev.TID, r.StartVal, pageOffset, "SWAP")
	} else {
		addrData.AddPageOffset(ev.TID, r.StartVal, pageOffset, "UNSWAP")
	}

	// Save AddressData
	addrKey := fmt.Sprintf("%d", ev.PID)
	if err := h.addressDB.Upsert(addrKey, addrData); err != nil {
		return fmt.Errorf("failed to upsert address data: %w", err)
	}

	// --- UPDATE MAIN DATABASE ---
	// Get counts from AddressData and update ProcessData
	counts := addrData.GetStatusCounts(ev.TID, r.StartVal)

	// Update counts in ProcessData
	if r.Statuses == nil {
		r.Statuses = make(map[string]*data.StatusCount)
	}
	for status, count := range counts {
		if r.Statuses[status] == nil {
			r.Statuses[status] = &data.StatusCount{}
		}
		r.Statuses[status].Count = count
	}

	// Save ProcessData
	key := fmt.Sprintf("%d", ev.PID)
	if err := h.db.Upsert(key, record); err != nil {
		return fmt.Errorf("failed to upsert process: %w", err)
	}

	return nil
}

// LoadSmapsForProcess loads memory ranges from /proc/PID/smaps for a process
func (h *DBHelper) LoadSmapsForProcess(pid int) error {
	threads, err := data.LoadSmapsRanges(pid)
	if err != nil {
		// Failed to load smaps - ignore (process may have exited)
		return nil
	}

	record, err := h.getProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to get process for smaps %d: %w", pid, err)
	}

	if record == nil {
		// Process not found - ignore stale request
		return nil
	}

	if len(threads) > 0 {
		// Replace or merge threads - for simplicity, replace all ranges
		record.Threads = threads
	}

	// Save ProcessData (just structure, no Value)
	key := fmt.Sprintf("%d", pid)
	if err := h.db.Upsert(key, record); err != nil {
		return fmt.Errorf("failed to upsert process: %w", err)
	}

	// AddressData will be populated when first page fault/swap occurs
	return nil
}
