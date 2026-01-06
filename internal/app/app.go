package app

import (
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/timshannon/bolthold"

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
	refreshInterval time.Duration
}

// New constructs the app model with the provided DB, initial dataset, and refresh interval.
func New(db storedb.Db[data.ProcessData], initial data.Dataset, refresh time.Duration) *Model {
	return &Model{
		ui:              uimodel.New(initial),
		db:              db,
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
	case uimsg.ProcessFork:
		_ = m.persistFork(v)
		return m, nil
	case uimsg.ProcessExit:
		_ = m.handleExit(v)
		return m, nil
	case uimsg.ProcessMmap:
		_ = m.handleMmap(v)
		return m, nil
	case uimsg.ProcessMunmap:
		_ = m.handleMunmap(v)
		return m, nil
	case uimsg.ProcessPageFault:
		_ = m.handlePageFault(v)
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
		return
	}
	ds := data.BuildDataset(procs)
	m.ui.SetDataset(ds)
}

func (m *Model) persistFork(ev uimsg.ProcessFork) error {
	pid := strconv.Itoa(ev.ChildPID)
	record := data.ProcessData{
		Proc:    data.Process{PID: ev.ChildPID, Name: ev.Comm, StartTime: ev.Timestamp.UTC()},
		Virtual: make(map[string]*data.VirtualPage),
		PFNs:    make(map[string]*data.PFNState),
		Threads: []data.ThreadData{},
	}
	return m.db.Upsert(pid, &record)
}

func (m *Model) handleExit(ev uimsg.ProcessExit) error {
	key := strconv.Itoa(ev.PID)
	return m.db.Delete(key)
}

func (m *Model) handleMmap(ev uimsg.ProcessMmap) error {
	// Retrieve existing record
	key := strconv.Itoa(ev.PID)
	records, err := m.db.Query(bolthold.Where("Proc.PID").Eq(ev.PID))
	if err != nil || len(records) == 0 {
		// Process not found, create it
		record := &data.ProcessData{
			Proc:    data.Process{PID: ev.PID, Name: ev.Comm, StartTime: ev.Timestamp.UTC()},
			Virtual: make(map[string]*data.VirtualPage),
			PFNs:    make(map[string]*data.PFNState),
			Threads: []data.ThreadData{},
		}
		record.AddMmapRange(ev.TID, ev.Addr, ev.Len)
		return m.db.Upsert(key, record)
	}

	// Update existing record
	record := &records[0]
	record.AddMmapRange(ev.TID, ev.Addr, ev.Len)
	return m.db.Upsert(key, record)
}

func (m *Model) handleMunmap(ev uimsg.ProcessMunmap) error {
	// Retrieve existing record
	key := strconv.Itoa(ev.PID)
	records, err := m.db.Query(bolthold.Where("Proc.PID").Eq(ev.PID))
	if err != nil || len(records) == 0 {
		// Process not found, nothing to unmap
		return nil
	}

	// Update existing record
	record := &records[0]
	record.RemoveMmapRange(ev.TID, ev.Addr, ev.Len)
	return m.db.Upsert(key, record)
}

func (m *Model) handlePageFault(ev uimsg.ProcessPageFault) error {

	// Retrieve existing record
	key := strconv.Itoa(ev.PID)
	records, err := m.db.Query(bolthold.Where("Proc.PID").Eq(ev.PID))
	if err != nil || len(records) == 0 {
		return nil
	}

	// Update existing record
	record := &records[0]
	record.AddPageFault(ev.TID, ev.Address)

	return m.db.Upsert(key, record)
}
