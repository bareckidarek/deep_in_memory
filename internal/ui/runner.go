package ui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"deep_in_memory/internal/app"
	"deep_in_memory/internal/constants"
	"deep_in_memory/internal/data"
	"deep_in_memory/internal/persistence"
	uimodel "deep_in_memory/internal/ui/model"
	uimsg "deep_in_memory/internal/ui/msg"
)

// refreshMsg triggers a dataset reload from the DB.
type refreshMsg struct{}

// UIRunner manages the UI and reads data from persistent storage
type UIRunner struct {
	model           *uimodel.Model
	reader          persistence.Reader
	refreshInterval time.Duration
	logAggregator   *app.LogAggregator
	targetPIDSetter func(int) error
}

// RunUIConfig holds configuration for the UI runner
type RunUIConfig struct {
	Reader          persistence.Reader
	RefreshInterval time.Duration
	LogAggregator   *app.LogAggregator // Shared with collector
	TargetPIDSetter func(int) error    // Function to set target PID in collector
}

// NewUIRunner creates a new UI runner that reads from persistence layer
func NewUIRunner(cfg RunUIConfig) (*UIRunner, error) {
	// Load initial dataset
	procs, err := cfg.Reader.AllProcessData(context.Background())
	if err != nil {
		// Start with empty dataset on error
		procs = []data.ProcessData{}
	}
	initial := data.BuildDataset(procs)

	runner := &UIRunner{
		model:           uimodel.New(initial),
		reader:          cfg.Reader,
		refreshInterval: cfg.RefreshInterval,
		logAggregator:   cfg.LogAggregator,
		targetPIDSetter: cfg.TargetPIDSetter,
	}

	// Set target PID setter in UI model
	if cfg.TargetPIDSetter != nil {
		runner.model.SetTargetPIDSetter(cfg.TargetPIDSetter)
	}

	return runner, nil
}

// Init starts periodic refresh and initializes UI
func (r *UIRunner) Init() tea.Cmd {
	return tea.Batch(
		tea.Tick(r.refreshInterval, func(time.Time) tea.Msg { return refreshMsg{} }),
		r.model.Init(),
		// Start log request timer
		tea.Tick(constants.LogFlushInterval, func(time.Time) tea.Msg {
			return uimsg.GetLogsMsg{}
		}),
	)
}

// Update processes incoming messages (primarily refresh and UI events)
func (r *UIRunner) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case uimsg.GetLogsMsg:
		// Get formatted logs from aggregator
		var logs []string
		if r.logAggregator != nil {
			logs = r.logAggregator.GetFormattedLogs()
		}
		// Send logs response to UI and schedule next log request
		updated, cmd := r.model.Update(uimsg.LogsResponseMsg{Logs: logs})
		if ui, ok := updated.(*uimodel.Model); ok {
			r.model = ui
		}
		// Schedule next GetLogsMsg
		nextCmd := tea.Tick(constants.LogFlushInterval, func(t time.Time) tea.Msg {
			return uimsg.GetLogsMsg{}
		})
		return r, tea.Batch(cmd, nextCmd)

	case uimsg.LoadSmapsRequest:
		// Load smaps data for the process - read from /proc directly
		// This is a read-only operation, no need to involve collector
		// Just forward to model which handles it
		updated, cmd := r.model.Update(v)
		if ui, ok := updated.(*uimodel.Model); ok {
			r.model = ui
		}
		return r, cmd

	case refreshMsg:
		r.reload()
		return r, tea.Tick(r.refreshInterval, func(time.Time) tea.Msg { return refreshMsg{} })
	}

	// Forward all other messages to UI model
	updated, cmd := r.model.Update(msg)
	if ui, ok := updated.(*uimodel.Model); ok {
		r.model = ui
	}
	return r, cmd
}

// View delegates to the UI model
func (r *UIRunner) View() tea.View {
	return r.model.View()
}

// reload fetches fresh data from persistent storage and updates the UI
func (r *UIRunner) reload() {
	procs, err := r.reader.AllProcessData(context.Background())
	if err != nil {
		// Don't crash the UI, just continue with empty dataset
		r.model.SetDataset(data.Dataset{Processes: make(map[int]*data.ProcessData), Order: []int{}})
		return
	}
	ds := data.BuildDataset(procs)
	r.model.SetDataset(ds)
}

// GetModel returns the underlying UI model (for compatibility)
func (r *UIRunner) GetModel() *uimodel.Model {
	return r.model
}

// SetErrorCallback sets error callback for the UI
func (r *UIRunner) SetErrorCallback(callback func(error)) {
	// UI runner itself doesn't have error callback, but we can forward errors to UI
	// This is mainly for compatibility with existing code
}
