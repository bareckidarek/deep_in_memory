//go:build ebpf

package mmap

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"

	ebpfshared "deep_in_memory/internal/ebpf"
	uimsg "deep_in_memory/internal/ui/msg"
)

// Start attaches mmap/munmap tracepoints and streams events to the central event collector.
// Caller must cancel the context to stop the reader; the returned cleanup closes resources.
// Requires the SharedState from the sched module to filter events.
func Start(ctx context.Context, collector *ebpfshared.CentralEventCollector, sharedState *ebpfshared.SharedState) (func() error, error) {
	var objs MmapProgObjects
	spec, err := LoadMmapProg()
	if err != nil {
		return nil, fmt.Errorf("load BPF spec: %w", err)
	}

	// Reuse the target_pids map from sched
	spec.Maps["target_pids"].Pinning = 0

	opts := &ebpf.CollectionOptions{
		Maps: ebpf.MapOptions{
			PinPath: "",
		},
		MapReplacements: map[string]*ebpf.Map{
			"target_pids": sharedState.GetEbpfMap(),
		},
	}

	if err := spec.LoadAndAssign(&objs, opts); err != nil {
		return nil, fmt.Errorf("load and assign BPF objects: %w", err)
	}

	mmapEnterLink, err := link.Tracepoint("syscalls", "sys_enter_mmap", objs.HandleMmapEnter, nil)
	if err != nil {
		objs.Close()
		return nil, fmt.Errorf("attach mmap_enter: %w", err)
	}

	mmapExitLink, err := link.Tracepoint("syscalls", "sys_exit_mmap", objs.HandleMmapExit, nil)
	if err != nil {
		mmapEnterLink.Close()
		objs.Close()
		return nil, fmt.Errorf("attach mmap_exit: %w", err)
	}

	munmapEnterLink, err := link.Tracepoint("syscalls", "sys_enter_munmap", objs.HandleMunmapEnter, nil)
	if err != nil {
		mmapExitLink.Close()
		mmapEnterLink.Close()
		objs.Close()
		return nil, fmt.Errorf("attach munmap_enter: %w", err)
	}

	munmapExitLink, err := link.Tracepoint("syscalls", "sys_exit_munmap", objs.HandleMunmapExit, nil)
	if err != nil {
		munmapEnterLink.Close()
		mmapExitLink.Close()
		mmapEnterLink.Close()
		objs.Close()
		return nil, fmt.Errorf("attach munmap_exit: %w", err)
	}

	reader, err := ringbuf.NewReader(objs.Events)
	if err != nil {
		munmapExitLink.Close()
		munmapEnterLink.Close()
		mmapExitLink.Close()
		mmapEnterLink.Close()
		objs.Close()
		return nil, fmt.Errorf("create ringbuf reader: %w", err)
	}

	// Use batched event processing for medium-frequency mmap/munmap events
	processor := func(e MmapProgMmapEvent) tea.Msg {
		ts := time.Unix(0, int64(e.Ts))
		switch e.Type {
		case 1:
			return uimsg.ProcessMmap{
				Timestamp: ts,
				PID:       int(e.Pid),
				TID:       int(e.Tid),
				Addr:      e.Addr,
				Len:       e.Len,
				Comm:      cString(e.Comm[:]),
			}
		case 2:
			return uimsg.ProcessMunmap{
				Timestamp: ts,
				PID:       int(e.Pid),
				TID:       int(e.Tid),
				Addr:      e.Addr,
				Len:       e.Len,
				Comm:      cString(e.Comm[:]),
			}
		default:
			return nil // Unknown event type
		}
	}

	go ebpfshared.ProcessEventsBatched(
		ctx,
		reader,
		processor,
		collector,
	)

	cleanup := func() error {
		reader.Close()
		munmapExitLink.Close()
		munmapEnterLink.Close()
		mmapExitLink.Close()
		mmapEnterLink.Close()
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
