// Package agent – voice interfaces and shared audio utilities.
// Concrete STT/TTS implementations live in the skill/stt and skill/tts plugins.
package agent

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

// ──────────────────────────────────────────────────────────────
// Speech To Text (STT) Interfaces
// ──────────────────────────────────────────────────────────────

// SpeechToText transcribes audio to text.
type SpeechToText interface {
	Transcribe(ctx context.Context, audio []byte, format string, lang string) (string, error)
}

// SpeechCfg holds STT configuration for the engine.
type SpeechCfg struct {
	Enabled  bool
	Provider string
	Language string
	STT      SpeechToText
}

// ──────────────────────────────────────────────────────────────
// Text To Speech (TTS) Interfaces
// ──────────────────────────────────────────────────────────────

// TextToSpeech synthesizes text into audio bytes.
type TextToSpeech interface {
	Synthesize(ctx context.Context, text string, opts TTSSynthesisOpts) (audio []byte, format string, err error)
}

// TTSSynthesisOpts carries optional synthesis parameters.
type TTSSynthesisOpts struct {
	Voice        string  // voice name; empty = provider default
	LanguageType string  // e.g. "Chinese", "English"; empty = auto-detect
	Speed        float64 // speaking speed multiplier (0.5–2.0); 0 = default
}

// TTSCfg holds TTS configuration for the engine.
type TTSCfg struct {
	Enabled    bool
	Provider   string
	Voice      string // default voice used when TTSSynthesisOpts.Voice is empty
	TTS        TextToSpeech
	MaxTextLen int // max rune count before skipping TTS; 0 = no limit

	mu      sync.RWMutex
	ttsMode string // "voice_only" (default) | "always"
}

// GetTTSMode returns the current TTS mode safely.
func (c *TTSCfg) GetTTSMode() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.ttsMode == "" {
		return "voice_only"
	}
	return c.ttsMode
}

// SetTTSMode updates the TTS mode safely.
func (c *TTSCfg) SetTTSMode(mode string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ttsMode = mode
}

// ──────────────────────────────────────────────────────────────
// Shared Audio Utilities
// ──────────────────────────────────────────────────────────────

// NeedsConversion returns true if the audio format is not directly supported by
// the Whisper API.
func NeedsConversion(format string) bool {
	switch strings.ToLower(format) {
	case "mp3", "mp4", "mpeg", "mpga", "m4a", "wav", "webm":
		return false
	default:
		return true
	}
}

// HasFFmpeg reports whether ffmpeg is available in PATH.
func HasFFmpeg() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

// FormatToExt maps an audio format string to a file extension.
func FormatToExt(format string) string {
	switch strings.ToLower(format) {
	case "amr":
		return "amr"
	case "ogg", "oga", "opus":
		return "ogg"
	case "m4a", "mp4", "aac":
		return "m4a"
	case "mp3":
		return "mp3"
	case "wav":
		return "wav"
	case "webm":
		return "webm"
	case "silk":
		return "silk"
	default:
		return format
	}
}

// FormatToAudioMIME maps an audio format string to its MIME type.
func FormatToAudioMIME(format string) string {
	switch strings.ToLower(format) {
	case "mp3", "mpeg", "mpga":
		return "audio/mpeg"
	case "wav":
		return "audio/wav"
	case "ogg", "oga", "opus":
		return "audio/ogg"
	case "m4a", "mp4", "aac":
		return "audio/mp4"
	case "webm":
		return "audio/webm"
	default:
		return "audio/octet-stream"
	}
}

// TranscribeAudio is a convenience helper used by the Engine to transcribe an
// AudioAttachment via the configured SpeechToText provider.
func TranscribeAudio(ctx context.Context, stt SpeechToText, audio *AudioAttachment, lang string) (string, error) {
	data := audio.Data
	format := strings.ToLower(audio.Format)

	if NeedsConversion(format) {
		converted, err := ConvertAudioToMP3(data, format)
		if err != nil {
			return "", err
		}
		data = converted
		format = "mp3"
	}

	return stt.Transcribe(ctx, data, format, lang)
}

// ConvertAudioToMP3 converts audio from an unsupported format to MP3 via ffmpeg.
// Returns an error if ffmpeg is not installed.
func ConvertAudioToMP3(audio []byte, srcFormat string) ([]byte, error) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, fmt.Errorf("ffmpeg not found in PATH: install ffmpeg to enable voice message support")
	}

	var cmd *exec.Cmd
	if srcFormat == "amr" || srcFormat == "silk" {
		cmd = exec.Command(ffmpegPath,
			"-f", srcFormat,
			"-i", "pipe:0",
			"-f", "mp3", "-ac", "1", "-ar", "16000", "-y", "pipe:1",
		)
	} else {
		cmd = exec.Command(ffmpegPath,
			"-i", "pipe:0",
			"-f", "mp3", "-ac", "1", "-ar", "16000", "-y", "pipe:1",
		)
	}

	cmd.Stdin = bytes.NewReader(audio)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg conversion failed: %w (stderr: %s)", err, stderr.String())
	}
	return stdout.Bytes(), nil
}
