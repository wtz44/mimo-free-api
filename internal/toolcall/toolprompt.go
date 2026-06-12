package toolcall

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/wtz44/mimo-gateway/internal/adapter"
)

// BuildToolPrompt converts OpenAI tools to text instructions for system prompt injection.
func BuildToolPrompt(tools []adapter.OpenAITool) string {
	if len(tools) == 0 { return "" }
	var toolSchemas []string
	var toolNames []string
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Function.Name)
		if name == "" { continue }
		toolNames = append(toolNames, name)
		desc := strings.TrimSpace(tool.Function.Description)
		if desc == "" { desc = "No description available" }
		paramsJSON := "{}"
		if tool.Function.Parameters != nil {
			if b, err := json.Marshal(tool.Function.Parameters); err == nil { paramsJSON = string(b) }
		}
		toolSchemas = append(toolSchemas, fmt.Sprintf("Tool: %s\nDescription: %s\nParameters: %s", name, desc, paramsJSON))
	}
	if len(toolSchemas) == 0 { return "" }
	return "You have access to these tools:\n\n" +
		strings.Join(toolSchemas, "\n\n") + "\n\n" +
		buildToolCallInstructions(toolNames)
}

// InjectToolPrompt injects tool instructions into the system message.
func InjectToolPrompt(messages []adapter.OpenAIMessage, toolPrompt string) []adapter.OpenAIMessage {
	if toolPrompt == "" { return messages }
	for i := range messages {
		if messages[i].Role == "system" {
			old, _ := messages[i].Content.(string)
			messages[i].Content = strings.TrimSpace(old + "\n\n" + toolPrompt)
			return messages
		}
	}
	return append([]adapter.OpenAIMessage{{Role: "system", Content: toolPrompt}}, messages...)
}

func buildToolCallInstructions(toolNames []string) string {
	return buildInstructionTemplate() + buildCorrectToolExamples(toolNames)
}

func buildInstructionTemplate() string {
	return "TOOL CALL FORMAT - FOLLOW EXACTLY:\n\n<｜DSML｜tool_calls>\n  <｜DSML｜invoke name=\"TOOL_NAME_HERE\">\n    <｜DSML｜parameter name=\"PARAM_NAME\"><![CDATA[PARAM_VALUE]]></｜DSML｜parameter>\n  </｜DSML｜invoke>\n</｜DSML｜tool_calls>\n\nRULES:\n1) Use the <｜DSML｜tool_calls> wrapper format.\n2) Put one or more <｜DSML｜invoke> entries under a single <｜DSML｜tool_calls> root.\n3) Put the tool name in the invoke name attribute: <｜DSML｜invoke name=\"TOOL_NAME\">.\n4) All string values must use <![CDATA[...]]>, even short ones. This includes code, scripts, file contents, prompts, paths, names, and queries.\n5) Every top-level argument must be a <｜DSML｜parameter name=\"ARG_NAME\">...</｜DSML｜parameter> node.\n6) Objects use nested XML elements inside the parameter body. Arrays may repeat <item> children.\n7) Numbers, booleans, and null stay plain text.\n8) Use only the parameter names in the tool schema. Do not invent fields.\n9) Fill ALL parameters with actual values. Do not emit placeholder, blank, or whitespace-only parameters.\n10) If a required parameter value is unknown, ask the user or answer normally instead of outputting an empty tool call.\n11) For shell tools (Bash/execute_command), the command must be inside the command parameter. Never call them with an empty command.\n12) Do NOT wrap XML in markdown fences. Do NOT output explanations after tool calls.\n13) The first non-whitespace chars must be exactly <｜DSML｜tool_calls>.\n14) Never omit the opening <｜DSML｜tool_calls> tag.\n15) Legacy <tool_calls>/<invoke>/<parameter> tags also accepted, but prefer DSML form.\n\nPARAMETER SHAPES:\n- string => <｜DSML｜parameter name=\"x\"><![CDATA[value]]></｜DSML｜parameter>\n- object => <｜DSML｜parameter name=\"x\"><field>...</field></｜DSML｜parameter>\n- array => <｜DSML｜parameter name=\"x\"><item>...</item></｜DSML｜parameter>\n- number/bool/null => <｜DSML｜parameter name=\"x\">plain_text</｜DSML｜parameter>\n\nWRONG - Do NOT do these:\n\nWrong 1 - mixed text after XML:\n  <｜DSML｜tool_calls>...</｜DSML｜tool_calls> I hope this helps.\n\nWrong 2 - Markdown code fences:\n  ```xml\n  <｜DSML｜tool_calls>...</｜DSML｜tool_calls>\n  ```\n\nWrong 3 - missing opening wrapper:\n  <｜DSML｜invoke name=\"TOOL_NAME\">...</｜DSML｜invoke>\n  </｜DSML｜tool_calls>\n\nWrong 4 - empty parameters:\n  <｜DSML｜tool_calls>\n    <｜DSML｜invoke name=\"Bash\">\n      <｜DSML｜parameter name=\"command\"></｜DSML｜parameter>\n    </｜DSML｜invoke>\n  </｜DSML｜tool_calls>\n\nRemember: The ONLY valid way to use tools is the <｜DSML｜tool_calls>...</｜DSML｜tool_calls> block at the end of your response."
}

type promptToolExample struct {
	name   string
	params string
}

func buildCorrectToolExamples(toolNames []string) string {
	names := uniqueToolNames(toolNames)
	examples := make([]string, 0, 3)

	if single, ok := firstBasicExample(names); ok {
		examples = append(examples, "Example A - Single tool:\n"+renderToolExampleBlock([]promptToolExample{single}))
	}
	if parallel := firstNBasicExamples(names, 2); len(parallel) >= 2 {
		examples = append(examples, "Example B - Two tools in parallel:\n"+renderToolExampleBlock(parallel))
	}
	if script, ok := firstScriptExample(names); ok {
		examples = append(examples, "Example C - Tool with long script using CDATA:\n"+renderToolExampleBlock([]promptToolExample{script}))
	}

	if len(examples) == 0 { return "" }
	return "CORRECT EXAMPLES:\n\n" + strings.Join(examples, "\n\n") + "\n\n"
}

func renderToolExampleBlock(calls []promptToolExample) string {
	var b strings.Builder
	b.WriteString("<｜DSML｜tool_calls>\n")
	for _, call := range calls {
		b.WriteString("  <｜DSML｜invoke name=\"" + call.name + "\">\n")
		b.WriteString(indentPromptParameters(call.params, "    "))
		b.WriteString("\n  </｜DSML｜invoke>\n")
	}
	b.WriteString("</｜DSML｜tool_calls>")
	return b.String()
}

func indentPromptParameters(body, indent string) string {
	if strings.TrimSpace(body) == "" {
		return indent + "<｜DSML｜parameter name=\"content\"></｜DSML｜parameter>"
	}
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" { lines[i] = line; continue }
		lines[i] = indent + line
	}
	return strings.Join(lines, "\n")
}

func uniqueToolNames(names []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" || seen[n] { continue }
		seen[n] = true
		out = append(out, n)
	}
	return out
}

func firstBasicExample(names []string) (promptToolExample, bool) {
	for _, name := range names {
		if params, ok := exampleBasicParams(name); ok {
			return promptToolExample{name: name, params: params}, true
		}
	}
	return promptToolExample{}, false
}

func firstNBasicExamples(names []string, count int) []promptToolExample {
	out := make([]promptToolExample, 0, count)
	for _, name := range names {
		if params, ok := exampleBasicParams(name); ok {
			out = append(out, promptToolExample{name: name, params: params})
			if len(out) == count { return out }
		}
	}
	return out
}

func firstScriptExample(names []string) (promptToolExample, bool) {
	for _, name := range names {
		if params, ok := exampleScriptParams(name); ok {
			return promptToolExample{name: name, params: params}, true
		}
	}
	return promptToolExample{}, false
}

func wrapParam(name, inner string) string {
	return "<｜DSML｜parameter name=\"" + name + "\">" + inner + "</｜DSML｜parameter>"
}

func promptCDATA(text string) string {
	if text == "" { return "" }
	if strings.Contains(text, "]]>") {
		return "<![CDATA[" + strings.ReplaceAll(text, "]]>", "]]]]><![CDATA[>") + "]]>"
	}
	return "<![CDATA[" + text + "]]>"
}

func exampleBasicParams(name string) (string, bool) {
	switch strings.TrimSpace(name) {
	case "Read", "read_file":
		return wrapParam("file_path", promptCDATA("README.md")), true
	case "Glob", "list_files":
		return wrapParam("path", promptCDATA(".")), true
	case "search_files":
		return wrapParam("query", promptCDATA("tool call parser")), true
	case "Bash", "execute_command", "bash":
		return wrapParam("command", promptCDATA("pwd")), true
	case "Write", "write_to_file":
		return wrapParam("file_path", promptCDATA("notes.txt")) + "\n" + wrapParam("content", promptCDATA("Hello world")), true
	case "Edit":
		return wrapParam("file_path", promptCDATA("README.md")) + "\n" + wrapParam("old_string", promptCDATA("foo")) + "\n" + wrapParam("new_string", promptCDATA("bar")), true
	case "webfetch", "web_fetch":
		return wrapParam("url", promptCDATA("https://example.com")), true
	}
	return "", false
}

func exampleScriptParams(name string) (string, bool) {
	script := "echo hello\nls -la\ncat /etc/hosts"
	switch strings.TrimSpace(name) {
	case "Bash", "execute_command", "bash":
		return wrapParam("command", promptCDATA(script)), true
	}
	return "", false
}
