---
name: filesystem
description: "Read, write, and manage files on the local machine"
trigger: automatic
---

# Filesystem

Read, write, search, and manage files and directories on the machine.

## Tools

| Tool | Action |
|------|--------|
| `read_file` | Read file contents |
| `write_file` | Create or overwrite a file |
| `edit_file` | Replace specific text in a file |
| `list_files` | List directory contents |
| `search_files` | Search for text in files (grep) |
| `glob_files` | Find files by pattern |

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
# Output: File contents with line numbers
```

## Writing Files

```bash
write_file(
  path="/home/user/project/config.json",
  content='{"version": "1.0", "debug": true}'
)
# Output: File written successfully
```

## Editing Files

```bash
# Replace specific text
edit_file(
  path="/home/user/project/main.go",
  old_string="func main() {",
  new_string="func main() {\n\tlog.Println(\"Starting...\")"
)
# Output: File edited successfully
```

## Listing Files

```bash
list_files(path="/home/user/project")
# Output:
# - src/ (directory)
# - main.go (1.2KB, rw-r--r--)
# - config.json (256B, rw-r--r--)
# - README.md (3KB, rw-r--r--)
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
```

## Glob Pattern Matching

```bash
glob_files(
  path="/home/user/project",
  pattern="**/*.go"
)
# Output:
# - src/main.go
# - src/utils.go
# - src/api/handler.go
# - tests/main_test.go
```

## Workflow Examples

### Create New File
```bash
write_file(
  path="/home/user/project/hello.py",
  content='#!/usr/bin/env python3\nprint("Hello, World!")'
)
```

### Read and Modify
```bash
# 1. Read current content
read_file(path="/home/user/project/config.yaml")

# 2. Make precise edit
edit_file(
  path="/home/user/project/config.yaml",
  old_string="debug: false",
  new_string="debug: true"
)
```

### Find and Analyze
```bash
# 1. Find all Go files
glob_files(path="/home/user/project", pattern="**/*.go")

# 2. Search for specific pattern
search_files(path="/home/user/project", pattern="func.*Handler")

# 3. Read interesting file
read_file(path="/home/user/project/src/handler.go")
```

### Explore Project
```bash
# 1. List root directory
list_files(path="/home/user/project")

# 2. List subdirectory
list_files(path="/home/user/project/src")

# 3. Find configuration files
glob_files(path="/home/user/project", pattern="**/*.yaml")
```

## Important Notes

| Note | Reason |
|------|--------|
| Paths can be relative or absolute | Both work |
| `edit_file` requires exact match | `old_string` must exist exactly |
| `write_file` overwrites | Destroys existing content |
| `search_files` is recursive | Searches all subdirectories |
| Line limit on `read_file` | Large files may be truncated |

## Common Mistakes

| Mistake | Correct Approach |
|---------|-----------------|
| Using `write_file` for edits | Use `edit_file` for modifications |
| Wrong `old_string` | Copy exact text from `read_file` output |
| Forgetting to escape quotes | Use proper escaping in content strings |
| Not checking file exists | Use `list_files` first if unsure |
