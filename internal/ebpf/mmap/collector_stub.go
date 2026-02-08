//go:build !ebpf

package mmap

import (
	"context"
	"fmt"

	ebpfshared "deep_in_memory/internal/ebpf"
)

// Start is a stub for non-eBPF builds.
func Start(ctx context.Context, collector *ebpfshared.CentralEventCollector, sharedState interface{}) (func() error, error) {
	return nil, fmt.Errorf("mmap collector not available (build without -tags ebpf)")
}
