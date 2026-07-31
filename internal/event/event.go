// Package event defines the domain event types emitted by eBPF collectors.
// These types are independent of any UI framework and represent raw kernel
// observations: process lifecycle, memory operations, and page faults.
package event

import "time"

// TimestampedEvent is implemented by all domain events that carry a kernel timestamp.
// Used by CentralEventCollector to sort events chronologically across collectors.
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

// ProcessInitSwapUnswap represents paging-related events:
// do_anonymous_page, do_swap_page, and swap_writepage.
type ProcessInitSwapUnswap struct {
	Timestamp time.Time
	PID       int
	TID       int
	Address   uint64
	Type      int // 0=INIT (do_anonymous_page), 1=SWAP (swap_writepage), 2=UNSWAP (do_swap_page)
	Comm      string
	ErrorCode uint64
}

// GetTimestamp implementations for TimestampedEvent interface.
func (e ProcessExec) GetTimestamp() time.Time           { return e.Timestamp }
func (e ProcessExit) GetTimestamp() time.Time           { return e.Timestamp }
func (e ProcessClone) GetTimestamp() time.Time          { return e.Timestamp }
func (e ProcessMmap) GetTimestamp() time.Time           { return e.Timestamp }
func (e ProcessMunmap) GetTimestamp() time.Time         { return e.Timestamp }
func (e ProcessBrk) GetTimestamp() time.Time            { return e.Timestamp }
func (e ProcessInitSwapUnswap) GetTimestamp() time.Time { return e.Timestamp }
