// Package copilot – docker_tools.go implements native Docker tools
// for container management, image operations, and compose integration.
package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// ---------- Data Types ----------

type dockerContainer struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Image   string `json:"image"`
	Status  string `json:"status"`
	Ports   string `json:"ports"`
	Created string `json:"created"`
}

type dockerImage struct {
	ID         string `json:"id"`
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	Size       string `json:"size"`
	Created    string `json:"created"`
}

// ---------- Helpers ----------

func runDocker(args ...string) (string, error) {
	cmd := exec.Command("docker", args...)
	out, err := cmd.CombinedOutput()
	result := strings.TrimSpace(string(out))
	if err != nil {
		if result != "" {
			return "", fmt.Errorf("docker %s: %s", args[0], result)
		}
		return "", fmt.Errorf("docker %s: %w", args[0], err)
	}
	return result, nil
}

// ---------- Tool Registration ----------

// RegisterDockerTools registers Docker management tools in the executor.
func RegisterDockerTools(executor *ToolExecutor) {
	// docker_ps
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "docker_ps",
			Description: "List Docker containers. Returns structured JSON with ID, name, image, status, ports.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"all": map[string]any{"type": "boolean", "description": "Show all containers (including stopped)"},
				},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		dockerArgs := []string{"ps", "--format", "{{.ID}}|{{.Names}}|{{.Image}}|{{.Status}}|{{.Ports}}|{{.CreatedAt}}", "--no-trunc"}
		if all, _ := args["all"].(bool); all {
			dockerArgs = append(dockerArgs, "-a")
		}

		out, err := runDocker(dockerArgs...)
		if err != nil {
			return nil, err
		}
		if out == "" {
			return "No containers running.", nil
		}

		var containers []dockerContainer
		for _, line := range strings.Split(out, "\n") {
			parts := strings.SplitN(line, "|", 6)
			if len(parts) < 4 {
				continue
			}
			c := dockerContainer{
				ID:     parts[0][:12],
				Name:   parts[1],
				Image:  parts[2],
				Status: parts[3],
			}
			if len(parts) > 4 {
				c.Ports = parts[4]
			}
			if len(parts) > 5 {
				c.Created = parts[5]
			}
			containers = append(containers, c)
		}

		data, _ := json.MarshalIndent(containers, "", "  ")
		return string(data), nil
	})

	// docker_logs
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "docker_logs",
			Description: "Get logs from a Docker container.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"container": map[string]any{"type": "string", "description": "Container name or ID"},
					"tail":      map[string]any{"type": "integer", "description": "Number of lines from end (default: 100)"},
				},
				"required": []string{"container"},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		container, _ := args["container"].(string)
		tail := 100
		if v, ok := args["tail"].(float64); ok {
			tail = int(v)
		}

		out, err := runDocker("logs", "--tail", fmt.Sprintf("%d", tail), container)
		if err != nil {
			return nil, err
		}
		return out, nil
	})

	// docker_exec
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "docker_exec",
			Description: "Execute a command inside a running Docker container.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"container": map[string]any{"type": "string", "description": "Container name or ID"},
					"command":   map[string]any{"type": "string", "description": "Command to execute (e.g. 'ls -la /app')"},
				},
				"required": []string{"container", "command"},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		container, _ := args["container"].(string)
		command, _ := args["command"].(string)

		dockerArgs := []string{"exec", container, "sh", "-c", command}
		out, err := runDocker(dockerArgs...)
		if err != nil {
			return nil, err
		}
		return out, nil
	})

	// docker_images
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "docker_images",
			Description: "List Docker images. Returns structured JSON with repository, tag, size.",
			Parameters: mustJSON(map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			}),
		},
	}, func(_ context.Context, _ map[string]any) (any, error) {
		out, err := runDocker("images", "--format", "{{.ID}}|{{.Repository}}|{{.Tag}}|{{.Size}}|{{.CreatedAt}}")
		if err != nil {
			return nil, err
		}
		if out == "" {
			return "No images found.", nil
		}

		var images []dockerImage
		for _, line := range strings.Split(out, "\n") {
			parts := strings.SplitN(line, "|", 5)
			if len(parts) < 4 {
				continue
			}
			img := dockerImage{
				ID:         parts[0],
				Repository: parts[1],
				Tag:        parts[2],
				Size:       parts[3],
			}
			if len(parts) > 4 {
				img.Created = parts[4]
			}
			images = append(images, img)
		}

		data, _ := json.MarshalIndent(images, "", "  ")
		return string(data), nil
	})

	// docker_compose
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "docker_compose",
			Description: "Run Docker Compose commands: up, down, ps, logs, restart, build.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action":  map[string]any{"type": "string", "enum": []string{"up", "down", "ps", "logs", "restart", "build"}, "description": "Compose action"},
					"service": map[string]any{"type": "string", "description": "Specific service name (optional)"},
					"detach":  map[string]any{"type": "boolean", "description": "Run in background for 'up' (default: true)"},
				},
				"required": []string{"action"},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		action, _ := args["action"].(string)
		service, _ := args["service"].(string)

		composeArgs := []string{"compose", action}

		switch action {
		case "up":
			detach := true
			if v, ok := args["detach"].(bool); ok {
				detach = v
			}
			if detach {
				composeArgs = append(composeArgs, "-d")
			}
		case "logs":
			composeArgs = append(composeArgs, "--tail", "50")
		}

		if service != "" {
			composeArgs = append(composeArgs, service)
		}

		out, err := runDocker(composeArgs...)
		if err != nil {
			return nil, err
		}
		if out == "" {
			return fmt.Sprintf("docker compose %s completed.", action), nil
		}
		return out, nil
	})

	// docker_stop
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "docker_stop",
			Description: "Stop a running Docker container.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"container": map[string]any{"type": "string", "description": "Container name or ID"},
				},
				"required": []string{"container"},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		container, _ := args["container"].(string)
		out, err := runDocker("stop", container)
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("Stopped container: %s", out), nil
	})

	// docker_rm
	executor.Register(ToolDefinition{
		Type: "function",
		Function: FunctionDef{
			Name:        "docker_rm",
			Description: "Remove a stopped Docker container.",
			Parameters: mustJSON(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"container": map[string]any{"type": "string", "description": "Container name or ID"},
					"force":     map[string]any{"type": "boolean", "description": "Force remove running container"},
				},
				"required": []string{"container"},
			}),
		},
	}, func(_ context.Context, args map[string]any) (any, error) {
		container, _ := args["container"].(string)
		dockerArgs := []string{"rm"}
		if force, _ := args["force"].(bool); force {
			dockerArgs = append(dockerArgs, "-f")
		}
		dockerArgs = append(dockerArgs, container)

		out, err := runDocker(dockerArgs...)
		if err != nil {
			return nil, err
		}
		return fmt.Sprintf("Removed container: %s", out), nil
	})
}
