//go:build !ebpf

package swap

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/cilium/ebpf"
)

// Start is a stub for non-eBPF builds.
func Start(ctx context.Context, send func(tea.Msg), targetPidMap *ebpf.Map) (func() error, error) {
	return nil, fmt.Errorf("swap collector not available (build without -tags ebpf)")
}
