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
