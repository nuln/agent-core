package agent

import (
	"encoding/json"
	"time"
)

type sessionStateManager struct {
	kv KVStore
}

func NewSessionStateManager(kv KVStore) SessionStateManager {
	return &sessionStateManager{kv: kv}
}

func (s *sessionStateManager) GetState(sessionKey string) (*SessionState, error) {
	data, err := s.kv.Get([]byte(sessionKey))
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}

	var state SessionState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	// Check expiry
	if state.ExpiresAt > 0 && state.ExpiresAt < time.Now().UnixMilli() {
		_ = s.ClearState(sessionKey)
		return nil, nil
	}

	return &state, nil
}

func (s *sessionStateManager) SetState(sessionKey string, state *SessionState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return s.kv.Put([]byte(sessionKey), data)
}

func (s *sessionStateManager) ClearState(sessionKey string) error {
	return s.kv.Delete([]byte(sessionKey))
}

func (s *sessionStateManager) CleanupExpired() error {
	states, err := s.kv.List()
	if err != nil {
		return err
	}

	now := time.Now().UnixMilli()
	for k, v := range states {
		var state SessionState
		if err := json.Unmarshal(v, &state); err == nil {
			if state.ExpiresAt > 0 && state.ExpiresAt < now {
				_ = s.ClearState(k)
			}
		}
	}
	return nil
}
