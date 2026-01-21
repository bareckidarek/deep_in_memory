package app

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"deep_in_memory/internal/constants"
	"deep_in_memory/internal/data"
	"deep_in_memory/internal/persistence"
	uimodel "deep_in_memory/internal/ui/model"
	uimsg "deep_in_memory/internal/ui/msg"
)

// refreshMsg triggers a dataset reload from the DB.
type refreshMsg struct{}

// Model wraps the UI model with periodic reloads from persistence.Reader.
type Model struct {
	ui              *uimodel.Model
	reader          persistence.Reader
	refreshInterval time.Duration
	logAggregator   *LogAggregator
	targetPID       int // Currently monitored PID
}

// New creates a lightweight app Model that only manages UI state and reloads
// datasets from the provided persistence.Reader. eBPF and DB write logic
// should live in `internal/collector` and `internal/persistence` respectively.
func New(reader persistence.Reader, initial data.Dataset, refresh time.Duration, logAgg *LogAggregator) *Model {
	return &Model{
		ui:              uimodel.New(initial),
		reader:          reader,
		refreshInterval: refresh,
		logAggregator:   logAgg,
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
// SetErrorCallback is a no-op for the lightweight app Model; error callbacks
// should be provided to the collector or persistence layers.
func (m *Model) SetErrorCallback(callback func(error)) {}

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
		if m.targetPID != 0 && m.logAggregator != nil {
			m.logAggregator.Flush(m.targetPID)
		}
		var logs []string
		if m.logAggregator != nil {
			logs = m.logAggregator.GetFormattedLogs()
		}
		// Send logs response to UI and schedule next log request
		updated, cmd := m.ui.Update(uimsg.LogsResponseMsg{Logs: logs})
		if ui, ok := updated.(*uimodel.Model); ok {
			m.ui = ui
		}
		nextCmd := tea.Tick(constants.LogFlushInterval, func(t time.Time) tea.Msg { return uimsg.GetLogsMsg{} })
		return m, tea.Batch(cmd, nextCmd)
	case uimsg.LoadSmapsRequest:
		// UI handles smaps loading itself (read-only)
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
	if m.reader == nil {
		m.ui.SetDataset(data.Dataset{Processes: make(map[int]*data.ProcessData), Order: []int{}})
		return
	}
	procs, err := m.reader.AllProcessData(context.Background())
	if err != nil {
		// Don't crash the UI, just continue with empty dataset
		m.ui.SetDataset(data.Dataset{Processes: make(map[int]*data.ProcessData), Order: []int{}})
		return
	}
	ds := data.BuildDataset(procs)
	m.ui.SetDataset(ds)
}
