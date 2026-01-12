package render

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"deep_in_memory/internal/data"
)

var statusOrder = []string{"SWAP", "UNSWAP", "INIT"}

const (
	matrixColumns    = 8
	matrixMaxRows    = 6
	phaseSampleLimit = 5
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
	const unit = 1024
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
	legendPlainLen := 59 // "██ reservation  ██ allocation  ██ page-out  ██ page-in" = 59 chars visible
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
	grayBox := lipgloss.NewStyle().Background(lipgloss.Color("#666666")).Render("  ")
	initBox := lipgloss.NewStyle().Background(lipgloss.Color(colorForStatus("INIT"))).Render("  ")
	swapBox := lipgloss.NewStyle().Background(lipgloss.Color(colorForStatus("SWAP"))).Render("  ")
	unswapBox := lipgloss.NewStyle().Background(lipgloss.Color(colorForStatus("UNSWAP"))).Render("  ")

	return fmt.Sprintf("%s reservation  %s allocation  %s page-out  %s page-in", grayBox, initBox, swapBox, unswapBox)
}

func renderThreadRanges(threads []data.ThreadData, barWidth int, page int) string {
	if len(threads) == 0 {
		return "Virtual ranges: none"
	}

	const rangesPerPage = 25

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

func renderRangeBar(r data.AddrRange, barWidth int) string {
	if r.Size == 0 {
		return fmt.Sprintf("%s [invalid range] %s", r.Start, r.End)
	}

	// Calculate expanded bar width (addresses are now inside)
	// Estimate: ~16 chars per address + info text ~15 chars = ~40 chars
	expandedBarWidth := barWidth + 40

	// Build status segments with their widths
	type segment struct {
		status string
		width  int
	}
	var segmentList []segment
	totalWidthUsed := 0

	for _, status := range statusOrder {
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

	// Prepare overlay text
	startAddr := r.Start
	var endInfo string

	// If there's a label, show it in the middle instead of addresses
	if r.Label != "" {
		sizeBytes := r.Size * data.PageSizeBytes
		endInfo = fmt.Sprintf("%s (100%%=%s [%dp])", r.End, HumanBytes(int64(sizeBytes)), r.Size)
		// Put label in the middle
		labelText := r.Label
		middlePos := (expandedBarWidth - len(labelText)) / 2
		if middlePos < len(startAddr)+1 {
			middlePos = len(startAddr) + 1
		}
		endPos := expandedBarWidth - len(endInfo)
		if endPos < middlePos+len(labelText)+1 {
			endPos = middlePos + len(labelText) + 1
		}

		overlayText := startAddr + strings.Repeat(" ", middlePos-len(startAddr)) + labelText + strings.Repeat(" ", endPos-middlePos-len(labelText)) + endInfo
		if len(overlayText) > expandedBarWidth {
			overlayText = overlayText[:expandedBarWidth]
		} else if len(overlayText) < expandedBarWidth {
			overlayText = overlayText + strings.Repeat(" ", expandedBarWidth-len(overlayText))
		}

		// Build the bar with text overlay character by character
		var result strings.Builder
		textPos := 0

		for _, seg := range segmentList {
			bgColor := colorForStatus(seg.status)
			if seg.status == "gray" {
				bgColor = "#666666"
			}

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

	// Original behavior without label
	sizeBytes := r.Size * data.PageSizeBytes
	endInfo = fmt.Sprintf("%s (100%%=%s [%dp])", r.End, HumanBytes(int64(sizeBytes)), r.Size)

	// Build the bar with text overlay character by character
	var result strings.Builder
	textPos := 0
	overlayText := startAddr + strings.Repeat(" ", expandedBarWidth-len(startAddr)-len(endInfo)) + endInfo

	for _, seg := range segmentList {
		bgColor := colorForStatus(seg.status)
		if seg.status == "gray" {
			bgColor = "#666666"
		}

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
		return "#00af5f" // green
	case "SWAP":
		return "#ff5f5f" // red
	case "UNSWAP":
		return "#5f87ff" // blue
	default:
		return "#666666" // gray fallback
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
	return lipgloss.NewStyle().Background(lipgloss.Color("#666666")).Render(strings.Repeat(" ", width))
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
	rows := (len(entries) + matrixColumns - 1) / matrixColumns
	if rows > matrixMaxRows {
		rows = matrixMaxRows // cap rows to keep view compact
	}
	maxCells := rows * matrixColumns

	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s matrix (%d shown):\n", label, min(len(entries), maxCells)))

	for i := 0; i < maxCells && i < len(entries); i++ {
		if i%matrixColumns == 0 && i != 0 {
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
		if len(addrs) > phaseSampleLimit {
			samples = addrs[:phaseSampleLimit]
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
