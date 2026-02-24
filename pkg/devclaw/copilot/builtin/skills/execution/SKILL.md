---
name: execution
description: "Run shell commands locally and on remote machines via SSH"
trigger: automatic
---

# Execution

Execute shell commands locally with full system access or on remote machines via SSH.

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
│     bash      │                    │      ssh      │
│  (local shell)│                    │  (remote via  │
│               │                    │   ~/.ssh/config)
└───────────────┘                    └───────────────┘
        │                                     │
        │                             ┌───────┴───────┐
        │                             │               │
        ▼                             ▼               ▼
┌───────────────┐            ┌───────────────┐ ┌───────────────┐
│    set_env    │            │      scp      │ │     exec      │
│ (env vars for │            │ (file transfer│ │  (sandboxed)  │
│   session)    │            │   to remote)  │ │               │
└───────────────┘            └───────────────┘ └───────────────┘
```

## Tools

| Tool | Action | Access Level |
|------|--------|--------------|
| `bash` | Execute shell command locally | Full user access |
| `exec` | Execute in sandboxed environment | Limited |
| `ssh` | Execute command on remote machine | Via SSH config |
| `scp` | Copy files to/from remote machine | Via SSH config |
| `set_env` | Set persistent environment variable | Session only |

## When to Use

| Tool | When |
|------|------|
| `bash` | Local operations, git, docker, npm, builds |
| `exec` | Safer execution with limits |
| `ssh` | Run command on remote server |
| `scp` | Transfer files to/from remote |
| `set_env` | Set variable for subsequent commands |

## Local Execution

### Bash Commands
```bash
bash(command="git status")
# Output:
# On branch main
# Your branch is up to date with 'origin/main'.
# nothing to commit, working tree clean

bash(command="npm install && npm run build")
# Output: Build output...

bash(command="docker ps -a")
# Output:
# CONTAINER ID   IMAGE     STATUS    NAMES
# abc123def456   nginx     Up 2h     web-server
```

### Chaining Commands
```bash
# Sequential with && (stops on error)
bash(command="cd /project && npm install && npm test")

# Sequential with ; (continues on error)
bash(command="cd /project; npm install; npm test")

# Conditional execution
bash(command="test -f config.json && echo 'exists' || echo 'missing'")
```

### Environment Variables
```bash
# Set variable for session
set_env(name="NODE_ENV", value="production")

# Use in subsequent bash calls
bash(command="echo $NODE_ENV")
# Output: production

# Multiple variables
set_env(name="API_URL", value="https://api.example.com")
set_env(name="DEBUG", value="true")
```

## Remote Execution

### SSH Configuration

Uses `~/.ssh/config` for host aliases:

```
# ~/.ssh/config
Host production
    HostName 192.168.1.100
    User deploy
    IdentityFile ~/.ssh/id_rsa
    Port 22

Host staging
    HostName staging.example.com
    User ubuntu
    IdentityFile ~/.ssh/id_ed25519

Host *
    ServerAliveInterval 60
    Compression yes
```

### SSH Commands
```bash
# Using host alias from config
ssh(host="production", command="systemctl status nginx")
# Output:
# ● nginx.service - A high performance web server
#    Active: active (running) since Mon 2026-02-24 09:00:00 UTC

# Direct connection
ssh(host="user@192.168.1.50", command="df -h")
# Output:
# Filesystem      Size  Used Avail Use% Mounted on
# /dev/sda1       100G   45G   55G  45% /

# With port
ssh(host="user@example.com:2222", command="uptime")
```

### SCP File Transfer
```bash
# Upload to remote
scp(
  source="/local/project.tar.gz",
  dest="production:/home/deploy/project.tar.gz"
)
# Output: project.tar.gz  100%   15MB   5.0MB/s   00:03

# Download from remote
scp(
  source="production:/var/log/nginx/access.log",
  dest="/local/logs/access.log"
)
# Output: access.log  100% 2456KB   3.2MB/s   00:01

# Copy directory (recursive)
scp(
  source="/local/dist/",
  dest="production:/var/www/html/"
)
```

## Common Patterns

### Git Workflow
```bash
# Check status
bash(command="git status")

# Create branch
bash(command="git checkout -b feature/new-api")

# Stage and commit
bash(command="git add .")
bash(command='git commit -m "Add new API endpoint"')

# Push
bash(command="git push -u origin feature/new-api")

# Merge back
bash(command="git checkout main && git merge feature/new-api")
```

### Docker Workflow
```bash
# Build image
bash(command="docker build -t myapp:v1.0 .")

# Run container
bash(command="docker run -d -p 8080:80 --name web myapp:v1.0")

# Check logs
bash(command="docker logs -f web")

# Stop and remove
bash(command="docker stop web && docker rm web")
```

### Build and Deploy
```bash
# Local build
bash(command="npm run build")

# Upload to server
scp(source="./dist/", dest="production:/var/www/app/")

# Restart service on server
ssh(host="production", command="sudo systemctl restart nginx")

# Verify
ssh(host="production", command="curl -I localhost")
```

### Remote Debugging
```bash
# Check service status
ssh(host="production", command="systemctl status myapp")

# View recent logs
ssh(host="production", command="journalctl -u myapp -n 100 --no-pager")

# Check resources
ssh(host="production", command="free -h && df -h")

# Check network
ssh(host="production", command="ss -tlnp | grep 8080")
```

### Background Processes
```bash
# Start in background
bash(command="nohup npm start > app.log 2>&1 &")

# Check if running
bash(command="ps aux | grep node")

# View output
bash(command="tail -f app.log")
```

## Troubleshooting

### Command not found

**Cause:** Binary not in PATH or not installed.

**Debug:**
```bash
bash(command="which node")
bash(command="echo $PATH")
```

### SSH connection refused

**Cause:** Host unreachable, wrong port, or SSH not running.

**Debug:**
```bash
# Test connectivity
bash(command="ping server.example.com")

# Test SSH port
bash(command="nc -zv server.example.com 22")

# Check SSH config
bash(command="cat ~/.ssh/config")
```

### Permission denied (SSH)

**Cause:** Wrong key, wrong user, or server rejected key.

**Debug:**
```bash
# Test SSH manually
bash(command="ssh -v production 'echo connected'")

# Check key permissions
bash(command="ls -la ~/.ssh/")
# Should be 600 for private keys, 644 for public keys
```

### SCP transfer failed

**Cause:** Disk full, permission denied, or path doesn't exist.

**Debug:**
```bash
# Check remote disk space
ssh(host="production", command="df -h")

# Check remote permissions
ssh(host="production", command="ls -la /var/www/")

# Check local file exists
bash(command="ls -la /local/file.tar.gz")
```

### Command timeout

**Cause:** Long-running command without background.

**Solution:**
```bash
# Run in background
bash(command="long-command &")

# Or use nohup
bash(command="nohup long-command > output.log 2>&1 &")
```

## Tips

- **Working directory persists**: `cd` affects subsequent calls in same session
- **Quote commands properly**: Use single quotes when command contains `$` or special chars
- **Use `&&` for sequences**: Stops on first error
- **SSH uses your keys**: Must have access configured in `~/.ssh/config`
- **SCP overwrites**: Destination files are replaced silently
- **Check before transfer**: Verify source exists and destination is writable

## Common Mistakes

| Mistake | Correct Approach |
|---------|-----------------|
| Not quoting `$` variables | `bash(command='echo "$HOME"')` |
| `cd` in separate command | Chain with `&&`: `cd /dir && command` |
| SSH without configured host | Use `user@host` or add to `~/.ssh/config` |
| Long command without `&` | Background it: `command &` |
| Forgetting `set_env` is session-only | Not persisted across restarts |
