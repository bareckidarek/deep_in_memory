package ebpf

import (
	"context"
	"sort"
	"sync"
	"time"

	"deep_in_memory/internal/event"
)

// CentralEventCollector collects events from all eBPF collectors and batches them
// while maintaining chronological order across different event types.
// This is critical because events from different collectors (paging, mmap, brk)
// can be interleaved in time and must be processed in timestamp order.
type CentralEventCollector struct {
	mu            sync.Mutex
	events        []event.TimestampedEvent
	send          func([]event.TimestampedEvent)
	batchSize     int
	flushInterval time.Duration
	ctx           context.Context
	cancel        context.CancelFunc
}

// NewCentralEventCollector creates a new centralized event collector.
// batchSize: maximum number of events to collect before flushing.
// flushInterval: maximum time to wait before flushing collected events.
// send: callback invoked with each sorted batch; must be non-blocking.
func NewCentralEventCollector(send func([]event.TimestampedEvent), batchSize int, flushInterval time.Duration) *CentralEventCollector {
	ctx, cancel := context.WithCancel(context.Background())

	collector := &CentralEventCollector{
		events:        make([]event.TimestampedEvent, 0, batchSize),
		send:          send,
		batchSize:     batchSize,
		flushInterval: flushInterval,
		ctx:           ctx,
		cancel:        cancel,
	}

	// Start the periodic flush goroutine
	go collector.flushLoop()

	return collector
}

// AddEvent adds a timestamped event to the collector.
// Thread-safe and can be called from multiple collector goroutines.
func (c *CentralEventCollector) AddEvent(ev event.TimestampedEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.events = append(c.events, ev)

	// Flush if batch is full
	if len(c.events) >= c.batchSize {
		c.flushLocked()
	}
}

// flushLoop periodically flushes collected events.
func (c *CentralEventCollector) flushLoop() {
	ticker := time.NewTicker(c.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			c.Flush() // Final flush on shutdown
			return
		case <-ticker.C:
			c.Flush()
		}
	}
}

// Flush sends all collected events in chronological order.
// Thread-safe and can be called externally.
func (c *CentralEventCollector) Flush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.flushLocked()
}

// flushLocked performs the actual flush. Caller must hold the mutex.
func (c *CentralEventCollector) flushLocked() {
	if len(c.events) == 0 {
		return
	}

	// Sort events by timestamp to maintain chronological order
	sort.Slice(c.events, func(i, j int) bool {
		return c.events[i].GetTimestamp().Before(c.events[j].GetTimestamp())
	})

	// Copy the sorted batch before resetting the buffer
	batch := make([]event.TimestampedEvent, len(c.events))
	copy(batch, c.events)

	// Send the sorted batch
	c.send(batch)

	// Clear events while keeping capacity
	c.events = c.events[:0]
}

// Stop stops the collector and flushes remaining events.
func (c *CentralEventCollector) Stop() {
	c.cancel()
}
