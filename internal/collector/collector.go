//go:build ebpf

package collector

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/timshannon/bolthold"

	"deep_in_memory/internal/app"
	"deep_in_memory/internal/data"
	"deep_in_memory/internal/ebpf"
	"deep_in_memory/internal/ebpf/brk"
	"deep_in_memory/internal/ebpf/mmap"
	"deep_in_memory/internal/ebpf/page_fault"
	"deep_in_memory/internal/ebpf/sched"
	"deep_in_memory/internal/ebpf/swap"
	"deep_in_memory/internal/persistence"
	uimsg "deep_in_memory/internal/ui/msg"
)

// CollectorConfig holds configuration for the collector
type CollectorConfig struct {
	// ErrorCallback is called when non-critical errors occur
	ErrorCallback func(error)
}

// Collector manages eBPF event collection and persistence
type Collector struct {
	writer          persistence.Writer
	dbHelper        *app.DBHelper
	addressDbHelper *app.AddressDBHelper
	logAggregator   *app.LogAggregator
	sharedState     *ebpf.SharedState
	targetPID       int
	cfg             CollectorConfig
}

// StartCollector initializes eBPF collectors and starts processing events.
// It receives events from eBPF and writes them to persistent storage via Writer.
// Returns a cleanup function and SharedState for PID filtering.
func StartCollector(ctx context.Context, cfg CollectorConfig, writer persistence.Writer, reader persistence.Reader) (*Collector, func() error, error) {
	// Create a dummy process and address DB just to adapt existing helpers
	// In the future, we should refactor helpers to work directly with Writer interface
	dummyProcessDB := &writerToDbAdapter{writer: writer, reader: reader}
	dummyAddressDB := &writerToDbAdapterAddress{writer: writer, reader: reader}

	c := &Collector{
		writer:          writer,
		dbHelper:        app.NewDBHelper(dummyProcessDB, cfg.ErrorCallback),
		addressDbHelper: app.NewAddressDBHelper(dummyAddressDB, cfg.ErrorCallback),
		logAggregator:   app.NewLogAggregator(),
		cfg:             cfg,
	}

	// Create a message handler for eBPF events
	send := func(msg tea.Msg) {
		c.handleEvent(msg)
	}

	// Start sched collector and get the shared state
	schedCleanup, stateInterface, err := sched.Start(ctx, send)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to start sched eBPF collector: %w", err)
	}

	// Convert interface{} to SharedState
	sharedState, ok := stateInterface.(*ebpf.SharedState)
	if !ok {
		schedCleanup()
		return nil, nil, fmt.Errorf("failed to get shared state from sched collector")
	}
	c.sharedState = sharedState

	// Start other collectors
	mmapCleanup, err := mmap.Start(ctx, send, sharedState)
	if err != nil {
		schedCleanup()
		return nil, nil, fmt.Errorf("failed to start mmap eBPF collector: %w", err)
	}

	brkCleanup, err := brk.Start(ctx, send, sharedState)
	if err != nil {
		mmapCleanup()
		schedCleanup()
		return nil, nil, fmt.Errorf("failed to start brk eBPF collector: %w", err)
	}

	pageFaultCleanup, err := page_fault.Start(ctx, send, sharedState)
	if err != nil {
		brkCleanup()
		mmapCleanup()
		schedCleanup()
		return nil, nil, fmt.Errorf("failed to start page_fault eBPF collector: %w", err)
	}

	swapCleanup, err := swap.Start(ctx, send, sharedState)
	if err != nil {
		pageFaultCleanup()
		brkCleanup()
		mmapCleanup()
		schedCleanup()
		return nil, nil, fmt.Errorf("failed to start swap eBPF collector: %w", err)
	}

	cleanup := func() error {
		swapCleanup()
		pageFaultCleanup()
		brkCleanup()
		mmapCleanup()
		schedCleanup()
		return nil
	}

	return c, cleanup, nil
}

// SetTargetPID sets the target PID for filtering events
func (c *Collector) SetTargetPID(pid int) error {
	c.targetPID = pid
	if c.sharedState != nil {
		return c.sharedState.SetTargetPID(pid)
	}
	return nil
}

// GetLogAggregator returns the log aggregator for UI access
func (c *Collector) GetLogAggregator() *app.LogAggregator {
	return c.logAggregator
}

// handleEvent processes incoming eBPF events and persists them
func (c *Collector) handleEvent(msg tea.Msg) {
	switch v := msg.(type) {
	case uimsg.ProcessExec:
		c.dbHelper.HandleProcessExec(v)
		c.addressDbHelper.HandleProcessExec(v)

	case uimsg.ProcessExit:
		c.dbHelper.HandleProcessExit(v)
		c.addressDbHelper.HandleProcessExit(v)

	case uimsg.ProcessMmap:
		c.dbHelper.HandleMmapEvent(v)
		c.addressDbHelper.HandleMmap(v.ToMmapEvent())
		if v.PID == c.targetPID {
			c.logAggregator.Record("mmap", v.Addr, v.PID, v.TID)
		}

	case uimsg.ProcessMunmap:
		c.dbHelper.HandleMunmapEvent(v)
		c.addressDbHelper.HandleMunmap(v.ToMunmapEvent())
		if v.PID == c.targetPID {
			c.logAggregator.Record("munmap", v.Addr, v.PID, v.TID)
		}

	case uimsg.ProcessBrk:
		c.dbHelper.HandleBrkEvent(v)
		c.addressDbHelper.HandleBrk(v.ToBrkEvent())
		if v.PID == c.targetPID {
			c.logAggregator.Record("brk", v.Ret, v.PID, v.TID)
		}

	case uimsg.ProcessPageFault:
		c.dbHelper.HandlePageFaultEvent(v)
		c.addressDbHelper.HandlePageFault(v.ToPageFaultEvent())
		if v.PID == c.targetPID {
			c.logAggregator.Record("page_fault", v.Address, v.PID, v.TID)
		}

	case uimsg.ProcessSwapUnswap:
		c.dbHelper.HandleSwapEvent(v)
		c.addressDbHelper.HandleSwap(v.ToSwapEvent())
		if v.PID == c.targetPID {
			if v.IsSwap {
				c.logAggregator.Record("swap_write", v.Address, v.PID, v.TID)
			} else {
				c.logAggregator.Record("swap_read", v.Address, v.PID, v.TID)
			}
		}
	}
}

// writerToDbAdapter adapts persistence.Writer to storedb.Db interface for ProcessData
type writerToDbAdapter struct {
	writer persistence.Writer
	reader persistence.Reader
}

func (a *writerToDbAdapter) Query(query *bolthold.Query) ([]data.ProcessData, error) {
	// For now, just return all - proper query implementation would need refactoring
	return a.reader.AllProcessData(context.Background())
}

func (a *writerToDbAdapter) Upsert(key string, value *data.ProcessData) error {
	return a.writer.WriteProcessData(context.Background(), key, value)
}

func (a *writerToDbAdapter) Delete(key string) error {
	return a.writer.DeleteProcessData(context.Background(), key)
}

func (a *writerToDbAdapter) All() ([]data.ProcessData, error) {
	return a.reader.AllProcessData(context.Background())
}

func (a *writerToDbAdapter) Close() error {
	return nil // Don't close here, managed by persistence layer
}

// writerToDbAdapterAddress adapts persistence.Writer to storedb.Db interface for AddressData
type writerToDbAdapterAddress struct {
	writer persistence.Writer
	reader persistence.Reader
}

func (a *writerToDbAdapterAddress) Query(query *bolthold.Query) ([]data.AddressData, error) {
	return a.reader.AllAddressData(context.Background())
}

func (a *writerToDbAdapterAddress) Upsert(key string, value *data.AddressData) error {
	return a.writer.WriteAddressData(context.Background(), key, value)
}

func (a *writerToDbAdapterAddress) Delete(key string) error {
	return a.writer.DeleteAddressData(context.Background(), key)
}

func (a *writerToDbAdapterAddress) All() ([]data.AddressData, error) {
	return a.reader.AllAddressData(context.Background())
}

func (a *writerToDbAdapterAddress) Close() error {
	return nil // Don't close here, managed by persistence layer
}
