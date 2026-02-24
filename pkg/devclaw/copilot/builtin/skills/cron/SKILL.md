---
name: cron
description: "Schedule recurring tasks and automated jobs"
trigger: automatic
---

# Cron Scheduler

Schedule recurring tasks to run automatically at specified intervals.

## Tools

| Tool | Action |
|------|--------|
| `cron_add` | Add a new scheduled job |
| `cron_list` | List all scheduled jobs |
| `cron_remove` | Remove a scheduled job |

## When to Use

| Tool | When |
|------|------|
| `cron_add` | User wants to automate a recurring task |
| `cron_list` | Check what jobs are scheduled |
| `cron_remove` | User wants to stop automation |

## Cron Expression Format

Uses standard cron syntax: `minute hour day month weekday`

```
┌───────────── minute (0-59)
│ ┌───────────── hour (0-23)
│ │ ┌───────────── day of month (1-31)
│ │ │ ┌───────────── month (1-12)
│ │ │ │ ┌───────────── day of week (0-6, 0=Sunday)
│ │ │ │ │
* * * * *
```

## Common Patterns

| Expression | Meaning |
|------------|---------|
| `*/5 * * * *` | Every 5 minutes |
| `0 * * * *` | Every hour |
| `0 9 * * *` | Every day at 9:00 AM |
| `0 9 * * 1` | Every Monday at 9:00 AM |
| `0 9 1 * *` | First day of month at 9:00 AM |
| `0 9,17 * * *` | Twice daily (9 AM and 5 PM) |

## Examples

### Add a scheduled job
```bash
cron_add(
  name="daily-report",
  schedule="0 9 * * *",
  command="generate_daily_report"
)
# Output: Job 'daily-report' added. Runs at 0 9 * * *
```

### List all jobs
```bash
cron_list()
# Output:
# Scheduled jobs (2):
# - daily-report: 0 9 * * * → generate_daily_report
# - health-check: */5 * * * * → check_system_health
```

### Remove a job
```bash
cron_remove(name="daily-report")
# Output: Job 'daily-report' removed.
```

## Workflow Example

```
User: "I want to run a backup every night at 2 AM"

1. cron_add(
     name="nightly-backup",
     schedule="0 2 * * *",
     command="backup_database"
   )
2. cron_list() - verify job was added
3. Job runs automatically every night
```

## Important Notes

| Note | Reason |
|------|--------|
| Jobs persist across restarts | Stored in configuration |
| Use descriptive names | Helps identify jobs later |
| Verify with cron_list | Confirm job was added correctly |
| Test commands first | Run manually before scheduling |

## Common Mistakes

| Mistake | Correct Approach |
|---------|-----------------|
| `* * * * *` (every minute) | Use `*/5 * * * *` or longer interval |
| Forgetting to verify | Always run `cron_list` after `cron_add` |
| Vague job names | Use `db-backup-daily` not `backup1` |
