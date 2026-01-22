package app

import (
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"deep_in_memory/internal/constants"
	"deep_in_memory/internal/data"
	uimsg "deep_in_memory/internal/ui/msg"
)

func TestNew(t *testing.T) {
	mock := newMockDb()
	now := time.Now()
	processData := []data.ProcessData{
		{Proc: data.Process{PID: 1234, Name: "test", StartTime: now}},
	}
	mock.allData = processData

	dataset := data.BuildDataset(processData)
	model := New(mock, dataset, time.Second)

	require.NotNil(t, model)
	assert.NotNil(t, model.ui)
	assert.Equal(t, mock, model.db)
	assert.Equal(t, time.Second, model.refreshInterval)
	assert.NotNil(t, model.dbHelper)
}

func TestInit(t *testing.T) {
	mock := newMockDb()
	processData := []data.ProcessData{
		{Proc: data.Process{PID: 1234, Name: "test", StartTime: time.Now()}},
	}
	dataset := data.BuildDataset(processData)
	model := New(mock, dataset, 500*time.Millisecond)

	cmd := model.Init()

	require.NotNil(t, cmd)
}

func TestSetTargetPIDSetter(t *testing.T) {
	mock := newMockDb()
	processData := []data.ProcessData{
		{Proc: data.Process{PID: 1234, Name: "test", StartTime: time.Now()}},
	}
	dataset := data.BuildDataset(processData)
	model := New(mock, dataset, time.Second)

	called := false
	setter := func(pid int) error { called = true; return nil }

	model.SetTargetPIDSetter(setter)

	assert.NotNil(t, model.ui)
	assert.False(t, called)
}

func TestUpdate_ProcessExec(t *testing.T) {
	mock := newMockDb()
	processData := []data.ProcessData{
		{Proc: data.Process{PID: 1234, Name: "test", StartTime: time.Now()}},
	}
	dataset := data.BuildDataset(processData)
	model := New(mock, dataset, time.Second)

	ev := uimsg.ProcessExec{
		Timestamp: time.Now(),
		PID:       5678,
		Comm:      "new_proc",
	}

	newModel, cmd := model.Update(ev)

	assert.NotNil(t, newModel)
	assert.Nil(t, cmd)
	stored := mock.data["5678"]
	require.NotNil(t, stored)
	assert.Equal(t, 5678, stored.Proc.PID)
	assert.Equal(t, "new_proc", stored.Proc.Name)
}

func TestUpdate_ProcessExit(t *testing.T) {
	mock := newMockDb()
	processData := []data.ProcessData{
		{Proc: data.Process{PID: 1234, Name: "test", StartTime: time.Now()}},
	}
	mock.allData = processData
	dataset := data.BuildDataset(processData)
	model := New(mock, dataset, time.Second)

	ev := uimsg.ProcessExit{
		Timestamp: time.Now(),
		PID:       1234,
		Comm:      "test",
	}

	newModel, cmd := model.Update(ev)

	assert.NotNil(t, newModel)
	assert.Nil(t, cmd)
	_, exists := mock.data["1234"]
	assert.False(t, exists)
}

func TestUpdate_ProcessMmap(t *testing.T) {
	originalPageSize := data.PageSizeBytes
	defer func() { data.PageSizeBytes = originalPageSize }()
	data.PageSizeBytes = 4096

	mock := newMockDb()
	processData := []data.ProcessData{
		{Proc: data.Process{PID: 1234, Name: "test", StartTime: time.Now()}},
	}
	mock.allData = processData
	dataset := data.BuildDataset(processData)
	model := New(mock, dataset, time.Second)

	ev := uimsg.ProcessMmap{
		Timestamp: time.Now(),
		PID:       1234,
		TID:       1234,
		Addr:      0x1000000,
		Len:       0x1000,
		Comm:      "test",
	}

	newModel, cmd := model.Update(ev)

	assert.NotNil(t, newModel)
	assert.Nil(t, cmd)
	stored := mock.data["1234"]
	require.NotNil(t, stored)
	assert.Len(t, stored.Threads, 1)
}

func TestUpdate_ProcessMunmap(t *testing.T) {
	originalPageSize := data.PageSizeBytes
	defer func() { data.PageSizeBytes = originalPageSize }()
	data.PageSizeBytes = 4096

	thread := data.ThreadData{
		TID: 1234,
		Ranges: []data.AddrRange{
			{Start: "1000000", End: "1001000", StartVal: 0x1000000, EndVal: 0x1001000, Size: 1},
		},
	}
	mock := newMockDb()
	processData := []data.ProcessData{
		{Proc: data.Process{PID: 1234, Name: "test", StartTime: time.Now()}, Threads: []data.ThreadData{thread}},
	}
	mock.allData = processData
	dataset := data.BuildDataset(processData)
	model := New(mock, dataset, time.Second)

	ev := uimsg.ProcessMunmap{
		Timestamp: time.Now(),
		PID:       1234,
		TID:       1234,
		Addr:      0x1000000,
		Len:       0x1000,
		Comm:      "test",
	}

	newModel, cmd := model.Update(ev)

	assert.NotNil(t, newModel)
	assert.Nil(t, cmd)
	stored := mock.data["1234"]
	require.NotNil(t, stored)
	assert.Len(t, stored.Threads, 1)
}

func TestUpdate_ProcessBrk(t *testing.T) {
	originalPageSize := data.PageSizeBytes
	defer func() { data.PageSizeBytes = originalPageSize }()
	data.PageSizeBytes = 4096

	mock := newMockDb()
	processData := []data.ProcessData{
		{Proc: data.Process{PID: 1234, Name: "test", StartTime: time.Now()}},
	}
	mock.allData = processData
	dataset := data.BuildDataset(processData)
	model := New(mock, dataset, time.Second)

	ev := uimsg.ProcessBrk{
		Timestamp: time.Now(),
		PID:       1234,
		TID:       1234,
		Brk:       0,
		Ret:       0x2000000,
		Comm:      "test",
	}

	newModel, cmd := model.Update(ev)

	assert.NotNil(t, newModel)
	assert.Nil(t, cmd)
	stored := mock.data["1234"]
	require.NotNil(t, stored)
	assert.Len(t, stored.Threads, 1)
}

func TestUpdate_ProcessPageFault(t *testing.T) {
	originalPageSize := data.PageSizeBytes
	defer func() { data.PageSizeBytes = originalPageSize }()
	data.PageSizeBytes = 4096

	thread := data.ThreadData{
		TID: 1234,
		Ranges: []data.AddrRange{
			{Start: "1000000", End: "1002000", StartVal: 0x1000000, EndVal: 0x1002000, Size: 2},
		},
	}
	mock := newMockDb()
	processData := []data.ProcessData{
		{Proc: data.Process{PID: 1234, Name: "test", StartTime: time.Now()}, Threads: []data.ThreadData{thread}},
	}
	mock.allData = processData
	dataset := data.BuildDataset(processData)
	model := New(mock, dataset, time.Second)

	ev := uimsg.ProcessPageFault{
		Timestamp: time.Now(),
		PID:       1234,
		TID:       1234,
		Address:   0x1001000,
		ErrorCode: constants.PageFaultErrorCode,
		Comm:      "test",
	}

	newModel, cmd := model.Update(ev)

	assert.NotNil(t, newModel)
	assert.Nil(t, cmd)
	stored := mock.data["1234"]
	require.NotNil(t, stored)
}

func TestUpdate_ProcessSwapUnswap(t *testing.T) {
	originalPageSize := data.PageSizeBytes
	defer func() { data.PageSizeBytes = originalPageSize }()
	data.PageSizeBytes = 4096

	thread := data.ThreadData{
		TID: 1234,
		Ranges: []data.AddrRange{
			{
				Start:    "1000000",
				End:      "1002000",
				StartVal: 0x1000000,
				EndVal:   0x1002000,
				Size:     2,
				Statuses: map[string]*data.StatusCount{"INIT": {Count: 1}},
			},
		},
	}
	mock := newMockDb()
	processData := []data.ProcessData{
		{Proc: data.Process{PID: 1234, Name: "test", StartTime: time.Now()}, Threads: []data.ThreadData{thread}},
	}
	mock.allData = processData
	dataset := data.BuildDataset(processData)
	model := New(mock, dataset, time.Second)

	ev := uimsg.ProcessSwapUnswap{
		Timestamp: time.Now(),
		PID:       1234,
		TID:       1234,
		Address:   0x1000000,
		IsSwap:    true,
		ErrorCode: constants.SwapEventSuccessCode,
		Comm:      "test",
	}

	newModel, cmd := model.Update(ev)

	assert.NotNil(t, newModel)
	assert.Nil(t, cmd)
}

func TestUpdate_LoadSmapsRequest(t *testing.T) {
	mock := newMockDb()
	processData := []data.ProcessData{
		{Proc: data.Process{PID: 1234, Name: "test", StartTime: time.Now()}},
	}
	dataset := data.BuildDataset(processData)
	model := New(mock, dataset, time.Second)

	ev := uimsg.LoadSmapsRequest{PID: 1234}

	newModel, cmd := model.Update(ev)

	assert.NotNil(t, newModel)
	assert.Nil(t, cmd)
}

func TestUpdate_RefreshMsg(t *testing.T) {
	mock := newMockDb()
	processData := []data.ProcessData{
		{Proc: data.Process{PID: 1234, Name: "test", StartTime: time.Now()}},
	}
	mock.allData = processData
	dataset := data.BuildDataset(processData)
	model := New(mock, dataset, time.Second)

	newModel, cmd := model.Update(refreshMsg{})

	assert.NotNil(t, newModel)
	assert.NotNil(t, cmd)
}

func TestView(t *testing.T) {
	mock := newMockDb()
	processData := []data.ProcessData{
		{Proc: data.Process{PID: 1234, Name: "test", StartTime: time.Now()}},
	}
	dataset := data.BuildDataset(processData)
	model := New(mock, dataset, time.Second)

	view := model.View()

	// View() returns tea.View, convert to string
	if canvas, ok := view.Content.(*lipgloss.Canvas); ok {
		viewStr := canvas.Render()
		assert.NotEmpty(t, viewStr)
		assert.Contains(t, viewStr, "Processes")
	}
}

func TestReload_Success(t *testing.T) {
	mock := newMockDb()
	processData := []data.ProcessData{
		{Proc: data.Process{PID: 1234, Name: "test", StartTime: time.Now()}},
	}
	mock.allData = processData
	dataset := data.BuildDataset(processData)
	model := New(mock, dataset, time.Second)

	model.reload()

	assert.NotNil(t, model.ui)
}

func TestReload_DBError(t *testing.T) {
	mock := newMockDb()
	processData := []data.ProcessData{
		{Proc: data.Process{PID: 1234, Name: "test", StartTime: time.Now()}},
	}
	dataset := data.BuildDataset(processData)
	model := New(mock, dataset, time.Second)

	mock.allErr = assert.AnError

	model.reload()

	assert.NotNil(t, model.ui)
}
