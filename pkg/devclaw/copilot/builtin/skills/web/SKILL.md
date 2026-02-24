---
name: web
description: "Search the web and fetch content from URLs"
trigger: automatic
---

# Web Tools

Search the web for current information and fetch content from URLs.

## Tools

| Tool | Action |
|------|--------|
| `web_search` | Search the web for information |
| `web_fetch` | Fetch and read content from a URL |

## When to Use

| Tool | When |
|------|------|
| `web_search` | Need current information, news, documentation |
| `web_fetch` | Read specific webpage, API response, or document |

## Web Search

Search the web to find current information, documentation, or answers to questions.

```bash
web_search(query="latest golang best practices 2026")
# Output:
# [1] Go Best Practices 2026 - go.dev/blog
#     Official Go blog with updated guidelines...
# [2] Effective Go - golang.org
#     Comprehensive guide to writing idiomatic Go...
```

### Search Options
```bash
web_search(
  query="react hooks tutorial",
  num_results=5
)
```

## Web Fetch

Fetch and read content from a specific URL.

```bash
web_fetch(url="https://go.dev/blog/error-handling")
# Output: Full article content in text format...
```

### Fetch Options
```bash
web_fetch(
  url="https://api.example.com/data",
  timeout=30
)
```

## Workflow Examples

### Research a Topic
```bash
# 1. Search for information
web_search(query="postgresql performance tuning")

# 2. Fetch relevant article
web_fetch(url="https://example.com/postgres-tuning-guide")

# 3. Summarize findings for user
```

### Check Documentation
```bash
# 1. Search for official docs
web_search(query="docker compose v3 syntax reference")

# 2. Fetch the documentation page
web_fetch(url="https://docs.docker.com/compose/compose-file/")

# 3. Help user with syntax questions
```

### API Research
```bash
# 1. Find API documentation
web_search(query="stripe api create customer")

# 2. Fetch API reference
web_fetch(url="https://stripe.com/docs/api/customers/create")

# 3. Implement based on documentation
```

### News and Updates
```bash
# Search for recent news
web_search(query="kubernetes 1.30 release notes")

# Fetch release announcement
web_fetch(url="https://kubernetes.io/blog/...")
```

## Best Practices

| Practice | Reason |
|----------|--------|
| Use specific queries | Better search results |
| Verify sources | Check credibility of information |
| Fetch full content | Search snippets are limited |
| Check dates | Ensure information is current |

## Common Patterns

### Quick Answer
```bash
web_search(query="how to reverse a string in python")
# Get direct answer from search results
```

### Deep Research
```bash
# 1. Broad search
web_search(query="microservices architecture patterns")

# 2. Fetch multiple relevant articles
web_fetch(url="https://article1.com/...")
web_fetch(url="https://article2.com/...")

# 3. Synthesize information
```

### Documentation Lookup
```bash
# 1. Find official docs
web_search(query="terraform aws provider documentation")

# 2. Fetch specific page
web_fetch(url="https://registry.terraform.io/providers/...")
```

## Important Notes

| Note | Reason |
|------|--------|
| Search results vary | Different results for same query over time |
| Some sites block fetching | May need alternative sources |
| Content may be truncated | Very long pages might be cut |
| Always cite sources | Tell user where information came from |

## Common Mistakes

| Mistake | Correct Approach |
|---------|-----------------|
| Vague search queries | Be specific with keywords |
| Not verifying information | Cross-check important facts |
| Ignoring dates | Check when content was published |
| Not reading full article | Fetch and read completely |
