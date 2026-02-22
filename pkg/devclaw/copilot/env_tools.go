// Package copilot – env_tools.go implements system information and network
// diagnostic tools: port scanning, environment info, and process listing.
package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strings"
	"time"
)

// ---------- Data Types ----------

type envInfoResult struct {
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	GoVer    string `json:"go_version"`
	Shell    string `json:"shell"`
	User     string `json:"user"`
	Home     string `json:"home"`
	Hostname string `json:"hostname"`
	Cwd      string `json:"cwd"`
	NumCPU   int    `json:"num_cpu"`
}

type portScanResult struct {
	Port   int    `json:"port"`
	Status string `json:"status"` // open, closed
}

// ---------- Tool Registration ----------

// RegisterEnvTools registers system information and diagnostic tools.
func RegisterEnvTools(executor *ToolExecutor) {
	// env_info
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "env_info",
			Description: "Get system environment information: OS, architecture, shell, user, hostname, CPU count, working directory.",
			Parameters: mustJSON(map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			}),
		},
	}, func(_ context.Context, _ map[string]any) (any, error) {
		info := envInfoResult{
			OS:     runtime.GOOS,
			Arch:   runtime.GOARCH,
			GoVer:  runtime.Version(),
			Shell:  os.Getenv("SHELL"),
			Home:   os.Getenv("HOME"),
			NumCPU: runtime.NumCPU(),
		}

		if u, err := user.Current(); err == nil {
			info.User = u.Username
		}
		if h, err := os.Hostname(); err == nil {
			info.Hostname = h
		}
		if cwd, err := os.Getwd(); err == nil {
			info.Cwd = cwd
		}

		data, _ := json.MarshalIndent(info, "", "  ")
		return string(data), nil
	})

	// port_scan
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "port_scan",
			Description: "Check which ports are open on localhost. Scans a list of specific ports or a range.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"ports": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "integer"},
						"description": "List of ports to check (e.g. [3000, 5432, 8080]). Default: common dev ports.",
					},
				},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		ports := []int{22, 80, 443, 3000, 3306, 5000, 5432, 5433, 6379, 8000, 8080, 8090, 8443, 9090, 27017}

		if rawPorts, ok := args["ports"].([]any); ok && len(rawPorts) > 0 {
			ports = nil
			for _, p := range rawPorts {
				if pf, ok := p.(float64); ok {
					ports = append(ports, int(pf))
				}
			}
		}

		var results []portScanResult
		for _, port := range ports {
			addr := fmt.Sprintf("localhost:%d", port)
			conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
			if err != nil {
				results = append(results, portScanResult{Port: port, Status: "closed"})
			} else {
				conn.Close()
				results = append(results, portScanResult{Port: port, Status: "open"})
			}
		}

		// Only show open ports + summary
		var open []portScanResult
		for _, r := range results {
			if r.Status == "open" {
				open = append(open, r)
			}
		}

		summary := map[string]any{
			"scanned":    len(ports),
			"open_count": len(open),
			"open_ports": open,
		}

		data, _ := json.MarshalIndent(summary, "", "  ")
		return string(data), nil
	})

	// process_list
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "process_list",
			Description: "List running processes for the current user, with optional filter by name.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"filter": map[string]any{"type": "string", "description": "Filter processes by name (e.g. 'node', 'python', 'postgres')"},
				},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		filter, _ := args["filter"].(string)

		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.Command("tasklist", "/FO", "CSV", "/NH")
		} else {
			cmd = exec.Command("ps", "aux", "--sort=-%mem")
		}

		out, err := cmd.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("listing processes: %w", err)
		}

		lines := strings.Split(strings.TrimSpace(string(out)), "\n")

		if filter != "" {
			var filtered []string
			if len(lines) > 0 {
				filtered = append(filtered, lines[0]) // header
			}
			for _, line := range lines[1:] {
				if strings.Contains(strings.ToLower(line), strings.ToLower(filter)) {
					filtered = append(filtered, line)
				}
			}
			lines = filtered
		}

		// Truncate if too many
		const maxLines = 50
		if len(lines) > maxLines {
			lines = append(lines[:maxLines], fmt.Sprintf("\n... (%d more processes)", len(lines)-maxLines))
		}

		return strings.Join(lines, "\n"), nil
	})
}
