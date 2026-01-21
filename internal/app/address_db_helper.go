package app

import (
	"fmt"

	"github.com/timshannon/bolthold"

	"deep_in_memory/internal/data"
	storedb "deep_in_memory/internal/thirdparty/db"
	uimsg "deep_in_memory/internal/ui/msg"
)

// AddressDBHelper provides database operations for the address offsets database
type AddressDBHelper struct {
	db            storedb.Db[data.AddressData]
	errorCallback func(error)
}

// NewAddressDBHelper creates a new address database helper
func NewAddressDBHelper(db storedb.Db[data.AddressData], errorCallback func(error)) *AddressDBHelper {
	return &AddressDBHelper{
		db:            db,
		errorCallback: errorCallback,
	}
}

// SetErrorCallback sets the callback function for reporting non-critical errors
func (h *AddressDBHelper) SetErrorCallback(callback func(error)) {
	h.errorCallback = callback
}

// reportError sends a non-critical error to the UI if callback is set
func (h *AddressDBHelper) reportError(err error) {
	if h.errorCallback != nil {
		h.errorCallback(err)
	}
}

// GetOrCreateAddressData retrieves address data or creates new one if it doesn't exist
func (h *AddressDBHelper) GetOrCreateAddressData(pid int) (*data.AddressData, bool, error) {
	// Try to find existing address data
	records, err := h.db.Query(bolthold.Where("PID").Eq(pid))
	if err != nil {
		return nil, false, fmt.Errorf("failed to query address data %d: %w", pid, err)
	}

	if len(records) > 0 {
		return &records[0], false, nil // Found existing
	}

	// Create new address data
	record := &data.AddressData{
		PID:     pid,
		Threads: []data.ThreadAddressData{},
	}

	return record, true, nil // Created new
}

// UpsertAddressData safely stores address data record
func (h *AddressDBHelper) UpsertAddressData(pid int, record *data.AddressData) error {
	if record == nil {
		return fmt.Errorf("cannot upsert nil address data for PID %d", pid)
	}

	key := fmt.Sprintf("%d", pid)
	if err := h.db.Upsert(key, record); err != nil {
		return fmt.Errorf("failed to upsert address data %d: %w", pid, err)
	}

	return nil
}

// HandleProcessExec creates initial address data for a new process
func (h *AddressDBHelper) HandleProcessExec(ev uimsg.ProcessExec) error {
	record := &data.AddressData{
		PID:     ev.PID,
		Threads: []data.ThreadAddressData{},
	}

	return h.UpsertAddressData(ev.PID, record)
}

// HandleProcessExit removes address data when process exits
func (h *AddressDBHelper) HandleProcessExit(ev uimsg.ProcessExit) error {
	key := fmt.Sprintf("%d", ev.PID)
	if err := h.db.Delete(key); err != nil {
		return fmt.Errorf("failed to delete address data %d: %w", ev.PID, err)
	}

	return nil
}

// HandleMmap handles mmap events and stores them in address database
func (h *AddressDBHelper) HandleMmap(ev uimsg.MmapEvent) error {
	record, _, err := h.GetOrCreateAddressData(ev.PID)
	if err != nil {
		return err
	}

	record.AddMmapAddressRange(ev.TID, ev.Addr, ev.Length)

	return h.UpsertAddressData(ev.PID, record)
}

// HandleMunmap handles munmap events
func (h *AddressDBHelper) HandleMunmap(ev uimsg.MunmapEvent) error {
	record, _, err := h.GetOrCreateAddressData(ev.PID)
	if err != nil {
		return err
	}

	record.RemoveMmapAddressRange(ev.TID, ev.Addr, ev.Length)

	return h.UpsertAddressData(ev.PID, record)
}

// HandleBrk handles brk events
func (h *AddressDBHelper) HandleBrk(ev uimsg.BrkEvent) error {
	record, _, err := h.GetOrCreateAddressData(ev.PID)
	if err != nil {
		return err
	}

	record.ExtendBrkAddressRange(ev.TID, ev.Brk, ev.Ret)

	return h.UpsertAddressData(ev.PID, record)
}

// HandlePageFault handles page fault events and stores offsets
func (h *AddressDBHelper) HandlePageFault(ev uimsg.PageFaultEvent) error {
	record, _, err := h.GetOrCreateAddressData(ev.PID)
	if err != nil {
		return err
	}

	record.AddPageFaultOffset(ev.TID, ev.FaultAddr)

	return h.UpsertAddressData(ev.PID, record)
}

// HandleSwap handles swap events and stores offsets
func (h *AddressDBHelper) HandleSwap(ev uimsg.SwapEvent) error {
	record, _, err := h.GetOrCreateAddressData(ev.PID)
	if err != nil {
		return err
	}

	record.AddSwapOffset(ev.TID, ev.SwapAddr, ev.IsSwap)

	return h.UpsertAddressData(ev.PID, record)
}
