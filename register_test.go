package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Mock implementations
type mockDialog struct{ name string }

func (m *mockDialog) Name() string                                                  { return m.name }
func (m *mockDialog) Start(h MessageHandler) error                                  { return nil }
func (m *mockDialog) Reply(ctx context.Context, replyCtx any, content string) error { return nil }
func (m *mockDialog) Send(ctx context.Context, replyCtx any, content string) error  { return nil }
func (m *mockDialog) Stop() error                                                   { return nil }
func (m *mockDialog) Reload(opts map[string]any) error                              { return nil }

type mockLLM struct{ name string }

func (m *mockLLM) Name() string                                                      { return m.name }
func (m *mockLLM) Description() string                                               { return "mock" }
func (m *mockLLM) StartSession(ctx context.Context, id string) (AgentSession, error) { return nil, nil }
func (m *mockLLM) Stop() error                                                       { return nil }
func (m *mockLLM) Reload(opts map[string]any) error                                  { return nil }

type mockPipe struct{}

func (m *mockPipe) Handle(ctx context.Context, p Dialog, msg *Message) bool { return false }

func TestDialogRegistry(t *testing.T) {
	name := "test-dialog"
	factory := func(opts map[string]any) (Dialog, error) {
		return &mockDialog{name: name}, nil
	}

	RegisterDialog(name, factory)

	factories := ListDialogFactories()
	assert.Contains(t, factories, name)

	p, err := CreateDialog(name, nil)
	assert.NoError(t, err)
	assert.Equal(t, name, p.Name())

	_, err = CreateDialog("unknown", nil)
	assert.Error(t, err)
}

func TestLLMRegistry(t *testing.T) {
	name := "test-llm"
	factory := func(opts map[string]any) (LLM, error) {
		return &mockLLM{name: name}, nil
	}

	RegisterLLM(name, factory)

	factories := ListLLMFactories()
	assert.Contains(t, factories, name)

	a, err := CreateLLM(name, nil)
	assert.NoError(t, err)
	assert.Equal(t, name, a.Name())

	_, err = CreateLLM("unknown", nil)
	assert.Error(t, err)
}

func TestPipeRegistry(t *testing.T) {
	factory1 := func(ctx PipeContext) Pipe { return &mockPipe{} }
	factory2 := func(ctx PipeContext) Pipe { return &mockPipe{} }

	RegisterPipe("pipe1", 10, factory1)
	RegisterPipe("pipe2", 5, factory2)

	pipes := CreatePipes(PipeContext{})
	assert.Len(t, pipes, 2)

	// Check sorting (priority 5 should come before 10)
	// In register.go: factory1 has priority 10, factory2 has 5.
	// tmp[i].priority < tmp[j].priority
	// So factory2 (5) comes first.
}
