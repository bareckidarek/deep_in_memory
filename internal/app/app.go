package app

import (
	"fmt"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"

	"deep_in_memory/internal/constants"
	"deep_in_memory/internal/data"
	"deep_in_memory/internal/data/proc"
	storedb "deep_in_memory/internal/thirdparty/db"
	uimodel "deep_in_memory/internal/ui/model"
	uimsg "deep_in_memory/internal/ui/msg"
)

// refreshMsg triggers a dataset reload from the DB.
type refreshMsg struct{}

// Model wraps the UI model with persistence-backed refresh.
type Model struct {
	ui              *uimodel.Model
	dbHelper        *DBHelper
	asyncWriter     *AsyncDBWriter // ALL DB writes go through this
	refreshInterval time.Duration
	logAggregator   *LogAggregator
	targetPIDs      []int // Currently monitored PIDs
}

// New creates the main application model that coordinates UI, database, and eBPF collectors.
// Manages periodic dataset refresh and routes events between eBPF collectors and the UI.
// The refresh interval controls how often the UI updates with new process data.
func New(db storedb.Db[data.ProcessData], addressDB storedb.Db[data.AddressData], initial data.Dataset, refresh time.Duration) *Model {
	dbHelper := NewDBHelper(db, addressDB, nil)
	m := &Model{
		ui:              uimodel.New(initial),
		dbHelper:        dbHelper,
		asyncWriter:     NewAsyncDBWriter(dbHelper, nil), // errCallback set via SetErrorCallback
		refreshInterval: refresh,
		logAggregator:   NewLogAggregator(),
		targetPIDs:      []int{},
	}
	return m
}

// SetTargetPIDSetter wires the eBPF target PID setter into the UI model.
func (m *Model) SetTargetPIDSetter(fn func(int) error) {
	// Wrap the function to also track the target PID
	wrapper := func(pid int) error {
		m.targetPIDs = []int{pid}
		return fn(pid)
	}
	m.ui.SetTargetPIDSetter(wrapper)
}

// SetErrorCallback configures error reporting for database operations.
// Non-critical errors (like missing processes) will be reported through this callback
// instead of being silently ignored, allowing the UI to inform the user.
func (m *Model) SetErrorCallback(callback func(error)) {
	m.dbHelper.SetErrorCallback(callback)
	m.asyncWriter.SetErrorCallback(callback)
}

// StopAsyncWriter gracefully shuts down the async DB writer.
// Must be called before closing BoltDB databases.
func (m *Model) StopAsyncWriter() {
	if m.asyncWriter != nil {
		m.asyncWriter.Stop()
	}
}

// Init starts periodic refresh and initializes UI.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		tea.Tick(m.refreshInterval, func(time.Time) tea.Msg { return refreshMsg{} }),
		m.ui.Init(), // Initialize UI model
		// Start log request timer
		tea.Tick(constants.LogFlushInterval, func(time.Time) tea.Msg {
			return uimsg.GetLogsMsg{}
		}),
		// Load /proc/iomem on startup
		func() tea.Msg {
			return loadIomem()
		},
		// Enqueue myself process data
		func() tea.Msg {
			me := uimsg.ProcessExec{
				PID:       os.Getpid(),
				Comm:      os.Args[0],
				Timestamp: time.Now(),
			}
			m.asyncWriter.Enqueue(me.PID, func(h *DBHelper) error {
				return h.HandleProcessExec(me)
			})
			return nil
		},
	)
}

// updateUI forwards a message to the UI model and keeps the reference in sync.
func (m *Model) updateUI(msg tea.Msg) tea.Cmd {
	updated, cmd := m.ui.Update(msg)
	if ui, ok := updated.(*uimodel.Model); ok {
		m.ui = ui
	}
	return cmd
}

// handleTracedEvent enqueues an eBPF event for async DB processing.
// The DB write happens in the background goroutine — Update() never blocks on BoltDB.
// Logging happens immediately (logs don't depend on DB state).
func (m *Model) handleTracedEvent(pid int, handler func(*DBHelper) error, logType string, logAddr uint64, tid int) (tea.Model, tea.Cmd) {
	found := false
	for _, target := range m.targetPIDs {
		if target == pid {
			found = true
			break
		}
	}
	if !found {
		return m, nil
	}
	m.asyncWriter.Enqueue(pid, handler)
	m.logAggregator.Record(logType, logAddr, pid, tid)
	return m, nil
}

// Update processes incoming messages from eBPF collectors and PUI.
// Routes events to appropriate database handlers and manages application state.
// Handles process lifecycle events, memory operations, and periodic refresh.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case []interface{}:
		// Handle batched events from eBPF collectors
		for _, batchedMsg := range v {
			m.Update(batchedMsg)
		}
		return m, nil

	case uimsg.GetLogsMsg:
		if len(m.targetPIDs) > 0 {
			m.logAggregator.Flush(m.targetPIDs)
		}
		logs := m.logAggregator.GetFormattedLogs()
		cmd := m.updateUI(uimsg.LogsResponseMsg{Logs: logs})
		nextCmd := tea.Tick(constants.LogFlushInterval, func(t time.Time) tea.Msg {
			return uimsg.GetLogsMsg{}
		})
		return m, tea.Batch(cmd, nextCmd)

	case uimsg.ProcessExec:
		// Async: create process record in background
		m.asyncWriter.Enqueue(v.PID, func(h *DBHelper) error {
			return h.HandleProcessExec(v)
		})
		return m, nil

	case uimsg.ProcessExit:
		// EnqueueExit marks PID as dead FIRST, then queues deletion.
		// All future events for this PID are rejected by Enqueue().
		m.asyncWriter.EnqueueExit(v.PID, func(h *DBHelper) error {
			return h.HandleProcessExit(v)
		})
		for i, target := range m.targetPIDs {
			if target == v.PID {
				m.targetPIDs = append(m.targetPIDs[:i], m.targetPIDs[i+1:]...)
				break
			}
		}
		// Clear physical memory section state via UI message
		return m, m.updateUI(uimsg.ClearPhysicalMemoryMsg{PID: v.PID})

	case uimsg.ProcessClone:
		return m.handleTracedEvent(v.PID, func(h *DBHelper) error { return h.HandleCloneEvent(v) }, "clone", 0, v.TID)

	case uimsg.ProcessMmap:
		return m.handleTracedEvent(v.PID, func(h *DBHelper) error { return h.HandleMmapEvent(v) }, "mmap", v.Addr, v.TID)

	case uimsg.ProcessMunmap:
		return m.handleTracedEvent(v.PID, func(h *DBHelper) error { return h.HandleMunmapEvent(v) }, "munmap", v.Addr, v.TID)

	case uimsg.ProcessBrk:
		return m.handleTracedEvent(v.PID, func(h *DBHelper) error { return h.HandleBrkEvent(v) }, "brk", v.Ret, v.TID)

	case uimsg.ProcessInitSwapUnswap:
		logType := ""
		switch v.Type {
		case 0:
			logType = "init"
		case 1:
			logType = "swap_write"
		case 2:
			logType = "swap_read"
		}
		return m.handleTracedEvent(v.PID, func(h *DBHelper) error { return h.HandlePagingEvent(v) }, logType, v.Address, v.TID)

	case uimsg.LoadSmapsRequest:
		// Read /proc smaps synchronously (fast I/O), queue DB write async
		threads, err := proc.LoadSmapsRanges(v.PID)
		if err != nil || len(threads) == 0 {
			return m, nil
		}
		capturedThreads := threads
		m.asyncWriter.Enqueue(v.PID, func(h *DBHelper) error {
			record, err := h.getProcess(v.PID)
			if err != nil || record == nil {
				return nil // Process gone, skip
			}
			record.Threads = capturedThreads
			return h.db.Upsert(pidKey(v.PID), record)
		})
		return m, nil

	case uimsg.IomemLoadResponse:
		return m, m.updateUI(v)

	case uimsg.PagemapLookupResponse:
		return m, m.updateUI(v)

	case uimsg.ApplicationError:
		return m, m.updateUI(v)

	case uimsg.HideModalMsg:
		return m, m.updateUI(v)

	case refreshMsg:
		m.reload()
		return m, tea.Tick(m.refreshInterval, func(time.Time) tea.Msg { return refreshMsg{} })
	}

	return m, m.updateUI(msg)
}

// View delegates to the UI model.
func (m *Model) View() tea.View {
	return m.ui.View()
}

func (m *Model) reload() {
	procs, err := m.dbHelper.db.All()
	if err != nil {
		// Report error to UI but don't crash - continue with empty dataset
		m.dbHelper.reportError(fmt.Errorf("failed to reload database: %w", err))
		m.ui.SetDataset(data.Dataset{Processes: make(map[int]*data.ProcessData), Order: []int{}})
		return
	}
	ds := data.BuildDataset(procs)
	m.ui.SetDataset(ds)
}

// loadIomem reads /proc/iomem and returns an IomemLoadResponse message.
func loadIomem() uimsg.IomemLoadResponse {
	ranges, err := proc.LoadIomem()
	if err != nil {
		return uimsg.IomemLoadResponse{}
	}

	var msgRanges []uimsg.IomemRangeMsg
	for _, r := range ranges {
		msgRanges = append(msgRanges, uimsg.IomemRangeMsg{
			Start: r.Start,
			End:   r.End,
			Name:  r.Name,
			Level: r.Level,
		})
	}

	return uimsg.IomemLoadResponse{Ranges: msgRanges}
}
