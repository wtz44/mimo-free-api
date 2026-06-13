package handler

import (
	"fmt"
	"strings"

	"github.com/wtz44/mimo-gateway/internal/adapter"
)

// buildToolPrompt builds a concise tool description string from OpenAI tool definitions.
// Injected into the query so MiMo knows what tools are available.
func buildToolPrompt(tools []adapter.OpenAITool) string {
	var sb strings.Builder
	sb.WriteString("You have the following tools available. ")
	sb.WriteString("Use them when the user's request requires actions like reading/writing files, running commands, asking questions, or web searches.\n\n")
	for _, tool := range tools {
		name := tool.Function.Name
		desc := tool.Function.Description
		sb.WriteString(fmt.Sprintf("- %s: %s\n", name, desc))
		if tool.Function.Parameters != nil {
			if params, ok := tool.Function.Parameters.(map[string]interface{}); ok {
				if props, ok := params["properties"].(map[string]interface{}); ok && len(props) > 0 {
					sb.WriteString("  Parameters: ")
					first := true
					for pname := range props {
						if !first {
							sb.WriteString(", ")
						}
						sb.WriteString(pname)
						first = false
					}
					sb.WriteString("\n")
				}
			}
		}
	}
	return sb.String()
}
