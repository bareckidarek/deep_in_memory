package msg

import "deep_in_memory/internal/event"

// Domain event type aliases — the canonical definitions live in internal/event,
// re-exported here so existing consumers of this package require no import changes.
type (
	TimestampedEvent      = event.TimestampedEvent
	ProcessExec           = event.ProcessExec
	ProcessExit           = event.ProcessExit
	ProcessClone          = event.ProcessClone
	ProcessMmap           = event.ProcessMmap
	ProcessMunmap         = event.ProcessMunmap
	ProcessBrk            = event.ProcessBrk
	ProcessInitSwapUnswap = event.ProcessInitSwapUnswap
)

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
