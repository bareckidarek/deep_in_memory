package msg

import "time"

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
