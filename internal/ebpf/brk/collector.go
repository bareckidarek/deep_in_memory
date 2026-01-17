//go:build ebpf

package brk

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

	"deep_in_memory/internal/constants"
	"deep_in_memory/internal/ebpf"
	uimsg "deep_in_memory/internal/ui/msg"
)

// Start attaches brk tracepoints and streams events to the provided send function.
// Caller must cancel the context to stop the reader; the returned cleanup closes resources.
// Requires the target_pid map from the sched module to filter events.
func Start(ctx context.Context, send func(tea.Msg), sharedState *ebpf.SharedState) (func() error, error) {
	var objs BrkProgObjects
	spec, err := LoadBrkProg()
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
			"target_pid": sharedState.GetEbpfMap(),
		},
	}

	if err := spec.LoadAndAssign(&objs, opts); err != nil {
		return nil, fmt.Errorf("load and assign BPF objects: %w", err)
	}

	brkEnterLink, err := link.Tracepoint("syscalls", "sys_enter_brk", objs.HandleBrkEnter, nil)
	if err != nil {
		objs.Close()
		return nil, fmt.Errorf("attach brk_enter: %w", err)
	}

	brkExitLink, err := link.Tracepoint("syscalls", "sys_exit_brk", objs.HandleBrkExit, nil)
	if err != nil {
		brkEnterLink.Close()
		objs.Close()
		return nil, fmt.Errorf("attach brk_exit: %w", err)
	}

	reader, err := ringbuf.NewReader(objs.Events)
	if err != nil {
		brkExitLink.Close()
		brkEnterLink.Close()
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
				time.Sleep(constants.EbpfRetryDelay)
				continue
			}

			var e BrkProgBrkEvent
			if err := binary.Read(bytes.NewReader(record.RawSample), binary.LittleEndian, &e); err != nil {
				continue
			}

			ts := time.Unix(0, int64(e.Ts))
			send(uimsg.ProcessBrk{Timestamp: ts, PID: int(e.Pid), TID: int(e.Tid), Brk: e.Brk, Ret: e.Ret, Comm: cString(e.Comm[:])})
		}
	}()

	cleanup := func() error {
		reader.Close()
		brkExitLink.Close()
		brkEnterLink.Close()
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
