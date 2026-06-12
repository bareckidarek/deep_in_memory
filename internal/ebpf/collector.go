package ebpf

import (
	"sync"

	"github.com/cilium/ebpf"
)

// SharedState provides thread-safe access to shared eBPF resources.
// Manages the target PID for filtering events across multiple collectors.
// Uses RWMutex to allow concurrent reads while ensuring exclusive writes.
type SharedState struct {
	mu      sync.RWMutex
	ebpfMap *ebpf.Map
}

// NewSharedState creates a new shared state instance for managing eBPF resources.
// Initializes with the provided eBPF map for target PID filtering.
func NewSharedState(ebpfMap *ebpf.Map) *SharedState {
	return &SharedState{
		ebpfMap: ebpfMap,
	}
}

// SetTargetPID safely updates the target PID for event filtering.
// Acquires exclusive lock to ensure atomic update of both local state and eBPF map.
// This controls which process events are traced by all collectors.
func (s *SharedState) SetTargetPID(pid int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Update eBPF map - assuming it's an array map with single element at index 0
	if s.ebpfMap != nil {
		return s.ebpfMap.Put(uint32(0), uint32(pid))
	}

	return nil
}

// GetEbpfMap returns the underlying eBPF map for collectors requiring direct access.
// Used by collectors that need to share the map for map replacements.
func (s *SharedState) GetEbpfMap() *ebpf.Map {
	return s.ebpfMap
}
