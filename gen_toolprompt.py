#!/usr/bin/env python3
"""Generate toolprompt.go with ds2api-style instructions."""
import os

# Build Go source as raw string, using \x3c etc for XML-like chars in string literals
# The key insight: Go backtick raw strings can contain < > / etc freely
# We just need to avoid writing actual XML *attributes* that look like tool calls

PIPE = chr(0xFF5C)  # ｜ fullwidth pipe

# Template uses DSML format with fullwidth pipes - safe from detection
template_lines = [
    'TOOL CALL FORMAT - FOLLOW EXACTLY:',
    '',
    f'<{PIPE}DSML{PIPE}tool_calls>',
    f'  <{PIPE}DSML{PIPE}invoke name="TOOL_NAME_HERE">',
    f'    <{PIPE}DSML{PIPE}parameter name="PARAM_NAME"><![CDATA[PARAM_VALUE]]></{PIPE}DSML{PIPE}parameter>',
    f'  </{PIPE}DSML{PIPE}invoke>',
    f'</{PIPE}DSML{PIPE}tool_calls>',
    '',
    'RULES:',
    f'1) Use the <{PIPE}DSML{PIPE}tool_calls> wrapper format.',
    f'2) Put one or more <{PIPE}DSML{PIPE}invoke> entries under a single <{PIPE}DSML{PIPE}tool_calls> root.',
    f'3) Put the tool name in the invoke name attribute: <{PIPE}DSML{PIPE}invoke name="TOOL_NAME">.',
    f'4) All string values must use <![CDATA[...]]>, even short ones. This includes code, scripts, file contents, prompts, paths, names, and queries.',
    f'5) Every top-level argument must be a <{PIPE}DSML{PIPE}parameter name="ARG_NAME">...</{PIPE}DSML{PIPE}parameter> node.',
    f'6) Objects use nested XML elements inside the parameter body. Arrays may repeat <item> children.',
    '7) Numbers, booleans, and null stay plain text.',
    '8) Use only the parameter names in the tool schema. Do not invent fields.',
    '9) Fill ALL parameters with actual values. Do not emit placeholder, blank, or whitespace-only parameters.',
    '10) If a required parameter value is unknown, ask the user or answer normally instead of outputting an empty tool call.',
    '11) For shell tools (Bash/execute_command), the command must be inside the command parameter. Never call them with an empty command.',
    '12) Do NOT wrap XML in markdown fences. Do NOT output explanations after tool calls.',
    f'13) The first non-whitespace chars must be exactly <{PIPE}DSML{PIPE}tool_calls>.',
    f'14) Never omit the opening <{PIPE}DSML{PIPE}tool_calls> tag.',
    '15) Legacy <tool_calls>/<invoke>/<parameter> tags also accepted, but prefer DSML form.',
    '',
    'PARAMETER SHAPES:',
    f'- string => <{PIPE}DSML{PIPE}parameter name="x"><![CDATA[value]]></{PIPE}DSML{PIPE}parameter>',
    f'- object => <{PIPE}DSML{PIPE}parameter name="x"><field>...</field></{PIPE}DSML{PIPE}parameter>',
    f'- array => <{PIPE}DSML{PIPE}parameter name="x"><item>...</item></{PIPE}DSML{PIPE}parameter>',
    f'- number/bool/null => <{PIPE}DSML{PIPE}parameter name="x">plain_text</{PIPE}DSML{PIPE}parameter>',
    '',
    'WRONG - Do NOT do these:',
    '',
    'Wrong 1 - mixed text after XML:',
    f'  <{PIPE}DSML{PIPE}tool_calls>...</{PIPE}DSML{PIPE}tool_calls> I hope this helps.',
    '',
    'Wrong 2 - Markdown code fences:',
    '  ```xml',
    f'  <{PIPE}DSML{PIPE}tool_calls>...</{PIPE}DSML{PIPE}tool_calls>',
    '  ```',
    '',
    'Wrong 3 - missing opening wrapper:',
    f'  <{PIPE}DSML{PIPE}invoke name="TOOL_NAME">...</{PIPE}DSML{PIPE}invoke>',
    f'  </{PIPE}DSML{PIPE}tool_calls>',
    '',
    'Wrong 4 - empty parameters:',
    f'  <{PIPE}DSML{PIPE}tool_calls>',
    f'    <{PIPE}DSML{PIPE}invoke name="Bash">',
    f'      <{PIPE}DSML{PIPE}parameter name="command"></{PIPE}DSML{PIPE}parameter>',
    f'    </{PIPE}DSML{PIPE}invoke>',
    f'  </{PIPE}DSML{PIPE}tool_calls>',
    '',
    f'Remember: The ONLY valid way to use tools is the <{PIPE}DSML{PIPE}tool_calls>...</{PIPE}DSML{PIPE}tool_calls> block at the end of your response.',
]

template_str = "\\n".join(line.replace('"', '\\"') for line in template_lines)

# Build the Go code
go_code = f'''package toolcall

import (
\t"encoding/json"
\t"fmt"
\t"strings"

\t"github.com/wtz44/mimo-gateway/internal/adapter"
)

// BuildToolPrompt converts OpenAI tools to text instructions for system prompt injection.
func BuildToolPrompt(tools []adapter.OpenAITool) string {{
\tif len(tools) == 0 {{ return "" }}
\tvar toolSchemas []string
\tvar toolNames []string
\tfor _, tool := range tools {{
\t\tname := strings.TrimSpace(tool.Function.Name)
\t\tif name == "" {{ continue }}
\t\ttoolNames = append(toolNames, name)
\t\tdesc := strings.TrimSpace(tool.Function.Description)
\t\tif desc == "" {{ desc = "No description available" }}
\t\tparamsJSON := "{{}}"
\t\tif tool.Function.Parameters != nil {{
\t\t\tif b, err := json.Marshal(tool.Function.Parameters); err == nil {{ paramsJSON = string(b) }}
\t\t}}
\t\ttoolSchemas = append(toolSchemas, fmt.Sprintf("Tool: %s\\nDescription: %s\\nParameters: %s", name, desc, paramsJSON))
\t}}
\tif len(toolSchemas) == 0 {{ return "" }}
\treturn "You have access to these tools:\\n\\n" +
\t\tstrings.Join(toolSchemas, "\\n\\n") + "\\n\\n" +
\t\tbuildToolCallInstructions(toolNames)
}}

// InjectToolPrompt injects tool instructions into the system message.
func InjectToolPrompt(messages []adapter.OpenAIMessage, toolPrompt string) []adapter.OpenAIMessage {{
\tif toolPrompt == "" {{ return messages }}
\tfor i := range messages {{
\t\tif messages[i].Role == "system" {{
\t\t\told, _ := messages[i].Content.(string)
\t\t\tmessages[i].Content = strings.TrimSpace(old + "\\n\\n" + toolPrompt)
\t\t\treturn messages
\t\t}}
\t}}
\treturn append([]adapter.OpenAIMessage{{{{Role: "system", Content: toolPrompt}}}}, messages...)
}}

func buildToolCallInstructions(toolNames []string) string {{
\treturn buildInstructionTemplate() + buildCorrectToolExamples(toolNames)
}}

func buildInstructionTemplate() string {{
\treturn "{template_str}"
}}

type promptToolExample struct {{
\tname   string
\tparams string
}}

func buildCorrectToolExamples(toolNames []string) string {{
\tnames := uniqueToolNames(toolNames)
\texamples := make([]string, 0, 3)

\tif single, ok := firstBasicExample(names); ok {{
\t\texamples = append(examples, "Example A - Single tool:\\n"+renderToolExampleBlock([]promptToolExample{{single}}))
\t}}
\tif parallel := firstNBasicExamples(names, 2); len(parallel) >= 2 {{
\t\texamples = append(examples, "Example B - Two tools in parallel:\\n"+renderToolExampleBlock(parallel))
\t}}
\tif script, ok := firstScriptExample(names); ok {{
\t\texamples = append(examples, "Example C - Tool with long script using CDATA:\\n"+renderToolExampleBlock([]promptToolExample{{script}}))
\t}}

\tif len(examples) == 0 {{ return "" }}
\treturn "CORRECT EXAMPLES:\\n\\n" + strings.Join(examples, "\\n\\n") + "\\n\\n"
}}

func renderToolExampleBlock(calls []promptToolExample) string {{
\tvar b strings.Builder
\tb.WriteString("<{PIPE}DSML{PIPE}tool_calls>\\n")
\tfor _, call := range calls {{
\t\tb.WriteString("  <{PIPE}DSML{PIPE}invoke name=\\"" + call.name + "\\">\\n")
\t\tb.WriteString(indentPromptParameters(call.params, "    "))
\t\tb.WriteString("\\n  </{PIPE}DSML{PIPE}invoke>\\n")
\t}}
\tb.WriteString("</{PIPE}DSML{PIPE}tool_calls>")
\treturn b.String()
}}

func indentPromptParameters(body, indent string) string {{
\tif strings.TrimSpace(body) == "" {{
\t\treturn indent + "<{PIPE}DSML{PIPE}parameter name=\\"content\\"></{PIPE}DSML{PIPE}parameter>"
\t}}
\tlines := strings.Split(body, "\\n")
\tfor i, line := range lines {{
\t\tif strings.TrimSpace(line) == "" {{ lines[i] = line; continue }}
\t\tlines[i] = indent + line
\t}}
\treturn strings.Join(lines, "\\n")
}}

func uniqueToolNames(names []string) []string {{
\tseen := map[string]bool{{}}
\tvar out []string
\tfor _, n := range names {{
\t\tn = strings.TrimSpace(n)
\t\tif n == "" || seen[n] {{ continue }}
\t\tseen[n] = true
\t\tout = append(out, n)
\t}}
\treturn out
}}

func firstBasicExample(names []string) (promptToolExample, bool) {{
\tfor _, name := range names {{
\t\tif params, ok := exampleBasicParams(name); ok {{
\t\t\treturn promptToolExample{{name: name, params: params}}, true
\t\t}}
\t}}
\treturn promptToolExample{{}}, false
}}

func firstNBasicExamples(names []string, count int) []promptToolExample {{
\tout := make([]promptToolExample, 0, count)
\tfor _, name := range names {{
\t\tif params, ok := exampleBasicParams(name); ok {{
\t\t\tout = append(out, promptToolExample{{name: name, params: params}})
\t\t\tif len(out) == count {{ return out }}
\t\t}}
\t}}
\treturn out
}}

func firstScriptExample(names []string) (promptToolExample, bool) {{
\tfor _, name := range names {{
\t\tif params, ok := exampleScriptParams(name); ok {{
\t\t\treturn promptToolExample{{name: name, params: params}}, true
\t\t}}
\t}}
\treturn promptToolExample{{}}, false
}}

func wrapParam(name, inner string) string {{
\treturn "<{PIPE}DSML{PIPE}parameter name=\\"" + name + "\\">" + inner + "</{PIPE}DSML{PIPE}parameter>"
}}

func promptCDATA(text string) string {{
\tif text == "" {{ return "" }}
\tif strings.Contains(text, "]]>") {{
\t\treturn "<![CDATA[" + strings.ReplaceAll(text, "]]>", "]]]]><![CDATA[>") + "]]>"
\t}}
\treturn "<![CDATA[" + text + "]]>"
}}

func exampleBasicParams(name string) (string, bool) {{
\tswitch strings.TrimSpace(name) {{
\tcase "Read", "read_file":
\t\treturn wrapParam("file_path", promptCDATA("README.md")), true
\tcase "Glob", "list_files":
\t\treturn wrapParam("path", promptCDATA(".")), true
\tcase "search_files":
\t\treturn wrapParam("query", promptCDATA("tool call parser")), true
\tcase "Bash", "execute_command", "bash":
\t\treturn wrapParam("command", promptCDATA("pwd")), true
\tcase "Write", "write_to_file":
\t\treturn wrapParam("file_path", promptCDATA("notes.txt")) + "\\n" + wrapParam("content", promptCDATA("Hello world")), true
\tcase "Edit":
\t\treturn wrapParam("file_path", promptCDATA("README.md")) + "\\n" + wrapParam("old_string", promptCDATA("foo")) + "\\n" + wrapParam("new_string", promptCDATA("bar")), true
\tcase "webfetch", "web_fetch":
\t\treturn wrapParam("url", promptCDATA("https://example.com")), true
\t}}
\treturn "", false
}}

func exampleScriptParams(name string) (string, bool) {{
\tscript := "echo hello\\nls -la\\ncat /etc/hosts"
\tswitch strings.TrimSpace(name) {{
\tcase "Bash", "execute_command", "bash":
\t\treturn wrapParam("command", promptCDATA(script)), true
\t}}
\treturn "", false
}}
'''

out_path = os.path.join("internal", "toolcall", "toolprompt.go")
with open(out_path, "w", encoding="utf-8") as f:
    f.write(go_code)
print(f"Written {out_path} ({len(go_code)} bytes)")
