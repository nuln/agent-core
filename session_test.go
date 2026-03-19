package agent

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSession(t *testing.T) {
	s := &sessionImpl{
		ID:       "test-id",
		Metadata: make(map[string]any),
	}

	assert.Equal(t, "test-id", s.GetID())

	// Test Metadata
	s.SetMetadata("key", "val")
	assert.Equal(t, "val", s.GetMetadata()["key"])

	// Test Pending Action
	s.SetPendingAction("confirm")
	assert.Equal(t, "confirm", s.GetPendingAction())

	// Test Lock/Unlock
	assert.True(t, s.TryLock())
	assert.False(t, s.TryLock())
	s.Unlock()
	assert.True(t, s.TryLock())
}

func TestSessionManager(t *testing.T) {
	tmpFile, _ := os.CreateTemp("", "session-store-*.json")
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	sm := NewSessionManager(tmpPath)

	s1 := sm.GetOrCreateActive("user1")
	assert.NotNil(t, s1)
	assert.NotEmpty(t, s1.GetID())

	s2 := sm.GetOrCreateActive("user1")
	assert.Equal(t, s1.GetID(), s2.GetID()) // Should return the same session ID
}
