package data

// AddressData stores page offsets for a process in a separate database.
// This keeps the main ProcessData lightweight for UI rendering.
type AddressData struct {
	PID     int                 `json:"pid"`
	Threads []ThreadAddressData `json:"threads"`
}

// ThreadAddressData stores page offsets for a thread's memory ranges.
type ThreadAddressData struct {
	TID    int                `json:"tid"`
	Ranges []RangeAddressData `json:"ranges"`
}

// RangeAddressData stores page offsets for a specific memory range.
// Identified by StartVal to match with AddrRange in main database.
type RangeAddressData struct {
	StartVal uint64                    `json:"start_val"` // Identifies the range
	Statuses map[string]*StatusOffsets `json:"statuses"`  // Status -> page offsets
}

// StatusOffsets contains only the page offset values.
type StatusOffsets struct {
	Value []uint64 `json:"value"` // List of page offsets (in pages, not bytes) from range start
}

// AddPageOffset adds a page offset to a specific status in AddressData.
// Operates directly on AddressData structure without touching ProcessData.
func (ad *AddressData) AddPageOffset(tid int, rangeStartVal uint64, pageOffset uint64, status string) {
	if ad.Threads == nil {
		ad.Threads = []ThreadAddressData{}
	}

	// Find or create thread
	var targetThread *ThreadAddressData
	for i := range ad.Threads {
		if ad.Threads[i].TID == tid {
			targetThread = &ad.Threads[i]
			break
		}
	}

	if targetThread == nil {
		ad.Threads = append(ad.Threads, ThreadAddressData{
			TID:    tid,
			Ranges: []RangeAddressData{},
		})
		targetThread = &ad.Threads[len(ad.Threads)-1]
	}

	// Find or create range
	var targetRange *RangeAddressData
	for i := range targetThread.Ranges {
		if targetThread.Ranges[i].StartVal == rangeStartVal {
			targetRange = &targetThread.Ranges[i]
			break
		}
	}

	if targetRange == nil {
		targetThread.Ranges = append(targetThread.Ranges, RangeAddressData{
			StartVal: rangeStartVal,
			Statuses: make(map[string]*StatusOffsets),
		})
		targetRange = &targetThread.Ranges[len(targetThread.Ranges)-1]
	}

	if targetRange.Statuses == nil {
		targetRange.Statuses = make(map[string]*StatusOffsets)
	}

	// Remove page from all other statuses first (a page can only be in one status)
	for otherStatus := range targetRange.Statuses {
		if otherStatus != status {
			ad.removePageOffsetFromStatus(targetRange, pageOffset, otherStatus)
		}
	}

	// Initialize status if needed
	if targetRange.Statuses[status] == nil {
		targetRange.Statuses[status] = &StatusOffsets{Value: []uint64{}}
	}

	// Check if page already exists in this status
	for _, offset := range targetRange.Statuses[status].Value {
		if offset == pageOffset {
			return // Already in this status
		}
	}

	// Add page to status
	targetRange.Statuses[status].Value = append(targetRange.Statuses[status].Value, pageOffset)
}

// removePageOffsetFromStatus removes a page offset from a specific status in a range
func (ad *AddressData) removePageOffsetFromStatus(r *RangeAddressData, pageOffset uint64, status string) {
	if r.Statuses == nil || r.Statuses[status] == nil {
		return
	}

	// Find and remove page
	for i, offset := range r.Statuses[status].Value {
		if offset == pageOffset {
			// Remove by swapping with last element and truncating
			lastIdx := len(r.Statuses[status].Value) - 1
			r.Statuses[status].Value[i] = r.Statuses[status].Value[lastIdx]
			r.Statuses[status].Value = r.Statuses[status].Value[:lastIdx]
			return
		}
	}
}

// GetStatusCounts returns counts for all statuses in a specific range.
// Used to sync Count values to ProcessData.
func (ad *AddressData) GetStatusCounts(tid int, rangeStartVal uint64) map[string]int {
	counts := make(map[string]int)

	// Find thread
	for _, thread := range ad.Threads {
		if tid != 0 && thread.TID != tid {
			continue
		}

		// Find range
		for _, r := range thread.Ranges {
			if r.StartVal == rangeStartVal {
				// Count values in each status
				for status, offsets := range r.Statuses {
					counts[status] += len(offsets.Value)
				}
			}
		}
	}

	return counts
}
