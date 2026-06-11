package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/wtz44/mimo-gateway/internal/adapter"
	"github.com/wtz44/mimo-gateway/internal/mimo"
	"github.com/wtz44/mimo-gateway/internal/pool"
	"github.com/wtz44/mimo-gateway/internal/router"
	"github.com/wtz44/mimo-gateway/internal/stats"
	"github.com/wtz44/mimo-gateway/internal/toolcall"
)

type ChatHandler struct {
	pool *pool.Pool
}

func NewChatHandler(p *pool.Pool) *ChatHandler {
	return &ChatHandler{pool: p}
}

// parsedEvent 解析后的 SSE 事件
type parsedEvent struct {
	Event string
	Data  string
}

// usageData MiMo usage 事件结构
type usageData struct {
	PromptTokens     int `json:"promptTokens"`
	CompletionTokens int `json:"completionTokens"`
	TotalTokens      int `json:"totalTokens"`
	NativeUsage      *nativeUsage `json:"nativeUsage,omitempty"`
}

type nativeUsage struct {
	PromptTokens     int              `json:"prompt_tokens"`
	CompletionTokens int              `json:"completion_tokens"`
	TotalTokens      int              `json:"total_tokens"`
	PromptDetails    *promptDetails   `json:"prompt_tokens_details,omitempty"`
	CompletionDetails *completionDetails `json:"completion_tokens_details,omitempty"`
}

type promptDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

type completionDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

// dialogIdData dialogId 事件结构
type dialogIdData struct {
	Content string `json:"content"`
}

func (h *ChatHandler) Handle(w http.ResponseWriter, r *http.Request) {
	var req adapter.OpenAIChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// 如果有 tools 定义，注入工具调用指令到 system prompt
	if len(req.Tools) > 0 {
		toolPrompt := toolcall.BuildToolPrompt(req.Tools)
		if toolPrompt != "" {
			req.Messages = toolcall.InjectToolPrompt(req.Messages, toolPrompt)
			log.Printf("[tools] injected %d tool definitions into prompt", len(req.Tools))
		}
	}

	routeResult := router.RouteModel(req.Model, toMiMoMessages(req.Messages))
	log.Printf("[route] model=%s reason=%s", routeResult.Model, routeResult.Reason)

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	if !h.pool.HasAccounts() {
		writeError(w, http.StatusServiceUnavailable, "no accounts configured")
		return
	}

	h.handleWebChat(ctx, w, &req, routeResult.Model, req.Stream)
}

// handleWebChat 使用网页端反代
func (h *ChatHandler) handleWebChat(ctx context.Context, w http.ResponseWriter, req *adapter.OpenAIChatRequest, model string, stream bool) {
	client, err := h.pool.Next()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	query := buildQuery(req.Messages)

	// 对话持久化：复用 conversationId
	clientConvID := extractConversationID(req.Messages)
	convStore := stats.GetConvStore()
	mimoConvID := ""

	// 如果有 clientConvID，尝试复用已存储的 mimoConvID
	if clientConvID != "" {
		if stored, ok := convStore.GetMimoConvID(clientConvID); ok {
			mimoConvID = stored
		}
	}
	if mimoConvID == "" {
		mimoConvID = strings.ReplaceAll(uuid.New().String(), "-", "")
	}

	// 并发计数
	parentID := ""
	if clientConvID != "" {
		parentID = convStore.GetParentID(clientConvID)
	}

	// 并发计数
	stats.Get().IncrConcurrency()
	defer stats.Get().DecrConcurrency()

	body, err := client.Chat(ctx, query, model, mimoConvID, parentID, false)
	if err != nil {
		log.Printf("[error] web chat: %v", err)
		writeError(w, http.StatusBadGateway, fmt.Sprintf("mimo error: %v", err))
		return
	}

	// 解析所有事件
	events := make(chan mimo.WebSSEEvent, 64)
	go func() {
		defer close(events)
		mimo.ParseWebSSE(ctx, body, events)
	}()

	// 分发事件：提取 usage 和 dialogId，转发 message
	usageChan := make(chan *usageData, 1)
	dialogChan := make(chan string, 1)
	lastMsgIDChan := make(chan string, 1)
	msgChan := make(chan mimo.WebSSEEvent, 64)

	go func() {
		defer close(msgChan)
		lastMsgID := ""
		for ev := range events {
			if ev.Event == "message" && ev.ID != "" {
				lastMsgID = ev.ID
			}
			switch ev.Event {
			case "usage":
				var u usageData
				if json.Unmarshal([]byte(ev.Data), &u) == nil {
					usageChan <- &u
				}
			case "dialogId":
				var d dialogIdData
				if json.Unmarshal([]byte(ev.Data), &d) == nil {
					dialogChan <- d.Content
				}
			case "message":
				msgChan <- ev
			}
		}
		close(usageChan)
		close(dialogChan)
		lastMsgIDChan <- lastMsgID
		close(lastMsgIDChan)
	}()

	if stream {
		h.streamWebToOpenAI(w, model, msgChan, len(req.Tools) > 0)
	} else {
		h.nonStreamWebToOpenAI(w, model, msgChan)
	}

	// 后处理：记录 usage，保存对话映射
	if u := <-usageChan; u != nil {
		cached := 0
		reasoning := 0
		if u.NativeUsage != nil {
			if u.NativeUsage.PromptDetails != nil {
				cached = u.NativeUsage.PromptDetails.CachedTokens
			}
			if u.NativeUsage.CompletionDetails != nil {
				reasoning = u.NativeUsage.CompletionDetails.ReasoningTokens
			}
		}
		stats.Get().Record(model, u.PromptTokens, u.CompletionTokens, cached, reasoning, u.TotalTokens)
		log.Printf("[usage] model=%s prompt=%d completion=%d cached=%d reasoning=%d",
			model, u.PromptTokens, u.CompletionTokens, cached, reasoning)
	}

	// 保存我们自己的 conversationId（32位hex），供后续请求复用
	if clientConvID != "" && mimoConvID != "" {
		convStore.SetMimoConvID(clientConvID, mimoConvID)
	}

	// 保存最后一条 AI 消息 ID 作为下次请求的 parentId
	if lastMsgID := <-lastMsgIDChan; lastMsgID != "" && clientConvID != "" {
		convStore.SetParentID(clientConvID, lastMsgID)
	}

	// 保存对话到 MiMo 官网（维持服务端上下文的关键）
	if mimoConvID != "" {
		go client.SaveConversation(context.Background(), mimoConvID, query)
	}
}

func (h *ChatHandler) streamWebToOpenAI(w http.ResponseWriter, model string, events <-chan mimo.WebSSEEvent, hasTools bool) {
	flusher := w.(http.Flusher)
	inThinking := false

	var buffered strings.Builder

	writeChunk := func(content string, finish bool) {
		chunk := adapter.MakeOpenAIStreamChunk(model, content, finish)
		fmt.Fprintf(w, "data: %s\n\n", chunk)
		flusher.Flush()
	}

	for event := range events {
		if event.Event != "message" {
			continue
		}
		var msg struct {
			Type    string `json:"type"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal([]byte(event.Data), &msg); err != nil {
			continue
		}
		if msg.Type != "text" || msg.Content == "" {
			continue
		}
		c := strings.ReplaceAll(msg.Content, "\u0000", "")
		c, inThinking = filterThinkingChunk(c, inThinking)
		if c == "" {
			continue
		}
		if hasTools {
			buffered.WriteString(c)
		} else {
			writeChunk(c, false)
		}
	}

	if hasTools {
		finalText := strings.TrimSpace(buffered.String())
		log.Printf("[tools] raw output (len=%d): %q", len(finalText), finalText[:min(len(finalText), 500)])
		if toolcall.HasToolCallSyntax(finalText) {
			calls := toolcall.ParseToolCallsFromText(finalText)
			log.Printf("[tools] parsed %d calls from text", len(calls))
			for i, c := range calls {
				log.Printf("[tools] call[%d]: name=%s input=%v", i, c.Name, c.Input)
			}
			if len(calls) > 0 {
				toolCalls := toolcall.ConvertToolCallsToOpenAI(calls)
				log.Printf("[tools] detected %d tool calls in stream", len(toolCalls))
				toolChunk := adapter.MakeOpenAIStreamToolCallChunk(model, toolCalls, true)
				fmt.Fprintf(w, "data: %s\n\n", toolChunk)
				fmt.Fprintf(w, "data: [DONE]\n\n")
				flusher.Flush()
				return
			}
		}
		writeChunk(finalText, false)
	}

	writeChunk("", true)
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

// filterThinkingChunk 状态机方式过滤 thinking 内容
func filterThinkingChunk(content string, inThinking bool) (string, bool) {
	var result strings.Builder

	for len(content) > 0 {
		if inThinking {
			end := strings.Index(content, "</think>")
			if end == -1 {
				return "", true
			}
			content = content[end+8:]
			inThinking = false
			continue
		}

		start := strings.Index(content, "<think>")
		if start == -1 {
			result.WriteString(content)
			break
		}

		result.WriteString(content[:start])
		content = content[start+7:]
		inThinking = true
	}

	return result.String(), inThinking
}

func (h *ChatHandler) nonStreamWebToOpenAI(w http.ResponseWriter, model string, events <-chan mimo.WebSSEEvent) {
	var content strings.Builder
	inThinking := false

	for event := range events {
		if event.Event != "message" {
			continue
		}
		var msg struct {
			Type    string `json:"type"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal([]byte(event.Data), &msg); err != nil {
			continue
		}
		if msg.Type == "text" && msg.Content != "" {
			c := strings.ReplaceAll(msg.Content, "\u0000", "")
			c, inThinking = filterThinkingChunk(c, inThinking)
			content.WriteString(c)
		}
	}


	finalText := strings.TrimSpace(content.String())

	// 检测是否包含工具调用
	log.Printf("[tools] non-stream raw output (len=%d): %q", len(finalText), finalText[:min(len(finalText), 500)])
	if toolcall.HasToolCallSyntax(finalText) {
		calls := toolcall.ParseToolCallsFromText(finalText)
		log.Printf("[tools] non-stream parsed %d calls", len(calls))
		for i, c := range calls {
			log.Printf("[tools] call[%d]: name=%s input=%v", i, c.Name, c.Input)
		}
		if len(calls) > 0 {
			toolCalls := toolcall.ConvertToolCallsToOpenAI(calls)
			log.Printf("[tools] detected %d tool calls in response", len(toolCalls))
			resp := adapter.MakeOpenAIToolCallResponse(model, toolCalls)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(resp)
			return
		}
	}

	resp := adapter.MakeOpenAIResponse(model, finalText)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(resp)
}

// buildQuery 构建查询文本
// 如果只有一条消息，直接返回
// 如果有多条消息，把历史拼进 query（MiMo 不维持服务端上下文）
func buildQuery(msgs []adapter.OpenAIMessage) string {
	if len(msgs) <= 1 {
		return extractQuery(msgs)
	}

	// 多条消息：拼接历史
	var parts []string
	for _, msg := range msgs {
		content := extractContent(msg)
		if content == "" {
			continue
		}
		switch msg.Role {
		case "user":
			parts = append(parts, "User: "+content)
		case "assistant":
			parts = append(parts, "Assistant: "+content)
		case "system":
			parts = append(parts, "System: "+content)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	// 最后一条是用户消息，前面的作为历史
	return strings.Join(parts[:len(parts)-1], "\n") + "\n\n(以上是历史对话记录，请直接针对最后的问题作答)\n\n" + parts[len(parts)-1]
}

// extractContent 提取消息文本内容，剥离 conv:xxx: 前缀
func extractContent(msg adapter.OpenAIMessage) string {
	s, ok := msg.Content.(string)
	if !ok {
		return ""
	}
	if strings.HasPrefix(s, "conv:") {
		idx := strings.Index(s[5:], ":")
		if idx >= 0 {
			return strings.TrimSpace(s[5+idx+1:])
		}
	}
	return s
}

func extractQuery(msgs []adapter.OpenAIMessage) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			switch v := msgs[i].Content.(type) {
			case string:
				// 剥离 conv:xxx: 前缀
				if strings.HasPrefix(v, "conv:") {
					idx := strings.Index(v[5:], ":")
					if idx >= 0 {
						return strings.TrimSpace(v[5+idx+1:])
					}
				}
				return v
			}
		}
	}
	return ""
}

// extractConversationID 从消息中提取对话 ID
// 支持格式: "conv:xxx: actual message"
func extractConversationID(msgs []adapter.OpenAIMessage) string {
	// 检查最后一条用户消息
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			if s, ok := msgs[i].Content.(string); ok {
				if strings.HasPrefix(s, "conv:") {
					// 格式: "conv:convID: actual message"
					rest := s[5:] // 去掉 "conv:"
					idx := strings.Index(rest, ":")
					if idx >= 0 {
						return strings.TrimSpace(rest[:idx])
					}
					// 格式: "conv:convID" (无冒号后缀)
					return strings.TrimSpace(rest)
				}
			}
		}
	}
	return ""
}

func toMiMoMessages(msgs []adapter.OpenAIMessage) []mimo.Message {
	result := make([]mimo.Message, len(msgs))
	for i, m := range msgs {
		result[i] = mimo.Message{Role: m.Role, Content: m.Content}
	}
	return result
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"message": msg,
			"type":    "invalid_request_error",
			"code":    status,
		},
	})
}

func ModelsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"object": "list",
		"data":   router.SupportedModels(),
	})
}

// MessagesHandler Anthropic 格式
type MessagesHandler struct {
	pool *pool.Pool
}

func NewMessagesHandler(p *pool.Pool) *MessagesHandler {
	return &MessagesHandler{pool: p}
}

func (h *MessagesHandler) Handle(w http.ResponseWriter, r *http.Request) {
	var req adapter.AnthropicRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}

	routeResult := router.RouteModel(req.Model, nil)

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	if !h.pool.HasAccounts() {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "no accounts configured"})
		return
	}

	client, err := h.pool.Next()
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	query := ""
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			if s, ok := req.Messages[i].Content.(string); ok {
				query = s
			}
			break
		}
	}

	convID := strings.ReplaceAll(uuid.New().String(), "-", "")

	stats.Get().IncrConcurrency()
	defer stats.Get().DecrConcurrency()

	body, err := client.Chat(ctx, query, routeResult.Model, convID, "", false)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	events := make(chan mimo.WebSSEEvent, 64)
	go func() {
		defer close(events)
		mimo.ParseWebSSE(ctx, body, events)
	}()

	// 分发事件
	usageChan := make(chan *usageData, 1)
	msgChan := make(chan mimo.WebSSEEvent, 64)

	go func() {
		defer close(msgChan)
		for ev := range events {
			switch ev.Event {
			case "usage":
				var u usageData
				if json.Unmarshal([]byte(ev.Data), &u) == nil {
					usageChan <- &u
				}
			case "message":
				msgChan <- ev
			}
		}
		close(usageChan)
	}()

	if req.Stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		inThinking := false
		for event := range msgChan {
			if event.Event != "message" {
				continue
			}
			var msg struct {
				Type    string `json:"type"`
				Content string `json:"content"`
			}
			if err := json.Unmarshal([]byte(event.Data), &msg); err != nil {
				continue
			}
			if msg.Type == "text" && msg.Content != "" {
				c := strings.ReplaceAll(msg.Content, "\u0000", "")
				c, inThinking = filterThinkingChunk(c, inThinking)
				if c != "" {
					delta := adapter.AnthropicTextDelta{Type: "text_delta", Text: c}
					fmt.Fprintf(w, "event: content_block_delta\ndata: %s\n\n", adapter.MakeAnthropicStreamEvent("content_block_delta", delta))
					flusher.Flush()
				}
			}
		}
		fmt.Fprintf(w, "event: message_stop\ndata: %s\n\n", adapter.MakeAnthropicStreamEvent("message_stop", nil))
		flusher.Flush()
	} else {
		var content strings.Builder
		inThinking := false
		for event := range msgChan {
			if event.Event == "message" {
				var msg struct {
					Type    string `json:"type"`
					Content string `json:"content"`
				}
				if err := json.Unmarshal([]byte(event.Data), &msg); err == nil && msg.Type == "text" {
					c := strings.ReplaceAll(msg.Content, "\u0000", "")
					c, inThinking = filterThinkingChunk(c, inThinking)
					content.WriteString(c)
				}
			}
		}
		resp := adapter.MakeAnthropicResponse(routeResult.Model, strings.TrimSpace(content.String()))
		w.Header().Set("Content-Type", "application/json")
		w.Write(resp)
	}

	// 记录 usage
	if u := <-usageChan; u != nil {
		cached := 0
		reasoning := 0
		if u.NativeUsage != nil {
			if u.NativeUsage.PromptDetails != nil {
				cached = u.NativeUsage.PromptDetails.CachedTokens
			}
			if u.NativeUsage.CompletionDetails != nil {
				reasoning = u.NativeUsage.CompletionDetails.ReasoningTokens
			}
		}
		stats.Get().Record(routeResult.Model, u.PromptTokens, u.CompletionTokens, cached, reasoning, u.TotalTokens)
	}
}
