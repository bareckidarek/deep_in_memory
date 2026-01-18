package model

import (
	"testing"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"deep_in_memory/internal/data"
	uimsg "deep_in_memory/internal/ui/msg"
)

func TestNew(t *testing.T) {
	now := time.Now()
	processes := []data.ProcessData{
		{Proc: data.Process{PID: 1001, Name: "proc1", StartTime: now}},
		{Proc: data.Process{PID: 1002, Name: "proc2", StartTime: now}},
	}
	dataset := data.BuildDataset(processes)
	model := New(dataset)

	require.NotNil(t, model)
	assert.Equal(t, 0, model.selectedPID)
	assert.Equal(t, 1001, model.focusedPID)
	assert.NotNil(t, model.list)
}

func TestNew_EmptyDataset(t *testing.T) {
	processes := []data.ProcessData{}
	dataset := data.BuildDataset(processes)
	model := New(dataset)

	require.NotNil(t, model)
	assert.Equal(t, 0, model.selectedPID)
	assert.Equal(t, 0, model.focusedPID)
}

func TestSetDataset(t *testing.T) {
	now := time.Now()
	processes := []data.ProcessData{
		{Proc: data.Process{PID: 1001, Name: "proc1", StartTime: now}},
	}
	dataset := data.BuildDataset(processes)
	model := New(dataset)

	newProcesses := []data.ProcessData{
		{Proc: data.Process{PID: 1003, Name: "proc3", StartTime: now}},
		{Proc: data.Process{PID: 1002, Name: "proc2", StartTime: now}},
	}
	newDataset := data.BuildDataset(newProcesses)
	model.SetDataset(newDataset)

	assert.Equal(t, 1002, model.focusedPID)
	assert.Equal(t, 0, model.selectedPID)
}

func TestSetTargetPIDSetter(t *testing.T) {
	processes := []data.ProcessData{
		{Proc: data.Process{PID: 1001, Name: "proc1", StartTime: time.Now()}},
	}
	dataset := data.BuildDataset(processes)
	model := New(dataset)

	called := false
	setter := func(pid int) error { called = true; return nil }

	model.SetTargetPIDSetter(setter)

	assert.NotNil(t, model.setTargetPID)
	assert.False(t, called)
}

func TestInit(t *testing.T) {
	processes := []data.ProcessData{
		{Proc: data.Process{PID: 1001, Name: "proc1", StartTime: time.Now()}},
	}
	dataset := data.BuildDataset(processes)
	model := New(dataset)

	cmd := model.Init()

	assert.Nil(t, cmd)
}

func TestUpdate_KeyMsgQuit(t *testing.T) {
	processes := []data.ProcessData{
		{Proc: data.Process{PID: 1001, Name: "proc1", StartTime: time.Now()}},
	}
	dataset := data.BuildDataset(processes)
	model := New(dataset)

	newModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	assert.NotNil(t, newModel)
	assert.NotNil(t, cmd)
}

func TestUpdate_KeyMsgCtrlC(t *testing.T) {
	processes := []data.ProcessData{
		{Proc: data.Process{PID: 1001, Name: "proc1", StartTime: time.Now()}},
	}
	dataset := data.BuildDataset(processes)
	model := New(dataset)

	newModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})

	assert.NotNil(t, newModel)
	assert.NotNil(t, cmd)
}

func TestUpdate_KeyMsgEnter(t *testing.T) {
	processes := []data.ProcessData{
		{Proc: data.Process{PID: 1001, Name: "proc1", StartTime: time.Now()}},
	}
	dataset := data.BuildDataset(processes)
	model := New(dataset)

	var called bool
	var calledPid int
	model.setTargetPID = func(pid int) error {
		called = true
		calledPid = pid
		return nil
	}

	newModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})

	assert.NotNil(t, newModel)
	assert.NotNil(t, cmd)
	assert.True(t, called)
	assert.Equal(t, 1001, calledPid)
}

func TestUpdate_KeyMsgNextPage(t *testing.T) {
	processes := []data.ProcessData{
		{Proc: data.Process{PID: 1001, Name: "proc1", StartTime: time.Now()}},
	}
	dataset := data.BuildDataset(processes)
	model := New(dataset)
	model.selectedPID = 1001
	model.rangesPage = 0

	newModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})

	assert.NotNil(t, newModel)
	assert.Nil(t, cmd)
	assert.Equal(t, 1, model.rangesPage)
}

func TestUpdate_KeyMsgPrevPage(t *testing.T) {
	processes := []data.ProcessData{
		{Proc: data.Process{PID: 1001, Name: "proc1", StartTime: time.Now()}},
	}
	dataset := data.BuildDataset(processes)
	model := New(dataset)
	model.selectedPID = 1001
	model.rangesPage = 2

	newModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})

	assert.NotNil(t, newModel)
	assert.Nil(t, cmd)
	assert.Equal(t, 1, model.rangesPage)
}

func TestUpdate_KeyMsgPrevPageAtZero(t *testing.T) {
	processes := []data.ProcessData{
		{Proc: data.Process{PID: 1001, Name: "proc1", StartTime: time.Now()}},
	}
	dataset := data.BuildDataset(processes)
	model := New(dataset)
	model.selectedPID = 1001
	model.rangesPage = 0

	newModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})

	assert.NotNil(t, newModel)
	assert.Nil(t, cmd)
	assert.Equal(t, 0, model.rangesPage)
}

func TestUpdate_WindowSizeMsg(t *testing.T) {
	processes := []data.ProcessData{
		{Proc: data.Process{PID: 1001, Name: "proc1", StartTime: time.Now()}},
	}
	dataset := data.BuildDataset(processes)
	model := New(dataset)

	newModel, cmd := model.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	assert.NotNil(t, newModel)
	assert.Nil(t, cmd)
	assert.Equal(t, 100, model.width)
	assert.Equal(t, 50, model.height)
}

func TestUpdate_MouseMsgWheel(t *testing.T) {
	processes := []data.ProcessData{
		{Proc: data.Process{PID: 1001, Name: "proc1", StartTime: time.Now()}},
	}
	dataset := data.BuildDataset(processes)
	model := New(dataset)
	model.selectedPID = 1001
	model.width = 100
	model.height = 50

	newModel, _ := model.Update(tea.MouseMsg{Type: tea.MouseWheelDown, X: 50, Y: 10})

	assert.NotNil(t, newModel)
}

func TestView(t *testing.T) {
	processes := []data.ProcessData{
		{Proc: data.Process{PID: 1001, Name: "proc1", StartTime: time.Now()}},
	}
	dataset := data.BuildDataset(processes)
	model := New(dataset)

	view := model.View()

	assert.NotEmpty(t, view)
	assert.Contains(t, view, "Processes")
}

func TestCurrentProcessData(t *testing.T) {
	now := time.Now()
	processes := []data.ProcessData{
		{Proc: data.Process{PID: 1001, Name: "proc1", StartTime: now}},
	}
	dataset := data.BuildDataset(processes)
	model := New(dataset)
	model.selectedPID = 1001

	pd, ok := model.currentProcessData()

	assert.True(t, ok)
	assert.NotNil(t, pd)
	assert.Equal(t, 1001, pd.Proc.PID)
}

func TestCurrentProcessData_NotFound(t *testing.T) {
	now := time.Now()
	processes := []data.ProcessData{
		{Proc: data.Process{PID: 1001, Name: "proc1", StartTime: now}},
	}
	dataset := data.BuildDataset(processes)
	model := New(dataset)
	model.selectedPID = 9999

	pd, ok := model.currentProcessData()

	assert.False(t, ok)
	assert.Nil(t, pd)
}

func TestLeftPaneWidth(t *testing.T) {
	processes := []data.ProcessData{
		{Proc: data.Process{PID: 1001, Name: "proc1", StartTime: time.Now()}},
	}
	dataset := data.BuildDataset(processes)
	model := New(dataset)

	width := model.leftPaneWidth()

	assert.Greater(t, width, 0)
}

func TestRefreshList(t *testing.T) {
	now := time.Now()
	processes := []data.ProcessData{
		{Proc: data.Process{PID: 1001, Name: "proc1", StartTime: now}},
	}
	dataset := data.BuildDataset(processes)
	model := New(dataset)
	model.focusedPID = 1001

	newProcesses := []data.ProcessData{
		{Proc: data.Process{PID: 1002, Name: "proc2", StartTime: now}},
	}
	newDataset := data.BuildDataset(newProcesses)
	model.SetDataset(newDataset)

	assert.Equal(t, 1002, model.focusedPID)
}

func TestRenderListItem(t *testing.T) {
	proc := data.Process{
		PID:       1234,
		Name:      "test_proc",
		StartTime: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
	}
	item := renderListItem(proc, 1234)

	assert.Equal(t, 1234, item.pid)
	assert.Contains(t, item.title, "test_proc")
	assert.Contains(t, item.title, "1234")
	assert.Contains(t, item.description, "12:00:00")
}

func TestListItemInterface(t *testing.T) {
	proc := data.Process{
		PID:       1234,
		Name:      "test_proc",
		StartTime: time.Now(),
	}
	item := renderListItem(proc, 1234)

	assert.Equal(t, item.Title(), item.title)
	assert.Equal(t, item.Description(), item.description)
	assert.Equal(t, item.FilterValue(), item.title)
}

func TestSyncLayout_NoSelection(t *testing.T) {
	processes := []data.ProcessData{
		{Proc: data.Process{PID: 1001, Name: "proc1", StartTime: time.Now()}},
	}
	dataset := data.BuildDataset(processes)
	model := New(dataset)
	model.width = 100
	model.height = 50

	model.syncLayout()

	assert.Equal(t, "", model.viewportVirtual.View())
	assert.Equal(t, "", model.viewportPhysical.View())
}

func TestUpdate_ApplicationError(t *testing.T) {
	processes := []data.ProcessData{
		{Proc: data.Process{PID: 1001, Name: "proc1", StartTime: time.Now()}},
	}
	dataset := data.BuildDataset(processes)
	model := New(dataset)

	newModel, cmd := model.Update(uimsg.ApplicationError{
		Message:  "Test error message",
		Severity: "warning",
	})

	assert.NotNil(t, newModel)
	assert.Nil(t, cmd)
	assert.NotNil(t, model.err)
	assert.Contains(t, model.err.Error(), "warning")
	assert.Contains(t, model.err.Error(), "Test error message")
}

func TestSyncLayout_WithSelection(t *testing.T) {
	processes := []data.ProcessData{
		{Proc: data.Process{PID: 1001, Name: "proc1", StartTime: time.Now()}},
	}
	dataset := data.BuildDataset(processes)
	model := New(dataset)
	model.selectedPID = 1001
	model.width = 100
	model.height = 50

	model.syncLayout()

	assert.NotEmpty(t, model.viewportVirtual.View())
}
