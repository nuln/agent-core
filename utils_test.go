package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestIsOldMessage(t *testing.T) {
	if IsOldMessage(time.Now()) {
		t.Error("current time should not be old")
	}
	if !IsOldMessage(time.Now().Add(-10 * time.Minute)) {
		t.Error("10 minutes ago should be old")
	}
}

func TestAllowList(t *testing.T) {
	if !AllowList("", "user1") {
		t.Error("empty allow list should allow everyone")
	}
	if !AllowList("*", "user1") {
		t.Error("* should allow everyone")
	}
	if !AllowList("user1,user2", "user1") {
		t.Error("user1 should be allowed")
	}
	if AllowList("user1,user2", "user3") {
		t.Error("user3 should not be allowed")
	}
}

func TestSplitMessageCodeFenceAware_Short(t *testing.T) {
	chunks := SplitMessageCodeFenceAware("hello", 100)
	if len(chunks) != 1 || chunks[0] != "hello" {
		t.Errorf("unexpected: %v", chunks)
	}
}

func TestSplitMessageCodeFenceAware_PreservesCodeBlock(t *testing.T) {
	lines := []string{
		"before",
		"```python",
		"print('hello')",
		"print('world')",
		"```",
		"after",
	}
	text := strings.Join(lines, "\n")

	chunks := SplitMessageCodeFenceAware(text, 30)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}

	full := strings.Join(chunks, "\n")
	if !strings.Contains(full, "print('hello')") {
		t.Error("content should be preserved")
	}
}

func TestRedactArgs(t *testing.T) {
	args := []string{"--api-key", "secret", "--other", "val"}
	redacted := RedactArgs(args)
	if redacted[1] != "***" {
		t.Errorf("expected redacted token, got %v", redacted)
	}
}

func TestRedactToken(t *testing.T) {
	assert.Equal(t, "hello [REDACTED]", RedactToken("hello secret", "secret"))
	assert.Equal(t, "safe", RedactToken("safe", ""))
}

func TestStripMarkdown(t *testing.T) {
	assert.Equal(t, "Header\n\nContent", StripMarkdown("# Header\n\nContent"))
	assert.Equal(t, "Bold Italic", StripMarkdown("**Bold** _Italic_"))
}

func TestMergeEnv(t *testing.T) {
	base := []string{"A=1", "B=2"}
	extra := []string{"B=3", "C=4"}
	merged := MergeEnv(base, extra)
	assert.Contains(t, merged, "A=1")
	assert.Contains(t, merged, "B=3")
	assert.Contains(t, merged, "C=4")
}

func TestAtomicWriteFile(t *testing.T) {
	tmpDir := t.TempDir()
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()
	path := filepath.Join(tmpDir, "test.txt")

	err := AtomicWriteFile(path, []byte("hello"), 0o644)
	assert.NoError(t, err)

	data, _ := os.ReadFile(path)
	assert.Equal(t, "hello", string(data))
}
