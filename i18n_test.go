package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		text     string
		expected Language
	}{
		{"Hello world", LangEnglish},
		{"你好，世界", LangChinese},
		{"Hello 你好", LangChinese}, // Mixed, should pick Chinese if present
		{"Mixed 123 !@#", LangEnglish},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			assert.Equal(t, tt.expected, DetectLanguage(tt.text))
		})
	}
}

func TestI18n(t *testing.T) {
	i := NewI18n(LangAuto)

	// Test Initial state
	assert.Equal(t, LangEnglish, i.CurrentLang())
	assert.False(t, i.IsZhLike())

	// Test Detection
	i.DetectAndSet("你好")
	assert.Equal(t, LangChinese, i.CurrentLang())
	assert.True(t, i.IsZhLike())

	// Test Override
	i.SetLang(LangEnglish)
	assert.Equal(t, LangEnglish, i.CurrentLang())
	assert.False(t, i.IsZhLike())

	// Test SaveFunc
	var savedLang Language
	i.SetSaveFunc(func(l Language) error {
		savedLang = l
		return nil
	})
	i.SetLang(LangAuto)
	i.DetectAndSet("你好")
	assert.Equal(t, LangChinese, savedLang)
}

func TestTranslation(t *testing.T) {
	i := NewI18n(LangEnglish)

	// Assuming these keys exist in i18n.go (based on my previous cleanup)
	assert.NotEmpty(t, i.T("MsgHelpTitle"))

	i.SetLang(LangChinese)
	assert.NotEmpty(t, i.T("MsgHelpTitle"))

	// Fallback to English
	assert.NotEmpty(t, i.T("unknown-key"))
}
