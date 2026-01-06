package db

import (
	"deep_in_memory/internal/data"
	"fmt"

	"github.com/timshannon/bolthold"
	"go.etcd.io/bbolt"
)

type BoltDb[T data.ProcessData] struct {
	store *bolthold.Store
}

func New[T data.ProcessData](dbname string) (Db[T], error) {
	database, err := bolthold.Open(fmt.Sprintf("%s.db", dbname), 0600, nil)
	if err != nil {
		return nil, err
	}
	return &BoltDb[T]{database}, nil

}

func (boltDb *BoltDb[T]) Close() error {
	return boltDb.store.Close()
}

func (boltDb *BoltDb[T]) Query(query *bolthold.Query) ([]T, error) {
	var value []T
	return value, boltDb.store.Bolt().View(func(tx *bbolt.Tx) error {
		err := boltDb.store.TxFind(tx, &value, query)
		if err != nil {
			return err
		}
		return nil
	})
}

func (boltDb *BoltDb[T]) All() ([]T, error) {
	return boltDb.Query(bolthold.Where("Proc.PID").Ge(0))
}

func (boltDb *BoltDb[T]) Upsert(key string, value *T) error {
	return boltDb.store.Bolt().Update(
		func(tx *bbolt.Tx) error {
			return boltDb.store.TxUpsert(tx, key, value)
		})
}

func (boltDb *BoltDb[T]) Delete(key string) error {
	var zero T
	return boltDb.store.Bolt().Update(
		func(tx *bbolt.Tx) error {
			return boltDb.store.TxDelete(tx, key, zero)
		})
}
