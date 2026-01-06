package model

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"deep_in_memory/internal/data"
	uirender "deep_in_memory/internal/ui/render"
)

// Model wraps the Bubble Tea state for the TUI.
type Model struct {
	list             list.Model
	data             data.Dataset
	selectedPID      int
	width            int
	height           int
	err              error
	viewportVirtual  viewport.Model
	viewportPhysical viewport.Model
	setTargetPID     func(int) error
}

// New constructs a Model from dataset.
func New(d data.Dataset) *Model {
	items := make([]list.Item, 0, len(d.Order))
	for _, pid := range d.Order {
		items = append(items, renderListItem(d.Processes[pid].Proc))
	}

	l := list.New(items, list.NewDefaultDelegate(), 30, 20)
	l.Title = "Processes"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.DisableQuitKeybindings()

	m := &Model{
		list:             l,
		data:             d,
		selectedPID:      0,
		viewportVirtual:  viewport.New(0, 0),
		viewportPhysical: viewport.New(0, 0),
	}

	if len(d.Order) > 0 {
		m.selectedPID = d.Order[0]
	}

	return m
}

// SetDataset replaces the underlying dataset and refreshes the list selection if possible.
func (m *Model) SetDataset(d data.Dataset) {
	m.data = d
	m.refreshList()
	m.recomputeLayout()
}

// SetTargetPIDSetter provides a callback used to push the selected PID into eBPF maps.
func (m *Model) SetTargetPIDSetter(fn func(int) error) {
	m.setTargetPID = fn
}

func (m *Model) Init() tea.Cmd { return nil }

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "enter":
			if sel, ok := m.list.SelectedItem().(listItem); ok {
				m.selectedPID = sel.pid
				if m.setTargetPID != nil {
					if err := m.setTargetPID(sel.pid); err != nil {
						m.err = err
					}
				}
				m.recomputeLayout()
			}
			return m, nil
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(32, m.height-2)
		m.recomputeLayout()
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
				m.selectedPID = sel.pid
				m.recomputeLayout()
			}
			return m, cmd
		}
	}

	var cmdList tea.Cmd
	m.list, cmdList = m.list.Update(msg)

	return m, cmdList
}

// recomputeLayout ensures viewport sizes and content stay in sync with current selection and window size.
func (m *Model) recomputeLayout() {
	if m.width == 0 || m.height == 0 {
		return
	}
	_ = m.renderSplitDetails() // side-effects update viewport sizes/content
}

// leftPaneWidth approximates rendered width of the bordered list box.
func (m *Model) leftPaneWidth() int {
	listWidth := m.list.Width()
	if listWidth == 0 {
		listWidth = 32
	}
	return listWidth + 4 // border (2) + padding (2)
}

// layoutMetrics returns key layout values used for routing events and rendering.
func (m *Model) layoutMetrics() (available, rowHeight, secondRowHeight, barWidth int) {
	available = m.width - m.leftPaneWidth()
	if available < 20 {
		available = 20
	}
	barWidth = available - 50
	if barWidth < 10 {
		barWidth = 10
	}

	contentHeight := m.height - 4
	if contentHeight < 6 {
		contentHeight = 6
	}
	rowHeight = contentHeight / 2
	if rowHeight < 6 {
		rowHeight = 6
	}
	secondRowHeight = contentHeight - rowHeight
	if secondRowHeight < 6 {
		secondRowHeight = 6
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
	pd, ok := m.data.Processes[m.selectedPID]
	if !ok {
		return "Select a process to view details"
	}

	proc := pd.Proc

	available, rowHeight, secondRowHeight, barWidth := m.layoutMetrics()

	startUTC := proc.StartTime.UTC().Format(time.RFC3339)
	summary := fmt.Sprintf("Process: %s (%d)\nStart (UTC): %s\nEvents: %d\nVirtual pages: %d\nPFNs: %d\nVirtual usage: %s\n",
		proc.Name, proc.PID,
		startUTC,
		len(pd.Events),
		uirender.ActiveVirtualCount(pd.Virtual),
		len(pd.PFNs),
		uirender.HumanBytes(int64(uirender.ActiveVirtualCount(pd.Virtual))*data.PageSizeBytes),
	)

	// Update viewport sizes based on available dimensions and padding/border (border=2 width, padding=2 width total).
	virtualWidth := available - 4
	if virtualWidth < 10 {
		virtualWidth = 10
	}
	physicalWidth := virtualWidth

	virtualHeight := rowHeight - 2
	if virtualHeight < 3 {
		virtualHeight = 3
	}
	physicalHeight := secondRowHeight - 2
	if physicalHeight < 3 {
		physicalHeight = 3
	}

	m.viewportVirtual.Width = virtualWidth
	m.viewportVirtual.Height = virtualHeight
	m.viewportPhysical.Width = physicalWidth
	m.viewportPhysical.Height = physicalHeight

	m.viewportVirtual.SetContent(uirender.RenderVirtualSectionContent(summary, pd, barWidth))
	m.viewportPhysical.SetContent(uirender.RenderPhysicalSectionContent(pd))

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

func renderListItem(proc data.Process) listItem {
	desc := fmt.Sprintf("Start: %s UTC", proc.StartTime.UTC().Format("15:04:05"))
	return listItem{title: fmt.Sprintf("%s (%d)", proc.Name, proc.PID), description: desc, pid: proc.PID}
}

func (m *Model) refreshList() {
	items := make([]list.Item, 0, len(m.data.Order))
	for _, pid := range m.data.Order {
		if procData, ok := m.data.Processes[pid]; ok {
			items = append(items, renderListItem(procData.Proc))
		}
	}
	m.list.SetItems(items)
	// Keep selection stable if possible; otherwise choose the first item.
	if m.selectedPID != 0 {
		for idx, item := range items {
			if li, ok := item.(listItem); ok && li.pid == m.selectedPID {
				m.list.Select(idx)
				return
			}
		}
	}
	if len(items) > 0 {
		if li, ok := items[0].(listItem); ok {
			m.selectedPID = li.pid
			m.list.Select(0)
		}
	} else {
		m.selectedPID = 0
	}
}
