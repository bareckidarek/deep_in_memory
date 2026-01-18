//go:build !ebpf

package page_fault

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// Start is a stub for when eBPF is not enabled. Returns an error indicating no support.
func Start(_ context.Context, _ func(tea.Msg), _ interface{}) (func() error, error) {
	return nil, fmt.Errorf("eBPF page fault collector requires build tag 'ebpf'")
}
