package agent

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockTranslator struct{ mock.Mock }

func (m *mockTranslator) T(key string, args ...any) string { return key }

type mockSessionProvider struct{ mock.Mock }

func (m *mockSessionProvider) GetOrCreateActive(key string) Session {
	args := m.Called(key)
	return args.Get(0).(Session)
}

func TestEngine_Initialization(t *testing.T) {
	tmpDir := t.TempDir()
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	tp := &mockTranslator{}
	sp := &mockSessionProvider{}

	e := NewEngine(sp, tp, nil, nil, tmpDir)
	assert.NotNil(t, e)
	assert.NotNil(t, e.pipes)
}

func TestEngine_Registration(t *testing.T) {
	e := NewEngine(nil, nil, nil, nil, "")

	dialog := &mockDialog{name: "d1"}
	e.RegisterDialog(dialog)
	assert.Len(t, e.dialogs, 1)

	llm := &mockLLM{name: "l1"}
	e.RegisterLLM(llm)
	assert.Len(t, e.llms, 1)
	assert.Equal(t, "l1", e.defaultLLM)

	e.SetDefaultLLM("l2")
	assert.Equal(t, "l2", e.defaultLLM)
}
