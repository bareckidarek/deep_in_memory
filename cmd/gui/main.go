package main

import (
	"context"
	"fmt"
	"log"
	"os/signal"
	"syscall"
	"time"

	"deep_in_memory/internal/app"

	tea "github.com/charmbracelet/bubbletea"

	"deep_in_memory/internal/data"
	"deep_in_memory/internal/ebpf/mmap"
	"deep_in_memory/internal/ebpf/page_fault"
	"deep_in_memory/internal/ebpf/sched"
	storedb "deep_in_memory/internal/thirdparty/db"
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
	appModel := app.New(store, initial, time.Second)
	p := tea.NewProgram(appModel, tea.WithAltScreen(), tea.WithMouseAllMotion())

	// Start sched collector and get the shared target_pid map
	schedCleanup, setTargetPID, targetPidMap, err := sched.Start(ctx, p.Send)
	if err != nil {
		log.Fatalf("failed to start sched eBPF collector: %v", err)
	}
	defer schedCleanup()

	// Start mmap collector with shared target_pid map
	mmapCleanup, err := mmap.Start(ctx, p.Send, targetPidMap)
	if err != nil {
		log.Fatalf("failed to start mmap eBPF collector: %v", err)
	}
	defer mmapCleanup()

	// Start page_fault collector with shared target_pid map
	pageFaultCleanup, err := page_fault.Start(ctx, p.Send, targetPidMap)
	if err != nil {
		log.Fatalf("failed to start page_fault eBPF collector: %v", err)
	}
	defer pageFaultCleanup()

	// Only sched owns the target_pid map, so we only use its setter
	appModel.SetTargetPIDSetter(setTargetPID)

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
			// Continue even if delete fails
			log.Printf("failed to delete process %d: %v", proc.Proc.PID, err)
		}
	}
	return nil
}
