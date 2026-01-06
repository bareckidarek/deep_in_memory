package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"
	"time"

	"deep_in_memory/internal/app"

	tea "github.com/charmbracelet/bubbletea"

	"deep_in_memory/internal/data"
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

	initialProcs, err := store.All()
	if err != nil {
		log.Fatalf("failed to read initial data: %v", err)
	}
	initial := data.BuildDataset(initialProcs)
	appModel := app.New(store, initial, time.Second)
	p := tea.NewProgram(appModel, tea.WithAltScreen(), tea.WithMouseAllMotion())

	cleanup, setTargetPID, err := sched.Start(ctx, p.Send)
	if err != nil {
		log.Fatalf("failed to start eBPF collector: %v", err)
	}
	appModel.SetTargetPIDSetter(setTargetPID)
	defer cleanup()

	if err := p.Start(); err != nil {
		log.Fatalf("tui error: %v", err)
	}
}
