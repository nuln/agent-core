package agent

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSessionHistory_Truncation(t *testing.T) {
	s := &sessionImpl{
		ID:      "test-sess",
		History: make([]HistoryEntry, 0),
	}

	// 1. Fill history to the limit
	for i := 1; i <= 20; i++ {
		s.AppendHistory(HistoryEntry{
			Role:    RoleUser,
			Content: fmt.Sprintf("msg %d", i),
		})
	}
	assert.Equal(t, 20, len(s.GetHistory()))

	// 2. Add one more message (truncate occurs)
	s.AppendHistory(HistoryEntry{
		Role:    RoleAssistant,
		Content: "trigger truncate",
	})

	history := s.GetHistory()
	assert.Equal(t, 20, len(history))

	// 3. Verify the oldest message (msg 1) is gone
	assert.Equal(t, "msg 2", history[0].Content)
	assert.Equal(t, "trigger truncate", history[19].Content)
}
