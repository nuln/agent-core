package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// RelayBinding represents a bot-to-bot relay binding.
type RelayBinding struct {
	Access string            `json:"access"`
	ChatID string            `json:"chat_id"`
	Bots   map[string]string `json:"bots"` // project name → bot display name
}

// RelayManager coordinates bot-to-bot message relay.
type RelayManager struct {
	mu        sync.RWMutex
	engine    *Engine
	bindings  map[string]*RelayBinding
	storePath string
}

func NewRelayManager(dataDir string, engine *Engine) *RelayManager {
	rm := &RelayManager{
		engine:   engine,
		bindings: make(map[string]*RelayBinding),
	}
	if dataDir != "" {
		rm.storePath = filepath.Join(dataDir, "relay_bindings.json")
		rm.load()
	}
	return rm
}

func (rm *RelayManager) Bind(accName, chatID string, bots map[string]string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.bindings[chatID] = &RelayBinding{
		Access: accName,
		ChatID: chatID,
		Bots:   bots,
	}
	rm.saveLocked()
}

func (rm *RelayManager) Send(ctx context.Context, fromProj, toProj, sessionKey, content string) (string, error) {
	accName, chatID, _ := parseSessionKeyParts(sessionKey)

	rm.mu.RLock()
	binding := rm.bindings[chatID]
	rm.mu.RUnlock()

	if binding == nil {
		return "", fmt.Errorf("relay: no binding for this chat")
	}

	// Execution: inject message into target llm session
	response, err := rm.engine.HandleRelay(ctx, fromProj, toProj, chatID, content)
	if err != nil {
		return "", err
	}

	// Post to group chat for visibility
	rm.sendToGroup(ctx, accName, chatID, fmt.Sprintf("[%s → %s] %s\n\n[%s] %s", fromProj, toProj, content, toProj, response))

	return response, nil
}

func (rm *RelayManager) sendToGroup(ctx context.Context, accName, chatID string, content string) {
	rm.engine.mu.RLock()
	defer rm.engine.mu.RUnlock()

	for _, p := range rm.engine.dialogs {
		if p.Name() != accName {
			continue
		}
		if rc, ok := p.(ReplyContextReconstructor); ok {
			sessionKey := accName + ":" + chatID + ":relay"
			rctx, err := rc.ReconstructReplyCtx(sessionKey)
			if err == nil {
				if err := p.Send(ctx, rctx, content); err != nil {
					slog.Debug("relay: failed to send message", "error", err)
				}
			}
		}
		break
	}
}

func parseSessionKeyParts(sessionKey string) (accName, chatID string, userID string) {
	parts := strings.SplitN(sessionKey, ":", 3)
	if len(parts) >= 1 {
		accName = parts[0]
	}
	if len(parts) >= 2 {
		chatID = parts[1]
	}
	if len(parts) >= 3 {
		userID = parts[2]
	}
	return
}

func (rm *RelayManager) saveLocked() {
	if rm.storePath == "" {
		return
	}
	data, err := json.MarshalIndent(rm.bindings, "", "  ")
	if err != nil {
		slog.Error("relay: failed to marshal bindings", "error", err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(rm.storePath), 0o755); err != nil {
		slog.Error("relay: failed to create directory", "error", err)
		return
	}
	if err := AtomicWriteFile(rm.storePath, data, 0o644); err != nil {
		slog.Error("relay: failed to save bindings", "error", err)
	}
}

func (rm *RelayManager) load() {
	data, err := os.ReadFile(rm.storePath)
	if err != nil {
		return
	}
	if err := json.Unmarshal(data, &rm.bindings); err != nil {
		slog.Warn("relay: failed to unmarshal bindings", "error", err)
	}
}
