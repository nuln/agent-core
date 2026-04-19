package agent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSessionStateManager(t *testing.T) {
	store := &mockKVStore{data: make(map[string][]byte)}
	sm := NewSessionStateManager(store)

	key := "session-1"
	state := &SessionState{
		SessionKey: key,
		Stage:      "ST1",
		LockedBy:   "plugin-1",
		ExpiresAt:  time.Now().Add(1 * time.Hour).UnixMilli(),
	}

	// 1. Set & Get
	err := sm.SetState(key, state)
	assert.NoError(t, err)

	got, err := sm.GetState(key)
	assert.NoError(t, err)
	assert.NotNil(t, got)
	assert.Equal(t, "ST1", got.Stage)
	assert.Equal(t, "plugin-1", got.LockedBy)

	// 2. Clear
	err = sm.ClearState(key)
	assert.NoError(t, err)
	got, _ = sm.GetState(key)
	assert.Nil(t, got)

	// 3. Expiry
	state.ExpiresAt = time.Now().Add(-1 * time.Minute).UnixMilli()
	_ = sm.SetState(key, state)
	got, err = sm.GetState(key)
	assert.NoError(t, err)
	assert.Nil(t, got, "Expired state should be nil")

	// 4. Cleanup
	_ = sm.SetState("s2", &SessionState{ExpiresAt: time.Now().Add(-5 * time.Minute).UnixMilli()})
	_ = sm.SetState("s3", &SessionState{ExpiresAt: time.Now().Add(5 * time.Minute).UnixMilli()})

	err = sm.CleanupExpired()
	assert.NoError(t, err)

	s2, _ := sm.GetState("s2")
	assert.Nil(t, s2)
	s3, _ := sm.GetState("s3")
	assert.NotNil(t, s3)
}
