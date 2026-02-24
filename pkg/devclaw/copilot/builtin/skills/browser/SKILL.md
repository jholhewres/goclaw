---
name: browser
description: "Automate web browsing, scraping, and form interaction"
trigger: automatic
---

# Browser Automation

Navigate websites, interact with elements, capture screenshots, and extract content.

## Tools

| Tool | Action |
|------|--------|
| `browser_navigate` | Navigate to a URL |
| `browser_screenshot` | Capture current page |
| `browser_content` | Get page content/text |
| `browser_click` | Click an element |
| `browser_fill` | Fill a form field |
| `browser_wait` | Wait for element/content |
| `browser_back` | Go back in history |

## When to Use

| Tool | When |
|------|------|
| `browser_navigate` | Start browsing a website |
| `browser_screenshot` | Visual verification, debugging |
| `browser_content` | Extract text, scrape data |
| `browser_click` | Navigate, submit forms |
| `browser_fill` | Enter text in inputs |
| `browser_wait` | Page loads, dynamic content |

## Workflow Pattern

```
1. NAVIGATE → browser_navigate(url="https://example.com")
2. WAIT     → browser_wait(text="Welcome")
3. INTERACT → browser_click(ref="login-button")
4. FILL     → browser_fill(ref="email", value="user@example.com")
5. CAPTURE  → browser_screenshot() or browser_content()
```

## Examples

### Basic Navigation and Scraping
```bash
# Navigate to site
browser_navigate(url="https://example.com/products")

# Wait for content to load
browser_wait(text="Products")

# Get page content
browser_content()
# Output: Full page text content...

# Take screenshot
browser_screenshot(filename="products-page.png")
```

### Form Interaction
```bash
# Navigate to login page
browser_navigate(url="https://example.com/login")

# Fill credentials
browser_fill(ref="email", value="user@example.com")
browser_fill(ref="password", value="secret123")

# Submit form
browser_click(ref="submit-button")

# Wait for result
browser_wait(text="Welcome")
```

### Data Extraction
```bash
browser_navigate(url="https://example.com/articles")
browser_wait(text="Articles")

# Extract content
content = browser_content()
# Parse content for specific data...
```

## Element References

After navigation, use `browser_content()` or snapshot to find element references:

```
[button id="submit-btn"] Submit [/button]
[input id="email" type="email"]
[link href="/dashboard"] Dashboard [/link]
```

Use these refs in `browser_click(ref="submit-btn")` or `browser_fill(ref="email", value="...")`.

## Best Practices

| Practice | Reason |
|----------|--------|
| Always wait after navigate | Pages need time to load |
| Use screenshots for debugging | See what the browser sees |
| Extract refs from content | Don't guess element IDs |
| Handle errors gracefully | Sites may change or fail |

## Common Patterns

### Login Flow
```bash
browser_navigate(url="https://site.com/login")
browser_fill(ref="username", value="myuser")
browser_fill(ref="password", value="mypass")
browser_click(ref="login")
browser_wait(text="Dashboard")
```

### Search and Extract
```bash
browser_navigate(url="https://site.com/search?q=query")
browser_wait(text="Results")
browser_content()
# Extract relevant data from content
```

### Multi-page Navigation
```bash
browser_navigate(url="https://site.com/page1")
browser_click(ref="next-page")
browser_wait(text="Page 2")
browser_content()
```

## Important Notes

| Note | Reason |
|------|--------|
| Sessions persist | Stay logged in across calls |
| Elements may change | Sites update, verify refs |
| Respect robots.txt | Don't scrape restricted pages |
| Rate limiting | Don't overload servers |
