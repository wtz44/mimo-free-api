package toolcall

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
	norm := normalizeFullwidth(r)
	// Also treat fullwidth pipe ｜ (U+FF5C) as tag boundary for DSML tags like ｜DSML｜invoke
	if norm == '|' || r == '|' {
		return true
	}
	return norm == '>'
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

// ===== DSML format parser (state-machine based) =====

var dsmlWrapperRe = regexp.MustCompile(`(?is)<(?:｜|)DSML(?:｜|)tool_calls(?:｜|)>([\s\S]*?)</(?:｜|)DSML(?:｜|)tool_calls(?:｜|)>`)
var dsmlInvokeRe = regexp.MustCompile(`(?is)<(?:｜|)DSML(?:｜|)invoke\s+name\s*=\s*\"([^\"]*)\"(?:｜|)?\s*>([\s\S]*?)</(?:｜|)DSML(?:｜|)invoke(?:｜|)?>`)
var dsmlParamRe = regexp.MustCompile(`(?is)<(?:｜|)DSML(?:｜|)parameter\s+name\s*=\s*\"([^\"]*)\"(?:｜|)?\s*>([\s\S]*?)</(?:｜|)DSML(?:｜|)parameter(?:｜|)?>`)

// normalizeDSMLText replaces fullwidth ｜ (U+FF5C) with ASCII | and strips DSML prefix
// from tags so findBlocks can match them as plain XML tags.
func normalizeDSMLText(text string) string {
	r := strings.ReplaceAll(text, "\uff5c", "|")
	// Strip DSML prefix: <|DSML|tag> → <tag>, </|DSML|tag> → </tag>
	r = regexp.MustCompile(`(?is)<(/?)\|DSML\|`).ReplaceAllString(r, "<$1")
	return r
}

// dsmlExtractName extracts the name attribute from a DSML block's opening tag.
func dsmlExtractName(b xmlBlock) string {
	if n := parseAttrs(b.Attrs)["name"]; n != "" {
		return n
	}
	return ""
}

// dsmlParseParams recursively parses DSML <parameter> children from a body string.
// Returns a map where keys are parameter names and values are either strings or nested maps.
// Repeated keys (e.g. multiple "item" parameters) are automatically collected into arrays.
// Parameters without a name attribute have their children merged into the parent result.
func dsmlParseParams(body string) map[string]any {
	norm := normalizeDSMLText(body)
	result := map[string]any{}
	for _, pb := range findBlocks(norm, "parameter") {
		name := dsmlExtractName(pb)
		children := dsmlParseParams(pb.Body)
		if name == "" {
			// No name attribute — merge children directly into parent result
			for k, v := range children {
				appendVal(result, k, v)
			}
			continue
		}
		if len(children) > 0 {
			appendVal(result, name, children)
		} else {
			appendVal(result, name, parseParamValue(pb.Body))
		}
	}
	return result
}

// parseDSML parses DSML-formatted tool calls using state-machine XML parsing.
// This replaces the previous regex-based parser which couldn't handle nested parameters.
func parseDSML(text string) []ParsedToolCall {
	norm := normalizeDSMLText(text)
	wrappers := findBlocks(norm, "tool_calls")
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
			input := dsmlParseParams(inv.Body)
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

// parseToolCall handles the <tool_call><function=X>...</function></tool_call> format
// where the model wraps function calls in a tool_call container.
func parseToolCall(text string) []ParsedToolCall {
	// Look for <tool_call>...</tool_call> blocks
	tcRe := regexp.MustCompile(`(?is)<tool_call>([\s\S]*?)</tool_call>`)
	matches := tcRe.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}
	var allCalls []ParsedToolCall
	for _, m := range matches {
		// Inside the tool_call block, parse function=X tags using existing parser
		inner := m[1]
		calls := parseFuncXML(inner)
		if len(calls) == 0 {
			// Also try plain <tool_calls>...</tool_calls> inside
			calls = parseToolCallsXML(inner)
		}
		allCalls = append(allCalls, calls...)
	}
	if len(allCalls) > 0 {
		return allCalls
	}
	return nil
}

// parsePercentToolCalls handles MiMo Code native format: % ToolName args
func parsePercentToolCalls(text string) []ParsedToolCall {
	lines := strings.Split(text, "\n")
	var calls []ParsedToolCall
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "%") {
			continue
		}
		// % ToolName arg1 arg2 ...
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		toolName := parts[1]
		// Skip internal MiMo Code tools that aren't in our tool list
		if toolName == "Build" || toolName == "Plan" || toolName == "Search" {
			continue
		}
		args := map[string]any{}
		if len(parts) > 2 {
			// First arg for webfetch/web_search is usually the URL/query
			switch strings.ToLower(toolName) {
			case "webfetch", "web_fetch":
				args["url"] = parts[2]
				if len(parts) > 3 {
					args["format"] = parts[3]
				}
			case "web_search", "websearch":
				args["query"] = strings.Join(parts[2:], " ")
			default:
				// For other tools, join remaining args as the main parameter
				args["command"] = strings.Join(parts[2:], " ")
			}
		}
		calls = append(calls, ParsedToolCall{Name: toolName, Input: args})
	}
	if len(calls) > 0 {
		return calls
	}
	return nil
}

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
	calls = parseFuncXML(text)
	if len(calls) > 0 {
		return calls
	}
	calls = parseToolCall(text)
	if len(calls) > 0 {
		return calls
	}
	return parsePercentToolCalls(text)
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

var percentToolRe = regexp.MustCompile(`(?m)^%\s+\w+\s+.*$`)

func HasToolCallSyntax(text string) bool {
	if text == "" {
		return false
	}
	lower := strings.ToLower(text)
	return strings.Contains(lower, "tool_calls") ||
		strings.Contains(lower, "dsml") ||
		strings.Contains(lower, "function=") ||
		strings.Contains(lower, "tool_call") ||
		percentToolRe.MatchString(text)
}

func StripToolCallSyntax(text string) string {
	if text == "" {
		return ""
	}
	result := dsmlWrapperRe.ReplaceAllString(text, "")
	result = regexp.MustCompile(`(?is)<tool_calls>[\s\S]*?</tool_calls>`).ReplaceAllString(result, "")
	result = regexp.MustCompile(`(?is)<function=[^>]*>[\s\S]*?</function>`).ReplaceAllString(result, "")
	// Strip <tool_call>...</tool_call> blocks
	result = regexp.MustCompile(`(?is)<tool_call>[\s\S]*?</tool_call>`).ReplaceAllString(result, "")
	// Strip MiMo Code native % ToolName args lines
	result = percentToolRe.ReplaceAllString(result, "")
	return strings.TrimSpace(result)
}
