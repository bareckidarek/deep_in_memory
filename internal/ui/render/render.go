package render

import (
"fmt"

"charm.land/lipgloss/v2"

"deep_in_memory/internal/constants"
"deep_in_memory/internal/data"
)

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
	prefix := "kMGTPE"[exp]
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), prefix)
}

// StyleTitle applies consistent styling to section titles.
func StyleTitle(title string) string {
	style := lipgloss.NewStyle().
		Background(lipgloss.Color("62")).
		Foreground(lipgloss.Color("230")).
		Bold(true)
	return style.Render(title)
}
