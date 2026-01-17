package main

import (
	"context"
	"fmt"
	"log"
	"os/signal"
	"syscall"

	"deep_in_memory/internal/app"

	tea "github.com/charmbracelet/bubbletea"

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

	store, err := storedb.New[data.ProcessData]("deep_in_memory")
	if err != nil {
		log.Fatalf("failed to open bolt db: %v", err)
	}
	defer store.Close()

	// Clear database on startup for fresh state
	if err := clearDatabase(store); err != nil {
		log.Fatalf("failed to clear database: %v", err)
	}

	initialProcs, err := store.All()
	if err != nil {
		log.Fatalf("failed to read initial data: %v", err)
	}
	initial := data.BuildDataset(initialProcs)
	appModel := app.New(store, initial, constants.DefaultRefreshInterval)
	p := tea.NewProgram(appModel, tea.WithAltScreen(), tea.WithMouseAllMotion())

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
		defer schedCleanup()
		// Convert interface{} to SharedState for eBPF mode
		if s, ok := stateInterface.(*ebpfshared.SharedState); ok {
			sharedState = s
		}
	}

	// Start mmap collector with shared state
	if sharedState != nil {
		mmapCleanup, err := mmap.Start(ctx, p.Send, sharedState)
		if err != nil {
			p.Send(uimsg.ApplicationError{
				Message:  fmt.Sprintf("Failed to start mmap eBPF collector: %v. Memory mapping tracing disabled.", err),
				Severity: "warning",
			})
		} else {
			defer mmapCleanup()
		}
	}

	// Start brk collector with shared state
	if sharedState != nil {
		brkCleanup, err := brk.Start(ctx, p.Send, sharedState)
		if err != nil {
			p.Send(uimsg.ApplicationError{
				Message:  fmt.Sprintf("Failed to start brk eBPF collector: %v. Heap tracing disabled.", err),
				Severity: "warning",
			})
		} else {
			defer brkCleanup()
		}
	}

	// Start page_fault collector with shared state
	if sharedState != nil {
		pageFaultCleanup, err := page_fault.Start(ctx, p.Send, sharedState)
		if err != nil {
			p.Send(uimsg.ApplicationError{
				Message:  fmt.Sprintf("Failed to start page_fault eBPF collector: %v. Page fault tracing disabled.", err),
				Severity: "warning",
			})
		} else {
			defer pageFaultCleanup()
		}
	}

	// Start swap collector with shared state
	if sharedState != nil {
		swapCleanup, err := swap.Start(ctx, p.Send, sharedState)
		if err != nil {
			p.Send(uimsg.ApplicationError{
				Message:  fmt.Sprintf("Failed to start swap eBPF collector: %v. Swap tracing disabled.", err),
				Severity: "warning",
			})
		} else {
			defer swapCleanup()
		}
	}

	// Set the target PID setter in the app model if eBPF is enabled
	if sharedState != nil {
		appModel.SetTargetPIDSetter(sharedState.SetTargetPID)
	}

	// Set error callback for database operations
	appModel.SetErrorCallback(func(err error) {
		p.Send(uimsg.ApplicationError{
			Message:  err.Error(),
			Severity: "info",
		})
	})

	if err := p.Start(); err != nil {
		log.Fatalf("tui error: %v", err)
	}
}

func clearDatabase(store storedb.Db[data.ProcessData]) error {
	allProcs, err := store.All()
	if err != nil {
		return err
	}
	for _, proc := range allProcs {
		key := fmt.Sprintf("%d", proc.Proc.PID)
		if err := store.Delete(key); err != nil {
			// Report delete failure but continue with other processes
			fmt.Printf("Warning: failed to delete process %d from database: %v\n", proc.Proc.PID, err)
		}
	}
	return nil
}
