package agent

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// IsOldMessage returns true if the message is older than 5 minutes.
func IsOldMessage(createTime time.Time) bool {
	return time.Since(createTime) > 5*time.Minute
}

// AllowList checks whether a user ID is permitted.
func AllowList(allowFrom, userID string) bool {
	allowFrom = strings.TrimSpace(allowFrom)
	if allowFrom == "" || allowFrom == "*" {
		return true
	}
	for _, id := range strings.Split(allowFrom, ",") {
		if strings.EqualFold(strings.TrimSpace(id), userID) {
			return true
		}
	}
	return false
}

// CheckAllowFrom logs a security warning.
func CheckAllowFrom(access, allowFrom string) {
	if strings.TrimSpace(allowFrom) == "" {
		slog.Warn("allow_from is not set — all users are permitted.", "access", access)
	}
}

// RedactArgs returns a copy of args with values after sensitive flag names masked.
func RedactArgs(args []string) []string {
	out := make([]string, len(args))
	copy(out, args)

	sensitiveFlags := []string{
		"--api-key", "--api_key", "--apikey",
		"--token", "--secret", "--password",
		"-k",
	}

	for i := 0; i < len(out); i++ {
		arg := strings.ToLower(out[i])

		// --flag=value format
		for _, f := range sensitiveFlags {
			if strings.HasPrefix(arg, f+"=") {
				out[i] = out[i][:strings.Index(out[i], "=")+1] + "***"
				break
			}
		}

		// --flag value format
		for _, f := range sensitiveFlags {
			if arg == f && i+1 < len(out) {
				out[i+1] = "***"
				i++
				break
			}
		}
	}
	return out
}

// MergeEnv merges two environment variable slices.
func MergeEnv(base, extra []string) []string {
	m := make(map[string]string)
	for _, e := range base {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			m[parts[0]] = parts[1]
		}
	}
	for _, e := range extra {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			m[parts[0]] = parts[1]
		}
	}
	res := make([]string, 0, len(m))
	for k, v := range m {
		res = append(res, k+"="+v)
	}
	return res
}

// SaveFilesToDisk saves a list of file attachments to a temporary directory.
func SaveFilesToDisk(workDir string, files []FileAttachment) []string {
	if len(files) == 0 {
		return nil
	}
	attachDir := filepath.Join(workDir, ".cc-connect", "attachments")
	_ = os.MkdirAll(attachDir, 0o755)

	var paths []string
	for _, f := range files {
		fname := f.FileName
		if fname == "" {
			fname = fmt.Sprintf("file_%d", time.Now().UnixNano())
		}
		path := filepath.Join(attachDir, fname)
		if err := os.WriteFile(path, f.Data, 0o644); err == nil {
			paths = append(paths, path)
		}
	}
	return paths
}

// AppendFileRefs appends file paths to the prompt.
func AppendFileRefs(prompt string, paths []string) string {
	if len(paths) == 0 {
		return prompt
	}
	var sb strings.Builder
	sb.WriteString(prompt)
	if !strings.HasSuffix(prompt, "\n") {
		sb.WriteString("\n")
	}
	sb.WriteString("\nAttached files:\n")
	for _, p := range paths {
		sb.WriteString(fmt.Sprintf("- %s\n", p))
	}
	return sb.String()
}

// RedactToken replaces a secret token in text with [REDACTED].
func RedactToken(text, token string) string {
	if token == "" || text == "" {
		return text
	}
	return strings.ReplaceAll(text, token, "[REDACTED]")
}

var (
	reCodeBlock      = regexp.MustCompile("(?s)```[a-zA-Z]*\n?(.*?)```")
	reInlineCode     = regexp.MustCompile("`([^`]+)`")
	reBoldAst        = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reBoldUnd        = regexp.MustCompile(`__(.+?)__`)
	reItalicAst      = regexp.MustCompile(`\*(.+?)\*`)
	reItalicUnd      = regexp.MustCompile(`_(.+?)_`)
	reStrike         = regexp.MustCompile(`~~(.+?)~~`)
	reLink           = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	rootReHeading    = regexp.MustCompile(`(?m)^#{1,6}\s+`)
	rootReHorizontal = regexp.MustCompile(`(?m)^---+\s*$`)
	reBlockquote     = regexp.MustCompile(`(?m)^>\s?`)
)

// StripMarkdown converts Markdown-formatted text to clean plain text.
func StripMarkdown(s string) string {
	s = reCodeBlock.ReplaceAllString(s, "$1")
	s = reInlineCode.ReplaceAllString(s, "$1")
	s = reBoldAst.ReplaceAllString(s, "$1")
	s = reBoldUnd.ReplaceAllString(s, "$1")
	s = reItalicAst.ReplaceAllString(s, "$1")
	s = reItalicUnd.ReplaceAllString(s, "$1")
	s = reStrike.ReplaceAllString(s, "$1")
	s = reLink.ReplaceAllString(s, "$1 ($2)")
	s = rootReHeading.ReplaceAllString(s, "")
	s = rootReHorizontal.ReplaceAllString(s, "")
	s = reBlockquote.ReplaceAllString(s, "")
	s = regexp.MustCompile(`\n{3,}`).ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

// SplitMessageCodeFenceAware splits text into chunks respecting code fence boundaries.
func SplitMessageCodeFenceAware(text string, maxLen int) []string {
	if len(text) <= maxLen {
		return []string{text}
	}

	lines := strings.Split(text, "\n")
	var chunks []string
	current := make([]string, 0, len(lines))
	currentLen := 0
	openFence := ""

	for _, line := range lines {
		lineLen := len(line) + 1

		if currentLen+lineLen > maxLen && len(current) > 0 {
			chunk := strings.Join(current, "\n")
			if openFence != "" {
				chunk += "\n```"
			}
			chunks = append(chunks, chunk)

			current = nil
			currentLen = 0
			if openFence != "" {
				current = append(current, openFence)
				currentLen = len(openFence) + 1
			}
		}

		current = append(current, line)
		currentLen += lineLen

		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if openFence != "" {
				openFence = ""
			} else {
				openFence = trimmed
			}
		}
	}

	if len(current) > 0 {
		chunk := strings.Join(current, "\n")
		if openFence != "" {
			chunk += "\n```"
		}
		chunks = append(chunks, chunk)
	}

	return chunks
}

// AtomicWriteFile writes data to a file atomically by first writing to a
// temporary file in the same directory, syncing, then renaming over the target.
// This prevents data loss / corruption on crash.
func AtomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Chmod(tmpPath, perm); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}
