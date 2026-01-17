package app

import (
	"log"
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
		if err := m.dbHelper.HandleProcessExec(v); err != nil {
			log.Printf("Failed to handle exec event for PID %d: %v", v.PID, err)
		}
		return m, nil
	case uimsg.ProcessExit:
		if err := m.dbHelper.HandleProcessExit(v); err != nil {
			log.Printf("Failed to handle exit event for PID %d: %v", v.PID, err)
		}
		return m, nil
	case uimsg.ProcessMmap:
		if err := m.dbHelper.HandleMmapEvent(v); err != nil {
			log.Printf("Failed to handle mmap event for PID %d: %v", v.PID, err)
		}
		return m, nil
	case uimsg.ProcessMunmap:
		if err := m.dbHelper.HandleMunmapEvent(v); err != nil {
			log.Printf("Failed to handle munmap event for PID %d: %v", v.PID, err)
		}
		return m, nil
	case uimsg.ProcessBrk:
		if err := m.dbHelper.HandleBrkEvent(v); err != nil {
			log.Printf("Failed to handle brk event for PID %d: %v", v.PID, err)
		}
		return m, nil
	case uimsg.ProcessPageFault:
		if err := m.dbHelper.HandlePageFaultEvent(v); err != nil {
			log.Printf("Failed to handle page fault for PID %d: %v", v.PID, err)
		}
		return m, nil
	case uimsg.ProcessSwapUnswap:
		if err := m.dbHelper.HandleSwapEvent(v); err != nil {
			log.Printf("Failed to handle swap event for PID %d: %v", v.PID, err)
		}
		return m, nil
	case uimsg.LoadSmapsRequest:
		if err := m.dbHelper.LoadSmapsForProcess(v.PID); err != nil {
			log.Printf("Failed to load smaps for PID %d: %v", v.PID, err)
		}
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
		log.Printf("Failed to reload processes from database: %v", err)
		// Don't crash the UI, just log the error and continue with empty dataset
		m.ui.SetDataset(data.Dataset{Processes: make(map[int]*data.ProcessData), Order: []int{}})
		return
	}
	ds := data.BuildDataset(procs)
	m.ui.SetDataset(ds)
}
