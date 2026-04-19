package agent

import (
	"context"
	"sync"
)

// MockKVStore is a thread-safe in-memory KVStore for testing.
type MockKVStore struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func NewMockKVStore() *MockKVStore {
	return &MockKVStore{
		data: make(map[string][]byte),
	}
}

func (m *MockKVStore) Get(key []byte) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.data[string(key)], nil
}

func (m *MockKVStore) Put(key, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[string(key)] = value
	return nil
}

func (m *MockKVStore) Delete(key []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, string(key))
	return nil
}

func (m *MockKVStore) List() (map[string][]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make(map[string][]byte)
	for k, v := range m.data {
		res[k] = v
	}
	return res, nil
}

// MockKVStoreProvider implements KVStoreProvider for testing.
type MockKVStoreProvider struct {
	mu     sync.RWMutex
	stores map[string]*MockKVStore
}

func NewMockKVStoreProvider() *MockKVStoreProvider {
	return &MockKVStoreProvider{
		stores: make(map[string]*MockKVStore),
	}
}

func (m *MockKVStoreProvider) GetStore(name string) (KVStore, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.stores[name]; ok {
		return s, nil
	}
	s := NewMockKVStore()
	m.stores[name] = s
	return s, nil
}

// MockDialog implements Dialog and DialogRecorder.
type MockDialog struct {
	NameVal string
	storage KVStoreProvider
	TraceID string
}

func (m *MockDialog) Name() string                                                  { return m.NameVal }
func (m *MockDialog) Start(h MessageHandler) error                                  { return nil }
func (m *MockDialog) Reply(ctx context.Context, replyCtx any, content string) error { return nil }
func (m *MockDialog) Send(ctx context.Context, replyCtx any, content string) error  { return nil }
func (m *MockDialog) Stop() error                                                   { return nil }
func (m *MockDialog) Reload(opts map[string]any) error                              { return nil }
func (m *MockDialog) SetStorage(s KVStoreProvider)                                  { m.storage = s }
func (m *MockDialog) RecordMessage(traceID string, msg *Message) error {
	m.TraceID = traceID
	return nil
}

// MockLLM implements LLM and StorageAware.
type MockLLM struct {
	NameVal string
	storage KVStoreProvider
	Session *MockSession
}

func (m *MockLLM) Name() string        { return m.NameVal }
func (m *MockLLM) Description() string { return "mock llm" }
func (m *MockLLM) StartSession(ctx context.Context, id string) (AgentSession, error) {
	if m.Session == nil {
		m.Session = &MockSession{LLM: m}
	}
	return m.Session, nil
}
func (m *MockLLM) Stop() error                      { return nil }
func (m *MockLLM) Reload(opts map[string]any) error { return nil }
func (m *MockLLM) SetStorage(s KVStoreProvider)     { m.storage = s }

// MockSession implements AgentSession and SessionRecorder.
type MockSession struct {
	LLM     *MockLLM
	TraceID string
	Closed  bool
}

func (m *MockSession) Send(p string, i []ImageAttachment, f []FileAttachment) error { return nil }
func (m *MockSession) RespondPermission(id string, res PermissionResult) error      { return nil }
func (m *MockSession) Events() <-chan Event {
	ch := make(chan Event, 1)
	ch <- Event{Type: EventResult, Done: true}
	close(ch)
	return ch
}
func (m *MockSession) CurrentSessionID() string { return "sess-1" }
func (m *MockSession) Alive() bool              { return !m.Closed }
func (m *MockSession) Close() error             { m.Closed = true; return nil }
func (m *MockSession) SetTraceID(id string)     { m.TraceID = id }
