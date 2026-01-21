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

// FlushLogsMsg is sent periodically to flush aggregated logs to the UI.
type FlushLogsMsg struct{}

// GetLogsMsg requests current logs from app layer.
type GetLogsMsg struct{}

// LogsResponseMsg contains formatted log entries for display.
type LogsResponseMsg struct {
	Logs []string // Pre-formatted log lines with timestamps
}

// ===== Address DB Event Types (simplified versions for address offset tracking) =====

// MmapEvent is simplified mmap event for address tracking
type MmapEvent struct {
	PID    int
	TID    int
	Addr   uint64
	Length uint64
}

// MunmapEvent is simplified munmap event for address tracking
type MunmapEvent struct {
	PID    int
	TID    int
	Addr   uint64
	Length uint64
}

// BrkEvent is simplified brk event for address tracking
type BrkEvent struct {
	PID int
	TID int
	Brk uint64
	Ret uint64
}

// PageFaultEvent is simplified page fault event for address tracking
type PageFaultEvent struct {
	PID       int
	TID       int
	FaultAddr uint64
}

// SwapEvent is simplified swap event for address tracking
type SwapEvent struct {
	PID      int
	TID      int
	SwapAddr uint64
	IsSwap   bool
}

// ToMmapEvent converts ProcessMmap to MmapEvent
func (p ProcessMmap) ToMmapEvent() MmapEvent {
	return MmapEvent{
		PID:    p.PID,
		TID:    p.TID,
		Addr:   p.Addr,
		Length: p.Len,
	}
}

// ToMunmapEvent converts ProcessMunmap to MunmapEvent
func (p ProcessMunmap) ToMunmapEvent() MunmapEvent {
	return MunmapEvent{
		PID:    p.PID,
		TID:    p.TID,
		Addr:   p.Addr,
		Length: p.Len,
	}
}

// ToBrkEvent converts ProcessBrk to BrkEvent
func (p ProcessBrk) ToBrkEvent() BrkEvent {
	return BrkEvent{
		PID: p.PID,
		TID: p.TID,
		Brk: p.Brk,
		Ret: p.Ret,
	}
}

// ToPageFaultEvent converts ProcessPageFault to PageFaultEvent
func (p ProcessPageFault) ToPageFaultEvent() PageFaultEvent {
	return PageFaultEvent{
		PID:       p.PID,
		TID:       p.TID,
		FaultAddr: p.Address,
	}
}

// ToSwapEvent converts ProcessSwapUnswap to SwapEvent
func (p ProcessSwapUnswap) ToSwapEvent() SwapEvent {
	return SwapEvent{
		PID:      p.PID,
		TID:      p.TID,
		SwapAddr: p.Address,
		IsSwap:   p.IsSwap,
	}
}
