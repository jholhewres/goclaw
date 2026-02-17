// Package copilot – ops_tools.go implements operations tools for
// deploy pipeline execution, server health monitoring, and tunnel management.
package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// ---------- Data Types ----------

type healthCheckResult struct {
	Target    string `json:"target"`
	Type      string `json:"type"` // http, tcp, dns
	Status    string `json:"status"` // healthy, unhealthy, timeout
	Latency   string `json:"latency"`
	Details   string `json:"details,omitempty"`
	Timestamp string `json:"timestamp"`
}

type deployResult struct {
	Command  string `json:"command"`
	ExitCode int    `json:"exit_code"`
	Output   string `json:"output"`
	Duration string `json:"duration"`
}

// ---------- Tool Registration ----------

// RegisterOpsTools registers operations and deployment tools.
func RegisterOpsTools(executor *ToolExecutor) {
	// server_health
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "server_health",
			Description: "Check server health via HTTP, TCP, or DNS. Supports multiple targets for batch checks.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"targets": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "List of targets (URLs for HTTP, host:port for TCP, domain for DNS)",
					},
					"type":    map[string]any{"type": "string", "enum": []string{"http", "tcp", "dns"}, "description": "Check type (default: auto-detect)"},
					"timeout": map[string]any{"type": "integer", "description": "Timeout in seconds (default: 10)"},
				},
				"required": []string{"targets"},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		rawTargets, _ := args["targets"].([]any)
		checkType, _ := args["type"].(string)
		timeout := 10
		if v, ok := args["timeout"].(float64); ok {
			timeout = int(v)
		}
		timeoutDur := time.Duration(timeout) * time.Second

		var results []healthCheckResult
		for _, raw := range rawTargets {
			target, ok := raw.(string)
			if !ok {
				continue
			}

			ct := checkType
			if ct == "" {
				ct = detectCheckType(target)
			}

			result := checkHealth(target, ct, timeoutDur)
			results = append(results, result)
		}

		data, _ := json.MarshalIndent(results, "", "  ")
		return string(data), nil
	})

	// deploy_run
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "deploy_run",
			Description: "Execute a deployment command or script. Supports common deployment tools (rsync, scp, docker push, kubectl, etc.).",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command":    map[string]any{"type": "string", "description": "Deploy command to execute"},
					"pre_check":  map[string]any{"type": "string", "description": "Command to run before deploy (e.g. tests, lint)"},
					"post_check": map[string]any{"type": "string", "description": "Command to run after deploy (e.g. health check)"},
					"dry_run":    map[string]any{"type": "boolean", "description": "Print commands without executing"},
				},
				"required": []string{"command"},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		command, _ := args["command"].(string)
		preCheck, _ := args["pre_check"].(string)
		postCheck, _ := args["post_check"].(string)
		dryRun, _ := args["dry_run"].(bool)

		if dryRun {
			var sb strings.Builder
			sb.WriteString("=== DRY RUN ===\n")
			if preCheck != "" {
				sb.WriteString(fmt.Sprintf("Pre-check: %s\n", preCheck))
			}
			sb.WriteString(fmt.Sprintf("Deploy: %s\n", command))
			if postCheck != "" {
				sb.WriteString(fmt.Sprintf("Post-check: %s\n", postCheck))
			}
			return sb.String(), nil
		}

		var results []deployResult

		// Pre-check
		if preCheck != "" {
			result := runDeployCmd(preCheck)
			results = append(results, result)
			if result.ExitCode != 0 {
				results = append(results, deployResult{
					Command: "ABORTED: pre-check failed",
				})
				data, _ := json.MarshalIndent(results, "", "  ")
				return string(data), nil
			}
		}

		// Deploy
		result := runDeployCmd(command)
		results = append(results, result)
		if result.ExitCode != 0 {
			data, _ := json.MarshalIndent(results, "", "  ")
			return string(data), fmt.Errorf("deploy failed (exit %d)", result.ExitCode)
		}

		// Post-check
		if postCheck != "" {
			result := runDeployCmd(postCheck)
			results = append(results, result)
		}

		data, _ := json.MarshalIndent(results, "", "  ")
		return string(data), nil
	})

	// tunnel_manage
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "tunnel_manage",
			Description: "Manage SSH tunnels and port forwarding for accessing remote services.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action":      map[string]any{"type": "string", "enum": []string{"create", "list", "close"}, "description": "Tunnel action"},
					"local_port":  map[string]any{"type": "integer", "description": "Local port to bind"},
					"remote_host": map[string]any{"type": "string", "description": "Remote host (e.g. db.internal:5432)"},
					"ssh_host":    map[string]any{"type": "string", "description": "SSH jump host (e.g. user@bastion.example.com)"},
				},
				"required": []string{"action"},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		action, _ := args["action"].(string)

		switch action {
		case "create":
			localPort, _ := args["local_port"].(float64)
			remoteHost, _ := args["remote_host"].(string)
			sshHost, _ := args["ssh_host"].(string)

			if localPort == 0 || remoteHost == "" || sshHost == "" {
				return nil, fmt.Errorf("create requires local_port, remote_host, and ssh_host")
			}

			sshCmd := fmt.Sprintf("ssh -f -N -L %d:%s %s", int(localPort), remoteHost, sshHost)
			cmd := exec.Command("sh", "-c", sshCmd)
			if err := cmd.Start(); err != nil {
				return nil, fmt.Errorf("creating tunnel: %w", err)
			}

			return fmt.Sprintf("Tunnel created: localhost:%d -> %s via %s (PID: %d)", int(localPort), remoteHost, sshHost, cmd.Process.Pid), nil

		case "list":
			out, err := exec.Command("sh", "-c", "ps aux | grep 'ssh -f -N -L' | grep -v grep").CombinedOutput()
			if err != nil || strings.TrimSpace(string(out)) == "" {
				return "No active tunnels.", nil
			}
			return strings.TrimSpace(string(out)), nil

		case "close":
			localPort, _ := args["local_port"].(float64)
			if localPort == 0 {
				return nil, fmt.Errorf("close requires local_port")
			}

			out, _ := exec.Command("sh", "-c", fmt.Sprintf("lsof -ti :%.0f | xargs kill 2>/dev/null", localPort)).CombinedOutput()
			return fmt.Sprintf("Closed tunnel on port %d. %s", int(localPort), strings.TrimSpace(string(out))), nil

		default:
			return nil, fmt.Errorf("unknown action: %s", action)
		}
	})

	// ssh_exec
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "ssh_exec",
			Description: "Execute a command on a remote server via SSH.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"host":    map[string]any{"type": "string", "description": "SSH host (e.g. user@server.example.com)"},
					"command": map[string]any{"type": "string", "description": "Command to execute remotely"},
				},
				"required": []string{"host", "command"},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		host, _ := args["host"].(string)
		command, _ := args["command"].(string)

		cmd := exec.Command("ssh", "-o", "ConnectTimeout=10", "-o", "StrictHostKeyChecking=accept-new", host, command)
		out, err := cmd.CombinedOutput()
		result := strings.TrimSpace(string(out))

		if err != nil {
			if result != "" {
				return nil, fmt.Errorf("ssh error: %s", result)
			}
			return nil, fmt.Errorf("ssh error: %w", err)
		}

		return truncateOutput(result, 6000), nil
	})
}

func detectCheckType(target string) string {
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		return "http"
	}
	if strings.Contains(target, ":") {
		return "tcp"
	}
	return "dns"
}

func checkHealth(target, checkType string, timeout time.Duration) healthCheckResult {
	result := healthCheckResult{
		Target:    target,
		Type:      checkType,
		Timestamp: time.Now().Format(time.RFC3339),
	}

	start := time.Now()

	switch checkType {
	case "http":
		client := &http.Client{Timeout: timeout}
		resp, err := client.Get(target)
		result.Latency = time.Since(start).Truncate(time.Millisecond).String()
		if err != nil {
			result.Status = "unhealthy"
			result.Details = err.Error()
		} else {
			resp.Body.Close()
			if resp.StatusCode < 400 {
				result.Status = "healthy"
			} else {
				result.Status = "unhealthy"
			}
			result.Details = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}

	case "tcp":
		conn, err := net.DialTimeout("tcp", target, timeout)
		result.Latency = time.Since(start).Truncate(time.Millisecond).String()
		if err != nil {
			result.Status = "unhealthy"
			result.Details = err.Error()
		} else {
			conn.Close()
			result.Status = "healthy"
			result.Details = "Connection successful"
		}

	case "dns":
		addrs, err := net.LookupHost(target)
		result.Latency = time.Since(start).Truncate(time.Millisecond).String()
		if err != nil {
			result.Status = "unhealthy"
			result.Details = err.Error()
		} else {
			result.Status = "healthy"
			result.Details = fmt.Sprintf("Resolved to: %s", strings.Join(addrs, ", "))
		}
	}

	return result
}

func runDeployCmd(cmdStr string) deployResult {
	start := time.Now()
	cmd := exec.Command("sh", "-c", cmdStr)
	out, err := cmd.CombinedOutput()
	duration := time.Since(start)

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	return deployResult{
		Command:  cmdStr,
		ExitCode: exitCode,
		Output:   truncateOutput(string(out), 4000),
		Duration: duration.Truncate(time.Millisecond).String(),
	}
}
