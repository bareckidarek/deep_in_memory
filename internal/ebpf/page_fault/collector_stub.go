//go:build !ebpf

package page_fault

import (
	"context"
	"fmt"

	ebpfshared "deep_in_memory/internal/ebpf"
)

// Start is a stub for when eBPF is not enabled. Returns an error indicating no support.
func Start(_ context.Context, _ *ebpfshared.CentralEventCollector, _ interface{}) (func() error, error) {
	return nil, fmt.Errorf("eBPF page fault collector requires build tag 'ebpf'")
}
