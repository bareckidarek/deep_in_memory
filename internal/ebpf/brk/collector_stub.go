//go:build !ebpf

package brk

import (
	"context"
	"fmt"

	ebpfshared "deep_in_memory/internal/ebpf"
)

// Start is a stub for non-eBPF builds.
func Start(ctx context.Context, collector *ebpfshared.CentralEventCollector, sharedState interface{}) (func() error, error) {
	return nil, fmt.Errorf("brk collector not available (build without -tags ebpf)")
}
