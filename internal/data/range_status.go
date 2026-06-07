package data

import "sort"

// addPageToStatus increments the count for a specific status.
// The page is removed from all other statuses first (a page can only be in one status).
// pageOffset is in pages (not bytes) from the range start.
func (r *AddrRange) addPageToStatus(pageOffset uint64, status string) {
	if r.Statuses == nil {
		r.Statuses = make(map[string]*StatusCount)
	}

	// Remove page from all other statuses
	for otherStatus := range r.Statuses {
		if otherStatus != status {
			r.removePageFromStatus(otherStatus)
		}
	}

	// Initialize status if needed
	if r.Statuses[status] == nil {
		r.Statuses[status] = &StatusCount{}
	}

	// Increment count
	r.Statuses[status].Count++
}

// removePageFromStatus decrements the count for a specific status.
func (r *AddrRange) removePageFromStatus(status string) {
	if r.Statuses == nil || r.Statuses[status] == nil {
		return
	}

	// Decrement count (but don't go below 0)
	if r.Statuses[status].Count > 0 {
		r.Statuses[status].Count--
	}
}

// SortRanges orders address ranges for consistent display in the UI.
func SortRanges(ranges []AddrRange) {
	sort.SliceStable(ranges, func(i, j int) bool {
		return ranges[i].StartVal < ranges[j].StartVal
	})
}
