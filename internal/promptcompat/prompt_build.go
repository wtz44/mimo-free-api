package promptcompat

import (
	"fmt"
	"log"
	"unicode/utf8"

	"github.com/wtz44/mimo-gateway/internal/adapter"
	"github.com/wtz44/mimo-gateway/internal/prompt"
)

const (
	// MaxToolResultChars truncates individual tool results to this length.
	// Web pages and file reads can be 10K+ chars; keeping 2K is enough for
	// the model to understand the result without bloating the prompt.
	MaxToolResultChars = 2000

	// MaxHistoryMessages is the maximum number of non-system messages to keep.
	// Older messages are dropped but the first user message (original question)
	// is always preserved so the model remembers what it's answering.
	MaxHistoryMessages = 20
)

// BuildOpenAIPrompt is the main entry point for OpenAI-format requests.
// It normalizes messages, injects tool prompts, trims context, and builds the final prompt string.
// This is fully stateless — no conversation caching.
func BuildOpenAIPrompt(messages []adapter.OpenAIMessage, tools []adapter.OpenAITool) string {
	// Step 1: Normalize all messages
	normalized := NormalizeOpenAIMessages(messages)

	// Step 2: Inject tool schemas + instructions into system message
	normalized, _ = InjectToolPrompt(normalized, tools)

	// Step 3: Trim context for long conversations
	normalized = TrimContext(normalized)

	// Step 4: Build final prompt string with special tokens
	return prompt.BuildPrompt(normalized)
}

// BuildAnthropicPrompt is the main entry point for Anthropic-format requests.
// It normalizes messages, injects tool prompts, trims context, and builds the final prompt string.
// This is fully stateless — no conversation caching.
func BuildAnthropicPrompt(messages []adapter.AnthropicMessage, system string, tools []adapter.AnthropicTool) string {
	// Step 1: Normalize all messages
	normalized := NormalizeAnthropicMessages(messages, system)

	// Step 2: Convert Anthropic tools to OpenAI format and inject
	if len(tools) > 0 {
		openaiTools := adapter.ConvertAnthropicToolsToOpenAI(tools)
		normalized, _ = InjectToolPrompt(normalized, openaiTools)
	}

	// Step 3: Trim context for long conversations
	normalized = TrimContext(normalized)

	// Step 4: Build final prompt string with special tokens
	return prompt.BuildPrompt(normalized)
}

// TrimContext reduces prompt size for long conversations:
//  1. Truncates individual tool results to MaxToolResultChars
//  2. Limits history to MaxHistoryMessages non-system messages
//  3. Always preserves system messages and the first user message
func TrimContext(messages []prompt.Message) []prompt.Message {
	if len(messages) == 0 {
		return messages
	}

	// Phase 1: Truncate large tool results and assistant messages
	origChars := 0
	for _, msg := range messages {
		origChars += utf8.RuneCountInString(msg.Content)
	}
	truncated := 0
	for i, msg := range messages {
		if msg.Role == "tool" && utf8.RuneCountInString(msg.Content) > MaxToolResultChars {
			messages[i].Content = truncateRunes(msg.Content, MaxToolResultChars) +
				fmt.Sprintf("\n[truncated — original was %d chars]", utf8.RuneCountInString(msg.Content))
			truncated++
		}
		// Also truncate very long user messages that might contain tool_result blocks
		if msg.Role == "user" && utf8.RuneCountInString(msg.Content) > MaxToolResultChars*2 {
			messages[i].Content = truncateRunes(msg.Content, MaxToolResultChars*2) +
				fmt.Sprintf("\n[truncated — original was %d chars]", utf8.RuneCountInString(msg.Content))
			truncated++
		}
		// Truncate large assistant messages (e.g. containing full file content in tool_use blocks)
		if msg.Role == "assistant" && utf8.RuneCountInString(msg.Content) > MaxToolResultChars*2 {
			messages[i].Content = truncateRunes(msg.Content, MaxToolResultChars*2) +
				fmt.Sprintf("\n[truncated — original was %d chars]", utf8.RuneCountInString(msg.Content))
			truncated++
		}
	}
	afterTruncChars := 0
	for _, msg := range messages {
		afterTruncChars += utf8.RuneCountInString(msg.Content)
	}
	if truncated > 0 || len(messages) > MaxHistoryMessages {
		log.Printf("[trim] messages=%d chars=%d → truncated %d msgs → chars=%d", len(messages), origChars, truncated, afterTruncChars)
	}

	// Phase 2: Limit message count
	// Separate system messages from conversation messages
	var systemMsgs []prompt.Message
	var convMsgs []prompt.Message
	for _, msg := range messages {
		if msg.Role == "system" {
			systemMsgs = append(systemMsgs, msg)
		} else {
			convMsgs = append(convMsgs, msg)
		}
	}

	if len(convMsgs) <= MaxHistoryMessages {
		return messages // No trimming needed
	}

	dropped := len(convMsgs) - MaxHistoryMessages
	log.Printf("[trim] dropping %d old messages (%d → %d)", dropped, len(convMsgs), MaxHistoryMessages)

	// Keep the first user message (original question) + last (MaxHistoryMessages-1) messages
	var trimmed []prompt.Message
	firstUserIdx := -1
	for i, msg := range convMsgs {
		if msg.Role == "user" {
			firstUserIdx = i
			break
		}
	}

	keepFrom := len(convMsgs) - MaxHistoryMessages + 1
	if firstUserIdx >= 0 && firstUserIdx < keepFrom {
		// First user message would be dropped — keep it
		trimmed = append(trimmed, convMsgs[firstUserIdx])
		trimmed = append(trimmed, convMsgs[keepFrom:]...)
	} else {
		trimmed = convMsgs[keepFrom:]
	}

	// Reassemble: system + trimmed conversation
	result := make([]prompt.Message, 0, len(systemMsgs)+len(trimmed))
	result = append(result, systemMsgs...)
	result = append(result, trimmed...)
	return result
}

// truncateRunes truncates a string to max runes, respecting UTF-8 boundaries.
func truncateRunes(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	i := 0
	for pos := range s {
		if i == max {
			return s[:pos]
		}
		i++
	}
	return s
}
