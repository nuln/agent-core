package agent

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// sessionImpl tracks one conversation between a user and the
type sessionImpl struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	History   []HistoryEntry `json:"history"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	Metadata  map[string]any `json:"metadata,omitempty"`

	// Security: Pending confirmation action
	PendingAction string `json:"pending_action,omitempty"`
	PendingData   any    `json:"pending_data,omitempty"`

	mu   sync.Mutex      `json:"-"`
	busy bool            `json:"-"`
	sm   *SessionManager `json:"-"`
}

func (s *sessionImpl) GetID() string            { return s.ID }
func (s *sessionImpl) GetPendingAction() string { return s.PendingAction }
func (s *sessionImpl) SetPendingAction(a string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.PendingAction = a
}
func (s *sessionImpl) GetHistory() []HistoryEntry { return s.History }
func (s *sessionImpl) AppendHistory(entry HistoryEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.History = append(s.History, entry)
	// Truncate history if it's too long (heuristic: 20 messages)
	if len(s.History) > 20 {
		s.History = s.History[len(s.History)-20:]
	}
}

func (s *sessionImpl) GetMetadata() map[string]any { return s.Metadata }
func (s *sessionImpl) SetMetadata(key string, val any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Metadata == nil {
		s.Metadata = make(map[string]any)
	}
	s.Metadata[key] = val
}

func (s *sessionImpl) TryLock() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy {
		return false
	}
	s.busy = true
	return true
}

func (s *sessionImpl) Unlock() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.busy = false
	s.UpdatedAt = time.Now()
	s.Save()
}

func (s *sessionImpl) Save() {
	if s.sm == nil {
		return
	}
	s.sm.saveSession(s)
}

// SessionManager supports multiple named sessions per user.
type SessionManager struct {
	mu     sync.RWMutex
	kv     KVStore
	active map[string]*sessionImpl
}

func NewSessionManager(kv KVStore) *SessionManager {
	return &SessionManager{
		kv:     kv,
		active: make(map[string]*sessionImpl),
	}
}

func (sm *SessionManager) GetOrCreateActive(userKey string) Session {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// 1. Check in-memory active sessions
	if s, ok := sm.active[userKey]; ok {
		return s
	}

	// 2. Check active session ID for user from KV
	data, _ := sm.kv.Get([]byte("a:" + userKey))
	if data != nil {
		sid := string(data)
		// 3. Load session
		sessData, _ := sm.kv.Get([]byte("s:" + sid))
		if sessData != nil {
			var s sessionImpl
			if err := json.Unmarshal(sessData, &s); err == nil {
				s.sm = sm // Inject manager
				sm.active[userKey] = &s
				return &s
			}
		}
	}

	return sm.createLocked(userKey, "default")
}

func (sm *SessionManager) createLocked(userKey, name string) *sessionImpl {
	id := fmt.Sprintf("s%d", time.Now().UnixNano())
	now := time.Now()
	s := &sessionImpl{
		ID:        id,
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
		sm:        sm, // Inject manager
	}

	// Save session
	sm.saveSession(s)

	// Update in-memory active map
	sm.active[userKey] = s

	// Update active session
	_ = sm.kv.Put([]byte("a:"+userKey), []byte(id))

	// Update user sessions list
	uKey := []byte("u:" + userKey)
	var sids []string
	if data, _ := sm.kv.Get(uKey); data != nil {
		_ = json.Unmarshal(data, &sids)
	}
	sids = append(sids, id)
	data, _ := json.Marshal(sids)
	_ = sm.kv.Put(uKey, data)

	return s
}

func (sm *SessionManager) saveSession(s *sessionImpl) {
	data, _ := json.Marshal(s)
	_ = sm.kv.Put([]byte("s:"+s.ID), data)
}

func (sm *SessionManager) Save() {
	// In the new BoltDB implementation, we could save progressively.
	// But since the engine often calls Save() explicitly (e.g. periodically),
	// we keep it as a no-op if we already save on each change,
	// or we use it as a trigger to sync if we had a cache.
}

// NOTE: sessionImpl needs to save itself back to the store when modified.
// Currently the Engine calls Unlock() which updates UpdatedAt, but doesn't trigger a save.
// I should probably update Unlock() to trigger a save via the manager,
// but sessionImpl doesn't have a back-pointer to the manager.
// I'll add a Save function to the Session interface or handle it in the Engine.
