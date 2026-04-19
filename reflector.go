package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
)

// SessionReflector implements post-session evaluation using an LLM.
type SessionReflector struct {
	sessions      SessionProvider
	evalLLM       LLM
	skillManagers []SkillManager
}

// NewSessionReflector creates a new reflector.
func NewSessionReflector(sessions SessionProvider, evalLLM LLM, managers []SkillManager) *SessionReflector {
	return &SessionReflector{
		sessions:      sessions,
		evalLLM:       evalLLM,
		skillManagers: managers,
	}
}

// SetEvalLLM sets the LLM used for evaluating skills.
func (r *SessionReflector) SetEvalLLM(llm LLM) {
	r.evalLLM = llm
}

const reflectionPromptTemplate = `You are a critical peer reviewer for AI assistants.
Below is a conversation fragment between a User and an AI Assistant.
The Assistant was assigned to use the skill: "%s".

### Conversation History:
%s

### Evaluation Task:
Evaluate the Assistant's performance in the last turn.
1. Did the assistant effectively apply the skill instructions?
2. Was the user's request satisfied?
3. Were there any errors, hallucinations, or instruction violations?

Provide a score from 1 to 10 (1=terrible, 10=perfect) and a short reason for your score.
Return ONLY a JSON block like: {"score": 8, "reason": "..."}`

// Reflect evaluates the session and updates skill metrics.
func (r *SessionReflector) Reflect(ctx context.Context, sessionKey, traceID string, usedSkills []string) error {
	if len(usedSkills) == 0 {
		return nil
	}

	session := r.sessions.GetOrCreateActive(sessionKey)
	history := session.GetHistory()
	if len(history) == 0 {
		return nil
	}

	// 1. Prepare history text (last few turns)
	var historyText strings.Builder
	start := len(history) - 4
	if start < 0 {
		start = 0
	}
	for i := start; i < len(history); i++ {
		fmt.Fprintf(&historyText, "[%s]: %s\n", history[i].Role, history[i].Content)
	}

	// 2. Reflect on each used skill
	for _, skillName := range usedSkills {
		go func(name string) {
			score, reason, err := r.evaluate(ctx, name, historyText.String())
			if err != nil {
				slog.Error("reflector: evaluation failed", "skill", name, "error", err)
				return
			}
			slog.Info("reflector: skill evaluated", "skill", name, "score", score, "reason", reason)

			// 3. Record result in all managers (only managers owning this skill will persist it)
			for _, m := range r.skillManagers {
				_ = m.RecordExecution(name, score)
			}
		}(skillName)
	}

	return nil
}

func (r *SessionReflector) evaluate(ctx context.Context, skillName, history string) (int, string, error) {
	if r.evalLLM == nil {
		return 0, "", fmt.Errorf("no evaluation LLM")
	}

	sess, err := r.evalLLM.StartSession(ctx, "reflector-"+skillName)
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = sess.Close() }()

	prompt := fmt.Sprintf(reflectionPromptTemplate, skillName, history)
	if err := sess.Send(prompt, nil, nil); err != nil {
		return 0, "", err
	}

	var response strings.Builder
	for ev := range sess.Events() {
		if ev.Type == EventText {
			response.WriteString(ev.Content)
		}
	}

	// Simple JSON parsing (look for { ... })
	data := response.String()
	if start := strings.Index(data, "{"); start >= 0 {
		if end := strings.LastIndex(data, "}"); end >= 0 && end > start {
			data = data[start : end+1]
		}
	}

	var result struct {
		Score  int    `json:"score"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(data), &result); err != nil {
		return 0, "", err
	}

	return result.Score, result.Reason, nil
}
