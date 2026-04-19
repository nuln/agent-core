package agent

import (
	"math/rand"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

func TestTruncateStr_UTF8_Table(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxBytes int
		expect   string // expected prefix before "..."
	}{
		{"English", "Hello World", 5, "Hello"},
		{"Emoji boundary fail", "Hello 🚀", 7, "Hello "}, // 🚀 is 4 bytes. 7 bytes cuts at first byte of 🚀
		{"Emoji boundary success", "Hello 🚀", 10, "Hello 🚀"},
		{"Chinese", "你好世界", 6, "你好"},          // 你好 is 6 bytes
		{"Chinese boundary", "你好世界", 7, "你好"}, // 3rd byte of 世 is at pos 9. 7 and 8 are middle of 世
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := TruncateStr(tt.input, tt.maxBytes)
			if len(tt.input) > tt.maxBytes {
				assert.Contains(t, res, "... [truncated")
				assert.True(t, utf8.ValidString(res), "Truncated string must be valid UTF-8")
			} else {
				assert.Equal(t, tt.input, res)
			}
		})
	}
}

func TestTruncateStr_Random(t *testing.T) {
	// Randomized testing of truncation safety
	runes := []rune("abcdefg123456!@#$%^&*()🔥🚀你好世界äöüß")

	for i := 0; i < 1000; i++ {
		// Generate random string
		length := rand.Intn(100) + 10
		sRunes := make([]rune, length)
		for j := 0; j < length; j++ {
			sRunes[j] = runes[rand.Intn(len(runes))]
		}
		s := string(sRunes)

		maxBytes := rand.Intn(len(s) + 5)
		res := TruncateStr(s, maxBytes)

		assert.True(t, utf8.ValidString(res), "Truncated string must ALWAYS be valid UTF-8. Input: %s, Max: %d", s, maxBytes)
	}
}
