//go:build ebpf

package sched

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"

	uimsg "deep_in_memory/internal/ui/msg"
)

// Start attaches sched tracepoints and streams events to the provided send function.
// Caller must cancel the context to stop the reader; the returned cleanup closes resources.
// A setter is returned to update the single-element target PID map from the UI.
// The target_pid map is also returned so it can be shared with other eBPF modules.
func Start(ctx context.Context, send func(tea.Msg)) (func() error, func(int) error, *ebpf.Map, error) {
	var objs SchedProgObjects
	if err := LoadSchedProgObjects(&objs, nil); err != nil {
		return nil, nil, nil, fmt.Errorf("load BPF objects: %w", err)
	}

	forkLink, err := link.Tracepoint("sched", "sched_process_fork", objs.HandleFork, nil)
	if err != nil {
		objs.Close()
		return nil, nil, nil, fmt.Errorf("attach fork: %w", err)
	}

	exitLink, err := link.Tracepoint("sched", "sched_process_exit", objs.HandleExit, nil)
	if err != nil {
		forkLink.Close()
		objs.Close()
		return nil, nil, nil, fmt.Errorf("attach exit: %w", err)
	}

	reader, err := ringbuf.NewReader(objs.Events)
	if err != nil {
		exitLink.Close()
		forkLink.Close()
		objs.Close()
		return nil, nil, nil, fmt.Errorf("create ringbuf reader: %w", err)
	}

	setTargetPID := func(pid int) error {
		if objs.TargetPid == nil {
			return fmt.Errorf("target_pid map not loaded")
		}
		key := uint32(0)
		val := uint32(pid)
		return objs.TargetPid.Update(&key, &val, ebpf.UpdateAny)
	}

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
				time.Sleep(10 * time.Millisecond)
				continue
			}

			var e SchedProgEvent
			if err := binary.Read(bytes.NewReader(record.RawSample), binary.LittleEndian, &e); err != nil {
				continue
			}

			ts := time.Unix(0, int64(e.Ts))
			switch e.Type {
			case 1:
				send(uimsg.ProcessFork{Timestamp: ts, ParentPID: int(e.Pid), ChildPID: int(e.ChildPid), Comm: cString(e.Comm[:])})
			case 2:
				send(uimsg.ProcessExit{Timestamp: ts, PID: int(e.Pid), Comm: cString(e.Comm[:])})
			}
		}
	}()

	cleanup := func() error {
		reader.Close()
		exitLink.Close()
		forkLink.Close()
		return objs.Close()
	}

	return cleanup, setTargetPID, objs.TargetPid, nil
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
