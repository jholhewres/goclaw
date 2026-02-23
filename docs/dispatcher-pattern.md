# Dispatcher Pattern for Tools

DevClaw uses a dispatcher pattern to reduce the number of tools exposed to LLMs. This is critical for OpenAI compatibility, which has a 128-tool limit.

---

## Problem

OpenAI models reject requests with more than 128 tools. DevClaw has ~196 registered tools, making it incompatible with OpenAI for agent teams.

**Before refactoring:**
- Team tools: 34 individual tools
- Memory tools: 4 individual tools
- Total reduction: 38 → 6 tools

---

## Solution

Replace multiple single-purpose tools with dispatcher tools that use an `action` parameter.

```
┌─────────────────────────────────────────────────────────────┐
│                   Single Tool (Before)                       │
│  team_create, team_list, team_get, team_update, ...         │
│  (34 tools for teams)                                        │
└─────────────────────────────────────────────────────────────┘
                           │
                           v
┌─────────────────────────────────────────────────────────────┐
│                   Dispatcher Tool (After)                    │
│  team_manage(action="create|list|get|update|delete", ...)   │
│  (5 tools for teams)                                         │
└─────────────────────────────────────────────────────────────┘
```

---

## Architecture

### Dispatcher Tool Structure

```go
func registerTeamManageTool(executor *ToolExecutor, teamMgr *TeamManager) {
    executor.Register(
        MakeToolDefinition("team_manage",
            "Manage teams with actions: create, list, get, update, delete",
            map[string]any{
                "type": "object",
                "properties": map[string]any{
                    "action": map[string]any{
                        "type":        "string",
                        "enum":        []string{"create", "list", "get", "update", "delete"},
                        "description": "Action to perform",
                    },
                    // ... other parameters
                },
                "required": []string{"action"},
            },
        ),
        func(ctx context.Context, args map[string]any) (any, error) {
            action, _ := args["action"].(string)
            switch action {
            case "create":
                return handleTeamCreate(ctx, teamMgr, args)
            case "list":
                return handleTeamList(ctx, teamMgr, args)
            case "get":
                return handleTeamGet(ctx, teamMgr, args)
            case "update":
                return handleTeamUpdate(ctx, teamMgr, args)
            case "delete":
                return handleTeamDelete(ctx, teamMgr, args)
            default:
                return nil, fmt.Errorf("unknown action: %s", action)
            }
        },
    )
}
```

### Internal Handlers

Handlers are private functions that implement each action:

```go
func handleTeamCreate(ctx context.Context, teamMgr *TeamManager, args map[string]any) (any, error) {
    name, _ := args["name"].(string)
    description, _ := args["description"].(string)
    // ... implementation
    return teamMgr.CreateTeam(name, description, callerJID, defaultModel)
}
```

---

## Current Dispatcher Tools

### Team Tools (5 dispatchers)

| Tool | Actions | Description |
|------|---------|-------------|
| `team_manage` | create, list, get, update, delete | Team CRUD |
| `team_agent` | create, list, get, update, start, stop, delete, working_get, working_update, working_clear | Agent management |
| `team_task` | create, list, get, update, assign, delete | Task management |
| `team_memory` | fact_save, fact_list, fact_delete, doc_create, doc_list, doc_get, doc_update, doc_delete, standup | Shared memory |
| `team_comm` | comment, mention_check, send_message, notify, notify_list | Communication |

### Memory Tools (1 dispatcher)

| Tool | Actions | Description |
|------|---------|-------------|
| `memory` | save, search, list, index | Long-term memory |

---

## Implementation Guidelines

### 1. Tool Definition

Always include the `action` parameter as the first required parameter:

```go
"action": map[string]any{
    "type":        "string",
    "enum":        []string{"action1", "action2", "action3"},
    "description": "Action to perform",
},
```

### 2. Handler Pattern

Each action should have a dedicated handler function:

```go
// Good: Separate handlers
func handleTeamCreate(ctx context.Context, teamMgr *TeamManager, args map[string]any) (any, error)
func handleTeamList(ctx context.Context, teamMgr *TeamManager, args map[string]any) (any, error)

// Bad: Everything in the dispatcher
func dispatcher(ctx context.Context, args map[string]any) (any, error) {
    // 500 lines of if/else...
}
```

### 3. Parameter Validation

Validate parameters in the handler, not the dispatcher:

```go
func handleFactSave(ctx context.Context, teamMgr *TeamManager, args map[string]any) (any, error) {
    key, ok := args["key"].(string)
    if !ok || key == "" {
        return nil, fmt.Errorf("key is required")
    }

    value, ok := args["value"].(string)
    if !ok || value == "" {
        return nil, fmt.Errorf("value is required")
    }

    return teamMgr.SaveFact(key, value)
}
```

### 4. Error Handling

Return descriptive errors that help the LLM understand what went wrong:

```go
// Good
return nil, fmt.Errorf("unknown action: %s. Valid actions: create, list, get, update, delete", action)

// Bad
return nil, fmt.Errorf("invalid action")
```

### 5. Response Format

Return structured responses that match the tool's schema:

```go
return map[string]any{
    "success": true,
    "team_id": team.ID,
    "name":    team.Name,
}, nil
```

---

## When to Use Dispatcher Pattern

### Good Candidates

1. **Related operations** - CRUD operations on the same resource
2. **Shared context** - Operations that use the same underlying manager/client
3. **High tool count** - Groups of 5+ related tools
4. **Namespace clarity** - When tool names would become verbose

### Poor Candidates

1. **Unrelated operations** - Different resources or concerns
2. **Single-purpose tools** - Only 1-2 operations per resource
3. **Complex parameters** - When each action has vastly different schemas

---

## Token Budget Considerations

Dispatcher tools have larger schemas because they include all possible parameters:

```go
// Before: Small schemas per tool
"team_create": { "name", "description" }  // 2 params
"team_update": { "team_id", "name", "description" }  // 3 params

// After: Combined schema
"team_manage": { "action", "team_id", "name", "description", "default_model" }  // 5 params
```

**Mitigation:**
- Keep descriptions concise
- Mark optional parameters clearly
- Use enums for action parameter
- Document in builtin skills instead of tool descriptions

---

## Migration Guide

### Step 1: Identify Tool Groups

Find related tools that operate on the same resource:

```bash
# Team tools
team_create
team_list
team_get
team_update
team_delete
```

### Step 2: Create Dispatcher

Create a single dispatcher tool with `action` parameter:

```go
func registerTeamManageTool(executor *ToolExecutor, teamMgr *TeamManager) {
    // ... dispatcher implementation
}
```

### Step 3: Extract Handlers

Move existing tool logic to handler functions:

```go
// Before: In tool handler
func(ctx context.Context, args map[string]any) (any, error) {
    name := args["name"].(string)
    return teamMgr.CreateTeam(name, ...)
}

// After: In handler function
func handleTeamCreate(ctx context.Context, teamMgr *TeamManager, args map[string]any) (any, error) {
    name := args["name"].(string)
    return teamMgr.CreateTeam(name, ...)
}
```

### Step 4: Remove Old Tools

Delete the old individual tool registrations.

### Step 5: Update Tests

Update tests to use the new dispatcher format:

```go
// Before
result, err := executor.Execute(ctx, "team_create", map[string]any{...})

// After
result, err := executor.Execute(ctx, "team_manage", map[string]any{
    "action": "create",
    ...
})
```

### Step 6: Update Documentation

Update all documentation that references the old tool names.

---

## Testing Dispatcher Tools

### Unit Tests

Test each action independently:

```go
func TestTeamManage_Create(t *testing.T) {
    result, err := executor.Execute(ctx, "team_manage", map[string]any{
        "action":      "create",
        "name":        "Test Team",
        "description": "Test description",
    })
    assert.NoError(t, err)
    assert.NotEmpty(t, result.(map[string]any)["team_id"])
}

func TestTeamManage_InvalidAction(t *testing.T) {
    _, err := executor.Execute(ctx, "team_manage", map[string]any{
        "action": "invalid",
    })
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "unknown action")
}
```

### Integration Tests

Test the full workflow:

```go
func TestTeamWorkflow(t *testing.T) {
    // Create
    team, _ := executor.Execute(ctx, "team_manage", map[string]any{
        "action": "create",
        "name":   "Workflow Test",
    })
    teamID := team.(map[string]any)["team_id"]

    // Get
    result, _ := executor.Execute(ctx, "team_manage", map[string]any{
        "action":  "get",
        "team_id": teamID,
    })
    assert.Equal(t, "Workflow Test", result.(map[string]any)["name"])

    // Delete
    executor.Execute(ctx, "team_manage", map[string]any{
        "action":  "delete",
        "team_id": teamID,
    })
}
```

---

## Documentation Updates

When migrating to dispatcher pattern, update:

1. **Tool documentation** - `docs/tools.md`, `docs/agent_teams.md`
2. **Builtin skills** - `pkg/devclaw/copilot/builtin/skills/*/SKILL.md`
3. **Tool guard** - Update `group:` annotations if needed
4. **CHANGELOG** - Document breaking changes

---

## Best Practices

1. **Consistent naming** - Use `{resource}_{verb}` pattern for actions
2. **Clear action descriptions** - Document what each action does
3. **Validate early** - Check action parameter first
4. **Return helpful errors** - Include valid actions in error messages
5. **Keep handlers small** - One handler per action
6. **Test edge cases** - Invalid actions, missing parameters
7. **Update skills** - Keep builtin skills in sync with tool changes

---

## Example: Memory Tool

```go
func RegisterMemoryTools(executor *ToolExecutor, cfg MemoryDispatcherConfig) {
    executor.Register(
        MakeToolDefinition("memory",
            "Manage long-term memory with actions: save, search, list, index",
            map[string]any{
                "type": "object",
                "properties": map[string]any{
                    "action": map[string]any{
                        "type":        "string",
                        "enum":        []string{"save", "search", "list", "index"},
                        "description": "Action to perform",
                    },
                    "content": map[string]any{
                        "type":        "string",
                        "description": "Content to save (for save)",
                    },
                    "category": map[string]any{
                        "type":        "string",
                        "description": "Category: fact, preference, event, summary",
                    },
                    "query": map[string]any{
                        "type":        "string",
                        "description": "Search query (for search)",
                    },
                    "limit": map[string]any{
                        "type":        "integer",
                        "description": "Max results (for search/list)",
                    },
                },
                "required": []string{"action"},
            },
        ),
        func(ctx context.Context, args map[string]any) (any, error) {
            action, _ := args["action"].(string)
            switch action {
            case "save":
                return handleMemorySave(ctx, cfg, args)
            case "search":
                return handleMemorySearch(ctx, cfg, args)
            case "list":
                return handleMemoryList(ctx, cfg, args)
            case "index":
                return handleMemoryIndex(ctx, cfg, args)
            default:
                return nil, fmt.Errorf("unknown action: %s. Valid actions: save, search, list, index", action)
            }
        },
    )
}
```

---

## Summary

| Aspect | Before | After |
|--------|--------|-------|
| Team tools | 34 | 5 |
| Memory tools | 4 | 1 |
| Total reduction | - | **32 tools saved** |
| OpenAI compatible | No | Yes |

The dispatcher pattern enables DevClaw to work with OpenAI models while maintaining all functionality.
