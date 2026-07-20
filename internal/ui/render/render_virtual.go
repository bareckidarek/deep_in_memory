package render

import (
	"fmt"
	"math"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"deep_in_memory/internal/constants"
	"deep_in_memory/internal/data"
	"github.com/charmbracelet/x/ansi"
	"github.com/dustin/go-humanize"
)

// RenderVirtualEmptyContent returns the virtual memory section text shown when no process is selected.
func RenderVirtualEmptyContent() string {
	return StyleTitle("[virtual memory]") + "\n\nNo process selected.\nPress Enter on a process to view details."
}

// RenderSummary creates a formatted summary header showing process information
// and memory statistics including reservation, allocation, page-out, and page-in counts.
// Returns a styled table with color-coded columns for different memory statuses.
func RenderSummary(proc data.Process, pd *data.ProcessData) string {
	startUTC := proc.StartTime.UTC().Format(time.RFC3339)
	stats := CalculateStatusStats(pd)

	// Build header
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Process: %s (%d)\n", proc.Name, proc.PID))
	sb.WriteString(fmt.Sprintf("Started (UTC): %s\n\n", startUTC))

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
		col1Style.Render(humanize.IBytes(reservationBytes)),
		col2Style.Render(humanize.IBytes(allocationBytes)),
		col3Style.Render(humanize.IBytes(pageOutBytes)),
		col4Style.Render(humanize.IBytes(pageInBytes)),
	)

	// Join all rows vertically and apply border to the entire table
	table := lipgloss.JoinVertical(lipgloss.Left, headerRow, pagesRow, sizeRow)
	borderedTable := borderStyle.Render(table)

	sb.WriteString(borderedTable)
	sb.WriteString("\n")

	return sb.String()
}

// RenderVirtualSectionContent builds the virtual memory section text.
// When showAll is false, only Heap and Empty ranges are rendered.
// It returns rendered content and the normalized page index.
func RenderVirtualSectionContent(summary string, pd *data.ProcessData, barWidth int, page int, viewportWidth int, viewportHeight int, showAll bool) (string, int) {
	legend := buildColorLegend()
	rangesPerPage := calcVirtualRowsPerPage(viewportHeight, summary)
	rangesContent, normalizedPage := renderThreadRanges(pd.Proc.PID, pd.Threads, barWidth, page, rangesPerPage, showAll)

	return renderSectionWithLegend(
		"[virtual memory]",
		legend,
		viewportWidth,
		summary,
		rangesContent,
	), normalizedPage
}

func calcVirtualRowsPerPage(viewportHeight int, summary string) int {
	summaryHeight := lipgloss.Height(strings.TrimRight(summary, "\n"))
	if summaryHeight < 0 {
		summaryHeight = 0
	}

	fixedLines := constants.VirtualTitleLines +
		constants.VirtualTitleSpacingLines +
		summaryHeight +
		constants.VirtualFilterLegendLines +
		constants.VirtualPaginationInfoLine +
		constants.VirtualReservedFooterLine
	rowsPerPage := viewportHeight - fixedLines
	if rowsPerPage < 1 {
		return 1
	}
	return rowsPerPage
}

func renderSectionWithLegend(title string, legend string, viewportWidth int, parts ...string) string {
	compact := make([]string, 0, len(parts)+2) // +2 for title and empty line
	// Create title line with legend on the right
	styledTitle := StyleTitle(title)

	// Calculate plain text length (lipgloss styles add ANSI codes that don't count for visual width)
	legendPlainLen := constants.StatusLegendWidth
	titleLen := len(title) // Use plain text length for spacing calculation
	spacing := viewportWidth - titleLen - legendPlainLen
	if spacing < constants.VirtualTitleLegendMinSpacing {
		spacing = constants.VirtualTitleLegendMinSpacing
	}
	titleLine := styledTitle + strings.Repeat(" ", spacing) + legend
	compact = append(compact, titleLine)
	compact = append(compact, "") // Empty line after title
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		compact = append(compact, part)
	}
	return strings.Join(compact, "\n")
}

func buildColorLegend() string {
	// Build colored legend with 2-character width boxes
	grayNormal, _ := StatusColors("gray")
	initNormal, _ := StatusColors("INIT")
	swapNormal, _ := StatusColors("SWAP")
	unswapNormal, _ := StatusColors("UNSWAP")

	grayBox := lipgloss.NewStyle().Background(lipgloss.Color(grayNormal)).Render("  ")
	initBox := lipgloss.NewStyle().Background(lipgloss.Color(initNormal)).Render("  ")
	swapBox := lipgloss.NewStyle().Background(lipgloss.Color(swapNormal)).Render("  ")
	unswapBox := lipgloss.NewStyle().Background(lipgloss.Color(unswapNormal)).Render("  ")

	return fmt.Sprintf("%s %s  %s %s  %s %s  %s %s",
		grayBox, constants.StatusReservation,
		initBox, constants.StatusAllocation,
		swapBox, constants.StatusPageOut,
		unswapBox, constants.StatusPageIn)
}

// defaultLabelTypes are the label types shown when filter is active.
var defaultLabelTypes = map[data.LabelType]bool{
	data.Heap:  true,
	data.Empty: true,
}

// filterThreadRanges returns a copy of threads with ranges filtered to the given label types.
// When showAll is true, no filtering is applied.
func filterThreadRanges(threads []data.ThreadData, showAll bool) []data.ThreadData {
	if showAll {
		return threads
	}
	result := make([]data.ThreadData, 0, len(threads))
	for _, th := range threads {
		var filtered []data.AddrRange
		for _, r := range th.Ranges {
			if defaultLabelTypes[r.LabelType] {
				filtered = append(filtered, r)
			}
		}
		if len(filtered) > 0 {
			result = append(result, data.ThreadData{TID: th.TID, Ranges: filtered})
		}
	}
	return result
}

func renderThreadRanges(procPID int, threads []data.ThreadData, barWidth int, page int, rangesPerPage int, showAll bool) (string, int) {
	threads = filterThreadRanges(threads, showAll)

	if len(threads) == 0 {
		return "Virtual ranges: none", 0
	}

	type displayRow struct {
		isHeader bool
		threadID int
		rangeVal data.AddrRange
	}

	var rows []displayRow
	for _, th := range threads {
		if len(th.Ranges) == 0 {
			continue
		}

		rows = append(rows, displayRow{
			isHeader: true,
			threadID: th.TID,
		})

		for _, r := range th.Ranges {
			rows = append(rows, displayRow{
				isHeader: false,
				threadID: th.TID,
				rangeVal: r,
			})
		}
	}

	if len(rows) == 0 {
		return "Virtual ranges: none", 0
	}

	window := CalculatePageWindow(len(rows), rangesPerPage, page)

	filterLabel := constants.FilterLabelHeapEmpty
	if showAll {
		filterLabel = constants.FilterLabelAll
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Filter: %s  %s\n", filterLabel, constants.VirtualFilterToggleHint))
	b.WriteString(fmt.Sprintf("Virtual ranges (by thread) - Page %d/%d (showing %d-%d of %d) %s\n",
		window.Page+1, window.TotalPages, window.StartIdx+1, window.EndIdx, len(rows), constants.VirtualPagingNavigateHint))

	pageRows := rows[window.StartIdx:window.EndIdx]
	for _, row := range pageRows {
		if row.isHeader {
			b.WriteString(fmt.Sprintf("PID: %d, TID: %d\n", procPID, row.threadID))
			continue
		}

		highlighted := !row.rangeVal.ChangedAt.IsZero() && time.Since(row.rangeVal.ChangedAt) < constants.RangeHighlightDuration
		bar := renderRangeBar(row.rangeVal, barWidth, highlighted)
		b.WriteString(fmt.Sprintf("  %s\n", bar))
	}

	return strings.TrimRight(b.String(), "\n"), window.Page
}

// segment represents a colored segment of the memory bar
type segment struct {
	status string
	width  int
}

// renderRangeBar creates a visual memory bar with status segments and text overlay.
// highlighted switches to lighter background colours and bold text for 3 seconds after a change.
func renderRangeBar(r data.AddrRange, barWidth int, highlighted bool) string {
	if r.Size == 0 {
		return fmt.Sprintf("%s [invalid range] %s", r.Start, r.End)
	}

	expandedBarWidth := barWidth + 40
	segmentList := calculateStatusSegments(r, expandedBarWidth)
	overlayText := buildOverlayText(r, expandedBarWidth)

	return renderColoredBar(segmentList, overlayText, highlighted)
}

// calculateStatusSegments computes the proportional width of each memory status segment.
func calculateStatusSegments(r data.AddrRange, expandedBarWidth int) []segment {
	var segmentList []segment
	totalWidthUsed := 0

	for _, status := range constants.StatusOrder {
		count := 0
		if statusCount := r.Statuses[status]; statusCount != nil {
			count = statusCount.Count
		}

		width := statusSegmentWidth(count, r.Size, expandedBarWidth)
		if width <= 0 {
			continue
		}
		segmentList = append(segmentList, segment{status: status, width: width})
		totalWidthUsed += width
		if totalWidthUsed >= expandedBarWidth {
			break
		}
	}

	// Add gray segment for remainder
	if remainder := expandedBarWidth - totalWidthUsed; remainder > 0 {
		segmentList = append(segmentList, segment{status: "gray", width: remainder})
	}

	return segmentList
}

// buildOverlayText creates the text overlay for the memory bar
func buildOverlayText(r data.AddrRange, expandedBarWidth int) string {
	startAddr := r.Start

	// If there's a label, show it in the middle instead of just addresses
	if r.Label != "" {
		return buildLabelOverlay(r, startAddr, expandedBarWidth)
	}

	return buildAddressOverlay(r, startAddr, expandedBarWidth)
}

// buildLabelOverlay creates overlay text with a centered label
func buildLabelOverlay(r data.AddrRange, startAddr string, expandedBarWidth int) string {
	endInfo := buildEndInfo(r)
	endInfoWidth := lipgloss.Width(endInfo)

	labelText := r.Label
	labelCenterPos := (expandedBarWidth - len(labelText)) / 2
	if labelCenterPos < len(startAddr)+1 {
		labelCenterPos = len(startAddr) + 1
	}
	endPos := expandedBarWidth - endInfoWidth
	if endPos < labelCenterPos+len(labelText)+1 {
		endPos = labelCenterPos + len(labelText) + 1
	}

	overlayText := startAddr + strings.Repeat(" ", labelCenterPos-len(startAddr)) + labelText +
		strings.Repeat(" ", endPos-labelCenterPos-len(labelText)) + endInfo

	return truncateOverlayText(overlayText, expandedBarWidth)
}

// buildAddressOverlay creates overlay text with start and end addresses
func buildAddressOverlay(r data.AddrRange, startAddr string, expandedBarWidth int) string {
	endInfo := buildEndInfo(r)
	endInfoWidth := lipgloss.Width(endInfo)

	overlayText := startAddr + strings.Repeat(" ", expandedBarWidth-len(startAddr)-endInfoWidth) + endInfo
	return truncateOverlayText(overlayText, expandedBarWidth)
}

func buildEndInfo(r data.AddrRange) string {
	sizeBytes := r.Size * data.PageSizeBytes
	return fmt.Sprintf("%s (100%%=%dp [%s%s])", r.End, r.Size, humanize.IBytes(sizeBytes), buildGrowthArrow(int64(r.Size-r.PrevSize)))
}

func buildGrowthArrow(growth int64) string {
	arrow := ""
	if growth > 0 {
		arrow = "➚"
	} else if growth < 0 {
		arrow = "➘"
	}
	return arrow
}

// truncateOverlayText ensures overlay text fits within the bar width
func truncateOverlayText(overlayText string, maxWidth int) string {
	width := lipgloss.Width(overlayText)
	if width > maxWidth {
		return ansi.Truncate(overlayText, maxWidth, "")
	}
	if width < maxWidth {
		return overlayText + strings.Repeat(" ", maxWidth-width)
	}
	return overlayText
}

// renderColoredBar renders the final colored bar with text overlay.
// When highlighted is true, lighter background colours and bold text are used.
func renderColoredBar(segmentList []segment, overlayText string, highlighted bool) string {
	var result strings.Builder
	textPos := 0
	runes := []rune(overlayText)

	for _, seg := range segmentList {
		normalColor, highlightColor := StatusColors(seg.status)
		bgColor := normalColor
		if highlighted {
			bgColor = highlightColor
		}
		style := lipgloss.NewStyle().
			Background(lipgloss.Color(bgColor)).
			Foreground(lipgloss.Color("#000000")).
			Bold(highlighted)

		// Render this segment with overlay text
		for i := 0; i < seg.width && textPos < len(runes); i++ {
			char := string(runes[textPos])
			result.WriteString(style.Render(char))
			textPos++
		}
	}

	return result.String()
}

func statusSegmentWidth(count int, rangeSize uint64, availableWidth int) int {
	if count <= 0 || rangeSize == 0 || availableWidth <= 0 {
		return 0
	}
	portion := float64(count) / float64(rangeSize)
	width := int(math.Round(portion * float64(availableWidth)))
	if width == 0 {
		width = 1
	}
	if width > availableWidth {
		return availableWidth
	}
	return width
}
