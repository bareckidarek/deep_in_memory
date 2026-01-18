package app

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

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
	db              storedb.Db[data.ProcessData]
	dbHelper        *DBHelper
	refreshInterval time.Duration
}

// New creates the main application model that coordinates UI, database, and eBPF collectors.
// Manages periodic dataset refresh and routes events between eBPF collectors and the UI.
// The refresh interval controls how often the UI updates with new process data.
func New(db storedb.Db[data.ProcessData], initial data.Dataset, refresh time.Duration) *Model {
	return &Model{
		ui:              uimodel.New(initial),
		db:              db,
		dbHelper:        NewDBHelper(db, nil), // Will be set later
		refreshInterval: refresh,
	}
}

// SetTargetPIDSetter wires the eBPF target PID setter into the UI model.
func (m *Model) SetTargetPIDSetter(fn func(int) error) {
	m.ui.SetTargetPIDSetter(fn)
}

// SetErrorCallback configures error reporting for database operations.
// Non-critical errors (like missing processes) will be reported through this callback
// instead of being silently ignored, allowing the UI to inform the user.
func (m *Model) SetErrorCallback(callback func(error)) {
	m.dbHelper.SetErrorCallback(callback)
}

// Init starts periodic refresh and event log flushing.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		tea.Tick(m.refreshInterval, func(time.Time) tea.Msg { return refreshMsg{} }),
		tea.Tick(1*time.Second, func(time.Time) tea.Msg {
			return uimsg.FlushEventLog{}
		}),
	)
}

// Update processes incoming messages from eBPF collectors and UI.
// Handles process lifecycle events, memory operations, and periodic refresh.
// Routes events to appropriate database handlers and manages application state.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case uimsg.ProcessExec:
		m.dbHelper.HandleProcessExec(v)
		return m, nil
	case uimsg.ProcessExit:
		m.dbHelper.HandleProcessExit(v)
		return m, nil
	case uimsg.ProcessMmap:
		m.dbHelper.HandleMmapEvent(v)
		return m, nil
	case uimsg.ProcessMunmap:
		m.dbHelper.HandleMunmapEvent(v)
		return m, nil
	case uimsg.ProcessBrk:
		m.dbHelper.HandleBrkEvent(v)
		return m, nil
	case uimsg.ProcessPageFault:
		m.dbHelper.HandlePageFaultEvent(v)
		return m, nil
	case uimsg.ProcessSwapUnswap:
		m.dbHelper.HandleSwapEvent(v)
		return m, nil
	case uimsg.FlushEventLog:
		// Flush event logs in UI model
		m.ui.FlushEventLogs()
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
func (m *Model) View() string {
	return m.ui.View()
}

func (m *Model) reload() {
	procs, err := m.db.All()
	if err != nil {
		// Don't crash the UI, just continue with empty dataset
		m.ui.SetDataset(data.Dataset{Processes: make(map[int]*data.ProcessData), Order: []int{}})
		return
	}
	ds := data.BuildDataset(procs)
	m.ui.SetDataset(ds)
}
