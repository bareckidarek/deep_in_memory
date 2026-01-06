package render

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"deep_in_memory/internal/data"
)

var statusOrder = []string{"INIT", "SWAP", "UNSWAP"}

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
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// RenderVirtualSectionContent builds the virtual memory section text.
func RenderVirtualSectionContent(summary string, pd *data.ProcessData, barWidth int) string {
	return renderSection(
		"[virtual memory]",
		summary,
		renderThreadRanges(pd.Threads, barWidth),
		renderPhases(pd.Virtual),
		renderMatrix("Virtual", virtualKeys(pd.Virtual)),
	)
}

// RenderPhysicalSectionContent builds the physical memory section text.
func RenderPhysicalSectionContent(pd *data.ProcessData) string {
	phSummary := fmt.Sprintf("PFNs: %d\nPhysical usage: %s\n",
		len(pd.PFNs), HumanBytes(int64(len(pd.PFNs))*data.PageSizeBytes))
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

func renderThreadRanges(threads []data.ThreadData, barWidth int) string {
	if len(threads) == 0 {
		return "Virtual ranges: none"
	}

	var b strings.Builder
	b.WriteString("Virtual ranges (by thread):\n")
	for tIdx, th := range threads {
		connector := treeConnector(tIdx, len(threads))
		b.WriteString(fmt.Sprintf("%s %d\n", connector, th.TID))
		prefix := branchPrefix(connector)

		for rIdx, r := range th.Ranges {
			rangeConn := treeConnector(rIdx, len(th.Ranges))
			b.WriteString(fmt.Sprintf("%s%s %s\n", prefix, rangeConn, renderRangeBar(r, barWidth)))
		}
	}

	return strings.TrimRight(b.String(), "\n")
}

func renderRangeBar(r data.AddrRange, barWidth int) string {
	if r.Size == 0 {
		return fmt.Sprintf("%s [invalid range] %s", r.Start, r.End)
	}

	totalWidthUsed := 0
	segments := make([]string, 0, len(statusOrder)+1)
	for _, status := range statusOrder {
		pages := len(r.Statuses[status])
		width := statusSegmentWidth(pages, r.Size, barWidth-totalWidthUsed)
		if width <= 0 {
			continue
		}
		segments = append(segments, statusBlock(status, width))
		totalWidthUsed += width
		if totalWidthUsed >= barWidth {
			break
		}
	}

	if remainder := barWidth - totalWidthUsed; remainder > 0 {
		segments = append(segments, grayBlock(remainder))
	}

	bar := strings.Join(segments, "")
	hundredVal := r.Size / 1000
	return fmt.Sprintf("%s %s %s (100%%=%d)", r.Start, bar, r.End, hundredVal)
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

func statusSegmentWidth(pages int, rangeSize uint64, availableWidth int) int {
	if pages <= 0 || rangeSize == 0 || availableWidth <= 0 {
		return 0
	}
	portion := float64(pages*data.PageSizeBytes) / float64(rangeSize)
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
