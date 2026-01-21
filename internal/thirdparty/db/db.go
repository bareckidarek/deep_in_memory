package db

import (
	"github.com/timshannon/bolthold"
)

type Db[T any] interface {
	Query(query *bolthold.Query) ([]T, error)
	Upsert(key string, value *T) error
	Delete(key string) error
	All() ([]T, error)
	Close() error
}
