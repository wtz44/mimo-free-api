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
	return "TOOL CALL FORMAT - FOLLOW EXACTLY:\n\n<｜DSML｜tool_calls>\n  <｜DSML｜invoke name=\"TOOL_NAME_HERE\">\n    <｜DSML｜parameter name=\"PARAM_NAME\"><![CDATA[PARAM_VALUE]]></｜DSML｜parameter>\n  </｜DSML｜invoke>\n</｜DSML｜tool_calls>\n\nRULES:\n1) Use the <｜DSML｜tool_calls> wrapper format.\n2) Put tool name in invoke name attribute.\n3) All string values must use CDATA: <![CDATA[...]]>\n4) Every argument must be a <｜DSML｜parameter name=\"X\"...</｜DSML｜parameter> node.\n5) Use only parameter names from the tool schema.\n6) Do NOT wrap XML in markdown fences.\n7) Do NOT output explanations after tool calls.\n8) The first non-whitespace chars must be exactly <｜DSML｜tool_calls>.\n9) Legacy <tool_calls> / <invoke> / <parameter> tags also accepted.\n"
}

type promptToolExample struct {
	name   string
	params string
}

func buildCorrectToolExamples(toolNames []string) string {
	names := uniqueToolNames(toolNames)
	for _, name := range names {
		if params, ok := exampleBasicParams(name); ok {
			return "CORRECT EXAMPLE:\n\n" + "<｜DSML｜tool_calls>\n" + "  <｜DSML｜invoke name=\"" + name + "\">\n" + indentParams(params, "    ") + "\n" + "  </｜DSML｜invoke>\n" + "</｜DSML｜tool_calls>"
		}
	}
	return ""
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

func exampleBasicParams(name string) (string, bool) {
	switch strings.TrimSpace(name) {
	case "Read", "read_file":
		return wrapParam("file_path", cdata("README.md")), true
	case "Bash", "execute_command", "bash":
		return wrapParam("command", cdata("pwd")), true
	case "Write", "write_to_file":
		return wrapParam("file_path", cdata("test.txt")) + "\n" + wrapParam("content", cdata("Hello")), true
	case "Edit":
		return wrapParam("file_path", cdata("README.md")) + "\n" + wrapParam("old_string", cdata("foo")) + "\n" + wrapParam("new_string", cdata("bar")), true
	case "webfetch", "web_fetch":
		return wrapParam("url", cdata("https://example.com")), true
	}
	return "", false
}

func wrapParam(name, inner string) string {
	return "<｜DSML｜parameter name=\"" + name + "\">" + inner + "</｜DSML｜parameter>"
}

func cdata(text string) string {
	if text == "" { return "" }
	if strings.Contains(text, "]]>") {
		return "<![CDATA[" + strings.ReplaceAll(text, "]]>", "]]]]><![CDATA[>") + "]]>"
	}
	return "<![CDATA[" + text + "]]>"
}

func indentParams(body, indent string) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != "" { lines[i] = indent + line }
	}
	return strings.Join(lines, "\n")
}