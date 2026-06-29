//go:build ebpf

package sched

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"

	"deep_in_memory/internal/constants"
	ebpfshared "deep_in_memory/internal/ebpf"
	uimsg "deep_in_memory/internal/ui/msg"
)

// Start attaches sched tracepoints and streams events to the provided send function.
// Caller must cancel the context to stop the reader; the returned cleanup closes resources.
// Returns a SharedState for thread-safe access to target PID and cleanup function.
func Start(ctx context.Context, send func(tea.Msg)) (func() error, interface{}, error) {
	var objs SchedProgObjects
	if err := LoadSchedProgObjects(&objs, nil); err != nil {
		return nil, nil, fmt.Errorf("load BPF objects: %w", err)
	}

	execLink, err := link.Tracepoint("sched", "sched_process_exec", objs.HandleExec, nil)
	if err != nil {
		objs.Close()
		return nil, nil, fmt.Errorf("attach exec: %w", err)
	}

	exitLink, err := link.Tracepoint("sched", "sched_process_exit", objs.HandleExit, nil)
	if err != nil {
		execLink.Close()
		objs.Close()
		return nil, nil, fmt.Errorf("attach exit: %w", err)
	}

	reader, err := ringbuf.NewReader(objs.Events)
	if err != nil {
		exitLink.Close()
		execLink.Close()
		objs.Close()
		return nil, nil, fmt.Errorf("create ringbuf reader: %w", err)
	}

	// Create shared state for thread-safe access
	sharedState := ebpfshared.NewSharedState(objs.TargetPids)

	go func() {
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
				// skip transient errors
				time.Sleep(constants.EbpfRetryDelay)
				continue
			}

			var e SchedProgEvent
			if err := binary.Read(bytes.NewReader(record.RawSample), binary.LittleEndian, &e); err != nil {
				continue
			}

			ts := time.Unix(0, int64(e.Ts))
			switch e.Type {
			case 1:
				send(uimsg.ProcessExec{Timestamp: ts, PID: int(e.Pid), Comm: cString(e.Comm[:])})
			case 2:
				send(uimsg.ProcessExit{Timestamp: ts, PID: int(e.Pid), Comm: cString(e.Comm[:])})
			}
		}
	}()

	cleanup := func() error {
		reader.Close()
		exitLink.Close()
		execLink.Close()
		return objs.Close()
	}

	return cleanup, sharedState, nil
}

func cString(b []int8) string {
	for i, v := range b {
		if v == 0 {
			return string(byteSlice(b[:i]))
		}
	}
	return string(byteSlice(b))
}

func byteSlice(b []int8) []byte {
	out := make([]byte, len(b))
	for i, v := range b {
		out[i] = byte(v)
	}
	return out
}
