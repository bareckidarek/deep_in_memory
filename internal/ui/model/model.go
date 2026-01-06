package model

import (
	"fmt"
	"io"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"deep_in_memory/internal/data"
	uirender "deep_in_memory/internal/ui/render"
)

const (
	defaultListWidth   = 30
	defaultListHeight  = 20
	leftPanePadding    = 4 // border (2) + padding (2)
	listWidthFallback  = defaultListWidth + 2
	minAvailableWidth  = 20
	minBarWidth        = 10
	minContentHeight   = 6
	minViewportHeight  = 3
	statusBarReduction = 4
	barWidthBuffer     = 50
)

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
	setTargetPID     func(int) error
}

// New constructs a Model from dataset.
func New(d data.Dataset) *Model {
	m := &Model{
		data:             d,
		selectedPID:      0, // No selection until user presses Enter
		focusedPID:       0,
		viewportVirtual:  viewport.New(0, 0),
		viewportPhysical: viewport.New(0, 0),
	}

	items := make([]list.Item, 0, len(d.Order))
	for _, pid := range d.Order {
		items = append(items, renderListItem(d.Processes[pid].Proc, pid))
	}

	delegate := newProcessDelegate(m)
	l := list.New(items, delegate, defaultListWidth, defaultListHeight)
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

func (m *Model) Init() tea.Cmd { return nil }

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if handler, ok := m.keyHandlers()[msg.String()]; ok {
			return handler()
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(listWidthFallback, max(m.height-2, defaultListHeight))
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
			const borderOffset = 1
			const paddingOffset = 1
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
}

// leftPaneWidth approximates rendered width of the bordered list box.
func (m *Model) leftPaneWidth() int {
	listWidth := m.list.Width()
	if listWidth == 0 {
		listWidth = listWidthFallback // default width plus default border padding used by list
	}
	return listWidth + leftPanePadding
}

// layoutMetrics returns key layout values used for routing events and rendering.
func (m *Model) layoutMetrics() (available, rowHeight, secondRowHeight, barWidth int) {
	available = m.width - m.leftPaneWidth()
	if available < minAvailableWidth {
		available = minAvailableWidth
	}
	barWidth = available - barWidthBuffer
	if barWidth < minBarWidth {
		barWidth = minBarWidth
	}

	contentHeight := m.height - statusBarReduction
	if contentHeight < minContentHeight {
		contentHeight = minContentHeight
	}
	rowHeight = contentHeight / 2
	if rowHeight < minContentHeight {
		rowHeight = minContentHeight
	}
	secondRowHeight = contentHeight - rowHeight
	if secondRowHeight < minContentHeight {
		secondRowHeight = minContentHeight
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
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).Width(available)
	virtualBox := box.Height(rowHeight).Render(m.viewportVirtual.View())
	physicalBox := box.Height(secondRowHeight).Render(m.viewportPhysical.View())

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
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	if isSelected {
		marker = "► "                                                       // Selected with Enter
		titleStyle = titleStyle.Foreground(lipgloss.Color("10")).Bold(true) // Green + bold
	}

	if isCursor {
		if !isSelected {
			marker = "• "                                            // Just cursor focus
			titleStyle = titleStyle.Foreground(lipgloss.Color("12")) // Blue
		}
		titleStyle = titleStyle.Background(lipgloss.Color("236")) // Dark background
		descStyle = descStyle.Background(lipgloss.Color("236"))
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
func (m *Model) applySelection(pid int, pushToEBPF bool) {
	m.selectedPID = pid
	if pushToEBPF && m.setTargetPID != nil {
		if err := m.setTargetPID(pid); err != nil {
			m.err = err
		}
	}
	m.syncLayout()
}

// ensureSelection keeps current selection if available or falls back to the first item.
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

// updateViewports recalculates sizes and content for both panes based on current layout.
func (m *Model) updateViewports(pd *data.ProcessData) {
	if pd == nil {
		return
	}

	available, rowHeight, secondRowHeight, barWidth := m.layoutMetrics()
	proc := pd.Proc
	virtualCount := uirender.ActiveVirtualCount(pd.Virtual)
	summary := m.renderSummary(proc, pd, virtualCount)

	virtualWidth := available - leftPanePadding
	if virtualWidth < minBarWidth {
		virtualWidth = minBarWidth
	}
	physicalWidth := virtualWidth

	virtualHeight := rowHeight - 2
	if virtualHeight < minViewportHeight {
		virtualHeight = minViewportHeight
	}
	physicalHeight := secondRowHeight - 2
	if physicalHeight < minViewportHeight {
		physicalHeight = minViewportHeight
	}

	m.viewportVirtual.Width = virtualWidth
	m.viewportVirtual.Height = virtualHeight
	m.viewportPhysical.Width = physicalWidth
	m.viewportPhysical.Height = physicalHeight

	m.viewportVirtual.SetContent(uirender.RenderVirtualSectionContent(summary, pd, barWidth))
	m.viewportPhysical.SetContent(uirender.RenderPhysicalSectionContent(pd))
}

func (m *Model) clearViewports() {
	m.viewportVirtual.SetContent("No process selected.\nPress Enter on a process to view details.")
	m.viewportPhysical.SetContent("")
}

func (m *Model) renderSummary(proc data.Process, _ *data.ProcessData, virtualCount int) string {
	startUTC := proc.StartTime.UTC().Format(time.RFC3339)
	return fmt.Sprintf("Process: %s (%d)\nStart (UTC): %s\nVirtual pages: %d\nVirtual usage: %s\n\nPhysical Memory:\n(coming soon)\n",
		proc.Name, proc.PID,
		startUTC,
		virtualCount,
		uirender.HumanBytes(int64(virtualCount)*data.PageSizeBytes),
	)
}

func (m *Model) keyHandlers() map[string]func() (tea.Model, tea.Cmd) {
	return map[string]func() (tea.Model, tea.Cmd){
		"q":      func() (tea.Model, tea.Cmd) { return m, tea.Quit },
		"ctrl+c": func() (tea.Model, tea.Cmd) { return m, tea.Quit },
		"enter": func() (tea.Model, tea.Cmd) {
			if sel, ok := m.list.SelectedItem().(listItem); ok {
				// Enter selects the focused process
				m.selectedPID = sel.pid
				if m.setTargetPID != nil {
					if err := m.setTargetPID(sel.pid); err != nil {
						m.err = err
					}
				}
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
