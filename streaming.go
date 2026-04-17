package agent

import (
	"context"
	"sync"
	"time"
)

// StreamPreviewCfg controls the streaming preview behavior.
type StreamPreviewCfg struct {
	Enabled    bool
	IntervalMs int
	MaxChars   int
}

func DefaultStreamPreviewCfg() StreamPreviewCfg {
	return StreamPreviewCfg{
		Enabled:    true,
		IntervalMs: 1500,
		MaxChars:   2000,
	}
}

// StreamPreview manages a single streaming preview.
type StreamPreview struct {
	mu sync.Mutex

	cfg      StreamPreviewCfg
	dialog   Dialog
	replyCtx any
	ctx      context.Context

	fullText     string
	lastSentText string
	lastSentAt   time.Time
	previewMsgID any
	degraded     bool

	timer *time.Timer
}

func NewStreamPreview(cfg StreamPreviewCfg, p Dialog, replyCtx any, ctx context.Context) *StreamPreview {
	return &StreamPreview{
		cfg:      cfg,
		dialog:   p,
		replyCtx: replyCtx,
		ctx:      ctx,
	}
}

func (sp *StreamPreview) AppendText(text string) {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	if sp.degraded || !sp.cfg.Enabled {
		return
	}

	sp.fullText += text
	displayText := sp.fullText
	if sp.cfg.MaxChars > 0 && len([]rune(displayText)) > sp.cfg.MaxChars {
		displayText = string([]rune(displayText)[:sp.cfg.MaxChars]) + "…"
	}

	elapsed := time.Since(sp.lastSentAt)
	interval := time.Duration(sp.cfg.IntervalMs) * time.Millisecond

	if elapsed < interval && !sp.lastSentAt.IsZero() {
		if sp.timer == nil {
			sp.timer = time.AfterFunc(interval-elapsed, func() {
				sp.mu.Lock()
				defer sp.mu.Unlock()
				sp.timer = nil
				sp.flushLocked(sp.fullText) // Simplified
			})
		}
		return
	}

	sp.flushLocked(displayText)
}

func (sp *StreamPreview) flushLocked(text string) {
	if text == sp.lastSentText || text == "" {
		return
	}

	updater, ok := sp.dialog.(MessageUpdater)
	if !ok {
		sp.degraded = true
		return
	}

	if sp.previewMsgID == nil {
		if starter, ok := sp.dialog.(PreviewStarter); ok {
			handle, err := starter.SendPreviewStart(sp.ctx, sp.replyCtx, text)
			if err != nil {
				sp.degraded = true
				return
			}
			sp.previewMsgID = handle
		} else {
			if err := sp.dialog.Send(sp.ctx, sp.replyCtx, text); err != nil {
				sp.degraded = true
				return
			}
			sp.previewMsgID = sp.replyCtx
		}
	} else {
		if err := updater.UpdateMessage(sp.ctx, sp.previewMsgID, text); err != nil {
			sp.degraded = true
			return
		}
	}
	sp.lastSentText = text
	sp.lastSentAt = time.Now()
}

func (sp *StreamPreview) Finish(finalText string) bool {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	if sp.timer != nil {
		sp.timer.Stop()
		sp.timer = nil
	}

	if sp.previewMsgID == nil || sp.degraded {
		return false
	}

	if cleaner, ok := sp.dialog.(PreviewCleaner); ok {
		_ = cleaner.DeletePreviewMessage(sp.ctx, sp.previewMsgID)
		return false
	}

	updater, ok := sp.dialog.(MessageUpdater)
	if !ok {
		return false
	}

	if finalText == "" {
		return false
	}

	if finalText == sp.lastSentText {
		return true
	}

	if err := updater.UpdateMessage(sp.ctx, sp.previewMsgID, finalText); err != nil {
		return false
	}
	return true
}
