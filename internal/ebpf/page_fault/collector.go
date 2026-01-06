//go:build ebpf

package page_fault

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

// Start attaches page_fault_user tracepoint and streams events to the provided send function.
// Caller must cancel the context to stop the reader; the returned cleanup closes resources.
// Requires the target_pid map from the sched module to filter events.
func Start(ctx context.Context, send func(tea.Msg), targetPidMap *ebpf.Map) (func() error, error) {
	var objs PageFaultProgObjects
	spec, err := LoadPageFaultProg()
	if err != nil {
		return nil, fmt.Errorf("load BPF spec: %w", err)
	}

	// Reuse the target_pid map from sched
	spec.Maps["target_pid"].Pinning = 0

	opts := &ebpf.CollectionOptions{
		Maps: ebpf.MapOptions{
			PinPath: "",
		},
		MapReplacements: map[string]*ebpf.Map{
			"target_pid": targetPidMap,
		},
	}

	if err := spec.LoadAndAssign(&objs, opts); err != nil {
		return nil, fmt.Errorf("load and assign BPF objects: %w", err)
	}

	pfLink, err := link.Tracepoint("exceptions", "page_fault_user", objs.HandlePageFaultUser, nil)
	if err != nil {
		objs.Close()
		return nil, fmt.Errorf("attach page_fault_user: %w", err)
	}

	reader, err := ringbuf.NewReader(objs.PageFaultEvents)
	if err != nil {
		pfLink.Close()
		objs.Close()
		return nil, fmt.Errorf("create ringbuf reader: %w", err)
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

			var e PageFaultProgPageFaultEvent
			if err := binary.Read(bytes.NewReader(record.RawSample), binary.LittleEndian, &e); err != nil {
				continue
			}

			ts := time.Unix(0, int64(e.Ts))
			msg := uimsg.ProcessPageFault{
				Timestamp: ts,
				PID:       int(e.Pid),
				TID:       int(e.Tid),
				Address:   e.Address,
				IP:        e.Ip,
				ErrorCode: e.ErrorCode,
				Comm:      cString(e.Comm[:]),
			}
			send(msg)
		}
	}()

	cleanup := func() error {
		reader.Close()
		pfLink.Close()
		return objs.Close()
	}

	return cleanup, nil
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
