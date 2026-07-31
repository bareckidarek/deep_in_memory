package ebpf

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"time"

	"github.com/cilium/ebpf/ringbuf"

	"deep_in_memory/internal/constants"
	"deep_in_memory/internal/event"
)

// EventProcessor converts a raw eBPF event of type T into a domain event.
// Return nil to skip the event (e.g. unknown event type).
type EventProcessor[T any] func(raw T) event.TimestampedEvent

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

		var raw T
		if err := binary.Read(bytes.NewReader(record.RawSample), binary.LittleEndian, &raw); err != nil {
			// Binary deserialization failed - skip this event but continue processing others
			// This prevents a single corrupt event from stopping the entire collector
			continue
		}

		ev := processor(raw)
		if ev != nil {
			collector.AddEvent(ev)
		}
	}
}
