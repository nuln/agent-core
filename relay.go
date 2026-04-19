package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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
	mu       sync.RWMutex
	engine   *Engine
	bindings map[string]*RelayBinding // L1 in-memory cache
	store    KVStore                  // persistent storage backend
}

// NewRelayManager creates a RelayManager backed by a KVStore.
// If store is nil, bindings are memory-only (lost on restart).
func NewRelayManager(store KVStore, engine *Engine) *RelayManager {
	rm := &RelayManager{
		engine:   engine,
		bindings: make(map[string]*RelayBinding),
		store:    store,
	}
	rm.load()
	return rm
}

// Bind creates or updates a relay binding. Write-Through: persists to store
// first, then updates the in-memory cache.
func (rm *RelayManager) Bind(accName, chatID string, bots map[string]string) {
	binding := &RelayBinding{
		Access: accName,
		ChatID: chatID,
		Bots:   bots,
	}

	// Write-Through: persist to store before updating cache.
	if rm.store != nil {
		data, err := json.Marshal(binding)
		if err != nil {
			slog.Error("relay: failed to marshal binding", "error", err)
			return
		}
		if err := rm.store.Put([]byte(chatID), data); err != nil {
			slog.Error("relay: failed to persist binding", "error", err)
			return
		}
	}

	rm.mu.Lock()
	rm.bindings[chatID] = binding
	rm.mu.Unlock()
}

// Unbind removes a relay binding.
func (rm *RelayManager) Unbind(chatID string) {
	if rm.store != nil {
		if err := rm.store.Delete([]byte(chatID)); err != nil {
			slog.Error("relay: failed to delete binding", "error", err)
		}
	}

	rm.mu.Lock()
	delete(rm.bindings, chatID)
	rm.mu.Unlock()
}

func (rm *RelayManager) Send(ctx context.Context, fromProj, toProj, sessionKey, content string) (string, error) {
	accName, chatID, _ := parseSessionKeyParts(sessionKey)

	binding := rm.getBinding(chatID)
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

// getBinding implements Read-Through: check L1 cache first, fall back to store.
func (rm *RelayManager) getBinding(chatID string) *RelayBinding {
	// L1 cache lookup
	rm.mu.RLock()
	binding := rm.bindings[chatID]
	rm.mu.RUnlock()
	if binding != nil {
		return binding
	}

	// Store fallback
	if rm.store == nil {
		return nil
	}
	data, err := rm.store.Get([]byte(chatID))
	if err != nil || data == nil {
		return nil
	}
	var b RelayBinding
	if err := json.Unmarshal(data, &b); err != nil {
		slog.Warn("relay: failed to unmarshal binding from store", "chatID", chatID, "error", err)
		return nil
	}

	// Backfill L1 cache
	rm.mu.Lock()
	rm.bindings[chatID] = &b
	rm.mu.Unlock()
	return &b
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

// load preloads all bindings from the store into the L1 cache.
func (rm *RelayManager) load() {
	if rm.store == nil {
		return
	}
	all, err := rm.store.List()
	if err != nil {
		slog.Warn("relay: failed to load bindings from store", "error", err)
		return
	}
	for chatID, data := range all {
		var b RelayBinding
		if err := json.Unmarshal(data, &b); err != nil {
			slog.Warn("relay: failed to unmarshal stored binding", "chatID", chatID, "error", err)
			continue
		}
		rm.bindings[chatID] = &b
	}
	if len(rm.bindings) > 0 {
		slog.Info("relay: loaded bindings from store", "count", len(rm.bindings))
	}
}
