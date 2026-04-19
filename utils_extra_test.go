package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedactArgs_Formats(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected []string
	}{
		{
			name:     "space separated",
			args:     []string{"--token", "secret123", "--other", "val"},
			expected: []string{"--token", "***", "--other", "val"},
		},
		{
			name:     "equal separated",
			args:     []string{"--api-key=secret456", "--verbose"},
			expected: []string{"--api-key=***", "--verbose"},
		},
		{
			name:     "short flag",
			args:     []string{"-k", "secret789"},
			expected: []string{"-k", "***"},
		},
		{
			name:     "mixed",
			args:     []string{"cmd", "--password", "p1", "--secret=p2"},
			expected: []string{"cmd", "--password", "***", "--secret=***"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, RedactArgs(tt.args))
		})
	}
}

func TestRedactToken_EdgeCases(t *testing.T) {
	assert.Equal(t, "", RedactToken("", "secret"))
	assert.Equal(t, "hello", RedactToken("hello", ""))
	assert.Equal(t, "[REDACTED] is [REDACTED]", RedactToken("secret is secret", "secret"))
}
