package persistence

import (
	"context"
	"fmt"

	"deep_in_memory/internal/data"
	storedb "deep_in_memory/internal/thirdparty/db"
)

// Writer defines the interface for writing events to persistent storage.
type Writer interface {
	WriteProcessData(ctx context.Context, key string, proc *data.ProcessData) error
	WriteAddressData(ctx context.Context, key string, addr *data.AddressData) error
	DeleteProcessData(ctx context.Context, key string) error
	DeleteAddressData(ctx context.Context, key string) error
	Close() error
}

// Reader defines the interface for reading data from persistent storage.
type Reader interface {
	AllProcessData(ctx context.Context) ([]data.ProcessData, error)
	AllAddressData(ctx context.Context) ([]data.AddressData, error)
	Close() error
}

// Storage provides both read and write access to persistent storage.
type Storage struct {
	processDB storedb.Db[data.ProcessData]
	addressDB storedb.Db[data.AddressData]
}

// Open creates a new Storage instance using BoltDB from thirdparty/db.
// Returns both Writer and Reader interfaces backed by the same storage.
func Open(processDBPath, addressDBPath string) (Writer, Reader, error) {
	processDB, err := storedb.New[data.ProcessData](processDBPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open process DB: %w", err)
	}

	addressDB, err := storedb.New[data.AddressData](addressDBPath)
	if err != nil {
		processDB.Close()
		return nil, nil, fmt.Errorf("failed to open address DB: %w", err)
	}

	storage := &Storage{
		processDB: processDB,
		addressDB: addressDB,
	}

	return storage, storage, nil
}

// WriteProcessData implements Writer interface.
func (s *Storage) WriteProcessData(ctx context.Context, key string, proc *data.ProcessData) error {
	return s.processDB.Upsert(key, proc)
}

// WriteAddressData implements Writer interface.
func (s *Storage) WriteAddressData(ctx context.Context, key string, addr *data.AddressData) error {
	return s.addressDB.Upsert(key, addr)
}

// DeleteProcessData implements Writer interface.
func (s *Storage) DeleteProcessData(ctx context.Context, key string) error {
	return s.processDB.Delete(key)
}

// DeleteAddressData implements Writer interface.
func (s *Storage) DeleteAddressData(ctx context.Context, key string) error {
	return s.addressDB.Delete(key)
}

// AllProcessData implements Reader interface.
func (s *Storage) AllProcessData(ctx context.Context) ([]data.ProcessData, error) {
	return s.processDB.All()
}

// AllAddressData implements Reader interface.
func (s *Storage) AllAddressData(ctx context.Context) ([]data.AddressData, error) {
	return s.addressDB.All()
}

// Close implements both Writer and Reader interfaces.
func (s *Storage) Close() error {
	err1 := s.processDB.Close()
	err2 := s.addressDB.Close()
	if err1 != nil {
		return err1
	}
	return err2
}
