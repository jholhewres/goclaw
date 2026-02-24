---
name: canvas
description: "Create and manage persistent interactive workspaces"
trigger: automatic
---

# Canvas

Persistent workspaces for long-running tasks, dashboards, and interactive content.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      Agent Context                          │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
                  ┌─────────────────┐
                  │ canvas_create   │
                  │ (HTML/JS page)  │
                  └────────┬────────┘
                           │
        ┌──────────────────┼──────────────────┐
        │                  │                  │
        ▼                  ▼                  ▼
┌───────────────┐  ┌───────────────┐  ┌───────────────┐
│ canvas_update │  │ canvas_list   │  │ canvas_stop   │
│ (refresh)     │  │ (status)      │  │ (cleanup)     │
└───────────────┘  └───────────────┘  └───────────────┘
        │
        ▼
┌───────────────────────────────────────────────────────┐
│                   Canvas Server                        │
│  http://localhost:PORT/__devclaw__/canvas/<id>.html   │
└───────────────────────────────────────────────────────┘
```

## Tools

| Tool | Action | Use When |
|------|--------|----------|
| `canvas_create` | Create new canvas | Need persistent display |
| `canvas_update` | Update content | Refresh, show progress |
| `canvas_list` | List all canvases | Check what's running |
| `canvas_stop` | Stop canvas | Clean up when done |

## When to Use

| Scenario | Canvas |
|----------|--------|
| Long-running task progress | Progress bar, status updates |
| Live dashboard | System monitoring, metrics |
| Interactive menu | User selection interface |
| Live log viewer | Streaming output |

## Creating a Canvas

```bash
canvas_create(
  name="build-progress",
  content="<h1>Build Progress</h1>
<div id='status'>Starting...</div>
<div id='steps'>
  <p>✓ Dependencies</p>
  <p>○ Compile</p>
  <p>○ Test</p>
</div>"
)
# Output: Canvas created with ID: canvas-abc123
# URL: http://localhost:8080/__devclaw__/canvas/canvas-abc123.html
```

## Updating Content

```bash
canvas_update(
  id="canvas-abc123",
  content="<h1>Build Progress</h1>
<div id='status'>Building...</div>
<div id='steps'>
  <p>✓ Dependencies</p>
  <p>✓ Compile</p>
  <p>○ Test</p>
</div>"
)
# Output: Canvas canvas-abc123 updated
```

## Listing Canvases

```bash
canvas_list()
# Output:
# Active canvases (2):
# - canvas-abc123 [build-progress]: running, 5m ago
# - canvas-def456 [dashboard]: running, 1h ago
```

## Stopping a Canvas

```bash
canvas_stop(id="canvas-abc123")
# Output: Canvas canvas-abc123 stopped and removed
```

## Common Patterns

### Progress Dashboard
```bash
# Create
canvas_create(
  name="deploy-status",
  content="<h1>🚀 Deployment Status</h1>
<style>
  .done { color: green; }
  .doing { color: orange; }
  .todo { color: gray; }
</style>
<div id='steps'>
  <p class='doing'>⏳ Building...</p>
  <p class='todo'>○ Testing</p>
  <p class='todo'>○ Deploying</p>
</div>
<div id='time'>Started: 14:30</div>"
)

# Update as progress happens
canvas_update(id="canvas-abc", content="...building done, testing...")

# Final state
canvas_update(id="canvas-abc", content="...
  <p class='done'>✓ Building</p>
  <p class='done'>✓ Testing</p>
  <p class='done'>✓ Deployed!</p>
</div>
<div id='time'>Completed: 14:35 (5m)</div>")

# Clean up after delay
canvas_stop(id="canvas-abc")
```

### Live Metrics Dashboard
```bash
canvas_create(
  name="system-metrics",
  content="<h1>System Metrics</h1>
<script>
  setInterval(() => {
    document.getElementById('time').textContent = new Date().toLocaleTimeString();
  }, 1000);
</script>
<div id='time'>--:--:--</div>
<div id='cpu'>CPU: --</div>
<div id='memory'>Memory: --</div>"
)

# Update periodically
canvas_update(id="canvas-metrics", content="...
<div id='cpu'>CPU: 45%</div>
<div id='memory'>Memory: 2.1GB / 8GB</div>
...")
```

### Interactive Selection Menu
```bash
canvas_create(
  name="options-menu",
  content="<h1>Select an Option</h1>
<div id='options'>
  <button onclick='select(1)'>Option 1: Quick Report</button>
  <button onclick='select(2)'>Option 2: Full Report</button>
  <button onclick='select(3)'>Option 3: Custom Range</button>
</div>
<script>
  function select(opt) {
    document.body.innerHTML = '<h1>Processing option ' + opt + '...</h1>';
  }
</script>"
)
```

### Build Log Viewer
```bash
canvas_create(
  name="build-logs",
  content="<h1>Build Output</h1>
<pre id='logs' style='font-family: monospace; font-size: 12px;'>
Starting build...
</pre>"
)

# Append logs by updating full content
canvas_update(id="canvas-logs", content="...
<pre id='logs'>
Starting build...
[14:30:01] Installing dependencies...
[14:30:15] Dependencies installed
[14:30:16] Compiling...
</pre>")
```

## HTML Template

Basic canvas template with live reload support:

```html
<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>Canvas Title</title>
  <style>
    body { font-family: system-ui, sans-serif; padding: 20px; }
    .status { padding: 10px; margin: 10px 0; border-radius: 4px; }
    .success { background: #d4edda; color: #155724; }
    .error { background: #f8d7da; color: #721c24; }
    .progress { background: #fff3cd; color: #856404; }
  </style>
</head>
<body>
  <h1>Title</h1>
  <div id="content">
    <!-- Dynamic content here -->
  </div>
</body>
</html>
```

## Troubleshooting

### Canvas not loading

**Cause:** Server not running or wrong URL.

**Debug:**
```bash
# Check if canvas server is running
bash(command="curl http://localhost:8080/__devclaw__/canvas/")

# List active canvases
canvas_list()
```

### Update not reflecting

**Cause:** Browser caching.

**Solution:**
```bash
# Add cache-busting meta tag in content
content="<meta http-equiv='refresh' content='5'>..."
```

### "Canvas not found"

**Cause:** Invalid canvas ID.

**Debug:**
```bash
# List all canvases to get correct ID
canvas_list()
```

### Content too large

**Cause:** HTML content exceeds limits.

**Solution:**
- Keep content concise
- Use external styles when possible
- Consider pagination for logs

## Tips

- **Keep content self-contained**: Inline CSS/JS for best results
- **Use semantic HTML**: Clear structure helps readability
- **Update frequency**: Balance between fresh data and performance
- **Clean up when done**: Stop unused canvases
- **One canvas per task**: Keep focused, not cluttered
- **Test responsiveness**: Use viewport-friendly layouts

## Common Mistakes

| Mistake | Correct Approach |
|---------|-----------------|
| Not stopping canvases | Clean up with `canvas_stop` |
| Content is replaced | Update = full content replacement |
| No styling | Add inline CSS for readability |
| Too frequent updates | Balance update frequency |
| Missing ID storage | Save ID from `canvas_create` response |

## Canvas vs Message

| Use Canvas | Use Message |
|------------|-------------|
| Long-running status | Quick update |
| Live updates | Static content |
| Dashboard display | Simple text |
| Progress tracking | One-time info |
| Interactive content | Read-only info |
