package ebpf

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/cilium/ebpf/ringbuf"

	"deep_in_memory/internal/constants"
)

// EventProcessor defines how to process a single eBPF event
type EventProcessor[T any] func(event T) tea.Msg

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
			// Binary deserialization failed - skip this event but continue processing others
			// This prevents a single corrupt event from stopping the entire collector
			continue
		}

		msg := processor(event)

		// Add to central collector if it's a timestamped event
		if tsEvent, ok := any(msg).(interface{ GetTimestamp() time.Time }); ok {
			collector.AddEvent(tsEvent)
		}
	}
}
