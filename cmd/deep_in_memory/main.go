package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"deep_in_memory/internal/app"

	tea "charm.land/bubbletea/v2"

	"deep_in_memory/internal/constants"
	"deep_in_memory/internal/data"
	ebpfshared "deep_in_memory/internal/ebpf"
	"deep_in_memory/internal/ebpf/brk"
	"deep_in_memory/internal/ebpf/mmap"
	"deep_in_memory/internal/ebpf/page_fault"
	"deep_in_memory/internal/ebpf/sched"
	"deep_in_memory/internal/ebpf/swap"
	storedb "deep_in_memory/internal/thirdparty/db"
	uimsg "deep_in_memory/internal/ui/msg"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	clearDatabases()

	store, err := storedb.New[data.ProcessData]("deep_in_memory")
	if err != nil {
		log.Fatalf("failed to open bolt db: %v", err)
	}
	defer store.Close()

	addressStore, err := storedb.New[data.AddressData]("deep_in_memory_addresses")
	if err != nil {
		log.Fatalf("failed to open address bolt db: %v", err)
	}
	defer addressStore.Close()

	// Database is always empty after clearDatabases() - start with empty dataset
	initial := data.BuildDataset([]data.ProcessData{})
	appModel := app.New(store, addressStore, initial, constants.DefaultRefreshInterval)
	p := tea.NewProgram(appModel)

	// Create central event collector for maintaining chronological order of events
	// This collector receives events from all eBPF collectors and sorts them by timestamp
	// before sending to the application, ensuring proper event ordering across different
	// event types (page_fault, swap, mmap, brk)
	eventCollector := ebpfshared.NewCentralEventCollector(
		p.Send,
		constants.EventBatchSize,
		constants.EventBatchFlushInterval,
	)
	defer eventCollector.Stop()

	// Start sched collector and get the shared state
	var sharedState *ebpfshared.SharedState
	schedCleanup, stateInterface, err := sched.Start(ctx, p.Send)
	if err != nil {
		// Send error to UI and continue without eBPF functionality
		p.Send(uimsg.ApplicationError{
			Message:  fmt.Sprintf("Failed to start sched eBPF collector: %v. Running without eBPF tracing.", err),
			Severity: "warning",
		})
		// Continue without sched collector - application will still work for process listing
	} else {
		defer func() {
			// Silently cleanup - TUI may already be closed during shutdown
			schedCleanup()
		}()
		// Convert interface{} to SharedState for eBPF mode
		if s, ok := stateInterface.(*ebpfshared.SharedState); ok {
			sharedState = s
		}
	}

	// Start secondary eBPF collectors with shared state
	cleanups := startCollectors(ctx, p.Send, eventCollector, sharedState)
	defer func() {
		// Silently cleanup all collectors - TUI may already be closed during shutdown
		for _, cleanup := range cleanups {
			cleanup()
		}
	}()

	// Set the target PID setter in the app model if eBPF is enabled
	if sharedState != nil {
		appModel.SetTargetPIDSetter(sharedState.SetTargetPID)
	}

	// Set error callback for database operations
	appModel.SetErrorCallback(func(err error) {
		p.Send(uimsg.ApplicationError{
			Message:  fmt.Sprintf("Database error: %v", err),
			Severity: "error",
		})
	})

	if _, err := p.Run(); err != nil {
		log.Fatalf("tui error: %v", err)
	}

	// Stop async DB writer before closing databases.
	// This drains any pending write operations.
	appModel.StopAsyncWriter()
}

// collectorDef describes a secondary eBPF collector to start.
type collectorDef struct {
	name  string
	start func(ctx context.Context, collector *ebpfshared.CentralEventCollector, state interface{}) (func() error, error)
}

// startCollectors launches all secondary eBPF collectors, logging warnings on failure.
// Returns a slice of cleanup functions for successfully started collectors.
func startCollectors(ctx context.Context, send func(tea.Msg), collector *ebpfshared.CentralEventCollector, state *ebpfshared.SharedState) []func() error {
	if state == nil {
		return nil
	}

	collectors := []collectorDef{
		{"mmap", func(ctx context.Context, col *ebpfshared.CentralEventCollector, s interface{}) (func() error, error) {
			return mmap.Start(ctx, col, s.(*ebpfshared.SharedState))
		}},
		{"brk", func(ctx context.Context, col *ebpfshared.CentralEventCollector, s interface{}) (func() error, error) {
			return brk.Start(ctx, col, s.(*ebpfshared.SharedState))
		}},
		{"page_fault", func(ctx context.Context, col *ebpfshared.CentralEventCollector, s interface{}) (func() error, error) {
			return page_fault.Start(ctx, col, s.(*ebpfshared.SharedState))
		}},
		{"swap", func(ctx context.Context, col *ebpfshared.CentralEventCollector, s interface{}) (func() error, error) {
			return swap.Start(ctx, col, s.(*ebpfshared.SharedState))
		}},
	}

	var cleanups []func() error
	for _, c := range collectors {
		cleanup, err := c.start(ctx, collector, state)
		if err != nil {
			send(uimsg.ApplicationError{
				Message:  fmt.Sprintf("Failed to start %s eBPF collector: %v. Tracing disabled.", c.name, err),
				Severity: "warning",
			})
			continue
		}
		cleanups = append(cleanups, cleanup)
	}
	return cleanups
}

func clearDatabases() {
	// Remove old database files - errors are silently ignored
	// This ensures we start with a clean state each time
	os.Remove("./deep_in_memory.db")
	os.Remove("./deep_in_memory_addresses.db")
}
