---
name: filesystem
description: "Read, write, and manage files on the local machine"
trigger: automatic
---

# Filesystem

Read, write, search, and manage files and directories on the machine.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      Agent Context                          │
└──────────────────────────┬──────────────────────────────────┘
                           │
       ┌───────────────────┼───────────────────┐
       │                   │                   │
       ▼                   ▼                   ▼
┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│  read_file  │    │ write_file  │    │ edit_file   │
│  (100KB)    │    │ (overwrite) │    │ (replace)   │
└─────────────┘    └─────────────┘    └─────────────┘
       │                   │                   │
       └───────────────────┼───────────────────┘
                           │
       ┌───────────────────┼───────────────────┐
       │                   │                   │
       ▼                   ▼                   ▼
┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│  list_files │    │search_files │    │ glob_files  │
│  (ls -la)   │    │  (grep -r)  │    │  (find)     │
└─────────────┘    └─────────────┘    └─────────────┘
```

## Tools

| Tool | Action | Limit |
|------|--------|-------|
| `read_file` | Read file contents | 100KB max |
| `write_file` | Create or overwrite | Full replacement |
| `edit_file` | Replace specific text | Must match exactly |
| `list_files` | List directory | Recursive |
| `search_files` | Grep for text | Recursive |
| `glob_files` | Pattern matching | `**/*.go` syntax |

## When to Use

| Tool | When |
|------|------|
| `read_file` | Need to see file contents |
| `write_file` | Create new file or replace entirely |
| `edit_file` | Make precise changes to existing file |
| `list_files` | Explore directory structure |
| `search_files` | Find code/text across files |
| `glob_files` | Find files by name pattern |

## Reading Files

```bash
read_file(path="/home/user/project/main.go")
# Output:
#      1→ package main
#      2→
#      3→ func main() {
#      4→     fmt.Println("Hello")
#      5→ }
```

### With Limits
```bash
read_file(path="/var/log/syslog", offset=100, limit=50)
# Reads lines 100-150
```

## Writing Files

```bash
write_file(
  path="/home/user/project/config.json",
  content='{"version": "1.0", "debug": true}'
)
# Output: Successfully wrote to /home/user/project/config.json
```

## Editing Files

**CRITICAL**: `old_string` must match EXACTLY (including whitespace, indentation).

```bash
# Read first to get exact content
read_file(path="/home/user/project/main.go")

# Then edit with exact match
edit_file(
  path="/home/user/project/main.go",
  old_string="func main() {",
  new_string="func main() {\n\tlog.Println(\"Starting...\")"
)
# Output: Successfully edited /home/user/project/main.go
```

## Listing Files

```bash
list_files(path="/home/user/project")
# Output:
# - src/ (directory, 755)
# - main.go (1.2KB, 644, modified: 2026-02-24)
# - config.json (256B, 644, modified: 2026-02-20)
```

## Searching Files

```bash
search_files(
  path="/home/user/project",
  pattern="TODO",
  ignore_case=true
)
# Output:
# src/main.go:45: // TODO: implement error handling
# src/utils.go:12: // TODO: add tests
# src/api.go:78: // TODO: handle timeout
```

## Glob Pattern Matching

```bash
glob_files(
  path="/home/user/project",
  pattern="**/*.go"
)
# Output:
# src/main.go
# src/utils.go
# src/api/handler.go
# src/api/middleware.go
# tests/main_test.go
```

## Common Patterns

### Create New File
```bash
write_file(
  path="/home/user/project/hello.py",
  content='#!/usr/bin/env python3
def main():
    print("Hello, World!")

if __name__ == "__main__":
    main()'
)
```

### Read-Modify-Write Pattern
```bash
# 1. Read current content
read_file(path="/home/user/project/config.yaml")
# Output:
#      1→ debug: false
#      2→ log_level: info

# 2. Make precise edit
edit_file(
  path="/home/user/project/config.yaml",
  old_string="debug: false",
  new_string="debug: true"
)
```

### Find and Analyze
```bash
# 1. Find all test files
glob_files(path="/home/user/project", pattern="**/*_test.go")

# 2. Search for specific pattern
search_files(path="/home/user/project", pattern="func Test")

# 3. Read interesting file
read_file(path="/home/user/project/src/api_test.go")
```

### Explore New Project
```bash
# 1. List root
list_files(path="/home/user/unknown-project")
# Output: src/, cmd/, go.mod, README.md

# 2. Find entry point
glob_files(path="/home/user/unknown-project", pattern="**/main.go")

# 3. Read entry point
read_file(path="/home/user/unknown-project/cmd/main.go")
```

### Search and Replace Across Files
```bash
# 1. Find occurrences
search_files(path="/home/user/project", pattern="oldFunction")

# 2. Edit each file (repeat for each)
edit_file(
  path="/home/user/project/src/file1.go",
  old_string="oldFunction",
  new_string="newFunction"
)
```

## Troubleshooting

### "file not found" error

**Cause:** Path doesn't exist or is incorrect.

**Debug:**
```bash
# Check parent directory
list_files(path="/home/user/project")

# Use absolute paths when unsure
read_file(path="/home/user/project/file.txt")
```

### "old_string not found" error

**Cause:** The text to replace doesn't match exactly.

**Debug:**
```bash
# Read file to see exact content (including whitespace)
read_file(path="/home/user/project/file.go")

# Copy exact text from output, preserving indentation
```

### "content truncated" warning

**Cause:** File exceeds 100KB limit.

**Solution:**
```bash
# Use offset and limit to read in chunks
read_file(path="/large/file.log", offset=1, limit=1000)
```

### Permission denied

**Cause:** File or directory not readable.

**Debug:**
```bash
# Check permissions in list_files output
list_files(path="/home/user/project")
# Look at the mode column (e.g., 644, 755)
```

## Tips

- **Always read before edit**: Ensures exact match for `old_string`
- **Use absolute paths**: Avoids confusion about working directory
- **Glob before search**: Narrow down files to search
- `write_file` overwrites: Destroys existing content completely
- `search_files` is case-sensitive: Use `ignore_case=true` for flexibility
- Line numbers in `read_file`: Use offset starting from 1

## Common Mistakes

| Mistake | Correct Approach |
|---------|-----------------|
| Using `write_file` for small edits | Use `edit_file` for modifications |
| Guessing `old_string` | Copy exact text from `read_file` output |
| Forgetting to escape quotes | Use proper escaping or different quotes |
| Relative path confusion | Use absolute paths when unsure |
| Not checking if file exists | Use `list_files` first |
| `offset=0` | Line numbers start at 1, use `offset=1` |
