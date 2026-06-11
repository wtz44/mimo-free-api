package adapter

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// OpenAIChatRequest 是 OpenAI 格式的请求
type OpenAIChatRequest struct {
	Model       string          `json:"model"`
	Messages    []OpenAIMessage `json:"messages"`
	Stream      bool            `json:"stream"`
	Temperature *float64        `json:"temperature,omitempty"`
	MaxTokens   *int            `json:"max_tokens,omitempty"`
	Tools       []OpenAITool    `json:"tools,omitempty"`
	ToolChoice  interface{}     `json:"tool_choice,omitempty"`
}

// OpenAITool 是 OpenAI 格式的工具定义
type OpenAITool struct {
	Type     string             `json:"type"`
	Function OpenAIToolFunction `json:"function"`
}

// OpenAIToolFunction 是工具函数定义
type OpenAIToolFunction struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Parameters  interface{} `json:"parameters,omitempty"`
}

// OpenAIMessage 是 OpenAI 格式的消息
// OpenAIMessage 是 OpenAI 格式的消息
type OpenAIMessage struct {
	Role       string           `json:"role"`
	Content    interface{}      `json:"content"` // string 或 []ContentPart
	ToolCalls  []OpenAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

// OpenAIToolCall 是 OpenAI 格式的工具调用
type OpenAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function OpenAIToolCallFunc `json:"function"`
}

// OpenAIToolCallFunc 是工具调用函数
type OpenAIToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// OpenAIChatResponse 是 OpenAI 格式的响应
type OpenAIChatResponse struct {
	ID      string           `json:"id"`
	Object  string           `json:"object"`
	Created int64            `json:"created"`
	Model   string           `json:"model"`
	Choices []OpenAIChoice   `json:"choices"`
	Usage   *OpenAIUsage     `json:"usage,omitempty"`
}

// OpenAIChoice 是 OpenAI 格式的选项
type OpenAIChoice struct {
	Index        int            `json:"index"`
	Message      *OpenAIMessage `json:"message,omitempty"`
	Delta        *OpenAIDelta   `json:"delta,omitempty"`
	FinishReason *string        `json:"finish_reason,omitempty"`
}

// OpenAIDelta 是流式增量
type OpenAIDelta struct {
	Role      string           `json:"role,omitempty"`
	Content   string           `json:"content,omitempty"`
	ToolCalls []OpenAIToolCall `json:"tool_calls,omitempty"`
}

// OpenAIUsage 是 token 使用量
type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// OpenAIStreamChunk 是流式响应块
type OpenAIStreamChunk struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []OpenAIChoice `json:"choices"`
}


func MakeOpenAIStreamChunk(model, content string, finish bool) []byte {
	now := time.Now().Unix()
	chunk := OpenAIStreamChunk{
		ID:      fmt.Sprintf("chatcmpl-%s", uuid.New().String()[:8]),
		Object:  "chat.completion.chunk",
		Created: now,
		Model:   model,
		Choices: []OpenAIChoice{
			{
				Index: 0,
				Delta: &OpenAIDelta{},
			},
		},
	}

	if finish {
		fr := "stop"
		chunk.Choices[0].FinishReason = &fr
	} else {
		chunk.Choices[0].Delta.Content = content
	}

	data, _ := json.Marshal(chunk)
	return data
}

// MakeOpenAIResponse 创建 OpenAI 非流式响应
func MakeOpenAIResponse(model, content string) []byte {
	now := time.Now().Unix()
	fr := "stop"
	resp := OpenAIChatResponse{
		ID:      fmt.Sprintf("chatcmpl-%s", uuid.New().String()[:8]),
		Object:  "chat.completion",
		Created: now,
		Model:   model,
		Choices: []OpenAIChoice{
			{
				Index: 0,
				Message: &OpenAIMessage{
					Role:    "assistant",
					Content: content,
				},
				FinishReason: &fr,
			},
		},
	}
	data, _ := json.Marshal(resp)
	return data
}

// OpenAIModelsResponse 是 /v1/models 的响应
type OpenAIModelsResponse struct {
	Object string      `json:"object"`
	Data   interface{} `json:"data"`
}

// MakeOpenAIStreamToolCallChunk 创建流式工具调用块
func MakeOpenAIStreamToolCallChunk(model string, toolCalls []OpenAIToolCall, finish bool) []byte {
	now := time.Now().Unix()
	chunk := OpenAIStreamChunk{
		ID:      fmt.Sprintf("chatcmpl-%s", uuid.New().String()[:8]),
		Object:  "chat.completion.chunk",
		Created: now,
		Model:   model,
		Choices: []OpenAIChoice{
			{
				Index: 0,
				Delta: &OpenAIDelta{ToolCalls: toolCalls},
			},
		},
	}
	if finish {
		fr := "tool_calls"
		chunk.Choices[0].FinishReason = &fr
	}
	data, _ := json.Marshal(chunk)
	return data
}

// MakeOpenAIToolCallResponse 创建 OpenAI 非流式工具调用响应
func MakeOpenAIToolCallResponse(model string, toolCalls []OpenAIToolCall) []byte {
	now := time.Now().Unix()
	fr := "tool_calls"
	resp := OpenAIChatResponse{
		ID:      fmt.Sprintf("chatcmpl-%s", uuid.New().String()[:8]),
		Object:  "chat.completion",
		Created: now,
		Model:   model,
		Choices: []OpenAIChoice{
			{
				Index: 0,
				Message: &OpenAIMessage{
					Role:      "assistant",
					Content:   "",
					ToolCalls: toolCalls,
				},
				FinishReason: &fr,
			},
		},
	}
	data, _ := json.Marshal(resp)
	return data
}
