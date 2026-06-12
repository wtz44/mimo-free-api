#!/usr/bin/env python3
"""Add support for format 7: <tool_name><key>value</key></tool_name>
Also add HasToolCallSyntax support for 'tool_call' singular.
"""
import re

path = "internal/toolcall/toolparse.go"
with open(path, "r", encoding="utf-8") as f:
    content = f.read()

# 1. Add parseDirectTagXML function before "Public API" section
direct_parser = '''
// ===== Direct tag parser: <tool_name><key>value</key></tool_name> =====
// Handles format where tool name IS the tag, params are direct children.

func parseDirectTagXML(text string, toolNames []string) []ParsedToolCall {
\tif len(toolNames) == 0 {
\t\treturn nil
\t}
\tvar calls []ParsedToolCall
\tfor _, name := range toolNames {
\t\tfor _, block := range findBlocks(text, name) {
\t\t\tinput := map[string]any{}
\t\t\tfor _, child := range findBlocks(block.Body, "") {
\t\t\t\t_ = child
\t\t\t}
\t\t\t// Parse child tags generically by scanning for <key>value</key>
\t\t\tchildMap := parseMarkupKV(block.Body)
\t\t\tif len(childMap) > 0 {
\t\t\t\tinput = childMap
\t\t\t}
\t\t\tcalls = append(calls, ParsedToolCall{Name: name, Input: input})
\t\t}
\t}
\treturn calls
}

// parseMarkupKV parses generic <key>value</key> pairs from text.
func parseMarkupKV(text string) map[string]any {
\tout := map[string]any{}
\ttrimmed := strings.TrimSpace(text)
\tif trimmed == "" {
\t\treturn out
\t}
\t// Find all top-level XML elements
\tpos := 0
\tfor pos < len(trimmed) {
\t\t// Find next <
\t\tltIdx := strings.Index(trimmed[pos:], "<")
\t\tif ltIdx < 0 {
\t\t\tbreak
\t\t}
\t\tltIdx += pos
\t\t// Skip closing tags
\t\tif ltIdx+1 < len(trimmed) && trimmed[ltIdx+1] == '/' {
\t\t\tpos = ltIdx + 1
\t\t\tcontinue
\t\t}
\t\t// Find tag end
\t\ttagEndIdx := strings.Index(trimmed[ltIdx:], ">")
\t\tif tagEndIdx < 0 {
\t\t\tbreak
\t\t}
\t\ttagEndIdx += ltIdx
\t\ttagName := strings.TrimSpace(trimmed[ltIdx+1 : tagEndIdx])
\t\t// Skip if tag name has spaces or = (it's an invoke/parameter style tag)
\t\tif strings.ContainsAny(tagName, " =\\"'") {
\t\t\tpos = tagEndIdx + 1
\t\t\tcontinue
\t\t}
\t\tbodyStart := tagEndIdx + 1
\t\tcloseTag := "</" + tagName + ">"
\t\tcloseIdx := strings.Index(trimmed[bodyStart:], closeTag)
\t\tif closeIdx < 0 {
\t\t\t// Try case-insensitive
\t\t\tcloseIdx = strings.Index(strings.ToLower(trimmed[bodyStart:]), strings.ToLower(closeTag))
\t\t\tif closeIdx < 0 {
\t\t\t\tpos = bodyStart
\t\t\t\tcontinue
\t\t\t}
\t\t}
\t\tcloseIdx += bodyStart
\t\tvalue := parseParamValue(trimmed[bodyStart:closeIdx])
\t\tappendVal(out, tagName, value)
\t\tpos = closeIdx + len(closeTag)
\t}
\treturn out
}
'''

# Insert before "// ===== Public API ====="
marker = "// ===== Public API ====="
if marker in content and "parseDirectTagXML" not in content:
    content = content.replace(marker, direct_parser + "\n" + marker)
    print("Added parseDirectTagXML")
else:
    print("parseDirectTagXML already exists or marker not found")

# 2. Update ParseToolCallsFromText to also try direct tag parsing
old_parse = '''\tcalls = parseToolCallsXML(text)
\tif len(calls) > 0 {
\t\treturn calls
\t}
\treturn parseFuncXML(text)'''

new_parse = '''\tcalls = parseToolCallsXML(text)
\tif len(calls) > 0 {
\t\treturn calls
\t}
\tcalls = parseFuncXML(text)
\tif len(calls) > 0 {
\t\treturn calls
\t}
\t// Try direct tag format: <tool_name><key>value</key></tool_name>
\tknownTools := []string{"webfetch", "web_search", "read_file", "write_file", "bash", "execute_command", "skill", "arxiv", "ddg-search"}
\treturn parseDirectTagXML(text, knownTools)'''

if old_parse in content:
    content = content.replace(old_parse, new_parse)
    print("Updated ParseToolCallsFromText")
else:
    print("ParseToolCallsFromText pattern not found, checking...")
    if "parseDirectTagXML" in content:
        print("  Already updated")
    else:
        print("  Need manual fix")

# 3. Update HasToolCallSyntax to also detect singular 'tool_call'
old_has = '''func HasToolCallSyntax(text string) bool {
\tif text == "" {
\t\treturn false
\t}
\tlower := strings.ToLower(text)
\treturn strings.Contains(lower, "tool_calls") ||
\t\tstrings.Contains(lower, "dsml") ||
\t\tstrings.Contains(lower, "function=")'''

new_has = '''func HasToolCallSyntax(text string) bool {
\tif text == "" {
\t\treturn false
\t}
\tlower := strings.ToLower(text)
\treturn strings.Contains(lower, "tool_calls") ||
\t\tstrings.Contains(lower, "tool_call") ||
\t\tstrings.Contains(lower, "dsml") ||
\t\tstrings.Contains(lower, "function=")'''

if old_has in content:
    content = content.replace(old_has, new_has)
    print("Updated HasToolCallSyntax")
else:
    print("HasToolCallSyntax already updated or pattern not found")

# 4. Update StripToolCallSyntax
old_strip = '''\tresult = regexp.MustCompile("(?is)<function=[^>]*>[\\\\s\\\\S]*?</function>").ReplaceAllString(result, "")
\treturn strings.TrimSpace(result)'''

new_strip = '''\tresult = regexp.MustCompile("(?is)<function=[^>]*>[\\\\s\\\\S]*?</function>").ReplaceAllString(result, "")
\tresult = regexp.MustCompile("(?is)<tool_call>[\\\\s\\\\S]*?</tool_call>").ReplaceAllString(result, "")
\treturn strings.TrimSpace(result)'''

if old_strip in content:
    content = content.replace(old_strip, new_strip)
    print("Updated StripToolCallSyntax")
else:
    print("StripToolCallSyntax already updated or pattern not found")

with open(path, "w", encoding="utf-8") as f:
    f.write(content)
print(f"Written {path}")
