package main

import (
	"context"
	"fmt"
	"log"
	"os/signal"
	"syscall"

	tea "charm.land/bubbletea/v2"

	"deep_in_memory/internal/collector"
	"deep_in_memory/internal/constants"
	"deep_in_memory/internal/persistence"
	"deep_in_memory/internal/ui"
	uimsg "deep_in_memory/internal/ui/msg"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Open persistent storage using the new persistence layer
	writer, reader, err := persistence.Open("deep_in_memory", "deep_in_memory_addresses")
	if err != nil {
		log.Fatalf("failed to open persistent storage: %v", err)
	}
	defer writer.Close()

	// Clear databases on startup for fresh state
	if err := clearDatabases(writer, reader); err != nil {
		log.Fatalf("failed to clear databases: %v", err)
	}

	// Start eBPF collector in background
	collectorCfg := collector.CollectorConfig{
		ErrorCallback: func(err error) {
			log.Printf("Collector error: %v", err)
		},
	}

	ebpfCollector, cleanupCollector, err := collector.StartCollector(ctx, collectorCfg, writer, reader)
	if err != nil {
		log.Fatalf("failed to start eBPF collector: %v", err)
	}
	defer cleanupCollector()

	// Create UI runner that reads from persistence
	uiRunner, err := ui.NewUIRunner(ui.RunUIConfig{
		Reader:          reader,
		RefreshInterval: constants.DefaultRefreshInterval,
		LogAggregator:   ebpfCollector.GetLogAggregator(),
		TargetPIDSetter: ebpfCollector.SetTargetPID,
	})
	if err != nil {
		log.Fatalf("failed to create UI runner: %v", err)
	}

	// Start Bubble Tea program with UI runner
	p := tea.NewProgram(uiRunner)

	// Set error callback to send errors to UI
	collectorCfg.ErrorCallback = func(err error) {
		p.Send(uimsg.ApplicationError{
			Message:  err.Error(),
			Severity: "info",
		})
	}

	if _, err := p.Run(); err != nil {
		log.Fatalf("tui error: %v", err)
	}
}

// clearDatabases removes all data from persistent storage on startup
func clearDatabases(writer persistence.Writer, reader persistence.Reader) error {
	ctx := context.Background()

	// Clear process database
	allProcs, err := reader.AllProcessData(ctx)
	if err != nil {
		return fmt.Errorf("failed to read process data: %w", err)
	}
	for _, proc := range allProcs {
		key := fmt.Sprintf("%d", proc.Proc.PID)
		if err := writer.DeleteProcessData(ctx, key); err != nil {
			fmt.Printf("Warning: failed to delete process %d from database: %v\n", proc.Proc.PID, err)
		}
	}

	// Clear address database
	allAddrs, err := reader.AllAddressData(ctx)
	if err != nil {
		return fmt.Errorf("failed to read address data: %w", err)
	}
	for _, addrData := range allAddrs {
		key := fmt.Sprintf("%d", addrData.PID)
		if err := writer.DeleteAddressData(ctx, key); err != nil {
			fmt.Printf("Warning: failed to delete address data %d from database: %v\n", addrData.PID, err)
		}
	}

	return nil
}
