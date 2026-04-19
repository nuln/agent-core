package agent

import (
	"fmt"

	"go.etcd.io/bbolt"
)

// BoltStore implements KVStore using bbolt.
type BoltStore struct {
	db     *bbolt.DB
	bucket []byte
}

func NewBoltStore(db *bbolt.DB, bucketName string) (*BoltStore, error) {
	bucket := []byte(bucketName)
	err := db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucket)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("failed to ensure bucket %q: %w", bucketName, err)
	}
	return &BoltStore{db: db, bucket: bucket}, nil
}

func (s *BoltStore) Get(key []byte) ([]byte, error) {
	var value []byte
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(s.bucket)
		if b == nil {
			return nil
		}
		v := b.Get(key)
		if v != nil {
			value = make([]byte, len(v))
			copy(value, v)
		}
		return nil
	})
	return value, err
}

func (s *BoltStore) Put(key, value []byte) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(s.bucket)
		if b == nil {
			var err error
			b, err = tx.CreateBucketIfNotExists(s.bucket)
			if err != nil {
				return err
			}
		}
		return b.Put(key, value)
	})
}

func (s *BoltStore) Delete(key []byte) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(s.bucket)
		if b == nil {
			return nil
		}
		return b.Delete(key)
	})
}

func (s *BoltStore) List() (map[string][]byte, error) {
	res := make(map[string][]byte)
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(s.bucket)
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			val := make([]byte, len(v))
			copy(val, v)
			res[string(k)] = val
			return nil
		})
	})
	return res, err
}

// ScopedStoreProvider implements KVStoreProvider using bbolt with namespace isolation.
type ScopedStoreProvider struct {
	db     *bbolt.DB
	prefix string
}

func NewScopedStoreProvider(db *bbolt.DB, prefix string) *ScopedStoreProvider {
	return &ScopedStoreProvider{
		db:     db,
		prefix: prefix,
	}
}

func (p *ScopedStoreProvider) GetStore(name string) (KVStore, error) {
	bucketName := p.prefix
	if name != "" {
		if bucketName != "" && bucketName[len(bucketName)-1] != '/' {
			bucketName += "/"
		}
		bucketName += name
	}
	return NewBoltStore(p.db, bucketName)
}
