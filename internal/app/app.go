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

// Update processes incoming messages from eBPF collectors and PUI.
// Routes events to appropriate database handlers and manages application state.
// Handles process lifecycle events, memory operations, and periodic refresh.
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
		if err := m.dbHelper.HandleProcessExec(v); err != nil {
			return m, func() tea.Msg {
				return uimsg.ApplicationError{
					Message:  fmt.Sprintf("Failed to handle process exec (PID %d): %v", v.PID, err),
					Severity: "error",
				}
			}
		}
		// Don't update UI - it will refresh on next refreshMsg
		return m, nil
	case uimsg.ProcessExit:
		m.dbHelper.HandleProcessExit(v)
		// If we're monitoring this process, clear target PID
		if v.PID == m.targetPID {
			m.targetPID = 0
		}
		// Don't update UI - it will refresh on next refreshMsg
		return m, nil
	case uimsg.ProcessMmap:
		// Ignore if no process is being monitored (stale events after process exit)
		if m.targetPID != v.PID {
			return m, nil
		}
		if err := m.dbHelper.HandleMmapEvent(v); err != nil {
			return m, func() tea.Msg {
				return uimsg.ApplicationError{
					Message:  fmt.Sprintf("Failed to handle mmap event (PID %d): %v", v.PID, err),
					Severity: "error",
				}
			}
		}
		// Aggregate in app layer
		if v.PID == m.targetPID {
			m.logAggregator.Record("mmap", v.Addr, v.PID, v.TID)
		}
		return m, nil
	case uimsg.ProcessMunmap:
		// Ignore if no process is being monitored (stale events after process exit)
		if m.targetPID != v.PID {
			return m, nil
		}
		if err := m.dbHelper.HandleMunmapEvent(v); err != nil {
			return m, func() tea.Msg {
				return uimsg.ApplicationError{
					Message:  fmt.Sprintf("Failed to handle munmap event (PID %d): %v", v.PID, err),
					Severity: "error",
				}
			}
		}
		// Aggregate in app layer
		if v.PID == m.targetPID {
			m.logAggregator.Record("munmap", v.Addr, v.PID, v.TID)
		}
		return m, nil
	case uimsg.ProcessBrk:
		// Ignore if no process is being monitored (stale events after process exit)
		if m.targetPID != v.PID {
			return m, nil
		}
		if err := m.dbHelper.HandleBrkEvent(v); err != nil {
			return m, func() tea.Msg {
				return uimsg.ApplicationError{
					Message:  fmt.Sprintf("Failed to handle brk event (PID %d): %v", v.PID, err),
					Severity: "error",
				}
			}
		}
		// Aggregate in app layer
		if v.PID == m.targetPID {
			m.logAggregator.Record("brk", v.Ret, v.PID, v.TID)
		}
		return m, nil
	case uimsg.ProcessPageFault:
		// Ignore if no process is being monitored (stale events after process exit)
		if m.targetPID != v.PID {
			return m, nil
		}
		if err := m.dbHelper.HandlePageFaultEvent(v); err != nil {
			return m, func() tea.Msg {
				return uimsg.ApplicationError{
					Message:  fmt.Sprintf("Failed to handle page fault event (PID %d): %v", v.PID, err),
					Severity: "error",
				}
			}
		}
		// Aggregate in app layer
		if v.PID == m.targetPID {
			m.logAggregator.Record("page_fault", v.Address, v.PID, v.TID)
		}
		return m, nil
	case uimsg.ProcessSwapUnswap:
		// Ignore if no process is being monitored (stale events after process exit)
		if m.targetPID != v.PID {
			return m, nil
		}
		if err := m.dbHelper.HandleSwapEvent(v); err != nil {
			return m, func() tea.Msg {
				return uimsg.ApplicationError{
					Message:  fmt.Sprintf("Failed to handle swap event (PID %d): %v", v.PID, err),
					Severity: "error",
				}
			}
		}
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
		// Forward error to UI model for display
		updated, cmd := m.ui.Update(v)
		if ui, ok := updated.(*uimodel.Model); ok {
			m.ui = ui
		}
		return m, cmd
	case uimsg.HideModalMsg:
		// Forward modal hide to UI
		updated, cmd := m.ui.Update(v)
		if ui, ok := updated.(*uimodel.Model); ok {
			m.ui = ui
		}
		return m, cmd
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
		// Report error to UI but don't crash - continue with empty dataset
		m.dbHelper.reportError(fmt.Errorf("failed to reload database: %w", err))
		m.ui.SetDataset(data.Dataset{Processes: make(map[int]*data.ProcessData), Order: []int{}})
		return
	}
	ds := data.BuildDataset(procs)
	m.ui.SetDataset(ds)
}
