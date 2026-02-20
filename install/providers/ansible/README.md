# DevClaw Ansible Deployment

Deploy DevClaw to a Linux server using Ansible.

## Requirements

- Ansible 2.9+
- Target server with SSH access
- Python 3 on target server

## Quick Start

1. Copy and edit inventory:
```bash
cp inventory.example inventory
# Edit inventory with your server details
```

2. Run the playbook:
```bash
ansible-playbook -i inventory playbook.yml
```

3. Access DevClaw:
```
http://your-server:8085/setup
```

## Configuration

### Inventory Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `devclaw_version` | `latest` | Version to install |
| `devclaw_user` | `devclaw` | System user |
| `devclaw_home` | `/home/devclaw` | Home directory |
| `devclaw_port_api` | `8085` | API port |
| `devclaw_port_web` | `8090` | Web UI port |
| `devclaw_env` | `{}` | Environment variables |

### Example Inventory

```ini
[devclaw_servers]
production ansible_host=192.168.1.100

[devclaw_servers:vars]
ansible_user=ubuntu
ansible_ssh_private_key_file=~/.ssh/id_rsa
devclaw_version=v1.6.1
devclaw_env={"TZ": "UTC", "LOG_LEVEL": "debug"}
```

### Environment Variables

Set environment variables via the `devclaw_env` dictionary:

```yaml
devclaw_env:
  TZ: "America/New_York"
  LOG_LEVEL: "info"
```

## Supported Platforms

- Debian 11+
- Ubuntu 20.04+
- RHEL/CentOS 8+
- Fedora 35+

## Managing the Service

```bash
# Check status
ansible devclaw_servers -i inventory -m systemd -a "name=devclaw state=started"

# Restart
ansible devclaw_servers -i inventory -m systemd -a "name=devclaw state=restarted"

# Stop
ansible devclaw_servers -i inventory -m systemd -a "name=devclaw state=stopped"

# View logs
ansible devclaw_servers -i inventory -m command -a "journalctl -u devclaw -f" -b
```

## Upgrading

Re-run the playbook with a new version:

```bash
ansible-playbook -i inventory playbook.yml -e "devclaw_version=v1.7.0"
```

## Uninstalling

```bash
# Stop and disable service
ansible devclaw_servers -i inventory -m systemd -a "name=devclaw state=stopped enabled=false"

# Remove files
ansible devclaw_servers -i inventory -m file -a "path=/etc/systemd/system/devclaw.service state=absent"
ansible devclaw_servers -i inventory -m file -a "path=/usr/local/bin/devclaw state=absent"

# Remove user (optional, keeps data)
ansible devclaw_servers -i inventory -m user -a "name=devclaw state=absent"
```

## Troubleshooting

### Check Service Logs
```bash
journalctl -u devclaw -f
```

### Check Binary
```bash
/usr/local/bin/devclaw --version
```

### Manual Run
```bash
sudo -u devclaw /usr/local/bin/devclaw serve
```
