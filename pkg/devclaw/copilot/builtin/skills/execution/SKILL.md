---
name: execution
description: "Run shell commands locally and on remote machines"
trigger: automatic
---

# Execution

Execute shell commands locally with full system access or on remote machines via SSH.

## Tools

| Tool | Action |
|------|--------|
| `bash` | Execute shell command locally |
| `exec` | Execute in sandboxed environment |
| `ssh` | Execute command on remote machine |
| `scp` | Copy files to/from remote machine |
| `set_env` | Set persistent environment variable |

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
# Output: On branch main, working tree clean...

bash(command="npm install && npm run build")
# Output: Build output...

bash(command="docker ps -a")
# Output: Container list...
```

### Environment Variables
```bash
# Set variable for session
set_env(name="NODE_ENV", value="production")

# Use in subsequent bash calls
bash(command="echo $NODE_ENV")
# Output: production
```

## Remote Execution

### SSH Commands
```bash
ssh(host="production-server", command="systemctl status nginx")
# Output: nginx service status...

ssh(host="user@192.168.1.100", command="df -h")
# Output: Disk usage...
```

### File Transfer with SCP
```bash
# Upload to remote
scp(
  source="/local/file.txt",
  dest="user@server:/remote/path/file.txt"
)

# Download from remote
scp(
  source="user@server:/remote/file.txt",
  dest="/local/path/file.txt"
)
```

## SSH Configuration

Uses `~/.ssh/config` for host aliases:

```
# ~/.ssh/config
Host production-server
    HostName 192.168.1.100
    User deploy
    IdentityFile ~/.ssh/id_rsa

Host staging
    HostName staging.example.com
    User ubuntu
```

Then use short names:
```bash
ssh(host="production-server", command="uptime")
ssh(host="staging", command="docker logs app")
```

## Workflow Examples

### Git Operations
```bash
# Check status
bash(command="git status")

# Create branch and commit
bash(command="git checkout -b feature/new-api")
bash(command="git add .")
bash(command='git commit -m "Add new API endpoint"')

# Push
bash(command="git push origin feature/new-api")
```

### Build and Deploy
```bash
# Local build
bash(command="npm run build")

# Deploy to server
scp(source="./dist/", dest="production-server:/var/www/app/")

# Restart service
ssh(host="production-server", command="sudo systemctl restart nginx")
```

### Docker Workflow
```bash
# Build image
bash(command="docker build -t myapp:latest .")

# Run container
bash(command="docker run -d -p 8080:80 myapp:latest")

# Check logs
bash(command="docker logs $(docker ps -q -f ancestor=myapp:latest)")
```

### Remote Debugging
```bash
# Check server status
ssh(host="production-server", command="systemctl status myapp")

# View logs
ssh(host="production-server", command="journalctl -u myapp -n 100")

# Check resources
ssh(host="production-server", command="free -h && df -h")
```

## Important Notes

| Note | Reason |
|------|--------|
| Working directory persists | `cd` affects subsequent calls |
| SSH uses your keys | Must have access configured |
| `bash` has full access | Runs as your user |
| `exec` is sandboxed | Limited for safety |
| `set_env` is session-only | Not persisted across restarts |

## Common Patterns

### Conditional Execution
```bash
bash(command="test -f config.json && echo 'exists' || echo 'missing'")
```

### Background Process
```bash
bash(command="nohup npm start > /dev/null 2>&1 &")
```

### Piped Commands
```bash
bash(command="cat logs.txt | grep ERROR | wc -l")
```

## Common Mistakes

| Mistake | Correct Approach |
|---------|-----------------|
| Not quoting commands with special chars | Use proper quoting |
| Forgetting `cd` doesn't persist in single command | Chain with `&&` |
| SSH without configured host | Use full `user@host` or configure `~/.ssh/config` |
| Running long commands synchronously | Consider timeouts or background execution |
