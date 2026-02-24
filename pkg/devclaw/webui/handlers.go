package webui

import (
	"encoding/json"
	"net/http"
	"strings"
)

// ── Dashboard ──

func (s *Server) handleAPIDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	sessions := s.api.ListSessions()
	if sessions == nil {
		sessions = []SessionInfo{}
	}
	channels := s.api.GetChannelHealth()
	if channels == nil {
		channels = []ChannelHealthInfo{}
	}
	jobs := s.api.GetSchedulerJobs()
	if jobs == nil {
		jobs = []JobInfo{}
	}

	data := map[string]any{
		"sessions": sessions,
		"usage":    s.api.GetUsageGlobal(),
		"channels": channels,
		"jobs":     jobs,
	}
	writeJSON(w, http.StatusOK, data)
}

// ── Sessions ──

func (s *Server) handleAPISessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		sessions := s.api.ListSessions()
		if sessions == nil {
			sessions = []SessionInfo{}
		}
		writeJSON(w, http.StatusOK, sessions)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (s *Server) handleAPISessionDetail(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	parts := strings.SplitN(path, "/", 2)
	sessionID := parts[0]

	if sessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing session ID"})
		return
	}

	// GET /api/sessions/{id}/messages
	if len(parts) > 1 && parts[1] == "messages" {
		if r.Method == http.MethodGet {
			msgs := s.api.GetSessionMessages(sessionID)
			if msgs == nil {
				msgs = []MessageInfo{}
			}
			writeJSON(w, http.StatusOK, msgs)
			return
		}
	}

	// DELETE /api/sessions/{id}
	if r.Method == http.MethodDelete {
		if err := s.api.DeleteSession(sessionID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
		return
	}

	writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
}

// ── Chat ──

func (s *Server) handleAPIChat(w http.ResponseWriter, r *http.Request) {
	// Parse: /api/chat/{sessionId}/{action}
	path := strings.TrimPrefix(r.URL.Path, "/api/chat/")
	parts := strings.SplitN(path, "/", 2)
	sessionID := parts[0]

	if sessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing session ID"})
		return
	}

	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	switch action {
	case "send":
		s.handleChatSend(w, r, sessionID)
	case "history":
		s.handleChatHistory(w, r, sessionID)
	case "abort":
		s.handleChatAbort(w, r, sessionID)
	case "stream":
		// Unified endpoint: POST with body starts a new run and streams inline.
		// GET with run_id connects to an existing run (legacy two-step flow).
		if r.Method == http.MethodPost {
			s.handleChatStreamUnified(w, r, sessionID)
		} else {
			s.handleChatStream(w, r, sessionID)
		}
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown action"})
	}
}

// handleChatSend starts an agent run and returns a run_id for SSE streaming.
func (s *Server) handleChatSend(w http.ResponseWriter, r *http.Request, sessionID string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Content == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing content"})
		return
	}

	// Start a streaming agent run.
	handle, err := s.api.StartChatStream(r.Context(), sessionID, body.Content)
	if err != nil {
		s.logger.Error("chat send failed", "session", sessionID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Register the run so the /stream endpoint can find it.
	s.registerRun(handle)

	writeJSON(w, http.StatusOK, map[string]string{"run_id": handle.RunID})
}

func (s *Server) handleChatHistory(w http.ResponseWriter, r *http.Request, sessionID string) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	msgs := s.api.GetSessionMessages(sessionID)
	if msgs == nil {
		msgs = []MessageInfo{}
	}
	writeJSON(w, http.StatusOK, msgs)
}

func (s *Server) handleChatAbort(w http.ResponseWriter, r *http.Request, sessionID string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	// Cancel via the webui's active stream registry (primary path for web UI runs).
	// Persist partial output before cancelling so the user sees what was generated.
	stopped := false
	s.activeStreamMu.Lock()
	for runID, handle := range s.activeStreams {
		if handle.SessionID == sessionID {
			// Signal abort — the event loop will detect cancellation and flush.
			handle.Cancel()
			delete(s.activeStreams, runID)
			stopped = true
			break
		}
	}
	s.activeStreamMu.Unlock()

	// Fallback: try via the assistant's active runs (for channel-driven runs).
	if !stopped {
		stopped = s.api.AbortRun(sessionID)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"stopped": stopped,
		"partial": stopped, // Indicates partial output may have been preserved.
	})
}

// handleChatStream serves SSE events for an active agent run.
// The frontend connects here after receiving a run_id from /send.
func (s *Server) handleChatStream(w http.ResponseWriter, r *http.Request, sessionID string) {
	runID := r.URL.Query().Get("run_id")
	if runID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing run_id"})
		return
	}

	// Look up the active run.
	handle := s.getRun(runID)
	if handle == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found or already completed"})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming not supported"})
		return
	}

	// Set SSE headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Stream events from the run handle until the channel is closed.
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			// Client disconnected — cancel the agent run and clean up.
			s.logger.Debug("SSE client disconnected", "run_id", runID)
			handle.Cancel()
			s.unregisterRun(runID)
			return

		case event, ok := <-handle.Events:
			if !ok {
				// Channel closed — run completed. Clean up.
				handle.Cancel() // Ensure context resources are released.
				s.unregisterRun(runID)
				return
			}
			writeSSE(w, flusher, event.Type, event.Data)
		}
	}
}

// handleChatStreamUnified combines send + stream in a single SSE connection.
// The frontend POSTs {"content":"..."} and receives SSE events on the same
// connection — no second round-trip needed. This eliminates ~200-500ms of
// latency compared to the two-step send → stream flow.
func (s *Server) handleChatStreamUnified(w http.ResponseWriter, r *http.Request, sessionID string) {
	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Content == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing content"})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming not supported"})
		return
	}

	// Start the agent run.
	handle, err := s.api.StartChatStream(r.Context(), sessionID, body.Content)
	if err != nil {
		s.logger.Error("unified stream failed", "session", sessionID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Register for abort support.
	s.registerRun(handle)

	// Switch to SSE mode — headers must be written before any events.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// First event: notify the frontend of the run_id (useful for abort).
	writeSSE(w, flusher, "run_start", map[string]string{"run_id": handle.RunID})

	// Stream events until the run completes or the client disconnects.
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			s.logger.Debug("unified SSE client disconnected", "run_id", handle.RunID)
			handle.Cancel()
			s.unregisterRun(handle.RunID)
			return

		case event, ok := <-handle.Events:
			if !ok {
				handle.Cancel()
				s.unregisterRun(handle.RunID)
				return
			}
			writeSSE(w, flusher, event.Type, event.Data)
		}
	}
}

// ── Skills ──

func (s *Server) handleAPISkills(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		skills := s.api.ListSkills()
		if skills == nil {
			skills = []SkillInfo{}
		}
		writeJSON(w, http.StatusOK, skills)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (s *Server) handleAPISkillsAction(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/skills/")

	switch {
	case path == "available" && r.Method == http.MethodGet:
		s.handleSkillsAvailable(w, r)
	case path == "install" && r.Method == http.MethodPost:
		s.handleSkillInstall(w, r)
	case strings.HasSuffix(path, "/toggle") && r.Method == http.MethodPost:
		name := strings.TrimSuffix(path, "/toggle")
		s.handleSkillToggle(w, r, name)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

func (s *Server) handleSkillsAvailable(w http.ResponseWriter, _ *http.Request) {
	result := fetchSkillsCatalogFromGitHub(s.logger)
	if len(result) == 0 {
		result = []map[string]any{}
	}

	installed := s.api.ListSkills()
	installedSet := make(map[string]bool, len(installed))
	for _, sk := range installed {
		installedSet[sk.Name] = true
	}
	for i := range result {
		name, _ := result[i]["name"].(string)
		result[i]["installed"] = installedSet[name]
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleSkillInstall(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	s.installSetupSkills([]string{body.Name})

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "Skill " + body.Name + " instalada. Reinicie para ativar.",
	})
}

func (s *Server) handleSkillToggle(w http.ResponseWriter, r *http.Request, name string) {
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}

	if err := s.api.ToggleSkill(name, body.Enabled); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ── Channels ──

func (s *Server) handleAPIChannels(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		channels := s.api.GetChannelHealth()
		if channels == nil {
			channels = []ChannelHealthInfo{}
		}
		writeJSON(w, http.StatusOK, channels)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// ── Domain & Network ──

func (s *Server) handleAPIDomain(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.api.GetDomainConfig())
	case http.MethodPut:
		var update DomainConfigUpdate
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if err := s.api.UpdateDomainConfig(update); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "ok",
			"message": "Domain configuration updated. Restart to apply port changes.",
		})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// ── Hooks (Lifecycle) ──

func (s *Server) handleAPIHooks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		hooks := s.api.ListHooks()
		if hooks == nil {
			hooks = []HookInfo{}
		}
		events := s.api.GetHookEvents()
		if events == nil {
			events = []HookEventInfo{}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"hooks":  hooks,
			"events": events,
		})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (s *Server) handleAPIHookByName(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/hooks/")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "hook name is required"})
		return
	}

	switch r.Method {
	case http.MethodPatch:
		var body struct {
			Enabled *bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if body.Enabled == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "enabled field is required"})
			return
		}
		if err := s.api.ToggleHook(name, *body.Enabled); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
	case http.MethodDelete:
		if err := s.api.UnregisterHook(name); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// ── Webhooks ──

func (s *Server) handleAPIWebhooks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		webhooks := s.api.ListWebhooks()
		if webhooks == nil {
			webhooks = []WebhookInfo{}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"webhooks":     webhooks,
			"valid_events": s.api.GetValidWebhookEvents(),
		})
	case http.MethodPost:
		var body struct {
			URL    string   `json:"url"`
			Events []string `json:"events"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if body.URL == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "URL is required"})
			return
		}
		wh, err := s.api.CreateWebhook(body.URL, body.Events)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, wh)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (s *Server) handleAPIWebhookByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/webhooks/")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "webhook ID is required"})
		return
	}

	switch r.Method {
	case http.MethodDelete:
		if err := s.api.DeleteWebhook(id); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	case http.MethodPatch:
		var body struct {
			Active *bool `json:"active"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if body.Active == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "active field is required"})
			return
		}
		if err := s.api.ToggleWebhook(id, *body.Active); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// ── Config ──

func (s *Server) handleAPIConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.api.GetConfigMap())
	case http.MethodPut:
		var updates map[string]any
		if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		if err := s.api.UpdateConfigMap(updates); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, s.api.GetConfigMap())
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// ── Usage ──

func (s *Server) handleAPIUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, s.api.GetUsageGlobal())
}

// ── Jobs ──

func (s *Server) handleAPIJobs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		jobs := s.api.GetSchedulerJobs()
		if jobs == nil {
			jobs = []JobInfo{}
		}
		writeJSON(w, http.StatusOK, jobs)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// ── Settings: Tool Profiles ──

func (s *Server) handleAPISettingsToolProfiles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		profiles := s.api.ListToolProfiles()
		groups := s.api.GetToolGroups()
		if profiles == nil {
			profiles = []ToolProfileInfo{}
		}
		if groups == nil {
			groups = map[string][]string{}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"profiles": profiles,
			"groups":   groups,
		})
	case http.MethodPost:
		var profile ToolProfileDef
		if err := json.NewDecoder(r.Body).Decode(&profile); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if profile.Name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
			return
		}
		if err := s.api.CreateToolProfile(profile); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, profile)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (s *Server) handleAPISettingsToolProfileByName(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/settings/tool-profiles/")
	name = strings.TrimSuffix(name, "/")

	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	switch r.Method {
	case http.MethodPut:
		var profile ToolProfileDef
		if err := json.NewDecoder(r.Body).Decode(&profile); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		profile.Name = name
		if err := s.api.UpdateToolProfile(profile); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, profile)
	case http.MethodDelete:
		if err := s.api.DeleteToolProfile(name); err != nil {
			if strings.Contains(err.Error(), "built-in") {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

