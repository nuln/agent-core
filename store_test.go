package agent

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.etcd.io/bbolt"
)

func TestScopedStoreProvider(t *testing.T) {
	tmpFile := t.TempDir() + "/test.db"
	db, err := bbolt.Open(tmpFile, 0600, nil)
	assert.NoError(t, err)
	defer func() { _ = db.Close() }()
	defer func() { _ = os.Remove(tmpFile) }()

	// Case 1: Root-level provider
	p := NewScopedStoreProvider(db, "plugins/lark")

	// GetStore with a name
	s1, err := p.GetStore("messages")
	assert.NoError(t, err)
	assert.Equal(t, []byte("plugins/lark/messages"), s1.(*BoltStore).bucket)

	// GetStore with empty name should return the prefix itself
	s2, err := p.GetStore("")
	assert.NoError(t, err)
	assert.Equal(t, []byte("plugins/lark"), s2.(*BoltStore).bucket)

	// Case 2: Deeply nested provider
	p2 := NewScopedStoreProvider(db, "plugins/llm/codex/") // trailing slash
	s3, err := p2.GetStore("history")
	assert.NoError(t, err)
	assert.Equal(t, []byte("plugins/llm/codex/history"), s3.(*BoltStore).bucket)

	// Verify isolation (simple check)
	err = s1.Put([]byte("k1"), []byte("v1"))
	assert.NoError(t, err)

	val, err := s3.Get([]byte("k1"))
	assert.NoError(t, err)
	assert.Nil(t, val, "Data should not leak between buckets")

	val, err = s1.Get([]byte("k1"))
	assert.NoError(t, err)
	assert.Equal(t, []byte("v1"), val)
}
