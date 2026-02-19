package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jholhewres/devclaw/pkg/devclaw/copilot"
)

const version = "1.0.0"

// openAIChatRequest is the standard OpenAI chat completions request format.
type openAIChatRequest struct {
	Model    string                 `json:"model"`
	Messages []openAIChatMessage     `json:"messages"`
	Stream   bool                   `json:"stream"`
	Tools    []openAIToolDef         `json:"tools,omitempty"`
}

type openAIChatMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"` // string or array of parts
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

type openAIToolDef struct {
	Type     string          `json:"type"`
	Function openAIFunctionDef `json:"function"`
}

type openAIFunctionDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type openAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function openAIFunctionCall  `json:"function"`
}

type openAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// openAIChatResponse is the standard OpenAI chat completions response.
type openAIChatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message *struct {
			Role       string           `json:"role"`
			Content    string           `json:"content"`
			ToolCalls  []openAIToolCall  `json:"tool_calls,omitempty"`
		} `json:"message,omitempty"`
		Delta        *struct {
			Role       string `json:"role,omitempty"`
			Content    string `json:"content,omitempty"`
			ToolCalls  []openAIStreamToolCall `json:"tool_calls,omitempty"`
		} `json:"delta,omitempty"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage,omitempty"`
}

type openAIStreamToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function,omitempty"`
}

// errorResponse is the consistent error format.
type errorResponse struct {
	Error struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error"`
}

func (g *Gateway) writeError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	_ = enc.Encode(errorResponse{Error: struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	}{Message: msg, Code: code}})
}

func (g *Gateway) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	_ = enc.Encode(v)
}

// handleHealth implements GET /health
func (g *Gateway) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		g.writeError(w, "method not allowed", 405)
		return
	}
	uptime := time.Since(g.startedAt).Round(time.Second).String()
	if uptime == "0s" {
		uptime = "<1s"
	}
	channelsMap := make(map[string]string)
	if g.assistant != nil {
		for name, st := range g.assistant.ChannelManager().HealthAll() {
			if st.Connected {
				channelsMap[name] = "connected"
			} else {
				channelsMap[name] = "disconnected"
			}
		}
	}
	g.writeJSON(w, 200, map[string]any{
		"status":   "ok",
		"version":  version,
		"uptime":   uptime,
		"channels": channelsMap,
	})
}

// handleChatCompletions implements POST /v1/chat/completions (OpenAI-compatible)
func (g *Gateway) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		g.writeError(w, "method not allowed", 405)
		return
	}
	// Limit request body to 2MB to prevent OOM from oversized payloads.
	body, err := io.ReadAll(io.LimitReader(r.Body, 2*1024*1024))
	if err != nil {
		g.writeError(w, "failed to read body", 400)
		return
	}
	var req openAIChatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		g.writeError(w, "invalid request body", 400)
		return
	}
	chatID := r.Header.Get("X-Session-ID")
	if chatID == "" {
		chatID = "default"
	}
	resolved := g.assistant.WorkspaceManager().Resolve("api", chatID, "api-client", false)
	session := resolved.Session
	workspace := resolved.Workspace
	model := req.Model
	if model == "" {
		model = g.assistant.Config().Model
		if workspace.Model != "" {
			model = workspace.Model
		}
	}
	messages, err := g.openAIToCopilotMessages(req.Messages)
	if err != nil {
		g.writeError(w, "invalid messages: "+err.Error(), 400)
		return
	}
	if len(messages) == 0 {
		g.writeError(w, "messages required", 400)
		return
	}
	lastUserIdx := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			lastUserIdx = i
			break
		}
	}
	if lastUserIdx < 0 {
		g.writeError(w, "last message must be from user", 400)
		return
	}
	lastUser := g.contentToString(messages[lastUserIdx].Content)
	systemPrompt := ""
	var history []copilot.ConversationEntry
	for i, m := range messages {
		if i >= lastUserIdx {
			break
		}
		if m.Role == "system" {
			systemPrompt = g.contentToString(m.Content)
		} else if m.Role == "user" && m.Content != nil {
			history = append(history, copilot.ConversationEntry{UserMessage: g.contentToString(m.Content)})
		} else if m.Role == "assistant" && m.Content != nil {
			if len(history) > 0 {
				history[len(history)-1].AssistantResponse = g.contentToString(m.Content)
			}
		}
	}
	prompt := g.assistant.ComposePrompt(session, lastUser)
	if systemPrompt != "" {
		prompt = systemPrompt + "\n\n" + prompt
	}
	// Propagate caller context via context.Context (goroutine-safe).
	reqCtx := copilot.ContextWithCaller(r.Context(), copilot.AccessOwner, "api-client")
	reqCtx = copilot.ContextWithSession(reqCtx, session.ID)
	if req.Stream {
		g.handleChatStream(w, r.WithContext(reqCtx), session, prompt, history, lastUser, model)
		return
	}
	resp := g.assistant.ExecuteAgent(reqCtx, prompt, session, lastUser)
	session.AddMessage(lastUser, resp)
	g.writeOpenAINonStream(w, model, resp)
}

func (g *Gateway) openAIToCopilotMessages(msgs []openAIChatMessage) ([]copilotChatMessage, error) {
	out := make([]copilotChatMessage, len(msgs))
	for i, m := range msgs {
		out[i] = copilotChatMessage{Role: m.Role, Content: m.Content}
	}
	return out, nil
}

type copilotChatMessage struct {
	Role    string
	Content json.RawMessage
}

func (g *Gateway) contentToString(c json.RawMessage) string {
	if len(c) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(c, &s); err == nil {
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(c, &parts); err == nil {
		var b strings.Builder
		for _, p := range parts {
			if p.Type == "text" {
				b.WriteString(p.Text)
			}
		}
		return b.String()
	}
	return string(c)
}

func (g *Gateway) handleChatStream(w http.ResponseWriter, r *http.Request, session *copilot.Session, systemPrompt string, history []copilot.ConversationEntry, userMsg, model string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(200)
	flusher, ok := w.(http.Flusher)
	if !ok {
		flusher = nil
	}
	var contentBuilder strings.Builder
	agent := copilot.NewAgentRunWithConfig(g.assistant.LLMClient(), g.assistant.ToolExecutor(), g.assistant.Config().Agent, g.logger)
	agent.SetModelOverride(model)
	agent.SetStreamCallback(func(chunk string) {
		contentBuilder.WriteString(chunk)
		chunkResp := openAIChatResponse{
			ID:      "chatcmpl-devclaw",
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   model,
			Choices: []struct {
				Index   int `json:"index"`
				Message *struct {
					Role       string          `json:"role"`
					Content    string          `json:"content"`
					ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
				} `json:"message,omitempty"`
				Delta        *struct {
					Role       string `json:"role,omitempty"`
					Content    string `json:"content,omitempty"`
					ToolCalls  []openAIStreamToolCall `json:"tool_calls,omitempty"`
				} `json:"delta,omitempty"`
				FinishReason *string `json:"finish_reason"`
			}{{
				Index: 0,
				Delta: &struct {
					Role       string `json:"role,omitempty"`
					Content    string `json:"content,omitempty"`
					ToolCalls  []openAIStreamToolCall `json:"tool_calls,omitempty"`
				}{Content: chunk},
			}},
		}
		fmt.Fprintf(w, "data: %s\n\n", mustJSON(chunkResp))
		if flusher != nil {
			flusher.Flush()
		}
	})
	if g.assistant.UsageTracker() != nil {
		agent.SetUsageRecorder(func(m string, u copilot.LLMUsage) {
			g.assistant.UsageTracker().Record(session.ID, m, u)
		})
	}
	resp, _, err := agent.RunWithUsage(r.Context(), systemPrompt, history, userMsg)
	if err != nil {
		g.logger.Error("chat stream failed", "error", err)
		errChunk := map[string]any{"error": map[string]string{"message": err.Error()}}
		fmt.Fprintf(w, "data: %s\n\n", mustJSON(errChunk))
		return
	}
	session.AddMessage(userMsg, resp)
	finishChunk := openAIChatResponse{
		ID:      "chatcmpl-devclaw",
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []struct {
			Index   int `json:"index"`
			Message *struct {
				Role       string          `json:"role"`
				Content    string          `json:"content"`
				ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
			} `json:"message,omitempty"`
			Delta        *struct {
				Role       string `json:"role,omitempty"`
				Content    string `json:"content,omitempty"`
				ToolCalls  []openAIStreamToolCall `json:"tool_calls,omitempty"`
			} `json:"delta,omitempty"`
			FinishReason *string `json:"finish_reason"`
		}{{Index: 0, FinishReason: strPtr("stop")}},
	}
	fmt.Fprintf(w, "data: %s\n\n", mustJSON(finishChunk))
	fmt.Fprintf(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
	_ = contentBuilder
}

func strPtr(s string) *string { return &s }

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func (g *Gateway) writeOpenAINonStream(w http.ResponseWriter, model, content string) {
	resp := openAIChatResponse{
		ID:      "chatcmpl-devclaw",
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []struct {
			Index   int `json:"index"`
			Message *struct {
				Role       string          `json:"role"`
				Content    string          `json:"content"`
				ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
			} `json:"message,omitempty"`
			Delta        *struct {
				Role       string `json:"role,omitempty"`
				Content    string `json:"content,omitempty"`
				ToolCalls  []openAIStreamToolCall `json:"tool_calls,omitempty"`
			} `json:"delta,omitempty"`
			FinishReason *string `json:"finish_reason"`
		}{{
			Index: 0,
			Message: &struct {
				Role       string          `json:"role"`
				Content    string          `json:"content"`
				ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
			}{Role: "assistant", Content: content},
		}},
	}
	g.writeJSON(w, 200, resp)
}

// handleListSessions implements GET /api/sessions
func (g *Gateway) handleListSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		g.writeError(w, "method not allowed", 405)
		return
	}
	sessions := g.assistant.WorkspaceManager().ListAllSessions()
	g.writeJSON(w, 200, map[string]any{"sessions": sessions})
}

// handleCompactSession implements POST /api/sessions/:id/compact
func (g *Gateway) handleCompactSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		g.writeError(w, "method not allowed", 405)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	path = strings.TrimSuffix(path, "/compact")
	if path == "" {
		g.writeError(w, "session id required", 400)
		return
	}
	session, _ := g.assistant.WorkspaceManager().GetSessionByID(path)
	if session == nil {
		g.writeError(w, "session not found", 404)
		return
	}
	oldLen, newLen := g.assistant.ForceCompactSession(session)
	g.writeJSON(w, 200, map[string]any{
		"status":   "compacted",
		"old_size": oldLen,
		"new_size": newLen,
	})
}

// handleGlobalUsage implements GET /api/usage
func (g *Gateway) handleGlobalUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		g.writeError(w, "method not allowed", 405)
		return
	}
	ut := g.assistant.UsageTracker()
	if ut == nil {
		g.writeJSON(w, 200, map[string]any{"usage": nil})
		return
	}
	u := ut.GetGlobal()
	g.writeJSON(w, 200, map[string]any{
		"usage": map[string]any{
			"prompt_tokens":     u.PromptTokens,
			"completion_tokens": u.CompletionTokens,
			"total_tokens":      u.TotalTokens,
			"requests":          u.Requests,
			"estimated_cost_usd": u.EstimatedCostUSD,
			"first_request_at":   u.FirstRequestAt,
			"last_request_at":    u.LastRequestAt,
		},
	})
}

// handleSessionUsage implements GET /api/usage/:session_id
func (g *Gateway) handleSessionUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		g.writeError(w, "method not allowed", 405)
		return
	}
	sessionID := strings.TrimPrefix(r.URL.Path, "/api/usage/")
	if sessionID == "" {
		g.writeError(w, "session id required", 400)
		return
	}
	ut := g.assistant.UsageTracker()
	if ut == nil {
		g.writeJSON(w, 200, map[string]any{"usage": nil})
		return
	}
	u := ut.GetSession(sessionID)
	if u == nil {
		g.writeError(w, "session not found", 404)
		return
	}
	g.writeJSON(w, 200, map[string]any{
		"session_id": sessionID,
		"usage": map[string]any{
			"prompt_tokens":     u.PromptTokens,
			"completion_tokens": u.CompletionTokens,
			"total_tokens":      u.TotalTokens,
			"requests":          u.Requests,
			"estimated_cost_usd": u.EstimatedCostUSD,
			"first_request_at":   u.FirstRequestAt,
			"last_request_at":    u.LastRequestAt,
		},
	})
}

// handleStatus implements GET /api/status
func (g *Gateway) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		g.writeError(w, "method not allowed", 405)
		return
	}
	channelsMap := make(map[string]string)
	for name, st := range g.assistant.ChannelManager().HealthAll() {
		if st.Connected {
			channelsMap[name] = "connected"
		} else {
			channelsMap[name] = "disconnected"
		}
	}
	sessionCount := g.assistant.WorkspaceManager().SessionCount()
	skillList := g.assistant.SkillRegistry().List()
	skillsInfo := make([]map[string]any, 0, len(skillList))
	for _, m := range skillList {
		skillsInfo = append(skillsInfo, map[string]any{"name": m.Name, "description": m.Description})
	}
	schedulerStatus := "disabled"
	if g.assistant.SchedulerEnabled() {
		schedulerStatus = "enabled"
	}
	memoryStatus := "disabled"
	if g.assistant.MemoryEnabled() {
		memoryStatus = "enabled"
	}
	g.writeJSON(w, 200, map[string]any{
		"channels":   channelsMap,
		"sessions":   sessionCount,
		"skills":     skillsInfo,
		"scheduler":  schedulerStatus,
		"memory":     memoryStatus,
	})
}

// ValidWebhookEvents lists all supported webhook event types.
var ValidWebhookEvents = []string{
	"message.received",
	"message.sent",
	"agent.started",
	"agent.completed",
	"agent.error",
	"tool.called",
	"tool.completed",
	"session.created",
	"session.deleted",
}

func isValidWebhookEvent(e string) bool {
	for _, v := range ValidWebhookEvents {
		if v == e {
			return true
		}
	}
	return false
}

// handleWebhooks implements GET /api/webhooks (list) and POST /api/webhooks (create).
func (g *Gateway) handleWebhooks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		g.writeJSON(w, 200, map[string]any{
			"webhooks":     g.ListWebhooks(),
			"valid_events": ValidWebhookEvents,
		})
	case http.MethodPost:
		var req struct {
			URL    string   `json:"url"`
			Events []string `json:"events"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			g.writeError(w, "invalid request body", 400)
			return
		}
		if req.URL == "" {
			g.writeError(w, "url required", 400)
			return
		}
		for _, e := range req.Events {
			if !isValidWebhookEvent(e) {
				g.writeError(w, "invalid event: "+e, 400)
				return
			}
		}
		entry, err := g.AddWebhook(req.URL, req.Events)
		if err != nil {
			g.writeError(w, err.Error(), 400)
			return
		}
		g.writeJSON(w, 201, entry)
	default:
		g.writeError(w, "method not allowed", 405)
	}
}

// handleWebhookByID implements DELETE /api/webhooks/{id} and PATCH /api/webhooks/{id}.
func (g *Gateway) handleWebhookByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/webhooks/")
	if id == "" {
		g.writeError(w, "webhook id required", 400)
		return
	}

	switch r.Method {
	case http.MethodDelete:
		if !g.DeleteWebhook(id) {
			g.writeError(w, "webhook not found", 404)
			return
		}
		g.writeJSON(w, 200, map[string]string{"status": "deleted"})
	case http.MethodPatch:
		var req struct {
			Active *bool `json:"active"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			g.writeError(w, "invalid request body", 400)
			return
		}
		if req.Active == nil {
			g.writeError(w, "active field required", 400)
			return
		}
		if !g.ToggleWebhook(id, *req.Active) {
			g.writeError(w, "webhook not found", 404)
			return
		}
		g.writeJSON(w, 200, map[string]string{"status": "updated"})
	default:
		g.writeError(w, "method not allowed", 405)
	}
}
