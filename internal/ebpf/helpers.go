package ebpf

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/cilium/ebpf/ringbuf"

	"deep_in_memory/internal/constants"
)

// EventProcessor defines how to process a single eBPF event
type EventProcessor[T any] func(event T) tea.Msg

// ProcessEvents implements the standard event processing loop used by all eBPF collectors.
// Reads events from the ring buffer, deserializes them, and forwards to the UI.
// Handles context cancellation and transient read errors gracefully.
func ProcessEvents[T any](
	ctx context.Context,
	reader *ringbuf.Reader,
	processor EventProcessor[T],
	send func(tea.Msg),
) {
	defer reader.Close()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		record, err := reader.Read()
		if err != nil {
			if errors.Is(err, ringbuf.ErrClosed) {
				return
			}
			// Skip transient errors
			time.Sleep(constants.EbpfRetryDelay)
			continue
		}

		var event T
		if err := binary.Read(bytes.NewReader(record.RawSample), binary.LittleEndian, &event); err != nil {
			continue
		}

		msg := processor(event)
		send(msg)
	}
}

// ProcessEventsBatched implements event processing with batching optimization.
// Collects events into batches before sending to reduce message passing overhead.
// Flushes batches when either batch size limit is reached or flush interval expires.
// This significantly improves performance for high-frequency event collectors.
//
// Uses a CentralEventCollector to maintain chronological order across different event types.
func ProcessEventsBatched[T any](
	ctx context.Context,
	reader *ringbuf.Reader,
	processor EventProcessor[T],
	collector *CentralEventCollector,
) {
	defer reader.Close()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Non-blocking read attempt
		}

		record, err := reader.Read()
		if err != nil {
			if errors.Is(err, ringbuf.ErrClosed) {
				return
			}
			// Skip transient errors
			time.Sleep(constants.EbpfRetryDelay)
			continue
		}

		var event T
		if err := binary.Read(bytes.NewReader(record.RawSample), binary.LittleEndian, &event); err != nil {
			continue
		}

		msg := processor(event)

		// Add to central collector if it's a timestamped event
		if tsEvent, ok := any(msg).(interface{ GetTimestamp() time.Time }); ok {
			collector.AddEvent(tsEvent)
		}
	}
}

// CleanupFunc creates a cleanup function that properly closes multiple resources.
// Closes resources in reverse order to ensure dependent resources are closed first.
// Returns the last encountered error if any close operation fails.
func CleanupFunc(closers ...interface{ Close() error }) func() error {
	return func() error {
		var lastErr error
		// Close in reverse order
		for i := len(closers) - 1; i >= 0; i-- {
			if err := closers[i].Close(); err != nil {
				lastErr = err
			}
		}
		return lastErr
	}
}

// FormatEbpfError creates a consistent error message for eBPF operations
func FormatEbpfError(operation string, err error) error {
	return fmt.Errorf("%s: %w", operation, err)
}
