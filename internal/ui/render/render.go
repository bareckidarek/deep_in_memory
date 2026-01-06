package render

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

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
	virtMatrix := renderMatrix("Virtual", virtualKeys(pd.Virtual))
	phases := renderPhases(pd.Virtual)
	ranges := renderThreadRanges(pd, barWidth)

	return strings.Join([]string{
		"[virtual memory]",
		summary,
		ranges,
		phases,
		virtMatrix,
	}, "\n")
}

// RenderPhysicalSectionContent builds the physical memory section text.
func RenderPhysicalSectionContent(pd *data.ProcessData) string {
	pfnMatrix := renderMatrix("PFN", pfnKeys(pd.PFNs))
	physicalSummary := fmt.Sprintf("PFNs: %d\nPhysical usage: %s\n",
		len(pd.PFNs), HumanBytes(int64(len(pd.PFNs))*data.PageSizeBytes))
	return strings.Join([]string{
		"[physical memory]",
		physicalSummary,
		pfnMatrix,
	}, "\n")
}

func renderThreadRanges(pd *data.ProcessData, barWidth int) string {
	if len(pd.Threads) == 0 {
		return "Virtual ranges: none"
	}

	var b strings.Builder
	b.WriteString("Virtual ranges (by thread):\n")
	for tIdx, th := range pd.Threads {
		threadConnector := "├─"
		if tIdx == len(pd.Threads)-1 {
			threadConnector = "└─"
		}
		b.WriteString(fmt.Sprintf("%s %d\n", threadConnector, th.TID))

		rangePrefix := "│  "
		if threadConnector == "└─" {
			rangePrefix = "   "
		}

		for rIdx, r := range th.Ranges {
			rangeConnector := "├─"
			if rIdx == len(th.Ranges)-1 {
				rangeConnector = "└─"
			}
			b.WriteString(fmt.Sprintf("%s%s %s\n", rangePrefix, rangeConnector, renderRangeBar(r, barWidth)))
		}
	}

	return strings.TrimRight(b.String(), "\n")
}

func renderRangeBar(r data.AddrRange, barWidth int) string {
	if r.Size == 0 {
		return fmt.Sprintf("%s [invalid range] %s", r.Start, r.End)
	}

	// Count pages per status; default state (0/null) is implicit.
	counts := map[string]int{
		"INIT":   len(r.Statuses["INIT"]),
		"SWAP":   len(r.Statuses["SWAP"]),
		"UNSWAP": len(r.Statuses["UNSWAP"]),
	}

	totalWidthUsed := 0
	segments := make([]string, 0, 4)

	for _, status := range []string{"INIT", "SWAP", "UNSWAP"} {
		pages := counts[status]
		if pages == 0 {
			continue
		}
		width := int(math.Round(float64(pages*data.PageSizeBytes) / float64(r.Size) * float64(barWidth)))
		if width == 0 {
			width = 1
		}
		if totalWidthUsed+width > barWidth {
			width = barWidth - totalWidthUsed
		}
		if width <= 0 {
			continue
		}
		segments = append(segments, lipgloss.NewStyle().Background(lipgloss.Color(colorForStatus(status))).Render(strings.Repeat(" ", width)))
		totalWidthUsed += width
		if totalWidthUsed >= barWidth {
			break
		}
	}

	if totalWidthUsed < barWidth {
		segments = append(segments, lipgloss.NewStyle().Background(lipgloss.Color("#666666")).Render(strings.Repeat(" ", barWidth-totalWidthUsed)))
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

func renderMatrix(label string, entries []string) string {
	if len(entries) == 0 {
		return fmt.Sprintf("%s matrix: none", label)
	}
	cols := 8
	rows := (len(entries) + cols - 1) / cols
	if rows > 6 {
		rows = 6 // cap rows to keep view compact
	}
	maxCells := rows * cols

	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s matrix (%d shown):\n", label, min(len(entries), maxCells)))

	for i := 0; i < maxCells && i < len(entries); i++ {
		if i%cols == 0 && i != 0 {
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
		if len(addrs) > 5 {
			samples = addrs[:5]
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
