package render

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"deep_in_memory/internal/constants"
	"deep_in_memory/internal/data"
)

// ActiveVirtualCount returns the number of non-freed virtual pages.
func ActiveVirtualCount(vmap map[string]*data.VirtualPage) int {
	count := 0
	for _, vp := range vmap {
		if vp.Phase != "freed" {
			count++
		}
	}
	return count
}

// TotalVirtualPages returns the total number of virtual pages across all thread ranges.
func TotalVirtualPages(pd *data.ProcessData) uint64 {
	var total uint64
	if pd == nil || pd.Threads == nil {
		return 0
	}
	for _, thread := range pd.Threads {
		for _, r := range thread.Ranges {
			total += r.Size
		}
	}
	return total
}

// StatusStats holds page counts for each status category.
type StatusStats struct {
	Reservation uint64 // Total reserved virtual address space (sum of all range sizes)
	Allocation  uint64 // INIT pages (physically allocated)
	PageOut     uint64 // SWAP pages (swapped out)
	PageIn      uint64 // UNSWAP pages (swapped in)
}

// CalculateStatusStats calculates page counts for each status category.
func CalculateStatusStats(pd *data.ProcessData) StatusStats {
	var stats StatusStats
	if pd == nil || pd.Threads == nil {
		return stats
	}

	for _, thread := range pd.Threads {
		for _, r := range thread.Ranges {
			// Reservation = total virtual address space (sum of all range sizes)
			stats.Reservation += r.Size

			// Count each status
			if r.Statuses != nil {
				if initCount := r.Statuses["INIT"]; initCount != nil {
					stats.Allocation += uint64(initCount.Count)
				}
				if swapCount := r.Statuses["SWAP"]; swapCount != nil {
					stats.PageOut += uint64(swapCount.Count)
				}
				if unswapCount := r.Statuses["UNSWAP"]; unswapCount != nil {
					stats.PageIn += uint64(unswapCount.Count)
				}
			}
		}
	}

	return stats
}

// HumanBytes formats bytes into a human-readable string.
func HumanBytes(b int64) string {
	const unit = constants.BytesPerKilobyte
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	// Use lowercase 'k' for kB, uppercase for MB, GB, TB, PB, EB
	prefix := "kMGTPE"[exp]
	if exp == 0 {
		return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), prefix)
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), prefix)
}

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

// RenderPhysicalSectionContent builds the physical memory section text.
func RenderPhysicalSectionContent(pd *data.ProcessData) string {
	phSummary := fmt.Sprintf("PFNs: %d\nPhysical usage: %s\n",
		len(pd.PFNs), HumanBytes(int64(len(pd.PFNs))*int64(data.PageSizeBytes)))
	return renderSection(
		"[physical memory]",
		phSummary,
		renderMatrix("PFN", pfnKeys(pd.PFNs)),
	)
}

func renderSection(title string, parts ...string) string {
	compact := make([]string, 0, len(parts)+1)
	compact = append(compact, title)
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		compact = append(compact, part)
	}
	return strings.Join(compact, "\n")
}

func renderSectionWithLegend(title string, legend string, viewportWidth int, parts ...string) string {
	compact := make([]string, 0, len(parts)+1)
	// Create title line with legend on the right
	// Calculate plain text length (lipgloss styles add ANSI codes that don't count for visual width)
	legendPlainLen := constants.StatusLegendWidth
	titleLen := len(title)
	spacing := viewportWidth - titleLen - legendPlainLen
	if spacing < 2 {
		spacing = 2
	}
	titleLine := title + strings.Repeat(" ", spacing) + legend
	compact = append(compact, titleLine)
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

// renderRangeBar renders a memory range bar with status visualization and text overlay
// renderRangeBar creates a visual memory bar with status segments and text overlay.
// Shows proportional colored segments for different memory states (INIT/SWAP/UNSWAP)
// with address range and size information overlaid on top.
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
// Converts page counts to pixel widths based on total range size, ensuring
// segments fit within the available bar width with proper scaling.
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
	endInfo := fmt.Sprintf("%s (100%%=%s [%dp])", r.End, HumanBytes(int64(sizeBytes)), r.Size)

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
	endInfo := fmt.Sprintf("%s (100%%=%s [%dp])", r.End, HumanBytes(int64(sizeBytes)), r.Size)

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

func statusBlock(status string, width int) string {
	return lipgloss.NewStyle().Background(lipgloss.Color(colorForStatus(status))).Render(strings.Repeat(" ", width))
}

func grayBlock(width int) string {
	return lipgloss.NewStyle().Background(lipgloss.Color(constants.GrayColor)).Render(strings.Repeat(" ", width))
}

func treeConnector(idx, total int) string {
	if idx == total-1 {
		return "└─"
	}
	return "├─"
}

func branchPrefix(connector string) string {
	if connector == "└─" {
		return "   "
	}
	return "│  "
}

func renderMatrix(label string, entries []string) string {
	if len(entries) == 0 {
		return fmt.Sprintf("%s matrix: none", label)
	}
	rows := (len(entries) + constants.MatrixColumns - 1) / constants.MatrixColumns
	if rows > constants.MatrixMaxRows {
		rows = constants.MatrixMaxRows // cap rows to keep view compact
	}
	maxCells := rows * constants.MatrixColumns

	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s matrix (%d shown):\n", label, min(len(entries), maxCells)))

	for i := 0; i < maxCells && i < len(entries); i++ {
		if i%constants.MatrixColumns == 0 && i != 0 {
			b.WriteString("\n")
		}
		b.WriteString(fmt.Sprintf("[%s] ", shorten(entries[i])))
	}

	if len(entries) > maxCells {
		b.WriteString(fmt.Sprintf("\n… %d more", len(entries)-maxCells))
	}

	return b.String()
}

func renderPhases(vmap map[string]*data.VirtualPage) string {
	if len(vmap) == 0 {
		return "No virtual addresses tracked."
	}
	// Group addresses by phase for quick inspection.
	buckets := map[string][]string{}
	for addr, vp := range vmap {
		phase := vp.Phase
		if phase == "" {
			phase = "unknown"
		}
		buckets[phase] = append(buckets[phase], addr)
	}
	for _, list := range buckets {
		sort.Strings(list)
	}

	var b strings.Builder
	b.WriteString("Lifecycle phases:\n")
	phases := []string{"created", "initialized", "swapped", "recovered", "freed", "unknown"}
	for _, ph := range phases {
		addrs := buckets[ph]
		if len(addrs) == 0 {
			continue
		}
		samples := addrs
		if len(addrs) > constants.PhaseSampleLimit {
			samples = addrs[:constants.PhaseSampleLimit]
		}
		b.WriteString(fmt.Sprintf("- %s: %d (e.g., %s)\n", ph, len(addrs), strings.Join(samples, ", ")))
	}
	return b.String()
}

func virtualKeys(vmap map[string]*data.VirtualPage) []string {
	keys := make([]string, 0, len(vmap))
	for k := range vmap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func pfnKeys(pmap map[string]*data.PFNState) []string {
	keys := make([]string, 0, len(pmap))
	for k := range pmap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func shorten(addr string) string {
	if addr == "" {
		return "--"
	}
	// Prefer the last 4 hex chars for a compact label.
	if len(addr) > 6 {
		return addr[len(addr)-4:]
	}
	return addr
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
