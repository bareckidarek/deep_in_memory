package model

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"deep_in_memory/internal/constants"
	"deep_in_memory/internal/data"
	uirender "deep_in_memory/internal/ui/render"
)

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
	m.viewportPhysical.SetContent(uirender.RenderPhysicalSectionContent(m.buildPhysicalMemoryState(physicalWidth, physicalHeight)))
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
	m.viewportPhysical.SetContent(uirender.RenderPhysicalSectionContent(m.buildPhysicalMemoryState(physicalWidth, physicalHeight)))
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
