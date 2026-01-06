package main

import (
	"context"
	"fmt"
	"os"
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
		fmt.Println("failed to open bolt db:", err)
		os.Exit(1)
	}
	defer store.Close()

	initialProcs, err := store.All()
	if err != nil {
		fmt.Println("failed to read initial data:", err)
		os.Exit(1)
	}
	initial := data.BuildDataset(initialProcs)
	appModel := app.New(store, initial, time.Second)
	p := tea.NewProgram(appModel, tea.WithAltScreen(), tea.WithMouseAllMotion())

	cleanup, setTargetPID, err := sched.Start(ctx, p.Send)
	if err != nil {
		fmt.Println("failed to start eBPF collector:", err)
		os.Exit(1)
	}
	appModel.SetTargetPIDSetter(setTargetPID)
	defer cleanup()

	if err := p.Start(); err != nil {
		fmt.Println("tui error:", err)
		os.Exit(1)
	}
}
