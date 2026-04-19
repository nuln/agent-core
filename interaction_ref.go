package agent

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"
)

// InteractionRef is the core index entry for a Dialog↔LLM interaction.
// It contains NO heavy payload data — only pointers to where each plugin
// stored its own side of the interaction, linked by TraceID.
type InteractionRef struct {
	TraceID         string `json:"trace_id"`
	SessionKey      string `json:"session_key"`
	Timestamp       int64  `json:"ts"`
	SenderPlugin    string `json:"sender"`    // dialog plugin name (e.g. "lark")
	ResponderPlugin string `json:"responder"` // llm plugin name (e.g. "codex")
	UserID          string `json:"user_id"`
	LatencyMs       int64  `json:"latency_ms"`
	Status          string `json:"status"` // completed / error / interrupted
}

// InteractionLogger provides asynchronous, non-blocking persistence of
// InteractionRef records to a KVStore. It uses a buffered channel and a
// background worker goroutine to avoid blocking the main message loop.
type InteractionLogger struct {
	store KVStore
	ch    chan InteractionRef
	done  chan struct{}
}

// NewInteractionLogger creates a logger backed by the given store.
// If store is nil, Record calls are silently dropped.
func NewInteractionLogger(store KVStore) *InteractionLogger {
	l := &InteractionLogger{
		store: store,
		ch:    make(chan InteractionRef, 1000),
		done:  make(chan struct{}),
	}
	go l.worker()
	return l
}

// Record enqueues an interaction reference for async persistence.
// Never blocks the caller — drops the entry if the buffer is full.
func (l *InteractionLogger) Record(ref InteractionRef) {
	if l.store == nil {
		return
	}
	select {
	case l.ch <- ref:
	default:
		slog.Warn("interaction_logger: buffer full, dropping entry", "trace_id", ref.TraceID)
	}
}

// Stop drains the buffer and shuts down the worker.
func (l *InteractionLogger) Stop() {
	close(l.ch)
	<-l.done
}

func (l *InteractionLogger) worker() {
	defer close(l.done)
	for ref := range l.ch {
		data, err := json.Marshal(ref)
		if err != nil {
			slog.Error("interaction_logger: marshal failed", "error", err)
			continue
		}
		key := fmt.Sprintf("%d:%s", ref.Timestamp, ref.TraceID)
		if err := l.store.Put([]byte(key), data); err != nil {
			slog.Error("interaction_logger: put failed", "error", err, "trace_id", ref.TraceID)
		}
	}
}

// traceCounter is an atomic counter for generating unique trace IDs.
var traceCounter atomic.Int64

// GenerateTraceID creates a trace ID combining timestamp and an atomic counter.
// Format: <unix_nano>-<counter> ensures uniqueness even under high concurrency.
func GenerateTraceID() string {
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), traceCounter.Add(1))
}
