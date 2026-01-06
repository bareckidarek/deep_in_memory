//go:build !ebpf

package sched

import (
	"context"
	"errors"

	tea "github.com/charmbracelet/bubbletea"
)

// Start is a stub when eBPF build tag is not enabled or eBPF assets are missing.
func Start(_ context.Context, _ func(tea.Msg)) (func() error, func(int) error, error) {
	return func() error { return nil }, func(int) error { return nil }, errors.New("eBPF support not built; run go generate ./internal/ebpf/sched and build with -tags ebpf")
}
