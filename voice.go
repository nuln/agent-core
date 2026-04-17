package agent

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// ──────────────────────────────────────────────────────────────
// Speech To Text (STT) Types & Interfaces
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

// OpenAIWhisper implements SpeechToText using the OpenAI-compatible Whisper API.
type OpenAIWhisper struct {
	APIKey  string
	BaseURL string
	Model   string
	Client  *http.Client
}

func NewOpenAIWhisper(apiKey, baseURL, model string) *OpenAIWhisper {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if model == "" {
		model = "whisper-1"
	}
	return &OpenAIWhisper{
		APIKey:  apiKey,
		BaseURL: strings.TrimRight(baseURL, "/"),
		Model:   model,
		Client:  &http.Client{Timeout: 60 * time.Second},
	}
}

func (w *OpenAIWhisper) Transcribe(ctx context.Context, audio []byte, format string, lang string) (string, error) {
	ext := formatToExt(format)

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("file", "audio."+ext)
	if err != nil {
		return "", fmt.Errorf("create form file: %w", err)
	}
	if _, err := part.Write(audio); err != nil {
		return "", fmt.Errorf("write audio: %w", err)
	}
	_ = writer.WriteField("model", w.Model)
	_ = writer.WriteField("response_format", "text")
	if lang != "" {
		_ = writer.WriteField("language", lang)
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("close multipart: %w", err)
	}

	url := w.BaseURL + "/audio/transcriptions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+w.APIKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := w.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("whisper request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("whisper API %d: %s", resp.StatusCode, string(body))
	}

	text := strings.TrimSpace(string(body))
	if strings.HasPrefix(text, "{") {
		var jr struct {
			Text string `json:"text"`
		}
		if json.Unmarshal(body, &jr) == nil {
			text = jr.Text
		}
	}
	return text, nil
}

// QwenASR implements SpeechToText using the Qwen ASR model.
type QwenASR struct {
	APIKey  string
	BaseURL string
	Model   string
	Client  *http.Client
}

func NewQwenASR(apiKey, baseURL, model string) *QwenASR {
	if baseURL == "" {
		baseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	}
	if model == "" {
		model = "qwen3-asr-flash"
	}
	return &QwenASR{
		APIKey:  apiKey,
		BaseURL: strings.TrimRight(baseURL, "/"),
		Model:   model,
		Client:  &http.Client{Timeout: 60 * time.Second},
	}
}

func (q *QwenASR) Transcribe(ctx context.Context, audio []byte, format string, _ string) (string, error) {
	b64 := base64.StdEncoding.EncodeToString(audio)
	dataURI := fmt.Sprintf("data:%s;base64,%s", formatToAudioMIME(format), b64)

	reqBody := map[string]any{
		"model": q.Model,
		"messages": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{
						"type": "input_audio",
						"input_audio": map[string]any{
							"data": dataURI,
						},
					},
				},
			},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := q.BaseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonData))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+q.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := q.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("qwen asr request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("qwen asr API %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("qwen asr: empty choices in response")
	}

	return strings.TrimSpace(result.Choices[0].Message.Content), nil
}

// ──────────────────────────────────────────────────────────────
// Text To Speech (TTS) Types & Interfaces
// ──────────────────────────────────────────────────────────────

// TextToSpeech synthesizes text into audio bytes.
type TextToSpeech interface {
	Synthesize(ctx context.Context, text string, opts TTSSynthesisOpts) (audio []byte, format string, err error)
}

// TTSSynthesisOpts carries optional synthesis parameters.
type TTSSynthesisOpts struct {
	Voice        string  // voice name, e.g. "Cherry", "Alloy"; empty = provider default
	LanguageType string  // e.g. "Chinese", "English"; empty = auto-detect
	Speed        float64 // speaking speed multiplier (0.5–2.0); 0 = default
}

// TTSCfg holds TTS configuration for the engine (mirrors SpeechCfg).
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

// QwenTTS — Alibaba DashScope TTS implementation
type QwenTTS struct {
	APIKey  string
	BaseURL string
	Model   string
	Client  *http.Client
}

func NewQwenTTS(apiKey, baseURL, model string, client *http.Client) *QwenTTS {
	if baseURL == "" {
		baseURL = "https://dashscope.aliyuncs.com/api/v1/services/aigc/multimedal-generation/generation"
	}
	if model == "" {
		model = "qwen3-tts-flash"
	}
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	return &QwenTTS{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Model:   model,
		Client:  client,
	}
}

func (q *QwenTTS) Synthesize(ctx context.Context, text string, opts TTSSynthesisOpts) ([]byte, string, error) {
	voice := opts.Voice
	if voice == "" {
		voice = "Cherry"
	}
	reqBody := map[string]any{
		"model": q.Model,
		"input": map[string]any{
			"text":          text,
			"voice":         voice,
			"language_type": opts.LanguageType,
		},
	}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, "", fmt.Errorf("qwen tts: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, q.BaseURL, bytes.NewReader(jsonData))
	if err != nil {
		return nil, "", fmt.Errorf("qwen tts: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+q.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := q.Client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("qwen tts: request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("qwen tts: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("qwen tts API %d: %s", resp.StatusCode, body)
	}

	var result struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Output  struct {
			Audio struct {
				URL string `json:"url"`
			} `json:"audio"`
		} `json:"output"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, "", fmt.Errorf("qwen tts: parse response: %w", err)
	}
	if result.Code != "" {
		return nil, "", fmt.Errorf("qwen tts API error %s: %s", result.Code, result.Message)
	}
	if result.Output.Audio.URL == "" {
		return nil, "", fmt.Errorf("qwen tts: empty audio URL in response")
	}

	audioReq, err := http.NewRequestWithContext(ctx, http.MethodGet, result.Output.Audio.URL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("qwen tts: create download request: %w", err)
	}
	audioResp, err := q.Client.Do(audioReq)
	if err != nil {
		return nil, "", fmt.Errorf("qwen tts: download audio: %w", err)
	}
	defer func() {
		_ = audioResp.Body.Close()
	}()

	wavData, err := io.ReadAll(audioResp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("qwen tts: read audio: %w", err)
	}
	return wavData, "wav", nil
}

// OpenAITTS — OpenAI-compatible TTS implementation
type OpenAITTS struct {
	APIKey  string
	BaseURL string
	Model   string
	Client  *http.Client
}

func NewOpenAITTS(apiKey, baseURL, model string, client *http.Client) *OpenAITTS {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if model == "" {
		model = "tts-1"
	}
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	return &OpenAITTS{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Model:   model,
		Client:  client,
	}
}

func (o *OpenAITTS) Synthesize(ctx context.Context, text string, opts TTSSynthesisOpts) ([]byte, string, error) {
	voice := opts.Voice
	if voice == "" {
		voice = "alloy"
	}
	reqBody := map[string]any{
		"model": o.Model,
		"input": text,
		"voice": voice,
	}
	if opts.Speed > 0 {
		reqBody["speed"] = opts.Speed
	}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, "", fmt.Errorf("openai tts: marshal request: %w", err)
	}

	url := strings.TrimRight(o.BaseURL, "/") + "/audio/speech"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonData))
	if err != nil {
		return nil, "", fmt.Errorf("openai tts: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+o.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.Client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("openai tts: request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("openai tts API %d: %s", resp.StatusCode, body)
	}

	mp3Data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("openai tts: read audio: %w", err)
	}
	return mp3Data, "mp3", nil
}

// ──────────────────────────────────────────────────────────────
// Shared Voice Utilities
// ──────────────────────────────────────────────────────────────

// NeedsConversion returns true if the audio format is not directly supported by Whisper API.
func NeedsConversion(format string) bool {
	switch strings.ToLower(format) {
	case "mp3", "mp4", "mpeg", "mpga", "m4a", "wav", "webm":
		return false
	default:
		return true
	}
}

// HasFFmpeg checks if ffmpeg is available.
func HasFFmpeg() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

func formatToExt(format string) string {
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

func formatToAudioMIME(format string) string {
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

// TranscribeAudio is a convenience function used by the Engine.
func TranscribeAudio(ctx context.Context, stt SpeechToText, audio *AudioAttachment, lang string) (string, error) {
	data := audio.Data
	format := strings.ToLower(audio.Format)

	if NeedsConversion(format) {
		slog.Debug("speech: converting audio", "from", format, "to", "mp3")
		converted, err := ConvertAudioToMP3(data, format)
		if err != nil {
			return "", err
		}
		data = converted
		format = "mp3"
	}

	slog.Debug("speech: transcribing", "format", format, "size", len(data))
	return stt.Transcribe(ctx, data, format, lang)
}

// ConvertAudioToMP3 uses ffmpeg to convert audio from unsupported formats to mp3.
// Returns the mp3 bytes. If ffmpeg is not installed, returns an error.
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
			"-f", "mp3",
			"-ac", "1",
			"-ar", "16000",
			"-y",
			"pipe:1",
		)
	} else {
		cmd = exec.Command(ffmpegPath,
			"-i", "pipe:0",
			"-f", "mp3",
			"-ac", "1",
			"-ar", "16000",
			"-y",
			"pipe:1",
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
