package data

import "sort"

// SortRanges orders address ranges for consistent display in the UI.
func SortRanges(ranges []AddrRange) {
	sort.SliceStable(ranges, func(i, j int) bool {
		return ranges[i].StartVal < ranges[j].StartVal
	})
}
