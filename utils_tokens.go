package agent

import "strings"

// EstimateTokens estimates the token count for a text string using a
// heuristic: rune count × 0.4 (±10% for Chinese/English mix).
// Zero external dependencies — no tiktoken required.
func EstimateTokens(text string) int {
	n := 0
	for range text {
		n++
	}
	return int(float64(n) * 0.4)
}

// EstimateHistoryTokens estimates the total token count for a conversation history.
// Each message carries a fixed 12-token overhead for role/separator markup.
func EstimateHistoryTokens(history []HistoryEntry) int {
	total := 0
	for _, h := range history {
		total += EstimateTokens(h.Content) + 12
	}
	return total
}

// RedactEnv masks values of environment variables whose names contain
// KEY, TOKEN, SECRET, or PASSWORD (case-insensitive). The key itself is preserved.
func RedactEnv(env []string) []string {
	result := make([]string, len(env))
	for i, e := range env {
		idx := strings.IndexByte(e, '=')
		if idx >= 0 {
			k := e[:idx]
			ku := strings.ToUpper(k)
			if strings.Contains(ku, "KEY") || strings.Contains(ku, "TOKEN") ||
				strings.Contains(ku, "SECRET") || strings.Contains(ku, "PASSWORD") {
				result[i] = k + "=[REDACTED]"
				continue
			}
		}
		result[i] = e
	}
	return result
}
