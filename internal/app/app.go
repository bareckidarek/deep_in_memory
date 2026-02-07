package app

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"deep_in_memory/internal/constants"
	"deep_in_memory/internal/data"
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
	refreshInterval time.Duration
	logAggregator   *LogAggregator
	targetPID       int // Currently monitored PID
}

// New creates the main application model that coordinates UI, database, and eBPF collectors.
// Manages periodic dataset refresh and routes events between eBPF collectors and the UI.
// The refresh interval controls how often the UI updates with new process data.
func New(db storedb.Db[data.ProcessData], addressDB storedb.Db[data.AddressData], initial data.Dataset, refresh time.Duration) *Model {
	return &Model{
		ui:              uimodel.New(initial),
		dbHelper:        NewDBHelper(db, addressDB, nil), // Will be set later
		refreshInterval: refresh,
		logAggregator:   NewLogAggregator(),
		targetPID:       0,
	}
}

// SetTargetPIDSetter wires the eBPF target PID setter into the UI model.
func (m *Model) SetTargetPIDSetter(fn func(int) error) {
	// Wrap the function to also track the target PID
	wrapper := func(pid int) error {
		m.targetPID = pid
		return fn(pid)
	}
	m.ui.SetTargetPIDSetter(wrapper)
}

// SetErrorCallback configures error reporting for database operations.
// Non-critical errors (like missing processes) will be reported through this callback
// instead of being silently ignored, allowing the UI to inform the user.
func (m *Model) SetErrorCallback(callback func(error)) {
	m.dbHelper.SetErrorCallback(callback)
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

// handleTracedEvent is a generic handler for eBPF events that follow the pattern:
// check targetPID → call dbHelper → optionally log → return error cmd on failure.
func (m *Model) handleTracedEvent(pid int, handler func() error, logType string, logAddr uint64, tid int) (tea.Model, tea.Cmd) {
	if m.targetPID != pid {
		return m, nil
	}
	if err := handler(); err != nil {
		return m, func() tea.Msg {
			return uimsg.ApplicationError{
				Message:  fmt.Sprintf("Failed to handle %s event (PID %d): %v", logType, pid, err),
				Severity: "error",
			}
		}
	}
	if pid == m.targetPID {
		m.logAggregator.Record(logType, logAddr, pid, tid)
	}
	return m, nil
}

// Update processes incoming messages from eBPF collectors and PUI.
// Routes events to appropriate database handlers and manages application state.
// Handles process lifecycle events, memory operations, and periodic refresh.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case uimsg.GetLogsMsg:
		if m.targetPID != 0 {
			m.logAggregator.Flush(m.targetPID)
		}
		logs := m.logAggregator.GetFormattedLogs()
		cmd := m.updateUI(uimsg.LogsResponseMsg{Logs: logs})
		nextCmd := tea.Tick(constants.LogFlushInterval, func(t time.Time) tea.Msg {
			return uimsg.GetLogsMsg{}
		})
		return m, tea.Batch(cmd, nextCmd)

	case uimsg.ProcessExec:
		if err := m.dbHelper.HandleProcessExec(v); err != nil {
			return m, func() tea.Msg {
				return uimsg.ApplicationError{
					Message:  fmt.Sprintf("Failed to handle process exec (PID %d): %v", v.PID, err),
					Severity: "error",
				}
			}
		}
		return m, nil

	case uimsg.ProcessExit:
		m.dbHelper.HandleProcessExit(v)
		if v.PID == m.targetPID {
			m.targetPID = 0
		}
		return m, nil

	case uimsg.ProcessMmap:
		return m.handleTracedEvent(v.PID, func() error { return m.dbHelper.HandleMmapEvent(v) }, "mmap", v.Addr, v.TID)

	case uimsg.ProcessMunmap:
		return m.handleTracedEvent(v.PID, func() error { return m.dbHelper.HandleMunmapEvent(v) }, "munmap", v.Addr, v.TID)

	case uimsg.ProcessBrk:
		return m.handleTracedEvent(v.PID, func() error { return m.dbHelper.HandleBrkEvent(v) }, "brk", v.Ret, v.TID)

	case uimsg.ProcessPageFault:
		return m.handleTracedEvent(v.PID, func() error { return m.dbHelper.HandlePageFaultEvent(v) }, "page_fault", v.Address, v.TID)

	case uimsg.ProcessSwapUnswap:
		logType := "swap_read"
		if v.IsSwap {
			logType = "swap_write"
		}
		return m.handleTracedEvent(v.PID, func() error { return m.dbHelper.HandleSwapEvent(v) }, logType, v.Address, v.TID)

	case uimsg.LoadSmapsRequest:
		if err := m.dbHelper.LoadSmapsForProcess(v.PID); err != nil {
			return m, func() tea.Msg {
				return uimsg.ApplicationError{
					Message:  fmt.Sprintf("Failed to load smaps (PID %d): %v", v.PID, err),
					Severity: "error",
				}
			}
		}
		return m, nil

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
