package agent

import (
	"context"
	"log/slog"
	"sync"
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
		if err := p.Reply(ctx, msg.ReplyCtx, e.translator.T("previous_processing")); err != nil {
			slog.Debug("engine: failed to reply", "error", err)
		}
		return
	}
	defer session.Unlock()

	// 3. Select llm
	var targetLLM LLM
	llmName, _ := session.GetMetadata()["llm"].(string)

	e.mu.RLock()
	if llmName != "" {
		targetLLM = e.llms[llmName]
	}
	if targetLLM == nil && e.defaultLLM != "" {
		targetLLM = e.llms[e.defaultLLM]
	}
	if targetLLM == nil {
		// Fallback to first registered llm
		for _, a := range e.llms {
			targetLLM = a
			break
		}
	}
	e.mu.RUnlock()

	if targetLLM == nil {
		if err := p.Reply(ctx, msg.ReplyCtx, e.translator.T("error", "no llm registered")); err != nil {
			slog.Debug("engine: failed to reply", "error", err)
		}
		return
	}

	// 4. Start llm session
	agSess, err := targetLLM.StartSession(ctx, session.GetID())
	if err != nil {
		if err := p.Reply(ctx, msg.ReplyCtx, e.translator.T("error", err)); err != nil {
			slog.Debug("engine: failed to reply", "error", err)
		}
		return
	}
	defer func() {
		if err := agSess.Close(); err != nil {
			slog.Debug("engine: failed to close session", "error", err)
		}
	}()

	// 5. Send message to llm
	err = agSess.Send(msg.Content, msg.Images, msg.Files)
	if err != nil {
		if err := p.Reply(ctx, msg.ReplyCtx, e.translator.T("error", err)); err != nil {
			slog.Debug("engine: failed to reply", "error", err)
		}
		return
	}

	// 6. Handle llm events
	for ev := range agSess.Events() {
		if ev.Type == "text" {
			if err := p.Reply(ctx, msg.ReplyCtx, ev.Content); err != nil {
				slog.Debug("engine: failed to reply", "error", err)
			}
		}
	}
}
