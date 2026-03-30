package agent

import (
	"regexp"
	"strings"
)

// ──────────────────────────────────────────────────────────────
// StripMarkdown — Markdown → plain text
// ──────────────────────────────────────────────────────────────

var (
	mdReCodeBlock  = regexp.MustCompile("(?s)```[a-zA-Z]*\n?(.*?)```")
	mdReInlineCode = regexp.MustCompile("`([^`]+)`")
	mdReBoldAst    = regexp.MustCompile(`\*\*(.+?)\*\*`)
	mdReBoldUnd    = regexp.MustCompile(`__(.+?)__`)
	mdReItalicAst  = regexp.MustCompile(`\*(.+?)\*`)
	mdReItalicUnd  = regexp.MustCompile(`_(.+?)_`)
	mdReStrike     = regexp.MustCompile(`~~(.+?)~~`)
	mdReLink       = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	mdReHeading    = regexp.MustCompile(`(?m)^#{1,6}\s+`)
	mdReHorizontal = regexp.MustCompile(`(?m)^---+\s*$`)
	mdReBlockquote = regexp.MustCompile(`(?m)^>\s?`)
)

// StripMarkdown converts Markdown-formatted text to clean plain text.
// Useful for platforms that don't support Markdown rendering (WeChat, LINE, etc.).
func StripMarkdown(s string) string {
	// Preserve code block content but remove fences
	s = mdReCodeBlock.ReplaceAllString(s, "$1")

	// Inline code — remove backticks
	s = mdReInlineCode.ReplaceAllString(s, "$1")

	// Bold / italic / strikethrough — keep text
	s = mdReBoldAst.ReplaceAllString(s, "$1")
	s = mdReBoldUnd.ReplaceAllString(s, "$1")
	s = mdReItalicAst.ReplaceAllString(s, "$1")
	s = mdReItalicUnd.ReplaceAllString(s, "$1")
	s = mdReStrike.ReplaceAllString(s, "$1")

	// Links [text](url) → text (url)
	s = mdReLink.ReplaceAllString(s, "$1 ($2)")

	// Headings — remove # prefix
	s = mdReHeading.ReplaceAllString(s, "")

	// Horizontal rules
	s = mdReHorizontal.ReplaceAllString(s, "")

	// Blockquotes
	s = mdReBlockquote.ReplaceAllString(s, "")

	// Collapse 3+ consecutive blank lines into 2
	s = regexp.MustCompile(`\n{3,}`).ReplaceAllString(s, "\n\n")

	return strings.TrimSpace(s)
}

// ──────────────────────────────────────────────────────────────
// MarkdownToSimpleHTML — Markdown → limited HTML subset
// ──────────────────────────────────────────────────────────────

var (
	htmlReInlineCode = regexp.MustCompile("`([^`]+)`")
	htmlReBoldItalic = regexp.MustCompile(`\*\*\*(.+?)\*\*\*`)
	htmlReBoldAst    = regexp.MustCompile(`\*\*(.+?)\*\*`)
	htmlReBoldUnd    = regexp.MustCompile(`__(.+?)__`)
	htmlReItalicAst  = regexp.MustCompile(`(?:^|[^*])\*([^*]+?)\*(?:[^*]|$)`)
	htmlReStrike     = regexp.MustCompile(`~~(.+?)~~`)
	htmlReLink       = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	htmlReWikilink   = regexp.MustCompile(`\[\[([^\]|]+)\|([^\]]+)\]\]|\[\[([^\]]+)\]\]`)
	htmlReUnordered  = regexp.MustCompile(`^(\s*)[-*]\s+(.*)$`)
	htmlReOrdered    = regexp.MustCompile(`^(\s*)\d+\.\s+(.*)$`)
	htmlReTableSep   = regexp.MustCompile(`^\|[\s:|-]+\|$`)
	htmlReCallout    = regexp.MustCompile(`^\[!(\w+)\]\s*(.*)$`)
	htmlReHeading    = regexp.MustCompile(`(?m)^#{1,6}\s+`)
	htmlReHorizontal = regexp.MustCompile(`(?m)^---+\s*$`)
)

// MarkdownToSimpleHTML converts common Markdown to a simplified HTML subset.
// Supported tags: <b>, <i>, <s>, <code>, <pre>, <a href="">, <blockquote>.
// Useful for platforms that accept a limited set of HTML (e.g. Telegram).
func MarkdownToSimpleHTML(md string) string {
	var b strings.Builder
	b.Grow(len(md) + len(md)/4)

	lines := strings.Split(md, "\n")
	inCodeBlock := false
	codeLang := ""
	var codeLines []string
	inBlockquote := false
	var bqLines []string
	inTable := false
	var tblLines []string

	flushBlockquote := func() {
		if len(bqLines) == 0 {
			return
		}
		b.WriteString("<blockquote>")
		startIdx := 0
		if len(bqLines) > 0 {
			if m := htmlReCallout.FindStringSubmatch(bqLines[0]); m != nil {
				calloutType := m[1]
				calloutTitle := m[2]
				if calloutTitle != "" {
					b.WriteString("<b>" + htmlEscape(calloutType) + ": " + htmlEscape(calloutTitle) + "</b>")
				} else {
					b.WriteString("<b>" + htmlEscape(calloutType) + "</b>")
				}
				startIdx = 1
				if startIdx < len(bqLines) {
					b.WriteByte('\n')
				}
			}
		}
		for j := startIdx; j < len(bqLines); j++ {
			if j > startIdx {
				b.WriteByte('\n')
			}
			b.WriteString(htmlConvertInline(bqLines[j]))
		}
		b.WriteString("</blockquote>")
		bqLines = bqLines[:0]
		inBlockquote = false
	}

	flushTable := func() {
		if len(tblLines) == 0 {
			return
		}
		for j, tl := range tblLines {
			if j > 0 {
				b.WriteByte('\n')
			}
			tl = strings.TrimSpace(tl)
			if htmlReTableSep.MatchString(tl) {
				b.WriteString("——————————")
			} else {
				inner := tl[1 : len(tl)-1]
				cells := strings.Split(inner, "|")
				for k := range cells {
					cells[k] = strings.TrimSpace(cells[k])
				}
				row := strings.Join(cells, " | ")
				b.WriteString(htmlConvertInline(row))
			}
		}
		tblLines = tblLines[:0]
		inTable = false
	}

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "```") {
			if !inCodeBlock {
				if inBlockquote {
					flushBlockquote()
					b.WriteByte('\n')
				}
				if inTable {
					flushTable()
					b.WriteByte('\n')
				}
				inCodeBlock = true
				codeLang = strings.TrimPrefix(trimmed, "```")
				codeLines = nil
			} else {
				inCodeBlock = false
				if codeLang != "" {
					b.WriteString("<pre><code class=\"language-" + htmlEscape(codeLang) + "\">")
				} else {
					b.WriteString("<pre><code>")
				}
				b.WriteString(htmlEscape(strings.Join(codeLines, "\n")))
				b.WriteString("</code></pre>")
				if i < len(lines)-1 {
					b.WriteByte('\n')
				}
			}
			continue
		}

		if inCodeBlock {
			codeLines = append(codeLines, line)
			continue
		}

		isQuote := strings.HasPrefix(trimmed, "> ") || trimmed == ">"
		isTable := len(trimmed) > 2 && trimmed[0] == '|' && trimmed[len(trimmed)-1] == '|'

		if !isQuote && inBlockquote {
			flushBlockquote()
			b.WriteByte('\n')
		}
		if !isTable && inTable {
			flushTable()
			b.WriteByte('\n')
		}

		if isQuote {
			quoteContent := strings.TrimPrefix(trimmed, "> ")
			if trimmed == ">" {
				quoteContent = ""
			}
			bqLines = append(bqLines, quoteContent)
			inBlockquote = true
			continue
		}

		if isTable {
			tblLines = append(tblLines, trimmed)
			inTable = true
			continue
		}

		if heading := htmlReHeading.FindString(line); heading != "" {
			rest := strings.TrimPrefix(line, heading)
			b.WriteString("<b>")
			b.WriteString(htmlConvertInline(rest))
			b.WriteString("</b>")
		} else if htmlReHorizontal.MatchString(trimmed) {
			b.WriteString("——————————")
		} else if m := htmlReUnordered.FindStringSubmatch(line); m != nil {
			indent := strings.Repeat("  ", len(m[1])/2)
			b.WriteString(indent + "• " + htmlConvertInline(m[2]))
		} else if m := htmlReOrdered.FindStringSubmatch(line); m != nil {
			indent := strings.Repeat("  ", len(m[1])/2)
			numDot := strings.TrimSpace(line[:len(line)-len(m[2])])
			b.WriteString(indent + htmlEscape(numDot) + " " + htmlConvertInline(m[2]))
		} else {
			b.WriteString(htmlConvertInline(line))
		}

		if i < len(lines)-1 {
			b.WriteByte('\n')
		}
	}

	if inBlockquote {
		flushBlockquote()
	}
	if inTable {
		flushTable()
	}
	if inCodeBlock && len(codeLines) > 0 {
		b.WriteString("<pre><code>")
		b.WriteString(htmlEscape(strings.Join(codeLines, "\n")))
		b.WriteString("</code></pre>")
	}

	return b.String()
}

// htmlConvertInline converts inline Markdown formatting to Telegram-safe HTML.
func htmlConvertInline(s string) string {
	type placeholder struct {
		key  string
		html string
	}
	var phs []placeholder
	phIdx := 0

	nextPH := func(html string) string {
		key := "\x00PH" + string(rune('0'+phIdx)) + "\x00"
		phs = append(phs, placeholder{key: key, html: html})
		phIdx++
		return key
	}

	// 1. Inline code → placeholder
	s = htmlReInlineCode.ReplaceAllStringFunc(s, func(m string) string {
		inner := m[1 : len(m)-1]
		return nextPH("<code>" + htmlEscape(inner) + "</code>")
	})

	// 2. Links → placeholder
	s = htmlReLink.ReplaceAllStringFunc(s, func(m string) string {
		sm := htmlReLink.FindStringSubmatch(m)
		if len(sm) < 3 {
			return m
		}
		return nextPH(`<a href="` + htmlEscape(sm[2]) + `">` + htmlEscape(sm[1]) + `</a>`)
	})

	// 2b. Wikilinks
	s = htmlReWikilink.ReplaceAllStringFunc(s, func(m string) string {
		sm := htmlReWikilink.FindStringSubmatch(m)
		if sm[1] != "" && sm[2] != "" {
			return sm[2]
		}
		if sm[3] != "" {
			return sm[3]
		}
		return m
	})

	// 3. HTML-escape remaining text
	s = htmlEscape(s)

	// 4. Bold-italic
	s = htmlReBoldItalic.ReplaceAllStringFunc(s, func(m string) string {
		inner := m[3 : len(m)-3]
		return nextPH("<b><i>" + inner + "</i></b>")
	})

	// 5. Bold
	s = htmlReBoldAst.ReplaceAllStringFunc(s, func(m string) string {
		inner := m[2 : len(m)-2]
		return nextPH("<b>" + inner + "</b>")
	})
	s = htmlReBoldUnd.ReplaceAllStringFunc(s, func(m string) string {
		inner := m[2 : len(m)-2]
		return nextPH("<b>" + inner + "</b>")
	})

	// 6. Strikethrough
	s = htmlReStrike.ReplaceAllStringFunc(s, func(m string) string {
		inner := m[2 : len(m)-2]
		return nextPH("<s>" + inner + "</s>")
	})

	// 7. Italic (last, on text with bold/strike already protected)
	s = htmlReItalicAst.ReplaceAllStringFunc(s, func(m string) string {
		idx := strings.Index(m, "*")
		if idx < 0 {
			return m
		}
		lastIdx := strings.LastIndex(m, "*")
		if lastIdx <= idx {
			return m
		}
		return m[:idx] + "<i>" + m[idx+1:lastIdx] + "</i>" + m[lastIdx+1:]
	})

	// 8. Restore placeholders
	for range len(phs) + 1 {
		changed := false
		for _, ph := range phs {
			if strings.Contains(s, ph.key) {
				s = strings.Replace(s, ph.key, ph.html, 1)
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	return s
}

func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

// SplitMessageCodeFenceAware splits text into chunks respecting code fence boundaries.
// When a chunk boundary falls inside a code block, the fence is closed at the end of
// the chunk and re-opened at the start of the next chunk.
func SplitMessageCodeFenceAware(text string, maxLen int) []string {
	if len(text) <= maxLen {
		return []string{text}
	}

	const closingFence = "\n```"

	lines := strings.Split(text, "\n")
	var chunks []string
	var current []string
	currentLen := 0
	openFence := ""

	for _, line := range lines {
		lineLen := len(line) + 1

		reservedLen := 0
		if openFence != "" {
			reservedLen = len(closingFence)
		}

		if currentLen+lineLen+reservedLen > maxLen && len(current) > 0 {
			chunk := strings.Join(current, "\n")
			if openFence != "" {
				chunk += closingFence
			}
			chunks = append(chunks, chunk)

			if openFence != "" {
				current = []string{openFence}
				currentLen = len(openFence) + 1
			} else {
				current = nil
				currentLen = 0
			}
		}

		current = append(current, line)
		currentLen += lineLen

		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if openFence == "" {
				openFence = trimmed
			} else {
				openFence = ""
			}
		}
	}

	if len(current) > 0 {
		chunks = append(chunks, strings.Join(current, "\n"))
	}

	return chunks
}
