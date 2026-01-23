package app

import (
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

// Update processes incoming messages from eBPF collectors and UI.
// Handles process lifecycle events, memory operations, and periodic refresh.
// Routes events to appropriate database handlers and manages application state.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case uimsg.GetLogsMsg:
		// Flush current aggregated logs and send to UI
		if m.targetPID != 0 {
			m.logAggregator.Flush(m.targetPID)
		}
		logs := m.logAggregator.GetFormattedLogs()
		// Send logs response to UI and schedule next log request
		updated, cmd := m.ui.Update(uimsg.LogsResponseMsg{Logs: logs})
		if ui, ok := updated.(*uimodel.Model); ok {
			m.ui = ui
		}
		// Schedule next GetLogsMsg
		nextCmd := tea.Tick(constants.LogFlushInterval, func(t time.Time) tea.Msg {
			return uimsg.GetLogsMsg{}
		})
		return m, tea.Batch(cmd, nextCmd)
	case uimsg.ProcessExec:
		m.dbHelper.HandleProcessExec(v)
		// Forward to UI model
		updated, cmd := m.ui.Update(v)
		if ui, ok := updated.(*uimodel.Model); ok {
			m.ui = ui
		}
		return m, cmd
	case uimsg.ProcessExit:
		m.dbHelper.HandleProcessExit(v)
		updated, cmd := m.ui.Update(v)
		if ui, ok := updated.(*uimodel.Model); ok {
			m.ui = ui
		}
		return m, cmd
	case uimsg.ProcessMmap:
		m.dbHelper.HandleMmapEvent(v)
		// Aggregate in app layer
		if v.PID == m.targetPID {
			m.logAggregator.Record("mmap", v.Addr, v.PID, v.TID)
		}
		return m, nil
	case uimsg.ProcessMunmap:
		m.dbHelper.HandleMunmapEvent(v)
		// Aggregate in app layer
		if v.PID == m.targetPID {
			m.logAggregator.Record("munmap", v.Addr, v.PID, v.TID)
		}
		return m, nil
	case uimsg.ProcessBrk:
		m.dbHelper.HandleBrkEvent(v)
		// Aggregate in app layer
		if v.PID == m.targetPID {
			m.logAggregator.Record("brk", v.Ret, v.PID, v.TID)
		}
		return m, nil
	case uimsg.ProcessPageFault:
		m.dbHelper.HandlePageFaultEvent(v)
		// Aggregate in app layer
		if v.PID == m.targetPID {
			m.logAggregator.Record("page_fault", v.Address, v.PID, v.TID)
		}
		return m, nil
	case uimsg.ProcessSwapUnswap:
		m.dbHelper.HandleSwapEvent(v)
		// Aggregate in app layer
		if v.PID == m.targetPID {
			if v.IsSwap {
				m.logAggregator.Record("swap_write", v.Address, v.PID, v.TID)
			} else {
				m.logAggregator.Record("swap_read", v.Address, v.PID, v.TID)
			}
		}
		return m, nil
	case uimsg.LoadSmapsRequest:
		m.dbHelper.LoadSmapsForProcess(v.PID)
		return m, nil
	case uimsg.ApplicationError:
		// Forward error to UI model for display
		return m, nil
	case refreshMsg:
		m.reload()
		return m, tea.Tick(m.refreshInterval, func(time.Time) tea.Msg { return refreshMsg{} })
	}

	updated, cmd := m.ui.Update(msg)
	if ui, ok := updated.(*uimodel.Model); ok {
		m.ui = ui
	}
	return m, cmd
}

// View delegates to the UI model.
func (m *Model) View() tea.View {
	return m.ui.View()
}

func (m *Model) reload() {
	procs, err := m.dbHelper.db.All()
	if err != nil {
		// Don't crash the UI, just continue with empty dataset
		m.ui.SetDataset(data.Dataset{Processes: make(map[int]*data.ProcessData), Order: []int{}})
		return
	}
	ds := data.BuildDataset(procs)
	m.ui.SetDataset(ds)
}
