package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

	mu   sync.Mutex `json:"-"`
	busy bool       `json:"-"`
}

func (s *sessionImpl) GetID() string            { return s.ID }
func (s *sessionImpl) GetPendingAction() string { return s.PendingAction }
func (s *sessionImpl) SetPendingAction(a string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.PendingAction = a
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
}

// SessionManager supports multiple named sessions per user.
type SessionManager struct {
	mu            sync.RWMutex
	sessions      map[string]*sessionImpl
	activeSession map[string]string
	userSessions  map[string][]string
	storePath     string
}

func NewSessionManager(storePath string) *SessionManager {
	sm := &SessionManager{
		sessions:      make(map[string]*sessionImpl),
		activeSession: make(map[string]string),
		userSessions:  make(map[string][]string),
		storePath:     storePath,
	}
	if storePath != "" {
		sm.load()
	}
	return sm
}

func (sm *SessionManager) GetOrCreateActive(userKey string) Session {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sid, ok := sm.activeSession[userKey]; ok {
		if s, ok := sm.sessions[sid]; ok {
			return s
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
	}
	sm.sessions[id] = s
	sm.activeSession[userKey] = id
	sm.userSessions[userKey] = append(sm.userSessions[userKey], id)
	return s
}

func (sm *SessionManager) Save() {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sm.storePath == "" {
		return
	}

	persist := struct {
		Sessions      map[string]*sessionImpl `json:"sessions"`
		ActiveSession map[string]string       `json:"active_session"`
		UserSessions  map[string][]string     `json:"user_sessions"`
	}{
		Sessions:      sm.sessions,
		ActiveSession: sm.activeSession,
		UserSessions:  sm.userSessions,
	}

	data, _ := json.MarshalIndent(persist, "", "  ")
	_ = os.MkdirAll(filepath.Dir(sm.storePath), 0o755)
	_ = AtomicWriteFile(sm.storePath, data, 0o644)
}

func (sm *SessionManager) load() {
	data, err := os.ReadFile(sm.storePath)
	if err != nil {
		return
	}
	persist := struct {
		Sessions      map[string]*sessionImpl `json:"sessions"`
		ActiveSession map[string]string       `json:"active_session"`
		UserSessions  map[string][]string     `json:"user_sessions"`
	}{}
	if err := json.Unmarshal(data, &persist); err == nil {
		sm.sessions = persist.Sessions
		sm.activeSession = persist.ActiveSession
		sm.userSessions = persist.UserSessions
	}
}
