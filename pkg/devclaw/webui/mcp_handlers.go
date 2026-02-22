package webui

import (
	"encoding/json"
	"net/http"
	"strings"
)

// handleAPIMCPServers handles GET /api/mcp/servers (list) and POST /api/mcp/servers (create)
func (s *Server) handleAPIMCPServers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		servers := s.api.ListMCPServers()
		writeJSON(w, http.StatusOK, map[string]any{"servers": servers})

	case http.MethodPost:
		var req struct {
			Name    string            `json:"name"`
			Command string            `json:"command"`
			Args    []string          `json:"args"`
			Env     map[string]string `json:"env"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if req.Name == "" || req.Command == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and command are required"})
			return
		}
		if req.Args == nil {
			req.Args = []string{}
		}
		if req.Env == nil {
			req.Env = make(map[string]string)
		}
		if err := s.api.CreateMCPServer(req.Name, req.Command, req.Args, req.Env); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"status": "created"})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// handleAPIMCPServerByName handles operations on a specific MCP server
func (s *Server) handleAPIMCPServerByName(w http.ResponseWriter, r *http.Request) {
	// Extract name from path /api/mcp/servers/{name}/...
	path := strings.TrimPrefix(r.URL.Path, "/api/mcp/servers/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "server name required"})
		return
	}
	name := parts[0]

	// Check for action subpath (start, stop)
	if len(parts) > 1 {
		action := parts[1]
		switch action {
		case "start":
			if r.Method != http.MethodPost {
				writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
				return
			}
			if err := s.api.StartMCPServer(name); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "started"})

		case "stop":
			if r.Method != http.MethodPost {
				writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
				return
			}
			if err := s.api.StopMCPServer(name); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})

		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown action"})
		}
		return
	}

	// Handle server-level operations
	switch r.Method {
	case http.MethodGet:
		// Return single server (filter from list)
		servers := s.api.ListMCPServers()
		for _, srv := range servers {
			if srv.Name == name {
				writeJSON(w, http.StatusOK, srv)
				return
			}
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "server not found"})

	case http.MethodPut:
		var req struct {
			Enabled *bool `json:"enabled,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if req.Enabled != nil {
			if err := s.api.UpdateMCPServer(name, *req.Enabled); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})

	case http.MethodDelete:
		if err := s.api.DeleteMCPServer(name); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// handleAPIDatabaseStatus handles GET /api/database/status
func (s *Server) handleAPIDatabaseStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	status := s.api.GetDatabaseStatus()
	writeJSON(w, http.StatusOK, status)
}
