---
name: browser
description: "Automate web browsing, scraping, and form interaction"
trigger: automatic
---

# Browser Automation

Navigate websites, interact with elements, capture screenshots, and extract content.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      Agent Context                          │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
                  ┌─────────────────┐
                  │ browser_navigate│
                  │   (load page)   │
                  └────────┬────────┘
                           │
        ┌──────────────────┼──────────────────┐
        │                  │                  │
        ▼                  ▼                  ▼
┌───────────────┐  ┌───────────────┐  ┌───────────────┐
│ browser_click │  │ browser_fill  │  │ browser_wait  │
│ (interact)    │  │ (input text)  │  │ (for content) │
└───────────────┘  └───────────────┘  └───────────────┘
        │                  │                  │
        └──────────────────┼──────────────────┘
                           │
        ┌──────────────────┼──────────────────┐
        │                  │                  │
        ▼                  ▼                  ▼
┌───────────────┐  ┌───────────────┐  ┌───────────────┐
│browser_content│  │browser_screenshot│ │  browser_back │
│ (extract text)│  │   (capture)     │ │   (navigate)  │
└───────────────┘  └───────────────┘  └───────────────┘
```

## Tools

| Tool | Action | Use When |
|------|--------|----------|
| `browser_navigate` | Go to URL | Starting browsing session |
| `browser_screenshot` | Capture page | Visual verification, debugging |
| `browser_content` | Get page text | Extract data, read content |
| `browser_click` | Click element | Navigate, submit, interact |
| `browser_fill` | Fill form field | Enter text in inputs |
| `browser_wait` | Wait for content | Page loads, dynamic content |
| `browser_back` | Go back | Navigate to previous page |

## Workflow Pattern

```
1. NAVIGATE → browser_navigate(url="https://example.com")
2. WAIT     → browser_wait(text="Expected content")
3. INTERACT → browser_click(ref="button-id") or browser_fill(...)
4. EXTRACT  → browser_content() or browser_screenshot()
```

## Navigation

```bash
browser_navigate(url="https://example.com/products")
# Output: Navigated to https://example.com/products
# Page loaded successfully
```

## Waiting for Content

Always wait after navigation for dynamic content to load:

```bash
# Wait for specific text
browser_wait(text="Welcome")

# Wait for element
browser_wait(selector="#content")

# Wait for time
browser_wait(time=3)  # 3 seconds
```

## Getting Content

### Page Text
```bash
browser_content()
# Output: Full page text content...
# Includes all visible text from the page
```

### Screenshot
```bash
browser_screenshot(filename="page.png")
# Output: Screenshot saved to page.png
# Returns: base64-encoded image
```

## Interacting with Elements

### Finding Element References

After `browser_content()`, elements are shown with refs:

```
[button ref="submit-btn"] Submit [/button]
[input ref="email" type="email"]
[link ref="login-link"] Login [/link]
[textarea ref="comments"]
```

### Clicking
```bash
browser_click(ref="submit-btn")
# Output: Clicked element: submit-btn

browser_click(ref="login-link")
# Output: Clicked element: login-link
```

### Filling Forms
```bash
browser_fill(ref="email", value="user@example.com")
# Output: Filled email with: user@example.com

browser_fill(ref="password", value="secret123")
# Output: Filled password with: ********
```

## Common Patterns

### Login Flow
```bash
# 1. Navigate to login page
browser_navigate(url="https://example.com/login")

# 2. Wait for form
browser_wait(text="Email")

# 3. Fill credentials
browser_fill(ref="email", value="user@example.com")
browser_fill(ref="password", value="secret123")

# 4. Submit
browser_click(ref="submit-btn")

# 5. Wait for result
browser_wait(text="Dashboard")
```

### Search and Extract
```bash
# 1. Navigate to search
browser_navigate(url="https://example.com/search")

# 2. Fill search
browser_fill(ref="search-input", value="golang tutorial")
browser_click(ref="search-btn")

# 3. Wait for results
browser_wait(text="Results")

# 4. Extract content
browser_content()
# Parse results from content...
```

### Form Submission
```bash
# 1. Navigate to form
browser_navigate(url="https://example.com/contact")

# 2. Fill all fields
browser_fill(ref="name", value="John Doe")
browser_fill(ref="email", value="john@example.com")
browser_fill(ref="message", value="Hello, this is my message.")

# 3. Submit
browser_click(ref="submit")

# 4. Verify submission
browser_wait(text="Thank you")
```

### Multi-Page Navigation
```bash
# Navigate through pages
browser_navigate(url="https://example.com/page1")
browser_content()

# Click to next page
browser_click(ref="next-page")
browser_wait(text="Page 2")
browser_content()

# Go back if needed
browser_back()
```

### Screenshot Verification
```bash
# Navigate and capture
browser_navigate(url="https://example.com/dashboard")
browser_wait(text="Dashboard")
browser_screenshot(filename="dashboard.png")

# Share with user
send_image(image_path="dashboard.png", caption="Current dashboard state")
```

## Troubleshooting

### "Element not found"

**Cause:** Element ref doesn't exist or page not fully loaded.

**Debug:**
```bash
# Get current content to see available refs
browser_content()

# Wait longer
browser_wait(time=3)
```

### "Page not loading"

**Cause:** Network issue, site down, or URL incorrect.

**Debug:**
```bash
# Try screenshot to see current state
browser_screenshot()

# Check with bash
bash(command="curl -I https://example.com")
```

### "Click had no effect"

**Cause:** Element not clickable, covered by overlay, or needs scrolling.

**Debug:**
```bash
# Screenshot to see page state
browser_screenshot()

# Try waiting first
browser_wait(time=2)
browser_click(ref="element")
```

### "Form not submitting"

**Cause:** Missing required fields or validation error.

**Debug:**
```bash
# Check content for error messages
browser_content()

# Screenshot to see visual errors
browser_screenshot()
```

### "Content is empty"

**Cause:** Page requires login or JavaScript rendering.

**Debug:**
```bash
# Wait longer for JS
browser_wait(time=5)

# Check if login needed
browser_content()
```

## Tips

- **Always wait after navigate**: Pages need time to load
- **Use screenshots for debugging**: See what the browser sees
- **Extract refs from content**: Don't guess element IDs
- **Chain operations**: navigate → wait → interact → extract
- **Session persists**: Stay logged in across calls
- **Handle errors gracefully**: Sites may change or fail

## Common Mistakes

| Mistake | Correct Approach |
|---------|-----------------|
| Not waiting after navigate | Always `browser_wait()` first |
| Guessing element refs | Get refs from `browser_content()` |
| Not handling dynamic content | Wait for specific text/elements |
| Ignoring screenshots | Use for visual debugging |
| Not checking for login | Verify session state |

## Browser vs Bash curl

| Use Browser | Use curl/bash |
|-------------|---------------|
| JavaScript-heavy sites | Static HTML pages |
| Need to login | Public APIs |
| Form interactions | Quick content fetch |
| Screenshots needed | JSON responses |
| Complex navigation | Simple GET requests |
