package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

type PluginConfig struct {
	Type    string         `json:"type"`
	Options map[string]any `json:"options"`
}

type EngineConfig struct {
	Dialogs    []PluginConfig `json:"dialogs"`
	LLMs       []PluginConfig `json:"llms"`
	DefaultLLM string         `json:"default_llm"`
}

// Engine routes messages between platforms and
type Engine struct {
	dialogs    map[string]Dialog
	llms       map[string]LLM
	sessions   SessionProvider
	translator Translator
	cron       *CronScheduler
	relay      *RelayManager
	stt        SpeechToText
	tts        TextToSpeech
	api        *APIServer
	pipes      []Pipe
	defaultLLM string
	mu         sync.RWMutex
}

func NewEngine(sessions SessionProvider, t Translator, stt SpeechToText, tts TextToSpeech, dataDir string) *Engine {
	e := &Engine{
		sessions:   sessions,
		translator: t,
		stt:        stt,
		tts:        tts,
		dialogs:    make(map[string]Dialog),
		llms:       make(map[string]LLM),
	}

	e.pipes = CreatePipes(PipeContext{
		Sessions:   sessions,
		Translator: t,
		GetAgents: func() []AgentInfo {
			e.mu.RLock()
			defer e.mu.RUnlock()
			var list []AgentInfo
			for name, a := range e.llms {
				list = append(list, AgentInfo{Name: name, Description: a.Description()})
			}
			return list
		},
		Inject: func(_ context.Context, sessionKey, content string) {
			e.mu.RLock()
			var firstDialog Dialog
			for _, d := range e.dialogs {
				firstDialog = d
				break
			}
			e.mu.RUnlock()

			if firstDialog == nil {
				slog.Warn("engine.Inject: no dialog registered, dropping message", "sessionKey", sessionKey)
				return
			}
			e.handleMessage(firstDialog, &Message{
				SessionKey: sessionKey,
				Content:    content,
			})
		},
	})

	// Note: Cron, Relay, and API require a dataDir
	if dataDir != "" {
		cStore, err := NewCronStore(dataDir)
		if err != nil {
			slog.Error("engine: failed to create cron store", "error", err)
		} else {
			e.cron = NewCronScheduler(cStore, e)
		}
		e.relay = NewRelayManager(dataDir, e)
		api, err := NewAPIServer(dataDir, e)
		if err != nil {
			slog.Error("engine: failed to create api server", "error", err)
		} else {
			e.api = api
		}
	}
	return e
}

func (e *Engine) RegisterDialog(p Dialog) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.dialogs[p.Name()] = p
}

func (e *Engine) RegisterLLM(a LLM) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.llms[a.Name()] = a
	if e.defaultLLM == "" {
		e.defaultLLM = a.Name()
	}
}

func (e *Engine) SetDefaultLLM(name string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.defaultLLM = name
}

func (e *Engine) LoadPlugins(cfg EngineConfig) error {
	for _, p := range cfg.Dialogs {
		pInst, err := CreateDialog(p.Type, p.Options)
		if err != nil {
			return err
		}
		e.RegisterDialog(pInst)
	}

	for _, a := range cfg.LLMs {
		aInst, err := CreateLLM(a.Type, a.Options)
		if err != nil {
			return err
		}
		e.RegisterLLM(aInst)
	}

	if cfg.DefaultLLM != "" {
		e.SetDefaultLLM(cfg.DefaultLLM)
	}
	return nil
}

func (e *Engine) AutoLoad() {
	// 1. Auto-discover dialogs
	for _, name := range ListDialogFactories() {
		if p, err := CreateDialog(name, nil); err == nil {
			e.RegisterDialog(p)
			slog.Info("auto-loaded dialog", "name", name)
		} else {
			slog.Debug("skipped dialog autoload", "name", name, "reason", err)
		}
	}

	// 2. Auto-discover llms
	for _, name := range ListLLMFactories() {
		if a, err := CreateLLM(name, nil); err == nil {
			e.RegisterLLM(a)
			slog.Info("auto-loaded llm", "name", name)
		} else {
			slog.Debug("skipped llm autoload", "name", name, "reason", err)
		}
	}
}

func (e *Engine) Start(ctx context.Context) error {
	if e.cron != nil {
		if err := e.cron.Start(); err != nil {
			slog.Error("engine: failed to start cron scheduler", "error", err)
		}
	}
	if e.api != nil {
		e.api.Start()
	}
	for _, p := range e.dialogs {
		if err := p.Start(e.handleMessage); err != nil {
			return err
		}
	}

	slog.Info("engine: started", "dialogs", len(e.dialogs), "llms", len(e.llms))
	<-ctx.Done()
	return nil
}
func (e *Engine) HandleCron(_ *CronJob) error {
	// Logic to resolve access and send message back to handleMessage
	return nil
}

// HandleRelay injects a message from another bot.
func (e *Engine) HandleRelay(_ context.Context, _, _, _, _ string) (string, error) {
	// Logic to handle inter-bot communication
	return "relay ok", nil
}

func (e *Engine) handleMessage(p Dialog, msg *Message) {
	ctx := context.Background()

	// 1. Run pipe pipeline (Dedup -> Safety -> Command)
	for _, pipe := range e.pipes {
		if pipe.Handle(ctx, p, msg) {
			return // Intercepted
		}
	}

	// 2. Get session
	session := e.sessions.GetOrCreateActive(msg.SessionKey)
	if !session.TryLock() {
		_ = p.Reply(ctx, msg.ReplyCtx, e.translator.T("previous_processing"))
		return
	}
	defer session.Unlock()

	// --- 权限处理逻辑开始 ---
	pending := session.GetPendingAction()
	if strings.HasPrefix(pending, "confirm_tool:") {
		if msg.Content == "继续" || msg.Content == "好" || msg.Content == "确认" || msg.Content == "允许" {
			// 清除挂起状态并告知 LLM 用户已同意
			session.SetPendingAction("")
			msg.Content = fmt.Sprintf("[User Authorized: %s] 请继续执行刚才的操作。", strings.TrimPrefix(pending, "confirm_tool:"))
		} else {
			// 用户发了别的，认为是在取消授权或提新问题
			session.SetPendingAction("")
		}
	}
	// --- 权限处理逻辑结束 ---

	// 4. Start llm session
	llmName, _ := session.GetMetadata()["llm"].(string)
	if llmName == "" {
		llmName = e.defaultLLM
	}

	actualSessionID := session.GetID()
	if persistedID, ok := session.GetMetadata()["llm_session_id_"+llmName].(string); ok && persistedID != "" {
		actualSessionID = persistedID
	}

	var targetLLM LLM
	e.mu.RLock()
	if llmName != "" {
		targetLLM = e.llms[llmName]
	}
	if targetLLM == nil {
		targetLLM = e.llms[e.defaultLLM]
	}
	if targetLLM == nil {
		for _, a := range e.llms {
			targetLLM = a
			break
		}
	}
	e.mu.RUnlock()

	if targetLLM == nil {
		_ = p.Reply(ctx, msg.ReplyCtx, "Error: no llm registered")
		return
	}

	agSess, err := targetLLM.StartSession(ctx, actualSessionID)
	if err != nil {
		_ = p.Reply(ctx, msg.ReplyCtx, "Error starting session: "+err.Error())
		return
	}
	defer func() {
		_ = agSess.Close()
	}()

	// 5. Send message to llm
	err = agSess.Send(msg.Content, msg.Images, msg.Files)
	if err != nil {
		_ = p.Reply(ctx, msg.ReplyCtx, "Error sending message: "+err.Error())
		return
	}

	// 6. Handle llm events
	var textBuffer strings.Builder
	lastFlush := time.Now()

	flushBuffer := func() {
		if textBuffer.Len() > 0 {
			if err := p.Reply(ctx, msg.ReplyCtx, textBuffer.String()); err != nil {
				slog.Error("engine: failed to send reply", "error", err, "platform", p.Name())
			}
			textBuffer.Reset()
			lastFlush = time.Now()
		}
	}

	for ev := range agSess.Events() {
		switch ev.Type {
		case EventText:
			if ev.Content != "" {
				textBuffer.WriteString(ev.Content)
				// 通用策略：150字符或3秒发送一次
				if textBuffer.Len() > 150 || time.Since(lastFlush) > 3*time.Second {
					flushBuffer()
				}
			}
		case EventThinking:
			if ev.Content != "" && textBuffer.Len() == 0 {
				slog.Debug("engine: assistant thinking", "content", ev.Content)
			}
		case EventToolUse:
			flushBuffer()
			slog.Info("engine: tool use", "tool", ev.ToolName, "input", ev.ToolInput)
			_ = p.Reply(ctx, msg.ReplyCtx, fmt.Sprintf("🔍 正在使用工具: %s...", ev.ToolName))
		case EventPermissionRequest:
			flushBuffer()
			_ = p.Reply(ctx, msg.ReplyCtx, fmt.Sprintf("🛡️ 权限请求: %s 想执行 %s(%s)。\n\n回复“继续”允许。", ev.ToolName, ev.ToolName, ev.ToolInput))
			session.SetPendingAction("confirm_tool:" + ev.ToolName)
			return
		case EventError:
			flushBuffer()
			slog.Error("engine: llm error", "error", ev.Error)
			_ = p.Reply(ctx, msg.ReplyCtx, "❌ 出错了: "+ev.Error.Error())
		case EventResult:
			flushBuffer()
			if ev.SessionID != "" {
				session.SetMetadata("llm_session_id_"+llmName, ev.SessionID)
			}
			if ev.Error != nil {
				_ = p.Reply(ctx, msg.ReplyCtx, "❌ 错误: "+ev.Error.Error())
			}
			if ev.Done {
				return
			}
		}
	}
}
