package agent

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type CheckResult struct {
	Name    string
	Passed  bool
	Detail  string
	Latency time.Duration
}

func RunDoctor(ctx context.Context, _ *Engine) []CheckResult {
	var results []CheckResult

	// 1. Check System
	results = append(results, CheckResult{
		Name:   "OS",
		Passed: true,
		Detail: fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	})

	// 2. Check Dependencies
	ffmpegPath, err := exec.LookPath("ffmpeg")
	results = append(results, CheckResult{
		Name:   "FFmpeg",
		Passed: err == nil,
		Detail: ffmpegPath,
	})

	// 3. Check Network
	start := time.Now()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", "api.openai.com:443")
	if err == nil {
		_ = conn.Close()
	}
	results = append(results, CheckResult{
		Name:    "Network (OpenAI)",
		Passed:  err == nil,
		Detail:  fmt.Sprintf("%v", err),
		Latency: time.Since(start),
	})

	return results
}

func FormatDoctor(results []CheckResult) string {
	var sb strings.Builder
	for _, r := range results {
		icon := "✅"
		if !r.Passed {
			icon = "❌"
		}
		sb.WriteString(fmt.Sprintf("%s %s: %s (%v)\n", icon, r.Name, r.Detail, r.Latency))
	}
	return sb.String()
}
