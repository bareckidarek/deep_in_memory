//go:build !ebpf

package collector

import (
	"context"
	"fmt"

	"deep_in_memory/internal/app"
	"deep_in_memory/internal/persistence"
)

// CollectorConfig holds configuration for the collector
type CollectorConfig struct {
	ErrorCallback func(error)
}

// Collector manages eBPF event collection and persistence (stub version)
type Collector struct {
	logAggregator *app.LogAggregator
	targetPID     int
}

// StartCollector returns an error indicating eBPF is not available
func StartCollector(ctx context.Context, cfg CollectorConfig, writer persistence.Writer, reader persistence.Reader) (*Collector, func() error, error) {
	return nil, nil, fmt.Errorf("eBPF support not enabled - rebuild with '-tags ebpf'")
}

// SetTargetPID stub
func (c *Collector) SetTargetPID(pid int) error {
	return fmt.Errorf("eBPF not available")
}

// GetLogAggregator stub
func (c *Collector) GetLogAggregator() *app.LogAggregator {
	return nil
}
