package msg

import "time"

// TimestampedEvent interface for events that have a timestamp.
// Used by the event collector to sort events chronologically.
type TimestampedEvent interface {
	GetTimestamp() time.Time
}

// ProcessExec is emitted when a process calls exec.
type ProcessExec struct {
	Timestamp time.Time
	PID       int
	Comm      string
}

// ProcessExit is emitted when a process exits.
type ProcessExit struct {
	Timestamp time.Time
	PID       int
	Comm      string
}

// ProcessClone is emitted when a process calls clone.
type ProcessClone struct {
	Timestamp  time.Time
	PID        int
	TID        int
	CloneFlags uint64
	TLS        uint64
	Comm       string
}

// ProcessMmap is emitted when mmap syscall succeeds.
type ProcessMmap struct {
	Timestamp time.Time
	PID       int
	TID       int
	Addr      uint64
	Len       uint64
	Comm      string
}

// ProcessMunmap is emitted when munmap syscall succeeds.
type ProcessMunmap struct {
	Timestamp time.Time
	PID       int
	TID       int
	Addr      uint64
	Len       uint64
	Comm      string
}

// ProcessBrk is emitted when brk syscall is called.
type ProcessBrk struct {
	Timestamp time.Time
	PID       int
	TID       int
	Brk       uint64 // requested brk address (0 for query)
	Ret       uint64 // returned brk address
	Comm      string
}

// ProcessInitSwapUnswap represents paging-related events (do_anonymous_page, do_swap_page, and swap_writepage).
type ProcessInitSwapUnswap struct {
	Timestamp time.Time
	PID       int
	TID       int
	Address   uint64
	Type      int // 0=INIT (do_anonymous_page), 1=SWAP (swap_writepage), 2=UNSWAP (do_swap_page)
	Comm      string
	ErrorCode uint64
}

// GetTimestamp implementations for TimestampedEvent interface
func (e ProcessExec) GetTimestamp() time.Time           { return e.Timestamp }
func (e ProcessExit) GetTimestamp() time.Time           { return e.Timestamp }
func (e ProcessClone) GetTimestamp() time.Time          { return e.Timestamp }
func (e ProcessMmap) GetTimestamp() time.Time           { return e.Timestamp }
func (e ProcessMunmap) GetTimestamp() time.Time         { return e.Timestamp }
func (e ProcessBrk) GetTimestamp() time.Time            { return e.Timestamp }
func (e ProcessInitSwapUnswap) GetTimestamp() time.Time { return e.Timestamp }

// LoadSmapsRequest is sent when user selects a process to load its memory maps from /proc/PID/smaps.
type LoadSmapsRequest struct {
	PID int
}

// ApplicationError is sent when a non-critical application error occurs.
// The UI should display this to the user but continue operating.
type ApplicationError struct {
	Message  string
	Severity string // "warning", "error", "info"
}

// HideModalMsg is sent after the modal timeout to hide the modal.
type HideModalMsg struct{}

// GetLogsMsg requests current logs from app layer.
type GetLogsMsg struct{}

type LogsResponseMsg struct {
	Logs []string // Pre-formatted log lines with timestamps
}

// PagemapLookupResponse contains the result of a pagemap lookup.
type PagemapLookupResponse struct {
	Lines      []string // Formatted pagemap entry lines
	PFN        uint64   // Page Frame Number (for iomem matching)
	HasPFN     bool     // Whether a valid PFN was found
	VAddrInput string   // The address that was looked up
}

// IomemLoadResponse contains parsed /proc/iomem data.
type IomemLoadResponse struct {
	Ranges []IomemRangeMsg
}

// IomemRangeMsg is a UI-safe copy of IomemRange.
type IomemRangeMsg struct {
	Start uint64
	End   uint64
	Name  string
	Level int
}

// ClearPhysicalMemoryMsg is sent when a process exits to clear physical memory state.
type ClearPhysicalMemoryMsg struct {
	PID int
}
