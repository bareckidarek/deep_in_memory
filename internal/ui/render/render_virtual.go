package render

import (
	"fmt"
	"math"
	"strings"

	"charm.land/lipgloss/v2"

	"deep_in_memory/internal/constants"
	"deep_in_memory/internal/data"
)

// RenderVirtualSectionContent builds the virtual memory section text.
func RenderVirtualSectionContent(summary string, pd *data.ProcessData, barWidth int, page int, viewportWidth int) string {
	legend := buildColorLegend()
	return renderSectionWithLegend(
		"[virtual memory]",
		legend,
		viewportWidth,
		summary,
		renderThreadRanges(pd.Threads, barWidth, page),
	)
}

func renderSectionWithLegend(title string, legend string, viewportWidth int, parts ...string) string {
	compact := make([]string, 0, len(parts)+2) // +2 for title and empty line
	// Create title line with legend on the right
	styledTitle := StyleTitle(title)

	// Calculate plain text length (lipgloss styles add ANSI codes that don't count for visual width)
	legendPlainLen := constants.StatusLegendWidth
	titleLen := len(title) // Use plain text length for spacing calculation
	spacing := viewportWidth - titleLen - legendPlainLen
	if spacing < 2 {
		spacing = 2
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
	grayBox := lipgloss.NewStyle().Background(lipgloss.Color(constants.GrayColor)).Render("  ")
	initBox := lipgloss.NewStyle().Background(lipgloss.Color(colorForStatus("INIT"))).Render("  ")
	swapBox := lipgloss.NewStyle().Background(lipgloss.Color(colorForStatus("SWAP"))).Render("  ")
	unswapBox := lipgloss.NewStyle().Background(lipgloss.Color(colorForStatus("UNSWAP"))).Render("  ")

	return fmt.Sprintf("%s %s  %s %s  %s %s  %s %s",
		grayBox, constants.StatusReservation,
		initBox, constants.StatusAllocation,
		swapBox, constants.StatusPageOut,
		unswapBox, constants.StatusPageIn)
}

func renderThreadRanges(threads []data.ThreadData, barWidth int, page int) string {
	if len(threads) == 0 {
		return "Virtual ranges: none"
	}

	const rangesPerPage = constants.RangesPerPage

	// Count total ranges across all threads
	totalRanges := 0
	for _, th := range threads {
		totalRanges += len(th.Ranges)
	}

	if totalRanges == 0 {
		return "Virtual ranges: none"
	}

	// Calculate pagination
	startIdx := page * rangesPerPage
	if startIdx >= totalRanges {
		// If page is out of bounds, show last page
		page = (totalRanges - 1) / rangesPerPage
		startIdx = page * rangesPerPage
	}
	endIdx := startIdx + rangesPerPage
	if endIdx > totalRanges {
		endIdx = totalRanges
	}

	totalPages := (totalRanges + rangesPerPage - 1) / rangesPerPage

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Virtual ranges (by thread) - Page %d/%d (showing %d-%d of %d) [use p/n to navigate]:\n",
		page+1, totalPages, startIdx+1, endIdx, totalRanges))

	// Flatten ranges with their thread context
	type rangeWithThread struct {
		threadIdx int
		threadID  int
		rangeIdx  int
		range_    data.AddrRange
	}

	var allRanges []rangeWithThread
	for tIdx, th := range threads {
		for rIdx, r := range th.Ranges {
			allRanges = append(allRanges, rangeWithThread{
				threadIdx: tIdx,
				threadID:  th.TID,
				rangeIdx:  rIdx,
				range_:    r,
			})
		}
	}

	// Extract page slice
	pageRanges := allRanges[startIdx:endIdx]

	// Group by thread for rendering
	currentThread := -1
	for i, item := range pageRanges {
		if item.threadID != currentThread {
			// New thread header
			if i > 0 {
				b.WriteString("\n")
			}

			// Determine if this is the last thread: check if it's the last range overall or last on page
			isLastThreadOverall := endIdx >= totalRanges && i == len(pageRanges)-1

			// Check if there are more threads after this one in the page
			hasMoreThreadsInPage := false
			for j := i + 1; j < len(pageRanges); j++ {
				if pageRanges[j].threadID != item.threadID {
					hasMoreThreadsInPage = true
					break
				}
			}

			connector := "├─"
			if isLastThreadOverall || (!hasMoreThreadsInPage && endIdx >= totalRanges) {
				connector = "└─"
			}

			b.WriteString(fmt.Sprintf("%s %d\n", connector, item.threadID))
			currentThread = item.threadID
		}

		// Render range
		// Determine prefix based on whether thread connector was └─ or ├─
		isLastThreadOverall := endIdx >= totalRanges && currentThread == item.threadID
		hasMoreThreadsAfter := false
		for j := i + 1; j < len(pageRanges); j++ {
			if pageRanges[j].threadID != currentThread {
				hasMoreThreadsAfter = true
				break
			}
		}

		prefix := "│  "
		if (isLastThreadOverall && !hasMoreThreadsAfter) || (endIdx >= totalRanges && !hasMoreThreadsAfter) {
			prefix = "   "
		}

		// Check if this is the last range in current thread on this page
		isLastInThread := (i == len(pageRanges)-1) || (i < len(pageRanges)-1 && pageRanges[i+1].threadID != item.threadID)

		rangeConn := "├─"
		if isLastInThread {
			rangeConn = "└─"
		}

		b.WriteString(fmt.Sprintf("%s%s %s\n", prefix, rangeConn, renderRangeBar(item.range_, barWidth)))
	}

	return strings.TrimRight(b.String(), "\n")
}

// segment represents a colored segment of the memory bar
type segment struct {
	status string
	width  int
}

// renderRangeBar creates a visual memory bar with status segments and text overlay.
func renderRangeBar(r data.AddrRange, barWidth int) string {
	if r.Size == 0 {
		return fmt.Sprintf("%s [invalid range] %s", r.Start, r.End)
	}

	expandedBarWidth := barWidth + 40
	segmentList := calculateStatusSegments(r, expandedBarWidth)
	overlayText := buildOverlayText(r, expandedBarWidth)

	return renderColoredBar(segmentList, overlayText)
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
	sizeBytes := r.Size * data.PageSizeBytes
	endInfo := fmt.Sprintf("%s (100%%=%dp [%s])", r.End, r.Size, HumanBytes(int64(sizeBytes)))

	labelText := r.Label
	middlePos := (expandedBarWidth - len(labelText)) / 2
	if middlePos < len(startAddr)+1 {
		middlePos = len(startAddr) + 1
	}
	endPos := expandedBarWidth - len(endInfo)
	if endPos < middlePos+len(labelText)+1 {
		endPos = middlePos + len(labelText) + 1
	}

	overlayText := startAddr + strings.Repeat(" ", middlePos-len(startAddr)) + labelText +
		strings.Repeat(" ", endPos-middlePos-len(labelText)) + endInfo

	return truncateOverlayText(overlayText, expandedBarWidth)
}

// buildAddressOverlay creates overlay text with start and end addresses
func buildAddressOverlay(r data.AddrRange, startAddr string, expandedBarWidth int) string {
	sizeBytes := r.Size * data.PageSizeBytes
	endInfo := fmt.Sprintf("%s (100%%=%dp [%s])", r.End, r.Size, HumanBytes(int64(sizeBytes)))

	overlayText := startAddr + strings.Repeat(" ", expandedBarWidth-len(startAddr)-len(endInfo)) + endInfo
	return truncateOverlayText(overlayText, expandedBarWidth)
}

// truncateOverlayText ensures overlay text fits within the bar width
func truncateOverlayText(overlayText string, maxWidth int) string {
	if len(overlayText) > maxWidth {
		return overlayText[:maxWidth]
	} else if len(overlayText) < maxWidth {
		return overlayText + strings.Repeat(" ", maxWidth-len(overlayText))
	}
	return overlayText
}

// renderColoredBar renders the final colored bar with text overlay
func renderColoredBar(segmentList []segment, overlayText string) string {
	var result strings.Builder
	textPos := 0

	for _, seg := range segmentList {
		bgColor := colorForStatus(seg.status)

		// Render this segment with overlay text
		for i := 0; i < seg.width && textPos < len(overlayText); i++ {
			char := string(overlayText[textPos])
			styled := lipgloss.NewStyle().
				Background(lipgloss.Color(bgColor)).
				Foreground(lipgloss.Color("#000000")).
				Bold(true).
				Render(char)
			result.WriteString(styled)
			textPos++
		}
	}

	return result.String()
}

func colorForStatus(status string) string {
	switch status {
	case "INIT":
		return constants.GreenColor
	case "SWAP":
		return constants.RedColor
	case "UNSWAP":
		return constants.BlueColor
	default:
		return constants.GrayColor
	}
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
