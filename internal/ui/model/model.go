package model

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"deep_in_memory/internal/constants"
	"deep_in_memory/internal/data"
	uimsg "deep_in_memory/internal/ui/msg"
	uirender "deep_in_memory/internal/ui/render"
)

// EventCounter tracks counts and addresses for a specific event type
type EventCounter struct {
	Count     int
	Addresses map[uint64]bool // Set of addresses for range merging
}

// EventLogger manages periodic logging of eBPF events
type EventLogger struct {
	mu            sync.RWMutex
	lastLogTime   time.Time
	eventCounters map[string]*EventCounter // key: event type (mmap, munmap, etc.)
	logLines      []string                 // Recent log entries for display
}

// Model wraps the Bubble Tea state for the TUI.
type Model struct {
	list             list.Model
	data             data.Dataset
	selectedPID      int // Process selected with Enter (shows details)
	focusedPID       int // Process under cursor (arrow navigation)
	width            int
	height           int
	err              error
	viewportVirtual  viewport.Model
	viewportPhysical viewport.Model
	viewportLog      viewport.Model
	eventLogger      *EventLogger
	setTargetPID     func(int) error
	rangesPage       int // Current page for virtual ranges pagination (0-indexed)
}

// RecordEvent records an eBPF event in the event logger
func (m *Model) RecordEvent(eventType string, address uint64) {
	m.eventLogger.RecordEvent(eventType, address)
}

// NewEventLogger creates a new event logger instance
func NewEventLogger() *EventLogger {
	return &EventLogger{
		eventCounters: make(map[string]*EventCounter),
		logLines:      make([]string, 0),
	}
}

// RecordEvent records an eBPF event for later logging
func (el *EventLogger) RecordEvent(eventType string, address uint64) {
	el.mu.Lock()
	defer el.mu.Unlock()

	if el.eventCounters[eventType] == nil {
		el.eventCounters[eventType] = &EventCounter{
			Count:     0,
			Addresses: make(map[uint64]bool),
		}
	}

	counter := el.eventCounters[eventType]
	counter.Count++
	if address != 0 {
		counter.Addresses[address] = true
	}
}

// FlushLog generates and stores a log entry if enough time has passed
func (el *EventLogger) FlushLog() {
	el.mu.Lock()
	defer el.mu.Unlock()

	now := time.Now()
	if now.Sub(el.lastLogTime) < constants.EventLogIntervalSeconds*time.Second {
		return
	}

	if len(el.eventCounters) == 0 {
		return
	}

	var logEntries []string
	for eventType, counter := range el.eventCounters {
		if counter.Count == 0 {
			continue
		}

		// Generate address ranges
		var ranges []string
		if len(counter.Addresses) > 0 {
			ranges = el.generateAddressRanges(counter.Addresses)
		}

		// Create log entry
		entry := fmt.Sprintf("%s: %d events", eventType, counter.Count)
		if len(ranges) > 0 {
			entry += fmt.Sprintf(" at %s", strings.Join(ranges, ", "))
		}
		logEntries = append(logEntries, entry)
	}

	if len(logEntries) > 0 {
		timestamp := now.Format("15:04:05")
		logLine := fmt.Sprintf("[%s] %s", timestamp, strings.Join(logEntries, "; "))
		el.logLines = append(el.logLines, logLine)

		// Keep only last 50 log lines
		if len(el.logLines) > 50 {
			el.logLines = el.logLines[len(el.logLines)-50:]
		}
	}

	// Reset counters and update timestamp
	el.eventCounters = make(map[string]*EventCounter)
	el.lastLogTime = now
}

// generateAddressRanges converts a set of addresses into merged ranges
func (el *EventLogger) generateAddressRanges(addresses map[uint64]bool) []string {
	if len(addresses) == 0 {
		return nil
	}

	// Convert map to sorted slice
	var addrList []uint64
	for addr := range addresses {
		addrList = append(addrList, addr)
	}
	sort.Slice(addrList, func(i, j int) bool {
		return addrList[i] < addrList[j]
	})

	// Merge consecutive addresses into ranges
	var ranges []string
	start := addrList[0]
	end := addrList[0]

	for i := 1; i < len(addrList); i++ {
		if addrList[i] == end+data.PageSizeBytes {
			// Consecutive address
			end = addrList[i]
		} else {
			// Gap found, add current range
			ranges = append(ranges, el.formatAddressRange(start, end))
			start = addrList[i]
			end = addrList[i]
		}
	}

	// Add final range
	ranges = append(ranges, el.formatAddressRange(start, end))

	return ranges
}

// formatAddressRange formats a start-end address range as hex
func (el *EventLogger) formatAddressRange(start, end uint64) string {
	if start == end {
		return fmt.Sprintf("0x%x", start)
	}
	return fmt.Sprintf("0x%x-0x%x", start, end)
}

// GetLogContent returns formatted log content for display
func (el *EventLogger) GetLogContent() string {
	el.mu.RLock()
	defer el.mu.RUnlock()

	if len(el.logLines) == 0 {
		return "EVENT LOG\nNo events recorded yet\nWaiting for eBPF events..."
	}

	return "Event log (last " + fmt.Sprintf("%d", len(el.logLines)) + " entries):\n" +
		strings.Join(el.logLines, "\n")
}

// New constructs a Model from dataset.
func New(d data.Dataset) *Model {
	m := &Model{
		data:             d,
		selectedPID:      0, // No selection until user presses Enter
		focusedPID:       0,
		viewportVirtual:  viewport.New(0, 0),
		viewportPhysical: viewport.New(0, 0),
		viewportLog:      viewport.New(0, 0),
		eventLogger:      NewEventLogger(),
	}

	items := make([]list.Item, 0, len(d.Order))
	for _, pid := range d.Order {
		items = append(items, renderListItem(d.Processes[pid].Proc, pid))
	}

	delegate := newProcessDelegate(m)
	l := list.New(items, delegate, constants.DefaultListWidth, constants.DefaultListHeight)
	l.Title = "Processes"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.DisableQuitKeybindings()
	m.list = l

	if len(d.Order) > 0 {
		m.focusedPID = d.Order[0]
	}

	return m
}

// SetDataset replaces the underlying dataset and refreshes the list selection if possible.
func (m *Model) SetDataset(d data.Dataset) {
	m.data = d
	m.refreshList()
	m.syncLayout()
}

// SetTargetPIDSetter provides a callback used to push the selected PID into eBPF maps.
func (m *Model) SetTargetPIDSetter(fn func(int) error) {
	m.setTargetPID = fn
}

// Init returns the initial command for the model
func (m *Model) Init() tea.Cmd {
	// Initialize log viewport
	m.updateLogViewport()

	// Start periodic event log flushing - send first message immediately
	return tea.Batch(
		// Immediate flush
		func() tea.Msg { return uimsg.FlushEventLog{} },
		// Then periodic
		tea.Tick(constants.EventLogIntervalSeconds*time.Second, func(time.Time) tea.Msg {
			return uimsg.FlushEventLog{}
		}),
	)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case uimsg.ApplicationError:
		// Set error for display in UI
		m.err = fmt.Errorf("%s: %s", msg.Severity, msg.Message)
		return m, nil
	case uimsg.FlushEventLog:
		// Flush accumulated event logs to display
		beforeCount := len(m.eventLogger.logLines)
		m.eventLogger.FlushLog()
		afterCount := len(m.eventLogger.logLines)
		m.updateLogViewport()

		// Debug: add to log content
		currentContent := m.viewportLog.View()
		if !strings.Contains(currentContent, "DEBUG:") {
			debugInfo := fmt.Sprintf("\nDEBUG: Flush called, lines: %d->%d", beforeCount, afterCount)
			m.viewportLog.SetContent(currentContent + debugInfo)
		}

		return m, tea.Tick(constants.EventLogIntervalSeconds*time.Second, func(time.Time) tea.Msg {
			return uimsg.FlushEventLog{}
		})
	case tea.KeyMsg:
		if handler, ok := m.keyHandlers()[msg.String()]; ok {
			return handler()
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(constants.ListWidthFallback, max(m.height-2, constants.DefaultListHeight))
		m.syncLayout()
	case tea.MouseMsg:
		if msg.Type == tea.MouseWheelUp || msg.Type == tea.MouseWheelDown {
			available, rowHeight, secondRowHeight, _ := m.layoutMetrics()
			_ = available

			if msg.X > m.leftPaneWidth() {
				// Route wheel to the appropriate viewport region; avoid double-handling by list.
				var cmd tea.Cmd
				if msg.Y <= rowHeight {
					m.viewportVirtual, cmd = m.viewportVirtual.Update(msg)
				} else if msg.Y <= rowHeight+secondRowHeight {
					m.viewportPhysical, cmd = m.viewportPhysical.Update(msg)
				}
				return m, cmd
			}
		}
		if msg.Type == tea.MouseLeft && msg.X <= m.leftPaneWidth() {
			const borderOffset = constants.BorderOffset
			const paddingOffset = constants.PaddingOffset
			adj := msg
			adj.X = msg.X - borderOffset - paddingOffset
			adj.Y = msg.Y - borderOffset

			var cmd tea.Cmd
			m.list, cmd = m.list.Update(adj)
			if sel, ok := m.list.SelectedItem().(listItem); ok {
				m.focusedPID = sel.pid
			}
			return m, cmd
		}
	}

	var cmdList tea.Cmd
	m.list, cmdList = m.list.Update(msg)

	// Update focused PID based on cursor position (not selection)
	if sel, ok := m.list.SelectedItem().(listItem); ok {
		m.focusedPID = sel.pid
	}

	return m, cmdList
}

// syncLayout ensures viewport sizes and content stay in sync with current selection and window size.
func (m *Model) syncLayout() {
	if m.width == 0 || m.height == 0 {
		return
	}
	// Only show details if a process is selected (via Enter)
	if m.selectedPID != 0 {
		if pd, ok := m.currentProcessData(); ok {
			m.updateViewports(pd)
		} else {
			// Selected process no longer exists, clear selection
			m.selectedPID = 0
			m.clearViewports()
		}
	} else {
		m.clearViewports()
	}

	// Always update log viewport as it's independent of process selection
	m.updateLogViewport()
}

// leftPaneWidth approximates rendered width of the bordered list box.
func (m *Model) leftPaneWidth() int {
	listWidth := m.list.Width()
	if listWidth == 0 {
		listWidth = constants.ListWidthFallback // default width plus default border padding used by list
	}
	return listWidth + constants.LeftPanePadding
}

// layoutMetrics returns key layout values used for routing events and rendering.
func (m *Model) layoutMetrics() (available, rowHeight, secondRowHeight, barWidth int) {
	available = m.width - m.leftPaneWidth()
	if available < constants.MinAvailableWidth {
		available = constants.MinAvailableWidth
	}
	barWidth = available - constants.BarWidthBuffer
	if barWidth < constants.MinBarWidth {
		barWidth = constants.MinBarWidth
	}

	contentHeight := m.height - constants.StatusBarReduction
	if contentHeight < constants.MinContentHeight {
		contentHeight = constants.MinContentHeight
	}
	// Virtual memory gets VirtualMemoryPercent%, physical memory gets PhysicalMemoryPercent%
	rowHeight = (contentHeight * constants.VirtualMemoryPercent) / 100
	if rowHeight < constants.MinContentHeight {
		rowHeight = constants.MinContentHeight
	}
	secondRowHeight = contentHeight - rowHeight
	if secondRowHeight < constants.MinContentHeight {
		secondRowHeight = constants.MinContentHeight
	}

	return
}

func (m *Model) View() string {
	if m.err != nil {
		return fmt.Sprintf("error: %v", m.err)
	}

	left := m.list.View()
	leftBox := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).Height(m.height - 2).Render(left)
	right := m.renderSplitDetails()

	return lipgloss.JoinHorizontal(lipgloss.Top, leftBox, right)
}

func (m *Model) renderSplitDetails() string {
	available, rowHeight, secondRowHeight, _ := m.layoutMetrics()
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)

	// Virtual memory section (top)
	virtualBox := box.Width(available).Height(rowHeight).Render(m.viewportVirtual.View())

	// Physical memory section (bottom) - split into two halves
	physicalWidth := (available - 1) / 2 // -1 for separator
	if physicalWidth < 10 {
		physicalWidth = available // fallback if too narrow
	}

	physicalLeft := box.Width(physicalWidth).Height(secondRowHeight).Render(m.viewportPhysical.View())

	// Ensure log viewport is updated before rendering
	m.updateLogViewport()
	logContent := m.viewportLog.View()
	if logContent == "" {
		logContent = "[LOG WINDOW]\nEmpty log"
	}
	physicalRight := box.Width(available - physicalWidth - 1).Height(secondRowHeight).Render(logContent)

	// Join left and right physical sections horizontally
	physicalBox := lipgloss.JoinHorizontal(lipgloss.Top, physicalLeft, physicalRight)

	stack := lipgloss.JoinVertical(lipgloss.Left, virtualBox, physicalBox)
	return stack
}

// listItem implements the list.Item interface for Bubble Tea lists.
type listItem struct {
	title       string
	description string
	pid         int
}

func (i listItem) Title() string       { return i.title }
func (i listItem) Description() string { return i.description }
func (i listItem) FilterValue() string { return i.title }

func renderListItem(proc data.Process, pid int) listItem {
	desc := fmt.Sprintf("Start: %s UTC", proc.StartTime.UTC().Format("15:04:05"))
	title := fmt.Sprintf("%s (%d)", proc.Name, proc.PID)
	return listItem{title: title, description: desc, pid: pid}
}

// Custom delegate to show selection and focus markers
type processDelegate struct {
	list.DefaultDelegate
	model *Model
}

func newProcessDelegate(m *Model) *processDelegate {
	d := list.NewDefaultDelegate()
	return &processDelegate{
		DefaultDelegate: d,
		model:           m,
	}
}

func (d *processDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	li, ok := item.(listItem)
	if !ok {
		d.DefaultDelegate.Render(w, m, index, item)
		return
	}

	isSelected := li.pid == d.model.selectedPID
	isCursor := index == m.Index()

	marker := "  "
	titleStyle := lipgloss.NewStyle()
	descStyle := lipgloss.NewStyle().Foreground(constants.DescriptionColor)

	if isSelected {
		marker = "► " // Selected with Enter
		titleStyle = titleStyle.Foreground(constants.SelectedColor).Bold(true)
	}

	if isCursor {
		if !isSelected {
			marker = "• " // Just cursor focus
			titleStyle = titleStyle.Foreground(constants.FocusColor)
		}
		titleStyle = titleStyle.Background(constants.BackgroundColor)
		descStyle = descStyle.Background(constants.BackgroundColor)
	}

	title := titleStyle.Render(marker + li.Title())
	desc := descStyle.Render("  " + li.Description())

	fmt.Fprintf(w, "%s\n%s", title, desc)
}

func (m *Model) refreshList() {
	items := make([]list.Item, 0, len(m.data.Order))
	for _, pid := range m.data.Order {
		if procData, ok := m.data.Processes[pid]; ok {
			items = append(items, renderListItem(procData.Proc, pid))
		}
	}

	// Find indices for focused and selected PIDs
	focusIdx := -1
	for idx, item := range items {
		if li, ok := item.(listItem); ok {
			if li.pid == m.focusedPID {
				focusIdx = idx
				break
			}
		}
	}

	m.list.SetItems(items)

	// Restore cursor to focused PID or default to first
	if focusIdx >= 0 {
		m.list.Select(focusIdx)
	} else if len(items) > 0 {
		m.list.Select(0)
		if li, ok := items[0].(listItem); ok {
			m.focusedPID = li.pid
		}
	}

	// Check if selected process still exists, if not clear selection
	if m.selectedPID != 0 {
		found := false
		for _, item := range items {
			if li, ok := item.(listItem); ok && li.pid == m.selectedPID {
				found = true
				break
			}
		}
		if !found {
			m.selectedPID = 0 // Clear selection if process disappeared
		}
	}
}

// applySelection updates the selected PID and optionally pushes it to the eBPF map.
// applySelection sets the selected process and optionally pushes it to eBPF.
// This triggers detailed tracing for the selected process and updates the UI layout.
func (m *Model) applySelection(pid int, pushToEBPF bool) {
	m.selectedPID = pid
	if pushToEBPF && m.setTargetPID != nil {
		if err := m.setTargetPID(pid); err != nil {
			m.err = err
		}
	}
	m.syncLayout()
}

// ensureSelection maintains the current process selection if the process still exists,
// otherwise falls back to selecting the first available process in the list.
// Returns true if a selection was made, false if the list is empty.
func (m *Model) ensureSelection(items []list.Item) bool {
	if m.selectedPID != 0 {
		for idx, item := range items {
			if li, ok := item.(listItem); ok && li.pid == m.selectedPID {
				m.list.Select(idx)
				return true
			}
		}
	}

	if len(items) > 0 {
		if li, ok := items[0].(listItem); ok {
			m.selectedPID = li.pid
			m.list.Select(0)
			return true
		}
	}

	m.selectedPID = 0
	return false
}

func (m *Model) currentProcessData() (*data.ProcessData, bool) {
	pd, ok := m.data.Processes[m.selectedPID]
	return pd, ok
}

// updateLogViewport updates the log viewport with current event log content
func (m *Model) updateLogViewport() {
	available, _, secondRowHeight, _ := m.layoutMetrics()
	logWidth := (available - 1) / 2
	if logWidth < 10 {
		logWidth = available
	}

	m.viewportLog.Width = logWidth
	m.viewportLog.Height = secondRowHeight - 2
	if m.viewportLog.Height < 3 {
		m.viewportLog.Height = 3
	}

	m.viewportLog.SetContent(m.eventLogger.GetLogContent())
}

// updateViewports calculates viewport dimensions and renders content for both the virtual
// memory pane (top) and physical memory pane (bottom) based on the selected process data.
// It handles layout constraints and pagination for memory range display.
func (m *Model) updateViewports(pd *data.ProcessData) {
	if pd == nil {
		return
	}

	available, rowHeight, secondRowHeight, barWidth := m.layoutMetrics()
	proc := pd.Proc
	virtualCount := uirender.ActiveVirtualCount(pd.Virtual)
	summary := m.renderSummary(proc, pd, virtualCount)

	virtualWidth := available - constants.LeftPanePadding
	if virtualWidth < constants.MinBarWidth {
		virtualWidth = constants.MinBarWidth
	}
	physicalWidth := virtualWidth

	virtualHeight := rowHeight - 2
	if virtualHeight < constants.MinViewportHeight {
		virtualHeight = constants.MinViewportHeight
	}
	physicalHeight := secondRowHeight - 2
	if physicalHeight < constants.MinViewportHeight {
		physicalHeight = constants.MinViewportHeight
	}

	m.viewportVirtual.Width = virtualWidth
	m.viewportVirtual.Height = virtualHeight
	m.viewportPhysical.Width = physicalWidth
	m.viewportPhysical.Height = physicalHeight

	m.viewportVirtual.SetContent(uirender.RenderVirtualSectionContent(summary, pd, barWidth, m.rangesPage, virtualWidth))
	m.viewportPhysical.SetContent(uirender.RenderPhysicalSectionContent(pd))
}

func (m *Model) clearViewports() {
	m.viewportVirtual.SetContent("No process selected.\nPress Enter on a process to view details.")
	m.viewportPhysical.SetContent("")
}

// renderSummary creates a formatted summary header showing process information
// and memory statistics including reservation, allocation, page-out, and page-in counts.
// Returns a styled table with color-coded columns for different memory statuses.
func (m *Model) renderSummary(proc data.Process, pd *data.ProcessData, virtualCount int) string {
	startUTC := proc.StartTime.UTC().Format(time.RFC3339)
	stats := uirender.CalculateStatusStats(pd)

	// Build header
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Process: %s (%d)\n", proc.Name, proc.PID))
	sb.WriteString(fmt.Sprintf("Start (UTC): %s\n\n", startUTC))

	// Use colors from constants package

	// Define column widths
	labelWidth := 8
	col1Width := 36
	col2Width := 30
	col3Width := 24
	col4Width := 24

	// Create styles
	borderStyle := lipgloss.NewStyle().Border(lipgloss.NormalBorder())
	labelStyle := lipgloss.NewStyle().Width(labelWidth)
	col1Style := lipgloss.NewStyle().Width(col1Width).Foreground(lipgloss.Color(constants.GrayColor))
	col2Style := lipgloss.NewStyle().Width(col2Width).Foreground(lipgloss.Color(constants.GreenColor))
	col3Style := lipgloss.NewStyle().Width(col3Width).Foreground(lipgloss.Color(constants.RedColor))
	col4Style := lipgloss.NewStyle().Width(col4Width).Foreground(lipgloss.Color(constants.BlueColor))

	// Build table rows
	headerRow := lipgloss.JoinHorizontal(lipgloss.Top,
		labelStyle.Render(""),
		col1Style.Render(constants.StatusReservation),
		col2Style.Render(constants.StatusAllocation),
		col3Style.Render(constants.StatusPageOut),
		col4Style.Render(constants.StatusPageIn),
	)

	pagesRow := lipgloss.JoinHorizontal(lipgloss.Top,
		labelStyle.Render("pages"),
		col1Style.Render(fmt.Sprintf("%d", stats.Reservation)),
		col2Style.Render(fmt.Sprintf("%d", stats.Allocation)),
		col3Style.Render(fmt.Sprintf("%d", stats.PageOut)),
		col4Style.Render(fmt.Sprintf("%d", stats.PageIn)),
	)

	reservationBytes := stats.Reservation * data.PageSizeBytes
	allocationBytes := stats.Allocation * data.PageSizeBytes
	pageOutBytes := stats.PageOut * data.PageSizeBytes
	pageInBytes := stats.PageIn * data.PageSizeBytes

	sizeRow := lipgloss.JoinHorizontal(lipgloss.Top,
		labelStyle.Render("size"),
		col1Style.Render(uirender.HumanBytes(int64(reservationBytes))),
		col2Style.Render(uirender.HumanBytes(int64(allocationBytes))),
		col3Style.Render(uirender.HumanBytes(int64(pageOutBytes))),
		col4Style.Render(uirender.HumanBytes(int64(pageInBytes))),
	)

	// Join all rows vertically
	table := lipgloss.JoinVertical(lipgloss.Left, headerRow, pagesRow, sizeRow)

	// Apply border to the entire table
	borderedTable := borderStyle.Render(table)

	sb.WriteString(borderedTable)
	sb.WriteString("\n")

	return sb.String()
}

// keyHandlers returns a map of keyboard shortcuts to their corresponding actions.
// Supports navigation (arrows), selection (Enter), pagination (n/p), and quitting (q/Ctrl+C).
func (m *Model) keyHandlers() map[string]func() (tea.Model, tea.Cmd) {
	return map[string]func() (tea.Model, tea.Cmd){
		"q":      func() (tea.Model, tea.Cmd) { return m, tea.Quit },
		"ctrl+c": func() (tea.Model, tea.Cmd) { return m, tea.Quit },
		"enter": func() (tea.Model, tea.Cmd) {
			if sel, ok := m.list.SelectedItem().(listItem); ok {
				// Enter selects the focused process
				m.selectedPID = sel.pid
				m.rangesPage = 0 // Reset to first page when selecting new process
				if m.setTargetPID != nil {
					if err := m.setTargetPID(sel.pid); err != nil {
						m.err = err
					}
				}
				m.syncLayout()

				// Send message to load smaps for this process
				return m, func() tea.Msg {
					return uimsg.LoadSmapsRequest{PID: sel.pid}
				}
			}
			return m, nil
		},
		"n": func() (tea.Model, tea.Cmd) {
			// Next page of virtual ranges
			if m.selectedPID != 0 {
				m.rangesPage++
				m.syncLayout()
			}
			return m, nil
		},
		"p": func() (tea.Model, tea.Cmd) {
			// Previous page of virtual ranges
			if m.selectedPID != 0 && m.rangesPage > 0 {
				m.rangesPage--
				m.syncLayout()
			}
			return m, nil
		},
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
