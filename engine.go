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
	Dialogs            []PluginConfig `json:"dialogs"`
	LLMs               []PluginConfig `json:"llms"`
	SkillManagers      []PluginConfig `json:"skill_managers"`
	SkillManagerLimits map[string]int `json:"skill_manager_limits"`
	DefaultLLM         string         `json:"default_llm"`
}

// Engine routes messages between Dialog platforms and LLM providers.
type Engine struct {
	dialogs            map[string]Dialog
	llms               map[string]LLM
	skillManagers      []SkillManager
	sessions           SessionProvider
	translator         Translator
	stt                SpeechToText
	tts                TextToSpeech
	api                *APIServer
	pipes              []Pipe
	defaultLLM         string
	skillManagerLimits map[string]int
	loadedSkillTypes   map[string]int
	store              KVStoreProvider
	interactionLogger  *InteractionLogger
	sessionState       SessionStateManager
	reflector          Reflector
	relay              *RelayManager
	uiAbilities        map[string]UIAbility
	instanceStore      *PluginInstanceStore
	dataDir            string
	started            bool
	mu                 sync.RWMutex
}

// NewEngine creates a new Engine with the given options.
// Dependencies are injected via EngineOption functions.
func NewEngine(opts ...EngineOption) *Engine {
	options := &EngineOptions{}
	for _, opt := range opts {
		opt(options)
	}

	e := &Engine{
		sessions:           options.Sessions,
		translator:         options.Translator,
		stt:                options.STT,
		tts:                options.TTS,
		store:              options.Store,
		reflector:          options.Reflector,
		dataDir:            options.DataDir,
		dialogs:            make(map[string]Dialog),
		llms:               make(map[string]LLM),
		uiAbilities:        make(map[string]UIAbility),
		skillManagerLimits: make(map[string]int),
		loadedSkillTypes:   make(map[string]int),
	}

	// Default translator if none provided
	if e.translator == nil {
		e.translator = &noopTranslator{}
	}

	// Initialize internal stores from KVStoreProvider
	if e.store != nil {
		// SessionManager (if not provided)
		if e.sessions == nil {
			sessKV, _ := e.store.GetStore("_core/sessions")
			if sessKV != nil {
				e.sessions = NewSessionManager(sessKV)
			}
		}

		// Interaction logger
		logStore, _ := e.store.GetStore("_core/interactions")
		if logStore != nil {
			e.interactionLogger = NewInteractionLogger(logStore)
		}

		// Session state manager
		stateKV, _ := e.store.GetStore("_core/session_states")
		if stateKV != nil {
			e.sessionState = NewSessionStateManager(stateKV)
		}

		// Reflector is optional — injected via WithReflector
		// No default reflector creation; use reflector plugins instead

		// Plugin instance store
		instKV, _ := e.store.GetStore("_core/plugin_instances")
		if instKV != nil {
			e.instanceStore = NewPluginInstanceStore(instKV)
		}
	}

	var storage KVStoreProvider
	if e.store != nil {
		storage = e.store
	}

	e.pipes = CreatePipes(PipeContext{
		Sessions:   e.sessions,
		Translator: e.translator,
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

	if e.dataDir != "" {
		var relayStore KVStore
		if e.store != nil {
			relayStore, _ = e.store.GetStore("_core/relays")
		}
		e.relay = NewRelayManager(relayStore, e)
		api, err := NewAPIServer(e.dataDir, e)
		if err != nil {
			slog.Error("engine: failed to create api server", "error", err)
		} else {
			e.api = api
		}
	}
	return e
}

// noopTranslator returns the key as-is when no translator is configured.
type noopTranslator struct{}

func (n *noopTranslator) T(key string, args ...any) string {
	if len(args) > 0 {
		return fmt.Sprintf(key, args...)
	}
	return key
}

func (e *Engine) RegisterDialog(p Dialog) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.dialogs[p.Name()] = p

	// Inject scoped storage if the Dialog plugin supports it.
	if sa, ok := p.(StorageAware); ok && e.store != nil {
		sa.SetStorage(&scopedProvider{parent: e.store, prefix: "plugins/" + p.Name()})
	}

	// Register UI abilities if supported
	if wp, ok := p.(WebProvider); ok {
		e.uiAbilities[p.Name()] = wp.GetUIAbility()
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
	if sa, ok := a.(StorageAware); ok && e.store != nil {
		sa.SetStorage(&scopedProvider{parent: e.store, prefix: "plugins/" + a.Name()})
	}

	// Register UI abilities if supported
	if wp, ok := a.(WebProvider); ok {
		e.uiAbilities[a.Name()] = wp.GetUIAbility()
	}

	// Update reflector's evaluator LLM if it supports SetEvalLLM
	if e.reflector != nil {
		type evalSetter interface{ SetEvalLLM(LLM) }
		if r, ok := e.reflector.(evalSetter); ok {
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

	e.skillManagerLimits = cfg.SkillManagerLimits
	for _, smCfg := range cfg.SkillManagers {
		var storage KVStoreProvider
		if e.store != nil {
			storage = &scopedProvider{parent: e.store, prefix: "plugins/" + smCfg.Type}
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
		limit = 1
	}

	currentCount := e.loadedSkillTypes[smType]
	if currentCount >= limit {
		slog.Warn("skipping skill manager: limit reached", "type", smType, "limit", limit, "manager", sm.Name())
		return
	}

	e.skillManagers = append(e.skillManagers, sm)
	e.loadedSkillTypes[smType]++

	if wp, ok := sm.(WebProvider); ok {
		e.uiAbilities[sm.Name()] = wp.GetUIAbility()
	}

	slog.Info("loaded skill manager", "name", sm.Name(), "type", smType, "count", e.loadedSkillTypes[smType], "limit", limit)
}

// AutoLoad auto-discovers and loads LLM providers and skill managers.
func (e *Engine) AutoLoad() {
	for _, name := range ListLLMFactories() {
		spec, ok := GetPluginConfigSpec(name)
		if ok && !anyEnvVarSet(spec) {
			slog.Debug("skipped llm autoload: no env vars configured", "name", name)
			continue
		}
		if a, err := CreateLLM(name, nil); err == nil {
			e.RegisterLLM(a)
			slog.Info("auto-loaded llm", "name", name)
		} else {
			slog.Debug("skipped llm autoload", "name", name, "reason", err)
		}
	}

	for _, name := range ListSkillManagerFactories() {
		var storage KVStoreProvider
		if e.store != nil {
			storage = &scopedProvider{parent: e.store, prefix: "plugins/" + name}
		}
		if sm, err := CreateSkillManager(name, nil, storage); err == nil {
			e.registerSkillManager(sm)
		} else {
			slog.Debug("skipped skill manager autoload", "name", name, "reason", err)
		}
	}
}

// LoadSavedInstances restores all enabled plugin instances from the store.
func (e *Engine) LoadSavedInstances() {
	if e.instanceStore == nil {
		return
	}
	instances, err := e.instanceStore.List()
	if err != nil {
		slog.Error("engine: failed to load plugin instances", "error", err)
		return
	}
	for _, inst := range instances {
		if !inst.Enabled {
			continue
		}
		switch inst.Category {
		case "dialog":
			d, err := CreateDialog(inst.PluginName, inst.Config)
			if err != nil {
				slog.Error("engine: failed to restore dialog", "name", inst.PluginName, "error", err)
				continue
			}
			e.RegisterDialog(d)
			slog.Info("engine: restored dialog", "name", inst.PluginName)
		}
	}
}

// EnablePlugin creates a dialog plugin, persists its config, and starts it.
func (e *Engine) EnablePlugin(pluginName string, config map[string]any) error {
	d, err := CreateDialog(pluginName, config)
	if err != nil {
		return fmt.Errorf("engine: failed to create plugin %q: %w", pluginName, err)
	}

	if e.instanceStore != nil {
		existing, _ := e.instanceStore.Get(pluginName)
		inst := PluginInstance{
			ID:         pluginName,
			PluginName: pluginName,
			Category:   "dialog",
			Label:      pluginName,
			Config:     config,
			Enabled:    true,
		}
		if existing != nil {
			inst.CreatedAt = existing.CreatedAt
		}
		if err := e.instanceStore.Save(inst); err != nil {
			slog.Warn("engine: failed to persist plugin", "name", pluginName, "error", err)
		}
	}

	e.mu.Lock()
	old, exists := e.dialogs[pluginName]
	e.mu.Unlock()
	if exists {
		_ = old.Stop()
	}

	e.RegisterDialog(d)

	e.mu.RLock()
	alreadyStarted := e.started
	e.mu.RUnlock()
	if alreadyStarted {
		return d.Start(e.handleMessage)
	}
	return nil
}

// DisablePlugin stops a running dialog and marks it disabled.
func (e *Engine) DisablePlugin(pluginName string) error {
	e.mu.Lock()
	d, ok := e.dialogs[pluginName]
	if ok {
		delete(e.dialogs, pluginName)
		delete(e.uiAbilities, pluginName)
	}
	e.mu.Unlock()

	if e.instanceStore != nil {
		inst, _ := e.instanceStore.Get(pluginName)
		if inst != nil {
			inst.Enabled = false
			_ = e.instanceStore.Save(*inst)
		}
	}

	if ok {
		return d.Stop()
	}
	return nil
}

// GetPluginInstances returns all persisted plugin instances.
func (e *Engine) GetPluginInstances() ([]PluginInstance, error) {
	if e.instanceStore == nil {
		return nil, nil
	}
	return e.instanceStore.List()
}

func (e *Engine) Start(ctx context.Context) error {
	if e.api != nil {
		e.api.Start()
	}
	if e.sessionState != nil {
		go func() {
			ticker := time.NewTicker(5 * time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					_ = e.sessionState.CleanupExpired()
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	e.mu.Lock()
	e.started = true
	e.mu.Unlock()

	for _, p := range e.dialogs {
		if err := p.Start(e.handleMessage); err != nil {
			return err
		}
	}

	slog.Info("engine: started", "dialogs", len(e.dialogs), "llms", len(e.llms))
	<-ctx.Done()
	return nil
}

// HandleRelay handles inter-bot communication.
func (e *Engine) HandleRelay(_ context.Context, _, _, _, _ string) (string, error) {
	return "relay ok", nil
}

func (e *Engine) GetUIManifest() map[string]UIAbility {
	e.mu.RLock()
	defer e.mu.RUnlock()
	m := make(map[string]UIAbility)
	for k, v := range e.uiAbilities {
		m[k] = v
	}
	return m
}

// EngineStatus describes the full internal state of the engine.
type EngineStatus struct {
	Dialogs []ComponentInfo `json:"dialogs"`
	LLMs    []ComponentInfo `json:"llms"`
	Skills  []SkillInfo     `json:"skills"`
}

type ComponentInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type SkillInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Manager     string `json:"manager"`
}

func (e *Engine) GetStatus(ctx context.Context) EngineStatus {
	e.mu.RLock()
	defer e.mu.RUnlock()

	status := EngineStatus{}
	for _, d := range e.dialogs {
		status.Dialogs = append(status.Dialogs, ComponentInfo{Name: d.Name()})
	}
	for _, l := range e.llms {
		status.LLMs = append(status.LLMs, ComponentInfo{Name: l.Name(), Description: l.Description()})
	}
	for _, sm := range e.skillManagers {
		skills, err := sm.List(ctx)
		if err == nil {
			for _, s := range skills {
				status.Skills = append(status.Skills, SkillInfo{Name: s.Name, Description: s.Description, Manager: sm.Name()})
			}
		}
	}
	return status
}

func (e *Engine) GetDialogInstances() map[string][]DialogInstanceStatus {
	e.mu.RLock()
	defer e.mu.RUnlock()

	results := make(map[string][]DialogInstanceStatus)
	for name, d := range e.dialogs {
		if md, ok := d.(ManageableDialog); ok {
			results[name] = md.GetInstances()
		} else {
			results[name] = []DialogInstanceStatus{{ID: "default", Status: "connected", Description: "Static platform"}}
		}
	}
	return results
}

func (e *Engine) StartDialogAuth(ctx context.Context, platform string) (*AuthSession, error) {
	e.mu.RLock()
	d, ok := e.dialogs[platform]
	e.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("platform %q not found", platform)
	}
	md, ok := d.(ManageableDialog)
	if !ok {
		return nil, fmt.Errorf("platform %q does not support interactive auth", platform)
	}
	return md.StartAuth(ctx)
}

func (e *Engine) PollDialogAuth(ctx context.Context, platform, sessionID string) (*AuthSession, error) {
	e.mu.RLock()
	d, ok := e.dialogs[platform]
	e.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("platform %q not found", platform)
	}
	md, ok := d.(ManageableDialog)
	if !ok {
		return nil, fmt.Errorf("platform %q does not support interactive auth", platform)
	}
	return md.PollAuth(ctx, sessionID)
}

func (e *Engine) AddDialogInstance(ctx context.Context, platform string, params map[string]string) error {
	e.mu.RLock()
	d, ok := e.dialogs[platform]
	e.mu.RUnlock()
	if !ok {
		return fmt.Errorf("platform %q not found", platform)
	}
	md, ok := d.(ManageableDialog)
	if !ok {
		return fmt.Errorf("platform %q does not support manual instance addition", platform)
	}
	return md.AddInstance(ctx, params)
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
			content := strings.ToLower(strings.TrimSpace(msg.Content))
			if content == "exit" || content == "cancel" || content == "quit" || content == "退出" || content == "取消" {
				_ = e.sessionState.ClearState(msg.SessionKey)
				_ = p.Reply(ctx, msg.ReplyCtx, e.translator.T("session_released"))
				return
			}

			if state.LockedBy != "" {
				for _, pipe := range e.pipes {
					if pipe.Handle(ctx, p, msg) {
						return
					}
				}
				_ = p.Reply(ctx, msg.ReplyCtx, fmt.Sprintf(e.translator.T("session_locked_wait"), state.Action))
				return
			}
		}
	}

	// 1. Run pipe pipeline
	for _, pipe := range e.pipes {
		if pipe.Handle(ctx, p, msg) {
			return
		}
	}

	// Record incoming message in Dialog plugin's own storage.
	if recorder, ok := p.(DialogRecorder); ok {
		if err := recorder.RecordMessage(traceID, msg); err != nil {
			slog.Warn("engine: dialog record failed", "error", err, "trace_id", traceID)
		}
	}

	// 2. Get session
	if e.sessions == nil {
		_ = p.Reply(ctx, msg.ReplyCtx, "Error: no session provider configured")
		return
	}
	session := e.sessions.GetOrCreateActive(msg.SessionKey)
	if !session.TryLock() {
		_ = p.Reply(ctx, msg.ReplyCtx, e.translator.T("previous_processing"))
		return
	}
	defer session.Unlock()

	session.AppendHistory(HistoryEntry{Role: RoleUser, Content: msg.Content})

	// Permission handling
	pending := session.GetPendingAction()
	if strings.HasPrefix(pending, "confirm_tool:") {
		if msg.Content == "继续" || msg.Content == "好" || msg.Content == "确认" || msg.Content == "允许" {
			session.SetPendingAction("")
			msg.Content = fmt.Sprintf("[User Authorized: %s] 请继续执行刚才的操作。", strings.TrimPrefix(pending, "confirm_tool:"))
		} else {
			session.SetPendingAction("")
		}
	}

	// 3. Select LLM
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

	if sr, ok := agSess.(SessionRecorder); ok {
		sr.SetTraceID(traceID)
	}

	var usedSkillList []string
	defer func() {
		_ = agSess.Close()

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

		if e.reflector != nil && interactionStatus == "completed" && len(usedSkillList) > 0 {
			_ = e.reflector.Reflect(context.Background(), msg.SessionKey, traceID, usedSkillList)
		}
	}()

	// 4. Inject skills
	finalContent := msg.Content
	var allSkills []*Skill
	for _, sm := range e.skillManagers {
		skills, err := sm.List(ctx)
		if err == nil {
			allSkills = append(allSkills, skills...)
		}
	}

	if len(allSkills) > 0 {
		index := buildSkillIndexPrompt(allSkills)
		for _, s := range allSkills {
			usedSkillList = append(usedSkillList, s.Name)
		}
		if pinj, ok := targetLLM.(PlatformPromptInjector); ok {
			pinj.SetPlatformPrompt(AgentSystemPrompt() + index)
		} else {
			finalContent = fmt.Sprintf("[SYSTEM: %s]\n\n%s", index, msg.Content)
		}
	}

	// 5. Send message to LLM
	err = agSess.Send(finalContent, msg.Images, msg.Files)
	if err != nil {
		_ = p.Reply(ctx, msg.ReplyCtx, "Error sending message: "+err.Error())
		return
	}

	// 6. Handle LLM events
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
			_ = p.Reply(ctx, msg.ReplyCtx, fmt.Sprintf("🛡️ 权限请求: %s 想执行 %s(%s)。\n\n回复「继续」允许。", ev.ToolName, ev.ToolName, ev.ToolInput))
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
				session.AppendHistory(HistoryEntry{Role: RoleAssistant, Content: textBuffer.String()})

				if len(e.skillManagers) > 0 {
					go func() {
						slog.Info("engine: triggering skill extraction", "session", actualSessionID)
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

// buildSkillIndexPrompt constructs a compact index of available skills.
func buildSkillIndexPrompt(skills []*Skill) string {
	if len(skills) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n## Available Skills\n")
	sb.WriteString("You have the following skills available.\n\n")
	for _, s := range skills {
		name := s.DisplayName
		if name == "" {
			name = s.Name
		}
		fmt.Fprintf(&sb, "- %s: %s\n", name, s.Description)
	}
	sb.WriteString("\nTo use a skill, refer to its instructions or ask the user to invoke it.")
	return sb.String()
}

// scopedProvider wraps a KVStoreProvider to add a prefix.
type scopedProvider struct {
	parent KVStoreProvider
	prefix string
}

func (sp *scopedProvider) GetStore(name string) (KVStore, error) {
	fullName := sp.prefix
	if name != "" {
		if fullName != "" && fullName[len(fullName)-1] != '/' {
			fullName += "/"
		}
		fullName += name
	}
	return sp.parent.GetStore(fullName)
}
