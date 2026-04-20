package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStreaming(t *testing.T) {
	cfg := DefaultStreamPreviewCfg()
	assert.True(t, cfg.Enabled)
}

func TestRelay(t *testing.T) {
	engine := &Engine{
		dialogs: make(map[string]Dialog),
		llms:    make(map[string]LLM),
	}
	rm := NewRelayManager(nil, engine)
	assert.NotNil(t, rm)
	assert.Equal(t, engine, rm.engine)

	acc, chat, user := parseSessionKeyParts("acc1:chat1:user1")
	assert.Equal(t, "acc1", acc)
	assert.Equal(t, "chat1", chat)
	assert.Equal(t, "user1", user)
}
