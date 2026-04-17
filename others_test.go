package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDoctor(t *testing.T) {
	results := RunDoctorChecks(context.Background(), nil, nil)
	assert.NotEmpty(t, results)

	report := FormatDoctorResults(results)
	assert.Contains(t, report, "Doctor Report")
}

func TestStreaming(t *testing.T) {
	cfg := DefaultStreamPreviewCfg()
	assert.True(t, cfg.Enabled)

	// Complex streaming tests would need more mocks
}

func TestRelay(t *testing.T) {
	engine := &Engine{
		dialogs: make(map[string]Dialog),
		llms:    make(map[string]LLM),
	}
	rm := NewRelayManager("", engine)
	assert.NotNil(t, rm)
	assert.Equal(t, engine, rm.engine)

	acc, chat, user := parseSessionKeyParts("acc1:chat1:user1")
	assert.Equal(t, "acc1", acc)
	assert.Equal(t, "chat1", chat)
	assert.Equal(t, "user1", user)
}
