package model

import (
	"fmt"
	"io"

	"charm.land/bubbles/v2/list"

	"deep_in_memory/internal/constants"
	"deep_in_memory/internal/data"
	uirender "deep_in_memory/internal/ui/render"
)

// listItem implements the list.Item interface for Bubble Tea lists.
type listItem struct {
	title       string
	description string
	pid         int
}

func (i listItem) Title() string       { return i.title }
func (i listItem) Description() string { return i.description }
func (i listItem) FilterValue() string { return i.title }

func renderListItem(proc data.Process, pid int) listItem {
	title, desc := uirender.FormatListItem(proc, pid)
	return listItem{title: title, description: desc, pid: pid}
}

// Custom delegate to show selection and focus markers
type processDelegate struct {
	list.DefaultDelegate
	model *Model
}

func newProcessDelegate(m *Model) *processDelegate {
	d := list.NewDefaultDelegate()
	return &processDelegate{
		DefaultDelegate: d,
		model:           m,
	}
}

func (d *processDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	li, ok := item.(listItem)
	if !ok {
		d.DefaultDelegate.Render(w, m, index, item)
		return
	}

	isSelected := li.pid == d.model.selectedPID
	isCursor := index == m.Index()

	title, desc := uirender.RenderProcessListItem(li.Title(), li.Description(), isCursor, isSelected)
	fmt.Fprintf(w, "%s\n%s", title, desc)
}

func (m *Model) refreshList() {
	items := make([]list.Item, 0, len(m.data.Order))
	for _, pid := range m.data.Order {
		if procData, ok := m.data.Processes[pid]; ok {
			items = append(items, renderListItem(procData.Proc, pid))
		}
	}

	// Find indices for focused and selected PIDs
	focusIdx := -1
	for idx, item := range items {
		if li, ok := item.(listItem); ok {
			if li.pid == m.focusedPID {
				focusIdx = idx
				break
			}
		}
	}

	m.list.SetItems(items)

	// Restore cursor to focused PID or default to first
	if focusIdx >= 0 {
		m.list.Select(focusIdx)
	} else if len(items) > 0 {
		m.list.Select(0)
		if li, ok := items[0].(listItem); ok {
			m.focusedPID = li.pid
		}
	}

	// Check if selected process still exists, if not clear selection and target PID
	if m.selectedPID != 0 {
		found := false
		for _, item := range items {
			if li, ok := item.(listItem); ok && li.pid == m.selectedPID {
				found = true
				break
			}
		}
		if !found {
			m.selectedPID = 0 // Clear selection if process disappeared
			// Also clear target PID in eBPF if this was the monitored process
			if m.setTargetPID != nil {
				if err := m.setTargetPID(0); err != nil {
					// Only surface modal when clear operation actually fails.
					m.showModal = true
					m.modalType = constants.ModalTypeError
					m.modalMessage = fmt.Sprintf("Failed to clear target PID in eBPF: %v", err)
					m.dirty = true
				}
			}
		}
	}
}

func (m *Model) currentProcessData() (*data.ProcessData, bool) {
	pd, ok := m.data.Processes[m.selectedPID]
	return pd, ok
}
