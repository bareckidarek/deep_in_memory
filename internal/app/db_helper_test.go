package app

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/timshannon/bolthold"

	"deep_in_memory/internal/data"
	uimsg "deep_in_memory/internal/ui/msg"
)

// --- Mock DB ---

type mockDB[T any] struct {
	store map[string]*T
}

func newMockDB[T any]() *mockDB[T] {
	return &mockDB[T]{store: make(map[string]*T)}
}

func (m *mockDB[T]) Query(query *bolthold.Query) ([]T, error) {
	// Simple PID-based query simulation: scan all values
	var results []T
	for _, v := range m.store {
		results = append(results, *v)
	}
	return results, nil
}

func (m *mockDB[T]) Upsert(key string, value *T) error {
	cp := *value
	m.store[key] = &cp
	return nil
}

func (m *mockDB[T]) Delete(key string) error {
	delete(m.store, key)
	return nil
}

func (m *mockDB[T]) All() ([]T, error) {
	var results []T
	for _, v := range m.store {
		results = append(results, *v)
	}
	return results, nil
}

func (m *mockDB[T]) Close() error {
	return nil
}

// --- PID-aware mock that supports Query filtering ---

type pidAwareMockDB struct {
	store map[string]*data.ProcessData
}

func newPIDAwareMockDB() *pidAwareMockDB {
	return &pidAwareMockDB{store: make(map[string]*data.ProcessData)}
}

func (m *pidAwareMockDB) Query(query *bolthold.Query) ([]data.ProcessData, error) {
	var results []data.ProcessData
	for _, v := range m.store {
		results = append(results, *v)
	}
	return results, nil
}

func (m *pidAwareMockDB) Upsert(key string, value *data.ProcessData) error {
	cp := *value
	m.store[key] = &cp
	return nil
}

func (m *pidAwareMockDB) Delete(key string) error {
	delete(m.store, key)
	return nil
}

func (m *pidAwareMockDB) All() ([]data.ProcessData, error) {
	var results []data.ProcessData
	for _, v := range m.store {
		results = append(results, *v)
	}
	return results, nil
}

func (m *pidAwareMockDB) Close() error {
	return nil
}

// --- Tests ---

func TestNewDBHelper(t *testing.T) {
	db := newMockDB[data.ProcessData]()
	addrDB := newMockDB[data.AddressData]()
	h := NewDBHelper(db, addrDB, nil)

	assert.NotNil(t, h)
	assert.Nil(t, h.errorCallback)
}

func TestSetErrorCallback(t *testing.T) {
	db := newMockDB[data.ProcessData]()
	addrDB := newMockDB[data.AddressData]()
	h := NewDBHelper(db, addrDB, nil)

	var received error
	h.SetErrorCallback(func(err error) { received = err })
	h.reportError(fmt.Errorf("test error"))

	assert.Equal(t, "test error", received.Error())
}

func TestReportError_NilCallback(t *testing.T) {
	db := newMockDB[data.ProcessData]()
	addrDB := newMockDB[data.AddressData]()
	h := NewDBHelper(db, addrDB, nil)

	// Should not panic when callback is nil
	h.reportError(fmt.Errorf("test"))
}

func TestPidKey(t *testing.T) {
	assert.Equal(t, "42", pidKey(42))
	assert.Equal(t, "0", pidKey(0))
	assert.Equal(t, "12345", pidKey(12345))
}

func TestHandleProcessExec(t *testing.T) {
	db := newPIDAwareMockDB()
	addrDB := newMockDB[data.AddressData]()
	h := NewDBHelper(db, addrDB, nil)

	ev := uimsg.ProcessExec{
		PID:       100,
		Comm:      "test_process",
		Timestamp: time.Now(),
	}

	err := h.HandleProcessExec(ev)
	require.NoError(t, err)

	// Verify process was stored
	stored, ok := db.store["100"]
	require.True(t, ok)
	assert.Equal(t, 100, stored.Proc.PID)
	assert.Equal(t, "test_process", stored.Proc.Name)
}

func TestHandleProcessExit(t *testing.T) {
	db := newPIDAwareMockDB()
	addrDB := newMockDB[data.AddressData]()
	h := NewDBHelper(db, addrDB, nil)

	// First create a process
	db.store["100"] = &data.ProcessData{
		Proc: data.Process{PID: 100, Name: "test"},
	}
	addrDB.store["100"] = &data.AddressData{PID: 100}

	ev := uimsg.ProcessExit{PID: 100}
	err := h.HandleProcessExit(ev)
	require.NoError(t, err)

	// Verify process was removed from both DBs
	_, ok := db.store["100"]
	assert.False(t, ok)
	_, ok = addrDB.store["100"]
	assert.False(t, ok)
}

func TestHandleMmapEvent_ProcessExists(t *testing.T) {
	db := newPIDAwareMockDB()
	addrDB := newMockDB[data.AddressData]()
	h := NewDBHelper(db, addrDB, nil)

	// Create process first
	db.store["100"] = &data.ProcessData{
		Proc:    data.Process{PID: 100, Name: "test"},
		Threads: []data.ThreadData{},
	}

	ev := uimsg.ProcessMmap{
		PID:  100,
		TID:  100,
		Addr: 0x7f0000000000,
		Len:  0x1000,
	}

	err := h.HandleMmapEvent(ev)
	require.NoError(t, err)

	// Verify mmap range was added
	stored := db.store["100"]
	require.NotNil(t, stored)
	require.Len(t, stored.Threads, 1)
	assert.Len(t, stored.Threads[0].Ranges, 1)
	assert.Equal(t, uint64(0x7f0000000000), stored.Threads[0].Ranges[0].StartVal)
}

func TestHandleMmapEvent_ProcessNotFound(t *testing.T) {
	db := newPIDAwareMockDB()
	addrDB := newMockDB[data.AddressData]()
	h := NewDBHelper(db, addrDB, nil)

	ev := uimsg.ProcessMmap{PID: 999, TID: 999, Addr: 0x1000, Len: 0x1000}
	err := h.HandleMmapEvent(ev)
	// Should return nil (ignore stale events)
	assert.NoError(t, err)
}

func TestHandleBrkEvent(t *testing.T) {
	db := newPIDAwareMockDB()
	addrDB := newMockDB[data.AddressData]()
	h := NewDBHelper(db, addrDB, nil)

	// Create process with initial heap range
	db.store["100"] = &data.ProcessData{
		Proc: data.Process{PID: 100, Name: "test"},
		Threads: []data.ThreadData{
			{TID: 100, Ranges: []data.AddrRange{
				{
					Start: "5000", End: "5000",
					StartVal: 0x5000, EndVal: 0x5000,
					Size: 0, Statuses: make(map[string]*data.StatusCount),
					Label: "[heap]",
				},
			}},
		},
	}

	ev := uimsg.ProcessBrk{PID: 100, TID: 100, Brk: 0x5000, Ret: 0x6000}
	err := h.HandleBrkEvent(ev)
	require.NoError(t, err)

	stored := db.store["100"]
	require.NotNil(t, stored)
	assert.Equal(t, uint64(0x6000), stored.Threads[0].Ranges[0].EndVal)
}

func TestHandleMunmapEvent(t *testing.T) {
	db := newPIDAwareMockDB()
	addrDB := newMockDB[data.AddressData]()
	h := NewDBHelper(db, addrDB, nil)

	// Create process with a range
	db.store["100"] = &data.ProcessData{
		Proc: data.Process{PID: 100, Name: "test"},
		Threads: []data.ThreadData{
			{TID: 100, Ranges: []data.AddrRange{
				{StartVal: 0x1000, EndVal: 0x2000, Size: 1},
				{StartVal: 0x3000, EndVal: 0x4000, Size: 1},
			}},
		},
	}

	ev := uimsg.ProcessMunmap{PID: 100, TID: 100, Addr: 0x1000, Len: 0x1000}
	err := h.HandleMunmapEvent(ev)
	require.NoError(t, err)

	stored := db.store["100"]
	require.Len(t, stored.Threads[0].Ranges, 1)
	assert.Equal(t, uint64(0x3000), stored.Threads[0].Ranges[0].StartVal)
}

func TestUpsertProcess_NilRecord(t *testing.T) {
	db := newMockDB[data.ProcessData]()
	addrDB := newMockDB[data.AddressData]()
	h := NewDBHelper(db, addrDB, nil)

	err := h.UpsertProcess(100, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot upsert nil")
}

func TestGetOrCreateProcess_Existing(t *testing.T) {
	db := newPIDAwareMockDB()
	addrDB := newMockDB[data.AddressData]()
	h := NewDBHelper(db, addrDB, nil)

	db.store["100"] = &data.ProcessData{
		Proc: data.Process{PID: 100, Name: "existing"},
	}

	proc, isNew, err := h.GetOrCreateProcess(100, "test")
	require.NoError(t, err)
	assert.False(t, isNew)
	assert.Equal(t, "existing", proc.Proc.Name)
}

func TestGetOrCreateProcess_New(t *testing.T) {
	db := newPIDAwareMockDB()
	addrDB := newMockDB[data.AddressData]()
	h := NewDBHelper(db, addrDB, nil)

	proc, isNew, err := h.GetOrCreateProcess(100, "new_proc")
	require.NoError(t, err)
	assert.True(t, isNew)
	assert.Equal(t, 100, proc.Proc.PID)
	assert.Equal(t, "new_proc", proc.Proc.Name)
}
