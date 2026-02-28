---
name: cron
description: "Schedule jobs to run at specific times or intervals"
trigger: automatic
---

# Scheduler

Schedule **jobs** to execute commands at specific times or intervals.

## CRITICAL RULES

| Rule | Reason |
|-------|--------|
| **NEVER** create schedule without explicit request | Don't anticipate user needs |
| **ALWAYS** confirm details before scheduling | Avoid incorrect schedules |
| **NEVER** add "follow-up" or extra items | Only what was requested |
| **ONE** action at a time | Don't batch multiple schedules |

### Wrong
```
User: "I have a meeting at 15:30"
Agent: Creates reminder for 15:20 automatically
```

### Correct
```
User: "I have a meeting at 15:30"
Agent: "Got it. Would you like me to create a reminder?"

User: "Yes, 10 minutes before"
Agent: Creates schedule as requested
```

## Terminology

| Term | Meaning |
|-------|---------|
| **Job** | A scheduled task |
| **Tool** | `scheduler` (single dispatcher with actions) |
| **Types** | `at` (one-time), `every` (interval), `cron` (cron expression) |

## Actions

| Action | Description | Only Use When |
|--------|-------------|---------------|
| `add` | Create a new job | User EXPLICITLY asks |
| `list` | List all jobs | User asks or needs to verify |
| `remove` | Remove a job | User EXPLICITLY asks |
| `search` | Search reminders history | User asks about past reminders |

## Schedule Types

### Type: `at` (One-Time)
Executes once and auto-removes. Use for reminders and delayed tasks.

```bash
scheduler(
  action="add",
  id="meeting-reminder",
  type="at",
  schedule="30m",           # In 30 minutes
  command="Reminder: meeting in 5 minutes"
)

scheduler(
  action="add",
  id="callback-client",
  type="at",
  schedule="14:30",         # Today at 14:30
  command="Call client about proposal"
)

scheduler(
  action="add",
  id="review-friday",
  type="at",
  schedule="2026-02-28 09:00",  # Specific date
  command="Review weekly report"
)
```

### Type: `every` (Recurring Interval)
Executes repeatedly at fixed intervals.

```bash
scheduler(
  action="add",
  id="health-check",
  type="every",
  schedule="5m",            # Every 5 minutes
  command="Check server status"
)

scheduler(
  action="add",
  id="sync-data",
  type="every",
  schedule="1h",            # Every hour
  command="Sync data with external API"
)
```

### Type: `cron` (Cron Expression)
Executes based on standard cron expression.

```bash
scheduler(
  action="add",
  id="daily-report",
  type="cron",
  schedule="0 9 * * *",     # Every day at 9:00
  command="Generate daily report"
)

scheduler(
  action="add",
  id="weekly-backup",
  type="cron",
  schedule="0 2 * * 0",     # Sunday at 2:00
  command="Execute weekly backup"
)

scheduler(
  action="add",
  id="monthly-invoice",
  type="cron",
  schedule="0 9 1 * *",     # 1st of each month at 9:00
  command="Send monthly invoices"
)
```

## Cron Expressions

```
minute (0-59)
| hour (0-23)
| | day of month (1-31)
| | | month (1-12)
| | | | day of week (0-6, 0=Sunday)
| | | | |
* * * * *
```

| Expression | Meaning |
|-----------|-------------|
| `*/5 * * * *` | Every 5 minutes |
| `0 * * * *` | Every hour |
| `0 9 * * *` | Every day at 9:00 |
| `0 9 * * 1-5` | Weekdays at 9:00 |
| `0 9,17 * * *` | Twice daily (9am and 5pm) |
| `0 2 * * 0` | Sundays at 2:00 |
| `0 9 1 * *` | 1st of each month |

## List Jobs

```bash
scheduler(action="list")
# Output:
# Scheduled jobs (3):
# 1. [at] meeting-reminder: 30m -> "Reminder: meeting..."
# 2. [every] health-check: 5m -> "Check server status..."
# 3. [cron] daily-report: 0 9 * * * -> "Generate daily report..."
```

## Remove Job

**Only remove when user explicitly asks**

```bash
scheduler(action="remove", id="health-check")
# Output: Job 'health-check' removed
```

## Parameters for `scheduler(action=add)`

| Parameter | Required | Description |
|-----------|-------------|-----------|
| `id` | Yes | Unique identifier for the job |
| `type` | No | `at`, `every`, or `cron` (default: `cron`) |
| `schedule` | Yes | Time/interval (format depends on type) |
| `command` | Yes | Command/prompt to execute |
| `channel` | No | Response channel (e.g. `whatsapp`) |
| `chat_id` | No | Chat/group ID for response |

## Schedule Formats by Type

| Type | Format | Examples |
|------|---------|----------|
| `at` | Relative duration or absolute time | `5m`, `1h`, `14:30`, `2026-03-01 09:00` |
| `every` | Interval | `5m`, `30m`, `1h`, `1d` |
| `cron` | Cron expression | `0 9 * * *`, `*/10 * * * *` |

## Correct Workflow

### User asks for reminder
```bash
# User: "Remind me to call the client in 20 minutes"

# 1. Create EXACTLY what was requested
scheduler(
  action="add",
  id="call-reminder",
  type="at",
  schedule="20m",
  command="Call the client"
)
# Output: Job 'call-reminder' scheduled: 20m (at)

# 2. Confirm (DO NOT add extras)
send_message("Reminder created! I'll notify you in 20 minutes.")
```

### User mentions event (without asking for reminder)
```bash
# User: "I have a meeting at 15:30"

# DO NOT create automatically! Just acknowledge
send_message("Got it, meeting at 15:30.")

# Optionally offer:
send_message("Would you like me to create a reminder?")
```

### User asks to verify
```bash
# User: "What schedules do I have?"

scheduler(action="list")
# Show list...
```

## Searching for Reminders

Use `scheduler(action=search)` to find past or current reminders:

```bash
# Search for a specific reminder
scheduler(action="search", query="meeting")

# List all reminders (including removed)
scheduler(action="search", include_removed=true)

# List only active reminders
scheduler(action="search")
```

This is useful when user asks "what reminders did I have?" or "remove the reminder about X".

## Common Mistakes

| Mistake | Correction |
|---------|-----------|
| Creating schedule without request | Only create when explicitly requested |
| Adding unsolicited "follow-up" | Only what the user asked for |
| Using `cron` for one-time reminder | Use `type="at"` |
| Forgetting to specify `type` | Default is `cron`, may not be desired |
| Invalid schedule for type | Each type accepts different format |
| Duplicate ID | Overwrites existing job |
| Can't find a reminder | Use `scheduler(action=search)` to search history |
