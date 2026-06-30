package data

import (
	"syscall"
	"time"
)

var PageSizeBytes uint64

func init() {
	PageSizeBytes = uint64(syscall.Getpagesize())
}

// LabelType represents the type of a memory range label.
type LabelType int

const (
    FileBacked LabelType = iota
    Heap
    Anonymous
	Stack
	Empty
	Other
)

// ProcessData aggregates all data for a single process.
type ProcessData struct {
	Proc       Process
	Threads    []ThreadData
	rangeCache map[int]*RangeCache // TID -> RangeCache for fast lookups
}

// Process mirrors a process definition.
type Process struct {
	PID       int
	Name      string
	StartTime time.Time
}

// ThreadData stores thread-level virtual ranges.
type ThreadData struct {
	TID    int
	Ranges []AddrRange
}

// AddrRange represents a virtual address range and statuses.
type AddrRange struct {
	Start     string
	End       string
	StartVal  uint64
	EndVal    uint64
	Size      uint64
	PrevSize  uint64 // Size before the most recent resize (0 == no previous size recorded)
	Statuses  map[string]*StatusCount
	Label     string    // Optional label like [heap], [stack], [anon: Go: gc bits], etc.
	LabelType LabelType
	ChangedAt time.Time // Timestamp of the most recent change to this range (zero = never changed)
}

// StatusCount tracks count of pages in a particular status.
// Page offsets are stored separately in AddressData in the address database.
type StatusCount struct {
	Count int `json:"count"`
}
