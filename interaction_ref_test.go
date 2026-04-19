package agent

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
)

type mockKVStore struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func (m *mockKVStore) Get(key []byte) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.data[string(key)], nil
}

func (m *mockKVStore) Put(key, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[string(key)] = value
	return nil
}

func (m *mockKVStore) Delete(key []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, string(key))
	return nil
}

func (m *mockKVStore) List() (map[string][]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make(map[string][]byte)
	for k, v := range m.data {
		res[k] = v
	}
	return res, nil
}

func TestInteractionLogger(t *testing.T) {
	store := &mockKVStore{data: make(map[string][]byte)}
	logger := NewInteractionLogger(store)
	defer logger.Stop()

	ref := InteractionRef{
		TraceID:         "test-trace",
		SessionKey:      "test-session",
		Timestamp:       time.Now().UnixMilli(),
		SenderPlugin:    "test-sender",
		ResponderPlugin: "test-responder",
		UserID:          "test-user",
		LatencyMs:       100,
		Status:          "completed",
	}

	logger.Record(ref)

	// Wait for async processing
	time.Sleep(100 * time.Millisecond)

	// Verify data was stored
	found := false
	all, _ := store.List()
	for _, v := range all {
		var decoded InteractionRef
		if err := json.Unmarshal(v, &decoded); err == nil {
			if decoded.TraceID == "test-trace" {
				found = true
				break
			}
		}
	}

	if !found {
		t.Error("interaction record not found in store")
	}
}

func TestGenerateTraceID(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := GenerateTraceID()
		if ids[id] {
			t.Errorf("duplicate trace ID generated: %s", id)
		}
		ids[id] = true
	}
}
