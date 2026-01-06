package msg

import "time"

// ProcessFork is emitted when a child process is created.
type ProcessFork struct {
	Timestamp time.Time
	ParentPID int
	ChildPID  int
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
