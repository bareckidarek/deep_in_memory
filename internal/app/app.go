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

// New constructs the app model with the provided DB, initial dataset, and refresh interval.
func New(db storedb.Db[data.ProcessData], initial data.Dataset, refresh time.Duration) *Model {
	return &Model{
		ui:              uimodel.New(initial),
		db:              db,
		dbHelper:        NewDBHelper(db),
		refreshInterval: refresh,
	}
}

// SetTargetPIDSetter wires the eBPF target PID setter into the UI model.
func (m *Model) SetTargetPIDSetter(fn func(int) error) {
	m.ui.SetTargetPIDSetter(fn)
}

// Init starts periodic refresh.
func (m *Model) Init() tea.Cmd {
	return tea.Tick(m.refreshInterval, func(time.Time) tea.Msg { return refreshMsg{} })
}

// Update routes eBPF events into the DB and refreshes UI data independently.
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
	case uimsg.LoadSmapsRequest:
		m.dbHelper.LoadSmapsForProcess(v.PID)
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
