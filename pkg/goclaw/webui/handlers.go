package webui

import (
	"net/http"
	"strings"
)

// handleDashboard renders the main dashboard overview.
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	data := map[string]any{
		"Page":     "dashboard",
		"Sessions": s.api.ListSessions(),
		"Usage":    s.api.GetUsageGlobal(),
		"Channels": s.api.GetChannelHealth(),
		"Jobs":     s.api.GetSchedulerJobs(),
	}
	s.renderTemplate(w, "dashboard.html", data)
}

// handleChat renders the chat interface.
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session")
	if sessionID == "" {
		sessionID = "webui:default"
	}

	messages := s.api.GetSessionMessages(sessionID)

	data := map[string]any{
		"Page":      "chat",
		"SessionID": sessionID,
		"Messages":  messages,
	}
	s.renderTemplate(w, "chat.html", data)
}

// handleChatSend processes a chat message submission (HTMX endpoint).
func (s *Server) handleChatSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.ParseForm()
	content := strings.TrimSpace(r.FormValue("message"))
	sessionID := r.FormValue("session_id")
	if content == "" {
		http.Error(w, "Empty message", http.StatusBadRequest)
		return
	}
	if sessionID == "" {
		sessionID = "webui:default"
	}

	response, err := s.api.SendChatMessage(sessionID, content)
	if err != nil {
		s.logger.Error("chat send failed", "error", err)
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<div class="p-3 bg-red-50 text-red-700 rounded-lg">Error: ` + err.Error() + `</div>`))
		return
	}

	// Return the message pair as HTML partial for HTMX swap.
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(`<div class="flex justify-end mb-4">
  <div class="bg-blue-500 text-white rounded-lg py-2 px-4 max-w-[70%]">` + escapeHTML(content) + `</div>
</div>
<div class="flex justify-start mb-4">
  <div class="bg-gray-100 text-gray-800 rounded-lg py-2 px-4 max-w-[70%] prose prose-sm">` + escapeHTML(response) + `</div>
</div>`))
}

// handleSessions renders the sessions list page.
func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{
		"Page":     "sessions",
		"Sessions": s.api.ListSessions(),
	}
	s.renderTemplate(w, "sessions.html", data)
}

// handleSessionDetail renders a single session's detail view.
func (s *Server) handleSessionDetail(w http.ResponseWriter, r *http.Request) {
	// Extract session ID from path: /sessions/{id}
	sessionID := strings.TrimPrefix(r.URL.Path, "/sessions/")
	if sessionID == "" {
		http.Redirect(w, r, "/sessions", http.StatusFound)
		return
	}

	messages := s.api.GetSessionMessages(sessionID)

	data := map[string]any{
		"Page":      "sessions",
		"SessionID": sessionID,
		"Messages":  messages,
	}
	s.renderTemplate(w, "session_detail.html", data)
}

// handleConfig renders the configuration viewer.
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{
		"Page":   "config",
		"Config": s.api.GetConfigMap(),
	}
	s.renderTemplate(w, "config.html", data)
}

// handleSkills renders the skills management page.
func (s *Server) handleSkills(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{
		"Page":   "skills",
		"Skills": s.api.ListSkills(),
	}
	s.renderTemplate(w, "skills.html", data)
}

// handleUsage renders the token usage page.
func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{
		"Page":  "usage",
		"Usage": s.api.GetUsageGlobal(),
	}
	s.renderTemplate(w, "usage.html", data)
}

// handleJobs renders the scheduler jobs page.
func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{
		"Page": "jobs",
		"Jobs": s.api.GetSchedulerJobs(),
	}
	s.renderTemplate(w, "jobs.html", data)
}

// escapeHTML escapes HTML special characters.
func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}
