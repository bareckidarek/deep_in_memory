package render

import (
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

// StyleTitle applies consistent styling to section titles.
func StyleTitle(title string) string {
	style := lipgloss.NewStyle().
		Background(lipgloss.Color(constants.SectionTitleBgColor)).
		Foreground(lipgloss.Color(constants.SectionTitleFgColor)).
		Bold(true)
	return style.Render(title)
}

// StatusColors returns normal and highlight colors for a memory status.
func StatusColors(status string) (string, string) {
	switch status {
	case "INIT":
		return constants.GreenColor, constants.LightGreenColor
	case "SWAP":
		return constants.RedColor, constants.LightRedColor
	case "UNSWAP":
		return constants.BlueColor, constants.LightBlueColor
	default:
		return constants.GrayColor, constants.LightGrayColor
	}
}

// PageWindow describes a page-based window over a list.
type PageWindow struct {
	StartIdx   int
	EndIdx     int
	Page       int
	TotalPages int
}

// CalculatePageWindow normalizes page inputs and computes a bounded [start,end) window.
func CalculatePageWindow(totalItems, pageSize, page int) PageWindow {
	if pageSize < 1 {
		pageSize = 1
	}
	if totalItems <= 0 {
		return PageWindow{StartIdx: 0, EndIdx: 0, Page: 0, TotalPages: 1}
	}
	if page < 0 {
		page = 0
	}

	totalPages := (totalItems + pageSize - 1) / pageSize
	if page >= totalPages {
		page = totalPages - 1
	}

	start := page * pageSize
	end := start + pageSize
	if end > totalItems {
		end = totalItems
	}

	return PageWindow{
		StartIdx:   start,
		EndIdx:     end,
		Page:       page,
		TotalPages: totalPages,
	}
}

// CalculateScrollWindow computes a bounded [start,end) window for a scrollable list.
func CalculateScrollWindow(totalItems, startIdx, windowSize int) (int, int) {
	if totalItems <= 0 {
		return 0, 0
	}
	if windowSize < 1 {
		windowSize = 1
	}
	if startIdx < 0 {
		startIdx = 0
	}
	if startIdx >= totalItems {
		startIdx = totalItems - 1
	}

	endIdx := startIdx + windowSize
	if endIdx > totalItems {
		endIdx = totalItems
	}

	return startIdx, endIdx
}
