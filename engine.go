package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.etcd.io/bbolt"
)

type PluginConfig struct {
	Type    string         `json:"type"`
	Options map[string]any `json:"options"`
}

type EngineConfig struct {
	Dialogs            []PluginConfig `json:"dialogs"`
	LLMs               []PluginConfig `json:"llms"`
	SkillManagers      []PluginConfig `json:"skill_managers"`
	SkillManagerLimits map[string]int `json:"skill_manager_limits"`
	DefaultLLM         string         `json:"default_llm"`
}

// Engine routes messages between platforms and
type Engine struct {
	dialogs            map[string]Dialog
	llms               map[string]LLM
	skillManagers      []SkillManager
	sessions           SessionProvider
	translator         Translator
	cron               *CronScheduler
	relay              *RelayManager
	stt                SpeechToText
	tts                TextToSpeech
	api                *APIServer
	pipes              []Pipe
	defaultLLM         string
	skillManagerLimits map[string]int
	loadedSkillTypes   map[string]int
	db                 *bbolt.DB
	interactionLogger  *InteractionLogger
	sessionState       SessionStateManager
	reflector          Reflector
	mu                 sync.RWMutex
}

func NewEngine(sessions SessionProvider, t Translator, stt SpeechToText, tts TextToSpeech, dataDir string) *Engine {
	e := &Engine{
		sessions:           sessions,
		translator:         t,
		stt:                stt,
		tts:                tts,
		dialogs:            make(map[string]Dialog),
		llms:               make(map[string]LLM),
		skillManagerLimits: make(map[string]int),
		loadedSkillTypes:   make(map[string]int),
	}

	var db *bbolt.DB
	if dataDir != "" {
		dbPath := filepath.Join(dataDir, "agent.db")
		_ = os.MkdirAll(dataDir, 0o755)
		var err error
		db, err = bbolt.Open(dbPath, 0o600, &bbolt.Options{Timeout: 1 * time.Second})
		if err != nil {
			slog.Error("engine: failed to open boltdb", "path", dbPath, "error", err)
		} else {
			e.db = db
		}
	}

	// Initialize Core Stores
	if e.db != nil {
		// SessionManager (if not provided)
		if e.sessions == nil {
			sessKV, _ := NewBoltStore(e.db, "_core/sessions")
			e.sessions = NewSessionManager(sessKV)
		}

		// Cron Store
		cronKV, _ := NewBoltStore(e.db, "_core/crons")
		cStore, _ := NewCronStore(cronKV)
		e.cron = NewCronScheduler(cStore, e)

		// Interaction logger for tracing Dialog<->LLM interactions
		logStore, _ := NewBoltStore(e.db, "_core/interactions")
		e.interactionLogger = NewInteractionLogger(logStore)

		// Session State Manager
		stateKV, _ := NewBoltStore(e.db, "_core/session_states")
		e.sessionState = NewSessionStateManager(stateKV)

		// Reflector
		e.reflector = NewSessionReflector(e.sessions, nil, e.skillManagers)
	}

	var storage KVStoreProvider
	if e.db != nil {
		storage = NewScopedStoreProvider(e.db, "_core/pipes")
	}

	e.pipes = CreatePipes(PipeContext{
		Sessions:   e.sessions,
		Translator: t,
		Storage:    storage,
		State:      e.sessionState,
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

	if dataDir != "" {
		var relayStore KVStore
		if e.db != nil {
			relayStore, _ = NewBoltStore(e.db, "_core/relays")
		}
		e.relay = NewRelayManager(relayStore, e)
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

	// Inject scoped storage if the Dialog plugin supports it.
	if sa, ok := p.(StorageAware); ok && e.db != nil {
		sa.SetStorage(NewScopedStoreProvider(e.db, "plugins/"+p.Name()))
	}
}

func (e *Engine) RegisterLLM(a LLM) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.llms[a.Name()] = a
	if e.defaultLLM == "" {
		e.defaultLLM = a.Name()
	}

	// Inject scoped storage if the LLM plugin supports it.
	if sa, ok := a.(StorageAware); ok && e.db != nil {
		sa.SetStorage(NewScopedStoreProvider(e.db, "plugins/"+a.Name()))
	}

	// Update reflector's evaluator LLM if it's the first one or matches default
	if e.reflector != nil {
		if r, ok := e.reflector.(*SessionReflector); ok {
			r.SetEvalLLM(a)
		}
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

	// 3. Load skill managers
	e.skillManagerLimits = cfg.SkillManagerLimits
	for _, smCfg := range cfg.SkillManagers {
		var storage KVStoreProvider
		if e.db != nil {
			// Scoped provider for this plugin
			storage = NewScopedStoreProvider(e.db, "plugins/"+smCfg.Type)
		}

		sm, err := CreateSkillManager(smCfg.Type, smCfg.Options, storage)
		if err != nil {
			return err
		}
		e.registerSkillManager(sm)
	}

	if cfg.DefaultLLM != "" {
		e.SetDefaultLLM(cfg.DefaultLLM)
	}
	return nil
}

func (e *Engine) registerSkillManager(sm SkillManager) {
	e.mu.Lock()
	defer e.mu.Unlock()

	smType := sm.Type()
	limit, ok := e.skillManagerLimits[smType]
	if !ok {
		limit = 1 // Default limit
	}

	currentCount := e.loadedSkillTypes[smType]
	if currentCount >= limit {
		slog.Warn("skipping skill manager: limit reached for type", "type", smType, "limit", limit, "manager", sm.Name())
		return
	}

	e.skillManagers = append(e.skillManagers, sm)
	e.loadedSkillTypes[smType]++
	slog.Info("loaded skill manager", "name", sm.Name(), "type", smType, "count", e.loadedSkillTypes[smType], "limit", limit)
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

	// 3. Auto-discover skill managers
	for _, name := range ListSkillManagerFactories() {
		var storage KVStoreProvider
		if e.db != nil {
			storage = NewScopedStoreProvider(e.db, "plugins/"+name)
		}
		if sm, err := CreateSkillManager(name, nil, storage); err == nil {
			e.registerSkillManager(sm)
		} else {
			slog.Debug("skipped skill manager autoload", "name", name, "reason", err)
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
	if e.sessionState != nil {
		go func() {
			ticker := time.NewTicker(5 * time.Minute)
			for range ticker.C {
				_ = e.sessionState.CleanupExpired()
			}
		}()
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
	startAt := time.Now()
	traceID := GenerateTraceID()
	interactionStatus := "completed"

	// 0. Session State check & Escape Logic
	if e.sessionState != nil {
		state, err := e.sessionState.GetState(msg.SessionKey)
		if err == nil && state != nil {
			// Check Global Escape keywords
			content := strings.ToLower(strings.TrimSpace(msg.Content))
			if content == "exit" || content == "cancel" || content == "quit" || content == "退出" || content == "取消" {
				_ = e.sessionState.ClearState(msg.SessionKey)
				_ = p.Reply(ctx, msg.ReplyCtx, e.translator.T("session_released"))
				return
			}

			// Logical Lock: Route to specific plugin
			if state.LockedBy != "" {
				// Based on design confirmed: "completely shield LLM".
				// So we ONLY run pipes and then RETURN.
				for _, pipe := range e.pipes {
					if pipe.Handle(ctx, p, msg) {
						return
					}
				}
				// If no pipe handled it but state is locked, it's a "hanging" lock or invalid input.
				_ = p.Reply(ctx, msg.ReplyCtx, e.translator.T("session_locked_wait", state.Action))
				return
			}
		}
	}

	// 1. Run pipe pipeline (Dedup -> Safety -> Command)
	for _, pipe := range e.pipes {
		if pipe.Handle(ctx, p, msg) {
			return // Intercepted
		}
	}

	// Record incoming message in the Dialog plugin's own storage.
	if recorder, ok := p.(DialogRecorder); ok {
		if err := recorder.RecordMessage(traceID, msg); err != nil {
			slog.Warn("engine: dialog record failed", "error", err, "trace_id", traceID)
		}
	}

	// 2. Get session
	session := e.sessions.GetOrCreateActive(msg.SessionKey)
	if !session.TryLock() {
		_ = p.Reply(ctx, msg.ReplyCtx, e.translator.T("previous_processing"))
		return
	}
	defer session.Unlock()

	// Record user message in history
	session.AppendHistory(HistoryEntry{Role: RoleUser, Content: msg.Content})

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

	// Notify the LLM session about the current trace ID for self-recording.
	if sr, ok := agSess.(SessionRecorder); ok {
		sr.SetTraceID(traceID)
	}

	var usedSkillList []string
	defer func() {
		_ = agSess.Close()

		// Record interaction index in Core.
		if e.interactionLogger != nil {
			e.interactionLogger.Record(InteractionRef{
				TraceID:         traceID,
				SessionKey:      msg.SessionKey,
				Timestamp:       startAt.UnixMilli(),
				SenderPlugin:    p.Name(),
				ResponderPlugin: targetLLM.Name(),
				UserID:          msg.UserID,
				LatencyMs:       time.Since(startAt).Milliseconds(),
				Status:          interactionStatus,
			})
		}

		// Trigger Reflector for skill evaluation
		if e.reflector != nil && interactionStatus == "completed" && len(usedSkillList) > 0 {
			_ = e.reflector.Reflect(context.Background(), msg.SessionKey, traceID, usedSkillList)
		}
	}()

	// 4.5 Inject skills if available
	finalContent := msg.Content
	var allSkills []*Skill
	for _, sm := range e.skillManagers {
		skills, err := sm.List(ctx)
		if err == nil {
			allSkills = append(allSkills, skills...)
		}
	}

	if len(allSkills) > 0 {
		index := BuildSkillIndexPrompt(allSkills)
		for _, s := range allSkills {
			usedSkillList = append(usedSkillList, s.Name)
		}
		if pinj, ok := targetLLM.(PlatformPromptInjector); ok {
			pinj.SetPlatformPrompt(AgentSystemPrompt() + index)
		} else {
			// Fallback: prepend to content for models without system prompt support
			finalContent = fmt.Sprintf("[SYSTEM: %s]\n\n%s", index, msg.Content)
		}
	}

	// 5. Send message to llm
	err = agSess.Send(finalContent, msg.Images, msg.Files)
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
			interactionStatus = "interrupted"
			return
		case EventError:
			flushBuffer()
			slog.Error("engine: llm error", "error", ev.Error)
			_ = p.Reply(ctx, msg.ReplyCtx, "❌ 出错了: "+ev.Error.Error())
			interactionStatus = "error"
		case EventResult:
			flushBuffer()
			if ev.SessionID != "" {
				session.SetMetadata("llm_session_id_"+llmName, ev.SessionID)
			}
			if ev.Error != nil {
				_ = p.Reply(ctx, msg.ReplyCtx, "❌ 错误: "+ev.Error.Error())
			}
			if ev.Done {
				// Record assistant response in history
				session.AppendHistory(HistoryEntry{Role: RoleAssistant, Content: textBuffer.String()})

				// Autonomous Evolution: Trigger skill extraction at end of session
				if len(e.skillManagers) > 0 {
					go func() {
						slog.Info("engine: triggering skill extraction", "session", actualSessionID)
						// Fetch history from LLM if supported
						var history []HistoryEntry
						if hp, ok := targetLLM.(HistoryProvider); ok {
							history, _ = hp.GetSessionHistory(ctx, actualSessionID, 50)
						}
						if len(history) > 0 {
							for _, sm := range e.skillManagers {
								_, _ = sm.Extract(ctx, targetLLM, history)
							}
						}
					}()
				}
				return
			}
		}
	}
}
