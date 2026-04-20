package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type mockSessionProviderManual struct {
	sessions map[string]Session
}

func (m *mockSessionProviderManual) GetOrCreateActive(key string) Session {
	if s, ok := m.sessions[key]; ok {
		return s
	}
	s := &mockSessionImpl{
		data: make(map[string]any),
	}
	m.sessions[key] = s
	return s
}

type mockSessionImpl struct {
	data    map[string]any
	history []HistoryEntry
}

func (m *mockSessionImpl) GetID() string                     { return "session-id" }
func (m *mockSessionImpl) GetPendingAction() string          { return "" }
func (m *mockSessionImpl) SetPendingAction(action string)    {}
func (m *mockSessionImpl) GetMetadata() map[string]any       { return nil }
func (m *mockSessionImpl) SetMetadata(key string, value any) {}
func (m *mockSessionImpl) TryLock() bool                     { return true }
func (m *mockSessionImpl) Unlock()                           {}
func (m *mockSessionImpl) Save()                             {}
func (m *mockSessionImpl) GetHistory() []HistoryEntry        { return m.history }
func (m *mockSessionImpl) AppendHistory(entry HistoryEntry)  { m.history = append(m.history, entry) }

func TestInteractionTraceFlow_Table(t *testing.T) {
	tests := []struct {
		name       string
		dialogName string
		llmName    string
		expectID   bool
	}{
		{
			name:       "lark to codex",
			dialogName: "lark",
			llmName:    "codex",
			expectID:   true,
		},
		{
			name:       "telegram to gemini",
			dialogName: "telegram",
			llmName:    "gemini",
			expectID:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			sp := &mockSessionProviderManual{sessions: make(map[string]Session)}
			e := NewEngine(WithSessions(sp), WithDataDir(tmpDir))

			md := &MockDialog{NameVal: tt.dialogName}
			e.RegisterDialog(md)

			ml := &MockLLM{NameVal: tt.llmName}
			e.RegisterLLM(ml)
			e.SetDefaultLLM(tt.llmName)

			msg := &Message{
				MessageID:  "m-1",
				UserID:     "u-1",
				Content:    "test content",
				SessionKey: tt.dialogName + ":c1:u1",
			}

			// Call the core logic
			e.handleMessage(md, msg)

			if tt.expectID {
				assert.NotEmpty(t, md.TraceID, "Dialog should have received trace ID")
				assert.NotNil(t, ml.Session, "LLM Session should have been created")
				assert.Equal(t, md.TraceID, ml.Session.TraceID, "Trace ID must be propagated from Dialog to LLM Session")

				// Verify history recording
				sess := sp.GetOrCreateActive(msg.SessionKey)
				hist := sess.GetHistory()
				assert.GreaterOrEqual(t, len(hist), 2, "History should contain at least user and assistant turns")
				assert.Equal(t, RoleUser, hist[0].Role)
				assert.Equal(t, RoleAssistant, hist[len(hist)-1].Role)
			}
		})
	}
}
