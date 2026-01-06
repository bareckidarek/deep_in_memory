package db

import (
	"deep_in_memory/internal/data"

	"github.com/timshannon/bolthold"
)

type Db[T data.ProcessData] interface {
	Query(query *bolthold.Query) ([]T, error)
	QueryMatchFunc(field string, matchFunc func(*T) (bool, error)) bolthold.Query
	Read(key string) (T, error)
	Upsert(key string, value *T) error
	Delete(key string) error
	All() ([]T, error)
	Close() error
}
