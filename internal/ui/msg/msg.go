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

// ProcessPageFault represents a page_fault_user tracepoint event.
type ProcessPageFault struct {
	Timestamp time.Time
	PID       int
	TID       int
	Address   uint64
	IP        uint64
	ErrorCode uint64
	Comm      string
}

// ProcessSwapUnswap represents swap-related events (do_swap_page and swap_writepage).
type ProcessSwapUnswap struct {
	Timestamp time.Time
	PID       int
	TID       int
	Address   uint64
	IsSwap    bool // true=SWAP (swap_writepage), false=UNSWAP (do_swap_page)
	Comm      string
	ErrorCode uint64
}

// GetTimestamp implementations for TimestampedEvent interface
func (e ProcessExec) GetTimestamp() time.Time       { return e.Timestamp }
func (e ProcessExit) GetTimestamp() time.Time       { return e.Timestamp }
func (e ProcessMmap) GetTimestamp() time.Time       { return e.Timestamp }
func (e ProcessMunmap) GetTimestamp() time.Time     { return e.Timestamp }
func (e ProcessBrk) GetTimestamp() time.Time        { return e.Timestamp }
func (e ProcessPageFault) GetTimestamp() time.Time  { return e.Timestamp }
func (e ProcessSwapUnswap) GetTimestamp() time.Time { return e.Timestamp }

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

// FlushLogsMsg is sent periodically to flush aggregated logs to the UI.
type FlushLogsMsg struct{}

// GetLogsMsg requests current logs from app layer.
type GetLogsMsg struct{}

// LogsResponseMsg contains formatted log entries for display.
type LogsResponseMsg struct {
	Logs []string // Pre-formatted log lines with timestamps
}

// EventBatch contains multiple events batched together for efficient processing.
// Used by eBPF collectors to reduce the number of messages sent to Bubble Tea.
type EventBatch struct {
	Messages []interface{} // Slice of event messages (ProcessMmap, ProcessPageFault, etc.)
}

// PagemapLookupRequest is sent when user submits a virtual address for pagemap lookup.
type PagemapLookupRequest struct {
	PID   int
	VAddr uint64
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
