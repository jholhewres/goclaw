---
name: canvas
description: "Create and manage persistent interactive workspaces"
trigger: automatic
---

# Canvas

Persistent workspaces for long-running tasks, dashboards, and interactive content.

## Tools

| Tool | Action |
|------|--------|
| `canvas_create` | Create a new canvas |
| `canvas_update` | Update canvas content |
| `canvas_list` | List all canvases |
| `canvas_stop` | Stop/remove a canvas |

## When to Use

| Tool | When |
|------|------|
| `canvas_create` | Need persistent display/workspace |
| `canvas_update` | Refresh content, show progress |
| `canvas_list` | Check active canvases |
| `canvas_stop` | Clean up when done |

## Workflow

```
1. CREATE   → canvas_create(name="dashboard", content="...")
2. UPDATE   → canvas_update(id="abc", content="New content")
3. REFRESH  → Repeat updates as needed
4. STOP     → canvas_stop(id="abc") when done
```

## Examples

### Create Dashboard
```bash
canvas_create(
  name="build-progress",
  content="# Build Progress\n\n- [x] Download dependencies\n- [ ] Compile\n- [ ] Test\n- [ ] Deploy"
)
# Output: Canvas created with ID: abc123
```

### Update Progress
```bash
canvas_update(
  id="abc123",
  content="# Build Progress\n\n- [x] Download dependencies\n- [x] Compile\n- [ ] Test\n- [ ] Deploy"
)
# Output: Canvas abc123 updated
```

### List Active Canvases
```bash
canvas_list()
# Output:
# Active canvases (2):
# - build-progress (abc123) - created 5m ago
# - status-board (def456) - created 1h ago
```

### Stop Canvas
```bash
canvas_stop(id="abc123")
# Output: Canvas abc123 stopped and removed
```

## Use Cases

| Use Case | Description |
|----------|-------------|
| Progress tracking | Long-running task updates |
| Status dashboard | System/service monitoring |
| Interactive menu | User selection interface |
| Live log viewer | Streaming log output |

## Best Practices

| Practice | Reason |
|----------|--------|
| Use descriptive names | Easy to identify in list |
| Update regularly | Keep content fresh |
| Clean up when done | Stop unused canvases |
| Keep content concise | Better display on all devices |

## Complete Example

### Build Process Dashboard
```bash
# Start
canvas_create(
  name="build-status",
  content="# Build Status\n\nStatus: Starting...\n\n## Steps\n- [ ] Setup\n- [ ] Build\n- [ ] Test"
)

# After setup
canvas_update(
  id="build-status",
  content="# Build Status\n\nStatus: Building...\n\n## Steps\n- [x] Setup\n- [ ] Build\n- [ ] Test"
)

# After build
canvas_update(
  id="build-status",
  content="# Build Status\n\nStatus: Testing...\n\n## Steps\n- [x] Setup\n- [x] Build\n- [ ] Test"
)

# Complete
canvas_update(
  id="build-status",
  content="# Build Status\n\nStatus: Complete!\n\n## Steps\n- [x] Setup\n- [x] Build\n- [x] Test\n\nDuration: 2m 34s"
)

# Clean up
canvas_stop(id="build-status")
```

## Important Notes

| Note | Reason |
|------|--------|
| Canvases persist | Until explicitly stopped |
| ID returned on create | Store it for updates |
| Content is replaced | Not appended (update = full replace) |
| One canvas per task | Keep focused, not cluttered |
