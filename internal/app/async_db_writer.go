package app

import (
	"context"
	"fmt"
	"sync"
)

// dbOp represents a single database operation to be executed serially
// by the background writer goroutine.
type dbOp struct {
	pid     int
	handler func(*DBHelper) error
	isExit  bool // Exit ops bypass the exitedPIDs check and clean up after themselves
}

// AsyncDBWriter serializes ALL database writes through a single background goroutine.
//
// This eliminates three classes of bugs:
//   - BoltDB write contention (single-writer model means concurrent Upserts block each other → UI freeze)
//   - TOCTOU race conditions (getProcess + Upsert is not atomic; Exit can slip between them)
//   - Zombie processes (async writes after ProcessExit re-create deleted processes)
//
// Key design:
//   - FIFO channel preserves event ordering (mmap before page_fault for same address)
//   - exitedPIDs prevents enqueuing events for dead processes
//   - EnqueueExit marks PID as dead BEFORE queueing the delete, so no future events sneak in
//   - Update() goroutine NEVER writes to BoltDB directly
type AsyncDBWriter struct {
	dbHelper   *DBHelper
	opChan     chan dbOp
	exitedPIDs sync.Map // int → struct{}: PIDs that have exited (prevents late event writes)

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu          sync.RWMutex // protects errCallback
	errCallback func(error)
}

// NewAsyncDBWriter creates a writer with a buffered channel.
// The errCallback is called from the background goroutine for non-fatal errors.
func NewAsyncDBWriter(dbHelper *DBHelper, errCallback func(error)) *AsyncDBWriter {
	ctx, cancel := context.WithCancel(context.Background())
	w := &AsyncDBWriter{
		dbHelper:    dbHelper,
		opChan:      make(chan dbOp, 5000),
		ctx:         ctx,
		cancel:      cancel,
		errCallback: errCallback,
	}
	w.wg.Add(1)
	go w.processLoop()
	return w
}

// SetErrorCallback updates the error callback (safe for concurrent use).
// Called during setup before Bubble Tea starts, but protected for safety.
func (w *AsyncDBWriter) SetErrorCallback(cb func(error)) {
	w.mu.Lock()
	w.errCallback = cb
	w.mu.Unlock()
}

// Enqueue adds a database write operation to the queue.
// Non-blocking: if the channel is full, the event is dropped (preferable to freezing UI).
// Returns false if the event was dropped (PID exited or queue full).
func (w *AsyncDBWriter) Enqueue(pid int, handler func(*DBHelper) error) bool {
	// Fast path: skip operations for processes that have already exited.
	// This catches events that arrive after ProcessExit but before the background
	// goroutine has processed the delete.
	if _, exited := w.exitedPIDs.Load(pid); exited {
		return false
	}

	select {
	case w.opChan <- dbOp{pid: pid, handler: handler}:
		return true
	default:
		// Channel full — drop event to avoid blocking the UI goroutine.
		w.reportError(fmt.Errorf("async DB queue full (pid=%d), dropping event", pid))
		return false
	}
}

// EnqueueExit handles process exit with special ordering guarantees:
//  1. Marks PID as exited IMMEDIATELY (prevents future Enqueue calls for this PID)
//  2. Queues the actual DB deletion (will run after all earlier ops for this PID)
//
// When the background goroutine processes the delete:
//   - All earlier operations for this PID have already been processed (FIFO)
//   - The delete removes the process from DB
//   - exitedPIDs entry is cleaned up (allows PID reuse)
func (w *AsyncDBWriter) EnqueueExit(pid int, handler func(*DBHelper) error) {
	// Mark as exited FIRST — this is the critical ordering guarantee.
	// Any Enqueue() call after this point for the same PID will be rejected.
	w.exitedPIDs.Store(pid, struct{}{})

	select {
	case w.opChan <- dbOp{pid: pid, handler: handler, isExit: true}:
		// Queued successfully
	default:
		// Channel full — execute exit synchronously as last resort.
		// This is critical: we MUST delete the process to prevent zombies.
		if err := handler(w.dbHelper); err != nil {
			w.reportError(fmt.Errorf("sync exit fallback for PID %d failed: %w", pid, err))
		}
		w.exitedPIDs.Delete(pid)
	}
}

// processLoop is the single background goroutine that executes all DB writes.
// Since it's the ONLY goroutine writing to BoltDB, there is zero write contention.
func (w *AsyncDBWriter) processLoop() {
	defer w.wg.Done()

	for {
		select {
		case <-w.ctx.Done():
			w.drain()
			return
		case op := <-w.opChan:
			w.executeOp(op)
		}
	}
}

// executeOp runs a single operation, checking exitedPIDs for non-exit ops.
func (w *AsyncDBWriter) executeOp(op dbOp) {
	// For non-exit ops: check if the process has been marked as exited
	// since this op was enqueued. This catches events that were already
	// in the channel when ProcessExit arrived.
	if !op.isExit {
		if _, exited := w.exitedPIDs.Load(op.pid); exited {
			return // Process is dead — skip silently
		}
	}

	if err := op.handler(w.dbHelper); err != nil {
		w.reportError(err)
	}

	// After processing an exit op, clean up the exitedPIDs entry.
	// At this point all earlier ops for this PID have been processed (FIFO),
	// and the process is deleted from DB. If a new process with the same PID
	// appears, it will be able to enqueue normally.
	if op.isExit {
		w.exitedPIDs.Delete(op.pid)
	}
}

// drain processes all remaining operations in the channel before shutdown.
func (w *AsyncDBWriter) drain() {
	for {
		select {
		case op := <-w.opChan:
			w.executeOp(op)
		default:
			return
		}
	}
}

// Stop gracefully shuts down the writer: cancels the context, waits for
// all pending operations to be processed, then returns.
// Must be called before closing the BoltDB databases.
func (w *AsyncDBWriter) Stop() {
	w.cancel()
	w.wg.Wait()
}

func (w *AsyncDBWriter) reportError(err error) {
	w.mu.RLock()
	cb := w.errCallback
	w.mu.RUnlock()
	if cb != nil {
		cb(err)
	}
}
