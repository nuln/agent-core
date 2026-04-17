package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVoiceUtilities(t *testing.T) {
	assert.True(t, NeedsConversion("amr"))
	assert.False(t, NeedsConversion("mp3"))
	assert.False(t, NeedsConversion("wav"))

	assert.Equal(t, "ogg", formatToExt("opus"))
	assert.Equal(t, "m4a", formatToExt("aac"))

	assert.Equal(t, "audio/mpeg", formatToAudioMIME("mp3"))
	assert.Equal(t, "audio/wav", formatToAudioMIME("wav"))
}

func TestVoiceConstructors(t *testing.T) {
	w := NewOpenAIWhisper("key", "", "")
	assert.Equal(t, "key", w.APIKey)
	assert.Equal(t, "https://api.openai.com/v1", w.BaseURL)
	assert.Equal(t, "whisper-1", w.Model)

	q := NewQwenASR("key", "", "")
	assert.Equal(t, "key", q.APIKey)
	assert.Equal(t, "qwen3-asr-flash", q.Model)

	ot := NewOpenAITTS("key", "", "", nil)
	assert.Equal(t, "tts-1", ot.Model)

	qt := NewQwenTTS("key", "", "", nil)
	assert.Equal(t, "qwen3-tts-flash", qt.Model)
}

type mockSTTForTranscribe struct{}

func (m *mockSTTForTranscribe) Transcribe(ctx context.Context, audio []byte, format string, lang string) (string, error) {
	return "transcribed", nil
}

func TestTranscribeAudio(t *testing.T) {
	stt := &mockSTTForTranscribe{}
	audio := &AudioAttachment{
		Data:   []byte("test"),
		Format: "wav",
	}
	text, err := TranscribeAudio(context.Background(), stt, audio, "en")
	assert.NoError(t, err)
	assert.Equal(t, "transcribed", text)
}
