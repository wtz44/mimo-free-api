#!/usr/bin/env python3
"""Generate toolparse.go - avoids embedding triggering XML tags directly."""
import os, sys

LT = chr(60)  # <
GT = chr(62)  # >
SL = chr(47)  # /
DQ = chr(34)  # "
BT = chr(96)  # ` (backtick for Go raw strings)
EQ = chr(61)  # =
SP = chr(32)  # space
NL = chr(10)  # newline

F = "function"
P = "parameter"
T = "tool_calls"
I = "invoke"
DSML = "\uff5cDSML\uff5c"

def tag_open(name, attrs=""):
    if attrs:
        return f"{LT}{name}{SP}{attrs}{GT}"
    return f"{LT}{name}{GT}"

def tag_close(name):
    return f"{LT}{SL}{name}{GT}"

def make_re(pattern):
    return f"regexp.MustCompile({DQ}{pattern}{DQ})"

# Build regex patterns
fullwidth_or = "(?:\uff5c|)"
dsml_open = f"{LT}{fullwidth_or}DSML{fullwidth_or}"

lines = []
lines.append("package toolcall")
lines.append("")
lines.append("import (")
lines.append('\t"encoding/json"')
lines.append('\t"html"')
lines.append('\t"regexp"')
lines.append('\t"strings"')
lines.append("")
lines.append('\t"github.com/google/uuid"')
lines.append('\t"github.com/wtz44/mimo-gateway/internal/adapter"')
lines.append(")")
lines.append("")
lines.append("type ParsedToolCall struct {")
lines.append("\tName  string")
lines.append("\tInput map[string]any")
lines.append("}")
lines.append("")

# var block
lines.append("var (")

# DSML regex
r1 = f"(?is){LT}{fullwidth_or}DSML{fullwidth_or}tool_calls{fullwidth_or}{GT}([\\s\\S]*?){LT}{SL}{fullwidth_or}DSML{fullwidth_or}tool_calls{fullwidth_or}{GT}"
lines.append(f'\tdsmlToolCallsRe = regexp.MustCompile({BT}{r1}{BT})')

r2 = f"(?is){LT}{fullwidth_or}DSML{fullwidth_or}invoke\\s+name\\s*=\\s*{DQ}([^\\{DQ}]*){DQ}{fullwidth_or}?\\s*{GT}([\\s\\S]*?){LT}{SL}{fullwidth_or}DSML{fullwidth_or}invoke{fullwidth_or}?{GT}"
lines.append(f'\tdsmlInvokeRe    = regexp.MustCompile({BT}{r2}{BT})')

r3 = f"(?is){LT}{fullwidth_or}DSML{fullwidth_or}parameter\\s+name\\s*=\\s*{DQ}([^\\{DQ}]*){DQ}{fullwidth_or}?\\s*{GT}([\\s\\S]*?){LT}{SL}{fullwidth_or}DSML{fullwidth_or}parameter{fullwidth_or}?{GT}"
lines.append(f'\tdsmlParamRe     = regexp.MustCompile({BT}{r3}{BT})')
lines.append("")

# Plain regex
r4 = f"(?is){LT}tool_calls{GT}([\\s\\S]*?){LT}{SL}tool_calls{GT}"
lines.append(f'\tplainToolCallsRe = regexp.MustCompile({BT}{r4}{BT})')

r5 = f"(?is){LT}invoke\\s+name\\s*=\\s*{DQ}([^\\{DQ}]*){DQ}{GT}([\\s\\S]*?){LT}{SL}invoke{GT}"
lines.append(f'\tplainInvokeRe    = regexp.MustCompile({BT}{r5}{BT})')

r6 = f"(?is){LT}parameter\\s+name\\s*=\\s*{DQ}([^\\{DQ}]*){DQ}{GT}([\\s\\S]*?){LT}{SL}parameter{GT}"
lines.append(f'\tplainParamRe     = regexp.MustCompile({BT}{r6}{BT})')
lines.append("")

# Standard function regex
r7 = f"(?is){LT}{F}=([^>\\s]+){GT}[\\r\\n\\s]*([\\s\\S]*?){LT}{SL}{F}{GT}"
lines.append(f'\tstandardFuncRe  = regexp.MustCompile({BT}{r7}{BT})')

r8 = f"(?is){LT}{P}\\s+name\\s*=\\s*{DQ}([^\\{DQ}]*){DQ}{GT}([\\s\\S]*?){LT}{SL}{P}{GT}"
lines.append(f'\tstandardParamRe    = regexp.MustCompile({BT}{r8}{BT})')
# Alt format: param=KEY without name= attribute
r9 = f"(?is){LT}{P}=([^>\s]+){GT}([\s\S]*?){LT}{SL}{P}{GT}"
lines.append(f'	standardParamEqRe = regexp.MustCompile({BT}{r9}{BT})')
lines.append(")")


# ParseToolCallsFromText
lines.append("")
lines.append("// ParseToolCallsFromText parses tool calls from model text output.")
lines.append("func ParseToolCallsFromText(text string) []ParsedToolCall {")
lines.append('\tif text == "" { return nil }')
lines.append("\tcalls := parseWithRegex(text, dsmlToolCallsRe, dsmlInvokeRe, dsmlParamRe)")
lines.append("\tif len(calls) > 0 { return calls }")
lines.append("\tcalls = parseWithRegex(text, plainToolCallsRe, plainInvokeRe, plainParamRe)")
lines.append("\tif len(calls) > 0 { return calls }")
lines.append("\treturn parseStandardFuncCalls(text)")
lines.append("}")
lines.append("")

# parseWithRegex
lines.append("func parseWithRegex(text string, wrapperRe, invokeRe, paramRe *regexp.Regexp) []ParsedToolCall {")
lines.append("\twrapperMatches := wrapperRe.FindAllStringSubmatch(text, -1)")
lines.append("\tif len(wrapperMatches) == 0 { return nil }")
lines.append("\tvar calls []ParsedToolCall")
lines.append("\tfor _, wm := range wrapperMatches {")
lines.append("\t\tif len(wm) < 2 { continue }")
lines.append("\t\tfor _, im := range invokeRe.FindAllStringSubmatch(wm[1], -1) {")
lines.append("\t\t\tif len(im) < 3 { continue }")
lines.append("\t\t\tname := strings.TrimSpace(im[1])")
lines.append("\t\t\tif name == \"\" { continue }")
lines.append("\t\t\tinput := map[string]any{}")
lines.append("\t\t\tfor _, pm := range paramRe.FindAllStringSubmatch(im[2], -1) {")
lines.append("\t\t\t\tif len(pm) < 3 { continue }")
lines.append("\t\t\t\tpname := strings.TrimSpace(pm[1])")
lines.append("\t\t\t\tif pname == \"\" { continue }")
lines.append("\t\t\t\tval := parseParameterValue(pm[2])")
lines.append("\t\t\t\tif existing, ok := input[pname]; ok {")
lines.append("\t\t\t\t\tswitch v := existing.(type) {")
lines.append("\t\t\t\t\tcase []any: input[pname] = append(v, val)")
lines.append("\t\t\t\t\tdefault: input[pname] = []any{v, val}")
lines.append("\t\t\t\t\t}")
lines.append("\t\t\t\t} else { input[pname] = val }")
lines.append("\t\t\t}")
lines.append("\t\t\tcalls = append(calls, ParsedToolCall{Name: name, Input: input})")
lines.append("\t\t}")
lines.append("\t}")
lines.append("\treturn calls")
lines.append("}")
lines.append("")

# parseStandardFuncCalls
lines.append("// parseStandardFuncCalls handles <" + F + "=name>...</" + F + "> format.")
lines.append("func parseStandardFuncCalls(text string) []ParsedToolCall {")
lines.append("\tmatches := standardFuncRe.FindAllStringSubmatch(text, -1)")
lines.append("\tif len(matches) == 0 { return nil }")
lines.append("\tvar calls []ParsedToolCall")
lines.append("\tfor _, m := range matches {")
lines.append("\t\tif len(m) < 3 { continue }")
lines.append("\t\tname := strings.TrimSpace(m[1])")
lines.append("\t\tif name == \"\" { continue }")
lines.append("\t\tbody := m[2]")
lines.append("\t\tinput := map[string]any{}")
lines.append("\t\tfor _, pm := range standardParamRe.FindAllStringSubmatch(body, -1) {")
lines.append("\t\t\tif len(pm) < 3 { continue }")
lines.append("\t\t\tpname := strings.TrimSpace(pm[1])")
lines.append("\t\t\tif pname == \"\" { continue }")
lines.append("\t\t\tval := parseParameterValue(pm[2])")
lines.append("\t\t\tif existing, ok := input[pname]; ok {")
lines.append("\t\t\t\tswitch v := existing.(type) {")
lines.append("\t\t\t\tcase []any: input[pname] = append(v, val)")
lines.append("\t\t\t\tdefault: input[pname] = []any{v, val}")
lines.append("\t\t\t\t}")
lines.append("\t\t\t} else { input[pname] = val }")
lines.append("\t\t}")
lines.append("		for _, pm := range standardParamEqRe.FindAllStringSubmatch(body, -1) {")
lines.append("			if len(pm) < 3 { continue }")
lines.append("			pname := strings.TrimSpace(pm[1])")
lines.append("			if pname == \"\"\" { continue }")
lines.append("			val := parseParameterValue(pm[2])")
lines.append("			if existing, ok := input[pname]; ok {")
lines.append("				switch v := existing.(type) {")
lines.append("				case []any: input[pname] = append(v, val)")
lines.append("				default: input[pname] = []any{v, val}")
lines.append("				}")
lines.append("			} else { input[pname] = val }")
lines.append("		}")
lines.append("\t\tcalls = append(calls, ParsedToolCall{Name: name, Input: input})")
lines.append("\t}")
lines.append("\treturn calls")
lines.append("}")
lines.append("")

# parseParameterValue
lines.append("func parseParameterValue(raw string) any {")
lines.append('\ttrimmed := strings.TrimSpace(raw)')
lines.append('\tif trimmed == "" { return "" }')
lines.append('\tif strings.HasPrefix(trimmed, ' + DQ + '<![CDATA[' + DQ + ') {')
lines.append('\t\tendIdx := strings.Index(trimmed, ' + DQ + ']]>' + DQ + ')')
lines.append('\t\tif endIdx >= 0 { return trimmed[9:endIdx] }')
lines.append('\t\treturn trimmed[9:]')
lines.append('\t}')
lines.append('\tvar jsonVal any')
lines.append('\tif err := json.Unmarshal([]byte(trimmed), &jsonVal); err == nil { return jsonVal }')
lines.append('\treturn html.UnescapeString(trimmed)')
lines.append("}")
lines.append("")

# ConvertToolCallsToOpenAI
lines.append("// ConvertToolCallsToOpenAI converts parsed tool calls to OpenAI format.")
lines.append("func ConvertToolCallsToOpenAI(calls []ParsedToolCall) []adapter.OpenAIToolCall {")
lines.append('\tif len(calls) == 0 { return nil }')
lines.append('\tresult := make([]adapter.OpenAIToolCall, 0, len(calls))')
lines.append('\tfor _, call := range calls {')
lines.append("\t\targs, _ := json.Marshal(call.Input)")
lines.append("\t\tresult = append(result, adapter.OpenAIToolCall{")
lines.append('\t\t\tID:   "call_" + strings.ReplaceAll(uuid.New().String(), "-", ""),')
lines.append('\t\t\tType: "function",')
lines.append("\t\t\tFunction: adapter.OpenAIToolCallFunc{Name: call.Name, Arguments: string(args)},")
lines.append("\t\t})")
lines.append("\t}")
lines.append("\treturn result")
lines.append("}")
lines.append("")

# HasToolCallSyntax
lines.append("// HasToolCallSyntax checks if text contains any recognized tool call format.")
lines.append("func HasToolCallSyntax(text string) bool {")
lines.append('\tif text == "" { return false }')
lines.append("\tdsml := dsmlToolCallsRe.MatchString(text)")
lines.append("\tplain := plainToolCallsRe.MatchString(text)")
lines.append("\tstandard := standardFuncRe.MatchString(text)")
lines.append("\treturn dsml || plain || standard")
lines.append("}")
lines.append("")

# StripToolCallSyntax
lines.append("// StripToolCallSyntax removes tool call XML from text.")
lines.append("func StripToolCallSyntax(text string) string {")
lines.append('\tif text == "" { return "" }')
lines.append("\tresult := dsmlToolCallsRe.ReplaceAllString(text, \"\")")
lines.append("\tresult = plainToolCallsRe.ReplaceAllString(result, \"\")")
lines.append("\tresult = standardFuncRe.ReplaceAllString(result, \"\")")
lines.append("\treturn strings.TrimSpace(result)")
lines.append("}")

content = "\n".join(lines) + "\n"
outpath = os.path.join(os.path.dirname(os.path.abspath(__file__)), "toolparse.go")
with open(outpath, "w", encoding="utf-8") as f:
    f.write(content)
print(f"Written {len(content)} bytes to {outpath}")
print(f"Lines: {len(lines)}")
