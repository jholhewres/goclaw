---
name: web
description: "Search the web for current information and fetch content from URLs"
trigger: automatic
---

# Web Tools

Search the web for current information and fetch content from URLs.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      Agent Context                          │
└──────────────────────────┬──────────────────────────────────┘
                           │
        ┌──────────────────┴──────────────────┐
        │                                     │
        ▼                                     ▼
┌───────────────┐                    ┌───────────────┐
│   web_search  │                    │   web_fetch   │
│  (search API) │                    │ (HTTP client) │
└───────┬───────┘                    └───────┬───────┘
        │                                     │
        ▼                                     ▼
┌───────────────┐                    ┌───────────────┐
│ Search Engine │                    │   Web Server  │
│  (Google, etc)│                    │  (Any URL)    │
└───────────────┘                    └───────────────┘
        │                                     │
        ▼                                     ▼
┌───────────────┐                    ┌───────────────┐
│  Results with │                    │  Page content │
│   snippets    │                    │  (text/html)  │
└───────────────┘                    └───────────────┘
```

## Tools

| Tool | Action | Use When |
|------|--------|----------|
| `web_search` | Search the web | Need current info, news, docs |
| `web_fetch` | Fetch URL content | Have specific URL to read |

## When to Use

| Scenario | Tool |
|----------|------|
| "What's the latest version of Go?" | `web_search` |
| "Find React hooks tutorial" | `web_search` |
| "Read this article: https://..." | `web_fetch` |
| "Check the API docs at ..." | `web_fetch` |

## Web Search

Search the web to find current information, documentation, or answers.

```bash
web_search(query="golang generics tutorial 2026")
# Output:
# [1] Go Generics - go.dev/blog
#     Official Go blog explaining generics introduction...
#     https://go.dev/blog/intro-generics
#
# [2] Generics in Go - DigitalOcean
#     Complete guide with examples and best practices...
#     https://www.digitalocean.com/community/tutorials/...
```

### Search Options
```bash
web_search(
  query="docker compose v3 syntax",
  num_results=10
)
```

### Effective Search Queries
```bash
# Include year for current info
web_search(query="best practices react 2026")

# Include "documentation" for official docs
web_search(query="kubernetes deployment documentation")

# Include specific error messages
web_search(query="golang "cannot use as type" error")

# Include technology + feature
web_search(query="postgresql jsonb query examples")
```

## Web Fetch

Fetch and read content from a specific URL.

```bash
web_fetch(url="https://go.dev/blog/error-handling")
# Output:
# Error Handling in Go
# By Andrew Gerrand
#
# Introduction
# Go's approach to error handling is different from many
# other programming languages. Instead of exceptions...
```

### Fetch Options
```bash
web_fetch(
  url="https://api.example.com/docs",
  timeout=30
)
```

### What Gets Returned
- Text content from HTML pages
- May include markdown formatting
- Scripts and styles are stripped
- Very long pages may be truncated

## Common Patterns

### Research Workflow
```bash
# 1. Broad search
web_search(query="microservices patterns best practices")

# 2. Identify relevant URLs from results

# 3. Fetch full articles
web_fetch(url="https://blog.example.com/microservices-guide")

# 4. Synthesize information
```

### Documentation Lookup
```bash
# 1. Find official docs
web_search(query="terraform aws provider documentation")

# 2. Get specific page
web_fetch(url="https://registry.terraform.io/providers/hashicorp/aws/")

# 3. Find specific resource
web_fetch(url="https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/instance")
```

### API Research
```bash
# 1. Find API docs
web_search(query="stripe api create payment intent")

# 2. Fetch API reference
web_fetch(url="https://stripe.com/docs/api/payment_intents/create")

# 3. Implement based on docs
```

### News and Updates
```bash
# 1. Search for news
web_search(query="kubernetes 1.30 release notes")

# 2. Fetch announcement
web_fetch(url="https://kubernetes.io/blog/2026/kubernetes-1-30-release/")

# 3. Summarize key changes
```

### Error Investigation
```bash
# 1. Search for error
web_search(query="golang "connection refused" docker")

# 2. Find Stack Overflow / GitHub issues

# 3. Fetch solutions
web_fetch(url="https://stackoverflow.com/questions/...")
```

### Version Check
```bash
# Check latest version
web_search(query="node.js latest version 2026")

# Fetch release notes
web_fetch(url="https://nodejs.org/en/blog/release/")
```

## Troubleshooting

### "No results found"

**Cause:** Query too specific or unusual terms.

**Solution:**
```bash
# Simplify query
web_search(query="golang http client")  # Instead of very specific

# Remove special characters
web_search(query="docker network error")  # Instead of "docker network error!"
```

### "Failed to fetch URL"

**Cause:** Site blocks requests, requires auth, or is down.

**Debug:**
```bash
# Try alternative source
web_search(query="site:github.com topic")  # GitHub often accessible

# Check if site is up
bash(command="curl -I https://example.com")
```

### Content is truncated

**Cause:** Page is very long.

**Solution:**
```bash
# Search within the page topic for specific sections
web_search(query="site:docs.example.com specific-feature")
```

### Outdated information

**Cause:** Search returned old results.

**Solution:**
```bash
# Include year
web_search(query="react best practices 2026")

# Search official sources
web_search(query="site:react.dev hooks")
```

### Paywalled content

**Cause:** Site requires subscription.

**Solution:**
```bash
# Look for alternative sources
web_search(query="topic summary blog")

# Search GitHub for code examples
web_search(query="site:github.com topic")
```

## Tips

- **Be specific**: Better queries get better results
- **Include year**: For current best practices
- **Use `site:`**: Search specific domains
- **Fetch full content**: Search snippets are limited
- **Verify dates**: Check when content was published
- **Cross-reference**: Don't rely on single source
- **Cite sources**: Tell user where info came from

## Search Query Patterns

| Need | Query Pattern |
|------|---------------|
| Official docs | `site:docs.example.com topic` |
| GitHub code | `site:github.com language feature` |
| Tutorial | `"how to" technology task` |
| Error fix | `technology "exact error message"` |
| Comparison | `"vs" option1 option2` |
| Latest | `technology "latest version" 2026` |
| Best practices | `technology "best practices" 2026` |

## Common Mistakes

| Mistake | Correct Approach |
|---------|-----------------|
| Vague queries | Be specific with keywords and year |
| Only reading snippets | Fetch full content for details |
| Ignoring dates | Check publication date |
| Single source | Cross-reference important info |
| Not citing | Always tell user the source |
