package model

import (
	"fmt"
	"io"
	"strings"
	"time"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"deep_in_memory/internal/constants"
	"deep_in_memory/internal/data"
	uimsg "deep_in_memory/internal/ui/msg"
	uirender "deep_in_memory/internal/ui/render"
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
	viewportLogs     viewport.Model
	setTargetPID     func(int) error
	rangesPage       int // Current page for virtual ranges pagination (0-indexed)

	// Modal state
	showModal    bool
	modalType    string // info, warn, error
	modalMessage string
}

// New constructs a Model from dataset.
func New(d data.Dataset) *Model {
	m := &Model{
		data:             d,
		selectedPID:      0, // No selection until user presses Enter
		focusedPID:       0,
		viewportVirtual:  viewport.New(),
		viewportPhysical: viewport.New(),
		viewportLogs:     viewport.New(),
	}

	// Initialize logs viewport with default content
	m.viewportLogs.SetContent("[logs]\n\nNo events recorded")

	items := make([]list.Item, 0, len(d.Order))
	for _, pid := range d.Order {
		items = append(items, renderListItem(d.Processes[pid].Proc, pid))
	}

	delegate := newProcessDelegate(m)
	l := list.New(items, delegate, constants.DefaultListWidth, constants.DefaultListHeight)
	l.Title = "[processes]"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.DisableQuitKeybindings()

	// Set title style to match other section headers
	l.Styles.Title = lipgloss.NewStyle().Background(lipgloss.Color("62")).Foreground(lipgloss.Color("230")).Bold(true)

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

func (m *Model) Init() tea.Cmd {
	return nil // App handles log request timing
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case uimsg.LogsResponseMsg:
		// Update logs viewport with received logs
		if len(msg.Logs) == 0 {
			styledTitle := uirender.StyleTitle("[logs]")
			m.viewportLogs.SetContent(styledTitle + "\n\nNo events recorded")
		} else {
			var lines []string
			lines = append(lines, uirender.StyleTitle("[logs]"))
			lines = append(lines, "") // Empty line after header

			// Word wrap each log line to viewport width
			for _, log := range msg.Logs {
				// Wrap line if it exceeds viewport width
				if m.viewportLogs.Width() > 0 {
					wrapped := ansi.Wordwrap(log, m.viewportLogs.Width(), "")
					lines = append(lines, wrapped)
				} else {
					lines = append(lines, log)
				}
			}

			m.viewportLogs.SetContent(strings.Join(lines, "\n"))
		}
		return m, nil
	case uimsg.ApplicationError:
		// Show modal based on severity
		m.showModal = true
		switch msg.Severity {
		case "error":
			m.modalType = constants.ModalTypeError
		case "warning", "warn":
			m.modalType = constants.ModalTypeWarn
		default:
			m.modalType = constants.ModalTypeInfo
		}
		m.modalMessage = msg.Message

		// Hide modal after configured duration
		return m, tea.Tick(constants.ModalDisplayDuration, func(time.Time) tea.Msg {
			return uimsg.HideModalMsg{}
		})
	case uimsg.HideModalMsg:
		m.showModal = false
		return m, nil
	case tea.KeyPressMsg:
		if handler, ok := m.keyHandlers()[msg.String()]; ok {
			return handler()
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(constants.ListWidthFallback, max(m.height-2, constants.DefaultListHeight))
		m.syncLayout()
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

	contentHeight := m.height
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

func (m *Model) View() tea.View {
	var content string
	if m.err != nil {
		content = fmt.Sprintf("error: %v", m.err)
	} else {
		left := m.list.View()
		leftBox := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).Width(m.leftPaneWidth()).Height(m.height - 2).Render(left)
		right := m.renderSplitDetails()
		content = lipgloss.JoinHorizontal(lipgloss.Top, leftBox, right)
	}

	// If modal is active, overlay it on top of the content
	if m.showModal {
		content = m.renderWithModal(content)
	}

	view := tea.NewView(content)
	view.AltScreen = true
	return view
}

// renderWithModal overlays a modal/notification dialog on top of the main content.
// The modal is centered on screen and displays over the application without hiding it.
// This is used for temporary notifications like process selections, errors, etc.
func (m *Model) renderWithModal(baseContent string) string {
	// Create modal box styled based on notification type
	var bgColor string
	switch m.modalType {
	case constants.ModalTypeInfo:
		bgColor = "12" // Blue
	case constants.ModalTypeWarn:
		bgColor = "214" // Orange
	case constants.ModalTypeError:
		bgColor = "9" // Red
	default:
		bgColor = "12" // Default to blue
	}

	// Create 3-line modal with text in the middle line
	// Width: message + 5 columns padding on each side
	horizontalPadding := 5

	// Build modal content with 3 lines: empty, text, empty
	textWidth := lipgloss.Width(m.modalMessage)
	totalWidth := textWidth + (horizontalPadding * 2)

	emptyLine := strings.Repeat(" ", totalWidth)
	centeredText := strings.Repeat(" ", horizontalPadding) + m.modalMessage + strings.Repeat(" ", horizontalPadding)

	modalLines := []string{
		emptyLine,
		centeredText,
		emptyLine,
	}

	// Apply styling to each line with black foreground
	modalStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(bgColor)).
		Foreground(lipgloss.Color("#000000")). // Black text (hex code)
		Bold(true)

	var styledLines []string
	for _, line := range modalLines {
		styledLines = append(styledLines, modalStyle.Render(line))
	}

	modalHeight := len(styledLines)
	modalWidth := lipgloss.Width(styledLines[0])

	// Split base content into lines
	baseLines := strings.Split(baseContent, "\n")
	if len(baseLines) > m.height {
		baseLines = baseLines[:m.height]
	}

	// Calculate modal position (2 lines from top)
	modalY := constants.ModalTopOffset
	if modalY < 0 {
		modalY = 0
	}

	modalX := (m.width - modalWidth) / 2
	if modalX < 0 {
		modalX = 0
	}

	// Build output line by line, overlaying modal preserving ANSI codes
	var outputLines []string

	for i := 0; i < m.height; i++ {
		var baseLine string
		if i < len(baseLines) {
			baseLine = baseLines[i]
		} else {
			baseLine = strings.Repeat(" ", m.width)
		}

		// Ensure base line has proper width (handle ANSI codes)
		visualWidth := ansi.StringWidth(baseLine)
		if visualWidth < m.width {
			baseLine += strings.Repeat(" ", m.width-visualWidth)
		}

		// Check if this line intersects with modal
		if i >= modalY && i < modalY+modalHeight {
			modalLineIdx := i - modalY
			modalLine := styledLines[modalLineIdx]
			modalLineWidth := lipgloss.Width(modalLine)

			// Extract left part (before modal) - use Truncate to keep ANSI codes
			leftPart := ""
			if modalX > 0 {
				leftPart = ansi.Truncate(baseLine, modalX, "")
			}

			// Extract right part (after modal) - need to skip first (modalX + modalLineWidth) characters
			rightPart := ""
			skipChars := modalX + modalLineWidth
			if skipChars < visualWidth {
				// We need everything after skipChars position
				// Truncate from left by removing first skipChars characters
				fullWidth := ansi.StringWidth(baseLine)
				charsToKeep := fullWidth - skipChars
				if charsToKeep > 0 {
					rightPart = ansi.TruncateLeft(baseLine, skipChars, "")
				}
			}

			outputLines = append(outputLines, leftPart+modalLine+rightPart)
		} else {
			outputLines = append(outputLines, baseLine)
		}
	}

	return strings.Join(outputLines, "\n")
}

func (m *Model) renderSplitDetails() string {
	available, rowHeight, secondRowHeight, _ := m.layoutMetrics()

	// Virtual memory box (top) - subtract 2 for border
	virtualBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1).
		Width(available).
		Height(rowHeight - 2).
		Render(m.viewportVirtual.View())

	// Split second row horizontally: 50% physical memory, 50% logs
	halfWidth := available / 2

	// Physical and logs boxes - subtract 2 for border
	physicalBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1).
		Width(halfWidth).
		Height(secondRowHeight - 2).
		Render(m.viewportPhysical.View())

	logsBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1).
		Width(available - halfWidth).
		Height(secondRowHeight - 2).
		Render(m.viewportLogs.View())

	secondRow := lipgloss.JoinHorizontal(lipgloss.Top, physicalBox, logsBox)
	stack := lipgloss.JoinVertical(lipgloss.Left, virtualBox, secondRow)
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

	// Check if selected process still exists, if not clear selection and target PID
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
			// Also clear target PID in eBPF if this was the monitored process
			if m.setTargetPID != nil {
				if err := m.setTargetPID(0); err != nil {
					m.showModal = true
					m.modalType = constants.ModalTypeError
					m.modalMessage = fmt.Sprintf("Failed to clear target PID in eBPF: %v", err)
				}
			}
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
			m.showModal = true
			m.modalType = constants.ModalTypeError
			m.modalMessage = fmt.Sprintf("Failed to set target PID %d in eBPF: %v", pid, err)
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

// updateViewports calculates viewport dimensions and renders content for both the virtual
// memory pane (top) and physical memory pane (bottom) based on the selected process data.
// It handles layout constraints and pagination for memory range display.
func (m *Model) updateViewports(pd *data.ProcessData) {
	if pd == nil {
		return
	}

	available, rowHeight, secondRowHeight, barWidth := m.layoutMetrics()
	proc := pd.Proc
	summary := m.renderSummary(proc, pd)

	virtualWidth := available - constants.LeftPanePadding
	if virtualWidth < constants.MinBarWidth {
		virtualWidth = constants.MinBarWidth
	}

	// Split physical/logs width (second row is split horizontally)
	halfWidth := available / 2
	physicalWidth := halfWidth - constants.LeftPanePadding
	logsWidth := available - halfWidth - constants.LeftPanePadding

	if physicalWidth < constants.MinBarWidth {
		physicalWidth = constants.MinBarWidth
	}
	if logsWidth < constants.MinBarWidth {
		logsWidth = constants.MinBarWidth
	}

	virtualHeight := rowHeight - 2
	if virtualHeight < constants.MinViewportHeight {
		virtualHeight = constants.MinViewportHeight
	}
	physicalHeight := secondRowHeight - 2
	if physicalHeight < constants.MinViewportHeight {
		physicalHeight = constants.MinViewportHeight
	}
	logsHeight := physicalHeight // Same height as physical

	m.viewportVirtual.SetWidth(virtualWidth)
	m.viewportVirtual.SetHeight(virtualHeight)
	m.viewportPhysical.SetWidth(physicalWidth)
	m.viewportPhysical.SetHeight(physicalHeight)
	m.viewportLogs.SetWidth(logsWidth)
	m.viewportLogs.SetHeight(logsHeight)

	m.viewportVirtual.SetContent(uirender.RenderVirtualSectionContent(summary, pd, barWidth, m.rangesPage, virtualWidth))
	m.viewportPhysical.SetContent(uirender.RenderPhysicalSectionContent(pd))
	// Logs content is updated via FlushLogsMsg timer
}

func (m *Model) clearViewports() {
	// Set dimensions same as updateViewports to maintain consistent layout
	available, rowHeight, secondRowHeight, _ := m.layoutMetrics()

	virtualWidth := available - constants.LeftPanePadding
	if virtualWidth < constants.MinBarWidth {
		virtualWidth = constants.MinBarWidth
	}

	halfWidth := available / 2
	physicalWidth := halfWidth - constants.LeftPanePadding
	logsWidth := available - halfWidth - constants.LeftPanePadding

	if physicalWidth < constants.MinBarWidth {
		physicalWidth = constants.MinBarWidth
	}
	if logsWidth < constants.MinBarWidth {
		logsWidth = constants.MinBarWidth
	}

	virtualHeight := rowHeight - 2
	if virtualHeight < constants.MinViewportHeight {
		virtualHeight = constants.MinViewportHeight
	}
	physicalHeight := secondRowHeight - 2
	if physicalHeight < constants.MinViewportHeight {
		physicalHeight = constants.MinViewportHeight
	}
	logsHeight := physicalHeight

	m.viewportVirtual.SetWidth(virtualWidth)
	m.viewportVirtual.SetHeight(virtualHeight)
	m.viewportPhysical.SetWidth(physicalWidth)
	m.viewportPhysical.SetHeight(physicalHeight)
	m.viewportLogs.SetWidth(logsWidth)
	m.viewportLogs.SetHeight(logsHeight)

	// Set content with styled titles
	m.viewportVirtual.SetContent(uirender.StyleTitle("[virtual memory]") + "\n\nNo process selected.\nPress Enter on a process to view details.")
	m.viewportPhysical.SetContent(uirender.StyleTitle("[physical memory]") + "\n\nNo process selected.")
	// Don't clear logs - they persist independently of selected process
}

// renderSummary creates a formatted summary header showing process information
// and memory statistics including reservation, allocation, page-out, and page-in counts.
// Returns a styled table with color-coded columns for different memory statuses.
func (m *Model) renderSummary(proc data.Process, pd *data.ProcessData) string {
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
		"q":      func() (tea.Model, tea.Cmd) { return m, func() tea.Msg { return tea.Quit() } },
		"ctrl+c": func() (tea.Model, tea.Cmd) { return m, func() tea.Msg { return tea.Quit() } },
		"enter": func() (tea.Model, tea.Cmd) {
			if sel, ok := m.list.SelectedItem().(listItem); ok {
				// Enter selects the focused process
				m.selectedPID = sel.pid
				m.rangesPage = 0 // Reset to first page when selecting new process
				if m.setTargetPID != nil {
					if err := m.setTargetPID(sel.pid); err != nil {
						m.showModal = true
						m.modalType = constants.ModalTypeError
						m.modalMessage = fmt.Sprintf("Failed to set target PID %d in eBPF: %v", sel.pid, err)
						// Don't proceed with success modal
						return m, tea.Tick(constants.ModalDisplayDuration, func(time.Time) tea.Msg {
							return uimsg.HideModalMsg{}
						})
					}
				}
				m.syncLayout()

				// Show modal with info notification
				m.showModal = true
				m.modalType = constants.ModalTypeInfo
				m.modalMessage = fmt.Sprintf("Selected process %d", sel.pid)

				// Send message to load smaps for this process and hide modal after configured duration
				return m, tea.Batch(
					func() tea.Msg {
						return uimsg.LoadSmapsRequest{PID: sel.pid}
					},
					tea.Tick(constants.ModalDisplayDuration, func(time.Time) tea.Msg {
						return uimsg.HideModalMsg{}
					}),
				)
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
		"E": func() (tea.Model, tea.Cmd) {
			// Test error popup (capital E for debug)
			m.showModal = true
			m.modalType = constants.ModalTypeError
			m.modalMessage = "Test error message - this is a red error popup!"

			return m, tea.Tick(constants.ModalDisplayDuration, func(time.Time) tea.Msg {
				return uimsg.HideModalMsg{}
			})
		},
		"W": func() (tea.Model, tea.Cmd) {
			// Test warning popup (capital W for debug)
			m.showModal = true
			m.modalType = constants.ModalTypeWarn
			m.modalMessage = "Test warning message - this is an orange warning popup!"

			return m, tea.Tick(constants.ModalDisplayDuration, func(time.Time) tea.Msg {
				return uimsg.HideModalMsg{}
			})
		},
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
