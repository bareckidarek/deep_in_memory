//go:build ebpf

package swap

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

// Start attaches swap kprobes and streams events to the central event collector.
// Caller must cancel the context to stop the reader; the returned cleanup closes resources.
// Requires the SharedState from the sched module to filter events.
func Start(ctx context.Context, collector *ebpfshared.CentralEventCollector, sharedState *ebpfshared.SharedState) (func() error, error) {
	var objs SwapProgObjects
	spec, err := LoadSwapProg()
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

	doSwapPageKprobe, err := link.Kprobe("do_swap_page", objs.KprobeDoSwapPage, nil)
	if err != nil {
		objs.Close()
		return nil, fmt.Errorf("attach kprobe do_swap_page: %w", err)
	}

	doSwapPageKretprobe, err := link.Kretprobe("do_swap_page", objs.KretprobeDoSwapPage, nil)
	if err != nil {
		doSwapPageKprobe.Close()
		objs.Close()
		return nil, fmt.Errorf("attach kretprobe do_swap_page: %w", err)
	}

	swapWritepageKprobe, err := link.Kprobe("swap_writepage", objs.KprobeSwapWritepage, nil)
	if err != nil {
		doSwapPageKretprobe.Close()
		doSwapPageKprobe.Close()
		objs.Close()
		return nil, fmt.Errorf("attach kprobe swap_writepage: %w", err)
	}

	swapWritepageKretprobe, err := link.Kretprobe("swap_writepage", objs.KretprobeSwapWritepage, nil)
	if err != nil {
		swapWritepageKprobe.Close()
		doSwapPageKretprobe.Close()
		doSwapPageKprobe.Close()
		objs.Close()
		return nil, fmt.Errorf("attach kretprobe swap_writepage: %w", err)
	}

	reader, err := ringbuf.NewReader(objs.Events)
	if err != nil {
		swapWritepageKretprobe.Close()
		swapWritepageKprobe.Close()
		doSwapPageKretprobe.Close()
		doSwapPageKprobe.Close()
		objs.Close()
		return nil, fmt.Errorf("create ringbuf reader: %w", err)
	}

	// Use batched event processing for high-frequency swap events
	processor := func(e SwapProgSwapEvent) tea.Msg {
		ts := time.Unix(0, int64(e.Ts))

		switch e.Type {
		case 1: // do_swap_page
			return uimsg.ProcessSwapUnswap{
				Timestamp: ts,
				PID:       int(e.Pid),
				TID:       int(e.Tid),
				Address:   e.Addr,
				IsSwap:    false, // UNSWAP
				ErrorCode: e.Ret,
			}
		case 2: // swap_writepage
			// Need to add three zeros at the end (shift left by 12 bits = multiply by 4096)
			addr := e.Addr << 12
			return uimsg.ProcessSwapUnswap{
				Timestamp: ts,
				PID:       int(e.Pid),
				TID:       int(e.Tid),
				Address:   addr,
				IsSwap:    true, // SWAP
				ErrorCode: e.Ret,
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
		swapWritepageKretprobe.Close()
		swapWritepageKprobe.Close()
		doSwapPageKretprobe.Close()
		doSwapPageKprobe.Close()
		return objs.Close()
	}

	return cleanup, nil
}
