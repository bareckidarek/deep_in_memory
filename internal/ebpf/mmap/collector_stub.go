//go:build !ebpf

package mmap

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
)

// Start is a stub for non-eBPF builds.
func Start(ctx context.Context, send func(tea.Msg), sharedState interface{}) (func() error, error) {
	return nil, fmt.Errorf("mmap collector not available (build without -tags ebpf)")
}
