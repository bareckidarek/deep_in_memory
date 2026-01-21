package app

import (
	"context"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/timshannon/bolthold"

	"deep_in_memory/internal/constants"
	"deep_in_memory/internal/data"
	uimsg "deep_in_memory/internal/ui/msg"
)

func TestNew(t *testing.T) {
	mock := newMockReader()
	now := time.Now()
	processData := []data.ProcessData{
		{Proc: data.Process{PID: 1234, Name: "test", StartTime: now}},
	}
	mock.allData = processData

	dataset := data.BuildDataset(processData)
	model := New(mock, dataset, time.Second, nil)

	require.NotNil(t, model)
	assert.NotNil(t, model.ui)
	assert.Equal(t, time.Second, model.refreshInterval)
}

func TestInit(t *testing.T) {
	mock := newMockReader()
	processData := []data.ProcessData{
		{Proc: data.Process{PID: 1234, Name: "test", StartTime: time.Now()}},
	}
	dataset := data.BuildDataset(processData)
	model := New(mock, dataset, 500*time.Millisecond, nil)

	cmd := model.Init()

	require.NotNil(t, cmd)
}

func TestSetTargetPIDSetter(t *testing.T) {
	mock := newMockReader()
	processData := []data.ProcessData{
		{Proc: data.Process{PID: 1234, Name: "test", StartTime: time.Now()}},
	}
	dataset := data.BuildDataset(processData)
	model := New(mock, dataset, time.Second, nil)

	called := false
	setter := func(pid int) error { called = true; return nil }

	model.SetTargetPIDSetter(setter)

	assert.NotNil(t, model.ui)
	assert.False(t, called)
}

func TestUpdate_EventMessages_NoOpWrites(t *testing.T) {
	mock := newMockReader()
	processData := []data.ProcessData{
		{Proc: data.Process{PID: 1234, Name: "test", StartTime: time.Now()}},
	}
	dataset := data.BuildDataset(processData)
	model := New(mock, dataset, time.Second, nil)

	// We only assert that Update handles messages without panicking and returns a model
	events := []interface{}{
		uimsg.ProcessExec{Timestamp: time.Now(), PID: 5678, Comm: "new_proc"},
		uimsg.ProcessExit{Timestamp: time.Now(), PID: 1234, Comm: "test"},
		uimsg.ProcessMmap{Timestamp: time.Now(), PID: 1234, TID: 1234, Addr: 0x1000000, Len: 0x1000, Comm: "test"},
		uimsg.ProcessMunmap{Timestamp: time.Now(), PID: 1234, TID: 1234, Addr: 0x1000000, Len: 0x1000, Comm: "test"},
		uimsg.ProcessBrk{Timestamp: time.Now(), PID: 1234, TID: 1234, Brk: 0, Ret: 0x2000000, Comm: "test"},
		uimsg.ProcessPageFault{Timestamp: time.Now(), PID: 1234, TID: 1234, Address: 0x1001000, ErrorCode: constants.PageFaultErrorCode, Comm: "test"},
		uimsg.ProcessSwapUnswap{Timestamp: time.Now(), PID: 1234, TID: 1234, Address: 0x1000000, IsSwap: true, ErrorCode: constants.SwapEventSuccessCode, Comm: "test"},
	}

	for _, e := range events {
		newModel, _ := model.Update(e)
		assert.NotNil(t, newModel)
	}
}

func TestUpdate_LoadSmapsRequest(t *testing.T) {
	mock := newMockReader()
	processData := []data.ProcessData{
		{Proc: data.Process{PID: 1234, Name: "test", StartTime: time.Now()}},
	}
	dataset := data.BuildDataset(processData)
	model := New(mock, dataset, time.Second, nil)

	ev := uimsg.LoadSmapsRequest{PID: 1234}

	newModel, cmd := model.Update(ev)

	assert.NotNil(t, newModel)
	assert.Nil(t, cmd)
}

func TestUpdate_RefreshMsg(t *testing.T) {
	mock := newMockReader()
	processData := []data.ProcessData{
		{Proc: data.Process{PID: 1234, Name: "test", StartTime: time.Now()}},
	}
	mock.allData = processData
	dataset := data.BuildDataset(processData)
	model := New(mock, dataset, time.Second, nil)

	newModel, cmd := model.Update(refreshMsg{})

	assert.NotNil(t, newModel)
	assert.NotNil(t, cmd)
}

func TestView(t *testing.T) {
	mock := newMockReader()
	processData := []data.ProcessData{
		{Proc: data.Process{PID: 1234, Name: "test", StartTime: time.Now()}},
	}
	dataset := data.BuildDataset(processData)
	model := New(mock, dataset, time.Second, nil)

	view := model.View()

	// View() returns tea.View, convert to string
	if canvas, ok := view.Content.(*lipgloss.Canvas); ok {
		viewStr := canvas.Render()
		assert.NotEmpty(t, viewStr)
		assert.Contains(t, viewStr, "Processes")
	}
}

func TestReload_Success(t *testing.T) {
	mock := newMockReader()
	processData := []data.ProcessData{
		{Proc: data.Process{PID: 1234, Name: "test", StartTime: time.Now()}},
	}
	mock.allData = processData
	dataset := data.BuildDataset(processData)
	model := New(mock, dataset, time.Second, nil)

	model.reload()

	assert.NotNil(t, model.ui)
}

func TestReload_DBError(t *testing.T) {
	mock := newMockReader()
	processData := []data.ProcessData{
		{Proc: data.Process{PID: 1234, Name: "test", StartTime: time.Now()}},
	}
	dataset := data.BuildDataset(processData)
	model := New(mock, dataset, time.Second, nil)

	mock.allErr = assert.AnError

	model.reload()

	assert.NotNil(t, model.ui)
}

// mockAddressDb mock for address database (kept for compatibility in other tests)
type mockAddressDb struct {
	data map[string]*data.AddressData
}

func newMockAddressDb() *mockAddressDb {
	return &mockAddressDb{
		data: make(map[string]*data.AddressData),
	}
}

func (m *mockAddressDb) Query(query *bolthold.Query) ([]data.AddressData, error) {
	result := []data.AddressData{}
	for _, v := range m.data {
		result = append(result, *v)
	}
	return result, nil
}

func (m *mockAddressDb) Upsert(key string, value *data.AddressData) error {
	m.data[key] = value
	return nil
}

func (m *mockAddressDb) Delete(key string) error {
	delete(m.data, key)
	return nil
}

func (m *mockAddressDb) All() ([]data.AddressData, error) {
	result := []data.AddressData{}
	for _, v := range m.data {
		result = append(result, *v)
	}
	return result, nil
}

func (m *mockAddressDb) Close() error {
	return nil
}

// mockReader implements persistence.Reader for tests
type mockReader struct {
	allData []data.ProcessData
	allAddr []data.AddressData
	allErr  error
}

func newMockReader() *mockReader {
	return &mockReader{}
}

func (m *mockReader) AllProcessData(ctx context.Context) ([]data.ProcessData, error) {
	if m.allErr != nil {
		return nil, m.allErr
	}
	return m.allData, nil
}

func (m *mockReader) AllAddressData(ctx context.Context) ([]data.AddressData, error) {
	if m.allErr != nil {
		return nil, m.allErr
	}
	return m.allAddr, nil
}

func (m *mockReader) Close() error { return nil }
