#!/usr/bin/env python3
"""Generate toolparse.go - state machine XML parser for tool calls.
Writes Go source with proper escaping.
"""
import os

# Go source code as raw string, using \x22 for double-quote inside Go strings
GO = r'''package toolcall

import (
	"encoding/json"
	"html"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/wtz44/mimo-gateway/internal/adapter"
)

type ParsedToolCall struct {
	Name  string
	Input map[string]any
}

// ===== Fullwidth ASCII normalization =====

func normalizeFullwidth(r rune) rune {
	switch r {
	case 0x3008:
		return '<'
	case 0x3009:
		return '>'
	}
	if r >= 0xFF01 && r <= 0xFF5E {
		return r - 0xFEE0
	}
	return r
}

func normalizedByte(text string, idx int) (byte, int) {
	if idx < 0 || idx >= len(text) {
		return 0, 0
	}
	r, size := utf8.DecodeRuneInString(text[idx:])
	if r == utf8.RuneError && size == 0 {
		return 0, 0
	}
	n := normalizeFullwidth(r)
	if n > 0x7f {
		return 0, 0
	}
	return byte(n), size
}

func asciiLower(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + 32
	}
	return b
}

func hasPrefixFold(text string, start int, prefix string) bool {
	idx := start
	for j := 0; j < len(prefix); j++ {
		if idx >= len(text) {
			return false
		}
		ch, size := normalizedByte(text, idx)
		if size <= 0 || asciiLower(ch) != asciiLower(prefix[j]) {
			return false
		}
		idx += size
	}
	return true
}

func findTagEnd(text string, from int) int {
	quote := rune(0)
	for i := from; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		if r == utf8.RuneError && size == 0 {
			break
		}
		ch := normalizeFullwidth(r)
		if quote != 0 {
			if ch == quote {
				quote = 0
			}
			i += size
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			i += size
			continue
		}
		if ch == '>' {
			return i
		}
		i += size
	}
	return -1
}

func isTagBoundary(text string, idx int) bool {
	if idx >= len(text) {
		return true
	}
	switch text[idx] {
	case ' ', '\t', '\n', '\r', '>', '/', '=':
		return true
	}
	r, _ := utf8.DecodeRuneInString(text[idx:])
	return normalizeFullwidth(r) == '>'
}

// ===== XML block finder (state machine) =====

type xmlBlock struct {
	Attrs string
	Body  string
}

func findBlocks(text, tag string) []xmlBlock {
	var out []xmlBlock
	pos := 0
	target := "<" + tag
	closeTarget := "</" + tag
	for pos < len(text) {
		startIdx := -1
		for i := pos; i < len(text); i++ {
			if hasPrefixFold(text, i, target) && isTagBoundary(text, i+len(target)) {
				startIdx = i
				break
			}
		}
		if startIdx < 0 {
			break
		}
		tagEnd := findTagEnd(text, startIdx+len(target))
		if tagEnd < 0 {
			pos = startIdx + 1
			continue
		}
		attrs := text[startIdx+len(target) : tagEnd]
		bodyStart := tagEnd + 1
		trimmed := strings.TrimSpace(text[startIdx : tagEnd+1])
		if strings.HasSuffix(trimmed, "/") {
			out = append(out, xmlBlock{Attrs: attrs, Body: ""})
			pos = tagEnd + 1
			continue
		}
		depth := 1
		closeStart, closeEnd := -1, -1
		for i := bodyStart; i < len(text); {
			if hasPrefixFold(text, i, closeTarget) && isTagBoundary(text, i+len(closeTarget)) {
				end := findTagEnd(text, i+len(closeTarget))
				if end >= 0 {
					depth--
					if depth == 0 {
						closeStart = i
						closeEnd = end + 1
						break
					}
					i = end + 1
					continue
				}
			}
			if hasPrefixFold(text, i, target) && isTagBoundary(text, i+len(target)) {
				end := findTagEnd(text, i+len(target))
				if end >= 0 {
					t := strings.TrimSpace(text[i : end+1])
					if !strings.HasSuffix(t, "/") {
						depth++
					}
					i = end + 1
					continue
				}
			}
			i++
		}
		if closeStart < 0 {
			pos = bodyStart
			continue
		}
		out = append(out, xmlBlock{Attrs: attrs, Body: text[bodyStart:closeStart]})
		pos = closeEnd
	}
	return out
}

var attrRe = regexp.MustCompile(`(?is)\b([a-z0-9_:-]+)\s*=\s*(\"([^\"]*)\"|'([^']*)')`)

func parseAttrs(raw string) map[string]string {
	out := map[string]string{}
	for _, m := range attrRe.FindAllStringSubmatch(raw, -1) {
		if len(m) < 5 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(m[1]))
		if key == "" {
			continue
		}
		val := m[3]
		if val == "" {
			val = m[4]
		}
		out[key] = val
	}
	return out
}

func parseParamValue(raw string) any {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "<![CDATA[") {
		endIdx := strings.Index(trimmed, "]]>")
		if endIdx >= 0 {
			return trimmed[9:endIdx]
		}
		return trimmed[9:]
	}
	var jsonVal any
	if err := json.Unmarshal([]byte(trimmed), &jsonVal); err == nil {
		return jsonVal
	}
	return html.UnescapeString(trimmed)
}

// ===== DSML format parser =====

var dsmlWrapperRe = regexp.MustCompile(`(?is)<(?:｜|)DSML(?:｜|)tool_calls(?:｜|)>([\s\S]*?)</(?:｜|)DSML(?:｜|)tool_calls(?:｜|)>`)
var dsmlInvokeRe = regexp.MustCompile(`(?is)<(?:｜|)DSML(?:｜|)invoke\s+name\s*=\s*\"([^\"]*)\"(?:｜|)?\s*>([\s\S]*?)</(?:｜|)DSML(?:｜|)invoke(?:｜|)?>`)
var dsmlParamRe = regexp.MustCompile(`(?is)<(?:｜|)DSML(?:｜|)parameter\s+name\s*=\s*\"([^\"]*)\"(?:｜|)?\s*>([\s\S]*?)</(?:｜|)DSML(?:｜|)parameter(?:｜|)?>`)

func parseDSML(text string) []ParsedToolCall {
	wm := dsmlWrapperRe.FindAllStringSubmatch(text, -1)
	if len(wm) == 0 {
		return nil
	}
	var calls []ParsedToolCall
	for _, w := range wm {
		if len(w) < 2 {
			continue
		}
		for _, im := range dsmlInvokeRe.FindAllStringSubmatch(w[1], -1) {
			if len(im) < 3 {
				continue
			}
			name := strings.TrimSpace(im[1])
			if name == "" {
				continue
			}
			input := map[string]any{}
			for _, pm := range dsmlParamRe.FindAllStringSubmatch(im[2], -1) {
				if len(pm) < 3 {
					continue
				}
				pname := strings.TrimSpace(pm[1])
				if pname == "" {
					continue
				}
				val := parseParamValue(pm[2])
				appendVal(input, pname, val)
			}
			calls = append(calls, ParsedToolCall{Name: name, Input: input})
		}
	}
	return calls
}

// ===== tool_calls XML parser (state machine) =====

func parseToolCallsXML(text string) []ParsedToolCall {
	wrappers := findBlocks(text, "tool_calls")
	if len(wrappers) == 0 {
		return nil
	}
	var calls []ParsedToolCall
	for _, w := range wrappers {
		for _, inv := range findBlocks(w.Body, "invoke") {
			attrs := parseAttrs(inv.Attrs)
			name := strings.TrimSpace(html.UnescapeString(attrs["name"]))
			if name == "" {
				continue
			}
			input := map[string]any{}
			for _, pm := range findBlocks(inv.Body, "parameter") {
				pAttrs := parseAttrs(pm.Attrs)
				pName := strings.TrimSpace(html.UnescapeString(pAttrs["name"]))
				if pName == "" {
					continue
				}
				val := parseParamValue(pm.Body)
				appendVal(input, pName, val)
			}
			calls = append(calls, ParsedToolCall{Name: name, Input: input})
		}
	}
	return calls
}

// ===== function=X parser (state machine) =====

func parseFuncXML(text string) []ParsedToolCall {
	blocks := findBlocks(text, "function")
	if len(blocks) == 0 {
		return nil
	}
	var calls []ParsedToolCall
	for _, b := range blocks {
		name := extractFuncName(b.Attrs)
		if name == "" {
			continue
		}
		input := map[string]any{}
		for _, pm := range findBlocks(b.Body, "parameter") {
			pName := extractParamName(pm.Attrs)
			if pName == "" {
				continue
			}
			val := parseParamValue(pm.Body)
			appendVal(input, pName, val)
		}
		calls = append(calls, ParsedToolCall{Name: name, Input: input})
	}
	return calls
}

// extractFuncName handles both name="X" and =X attribute formats.
func extractFuncName(attrs string) string {
	// Try standard name="X" first
	parsed := parseAttrs(attrs)
	if n, ok := parsed["name"]; ok && n != "" {
		return strings.TrimSpace(html.UnescapeString(n))
	}
	// Try =Value format (e.g. =WebFetch)
	trimmed := strings.TrimSpace(attrs)
	if strings.HasPrefix(trimmed, "=") {
		val := strings.TrimRight(trimmed[1:], " \t\n\r/")
		if val != "" {
			return strings.TrimSpace(html.UnescapeString(val))
		}
	}
	return ""
}

// extractParamName handles both name="X" and =X attribute formats.
func extractParamName(attrs string) string {
	parsed := parseAttrs(attrs)
	if n, ok := parsed["name"]; ok && n != "" {
		return strings.TrimSpace(html.UnescapeString(n))
	}
	trimmed := strings.TrimSpace(attrs)
	if strings.HasPrefix(trimmed, "=") {
		val := strings.TrimRight(trimmed[1:], " \t\n\r/")
		if val != "" {
			return strings.TrimSpace(html.UnescapeString(val))
		}
	}
	return ""
}

func appendVal(input map[string]any, key string, val any) {
	if existing, ok := input[key]; ok {
		switch v := existing.(type) {
		case []any:
			input[key] = append(v, val)
		default:
			input[key] = []any{v, val}
		}
	} else {
		input[key] = val
	}
}

// ===== Public API =====

func ParseToolCallsFromText(text string) []ParsedToolCall {
	if text == "" {
		return nil
	}
	calls := parseDSML(text)
	if len(calls) > 0 {
		return calls
	}
	calls = parseToolCallsXML(text)
	if len(calls) > 0 {
		return calls
	}
	return parseFuncXML(text)
}

func ConvertToolCallsToOpenAI(calls []ParsedToolCall) []adapter.OpenAIToolCall {
	if len(calls) == 0 {
		return nil
	}
	result := make([]adapter.OpenAIToolCall, 0, len(calls))
	for _, call := range calls {
		args, _ := json.Marshal(call.Input)
		result = append(result, adapter.OpenAIToolCall{
			ID:   "call_" + strings.ReplaceAll(uuid.New().String(), "-", ""),
			Type: "function",
			Function: adapter.OpenAIToolCallFunc{Name: call.Name, Arguments: string(args)},
		})
	}
	return result
}

func HasToolCallSyntax(text string) bool {
	if text == "" {
		return false
	}
	lower := strings.ToLower(text)
	return strings.Contains(lower, "tool_calls") ||
		strings.Contains(lower, "dsml") ||
		strings.Contains(lower, "function=")
}

func StripToolCallSyntax(text string) string {
	if text == "" {
		return ""
	}
	result := dsmlWrapperRe.ReplaceAllString(text, "")
	result = regexp.MustCompile(`(?is)<tool_calls>[\s\S]*?</tool_calls>`).ReplaceAllString(result, "")
	result = regexp.MustCompile(`(?is)<function=[^>]*>[\s\S]*?</function>`).ReplaceAllString(result, "")
	return strings.TrimSpace(result)
}
'''

out_path = os.path.join("internal", "toolcall", "toolparse.go")
with open(out_path, "w", encoding="utf-8") as f:
    f.write(GO)
print(f"Written {out_path} ({len(GO)} bytes)")
