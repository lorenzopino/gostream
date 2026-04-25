# GoStream AI Maintenance — Dispatcher Skill

## Purpose
Receive IssueBatch payloads from GoStream via webhook, manage the dispatch queue, and spawn isolated subagents for processing.

## Triggers
1. **Webhook** — POST from GoStream with IssueBatch JSON
2. **Cron** — Every 30 minutes, trigger deep scan

## Workflow

### On Webhook Receive
1. Parse the IssueBatch JSON payload
2. Validate: must have `id`, `issues` (non-empty array), `created`, `source` fields
3. Check: "Is a GoStream maintenance subagent already running?"
   - How to check: Look for active subagent sessions with "gostream-maintenance" in the prompt, or check if `~/.hermes/gostream-active.lock` exists
4. **If busy**:
   - Append batch to `~/.hermes/gostream-queue.json`
   - Format: `{"batch": <IssueBatch>, "status": "pending", "priority": "<highest_priority_letter>", "enqueued_at": "<ISO timestamp>"}`
   - Priority = highest priority letter from the batch's issues (B=1, A=2, C=3, D=4)
   - Log: "GoStream batch {id} queued ({N} issues, priority {P})"
   - Do NOT reply to webhook (GoStream has retry logic)
5. **If free**:
   - Create `~/.hermes/gostream-active.lock` (empty file)
   - Spawn isolated subagent with the IssueBatch payload
   - Subagent instructions: "Activate the gostream-maintenance skill. Here is your batch: {JSON}"
   - Log: "GoStream batch {id} dispatched ({N} issues, priority {P})"

### On Cron (every 30 min)
1. Check if queue has pending batches
2. If yes: dispatch the highest-priority pending batch
3. If no: trigger deep scan by calling the gostream-deep-scan skill
4. Log: "Deep scan triggered at {timestamp}"

### On Subagent Complete
1. Remove `~/.hermes/gostream-active.lock`
2. Update batch status to "complete" in queue file
3. Log result: "GoStream batch {id} completed: {summary}"
4. Notify user via Telegram gateway with result summary

## Queue File Format

`~/.hermes/gostream-queue.json`:
```json
{
  "entries": [
    {
      "batch": {"id": "...", "issues": [...], "created": "...", "source": "..."},
      "status": "pending|processing|complete|failed",
      "priority": "B",
      "enqueued_at": "2026-04-25T09:00:00Z",
      "retry_count": 0
    }
  ]
}
```

## Priority Ordering
- B (1) > A (2) > C (3) > D (4)
- Same priority: FIFO order

## Error Handling
- If webhook payload is invalid JSON: log error, respond 400
- If queue file is corrupted: create fresh file, log warning
- If subagent crashes: mark batch as "failed", increment retry_count, retry if < 3
- If lock file exists but no subagent is running (stale lock): remove lock, log warning, proceed

## Resource Requests
If the dispatcher identifies a need (missing API key, unconfigured endpoint, etc.):
1. Log the need
2. Send Telegram message via gateway: "🔧 I need: {what} to {why}"
3. Do NOT block processing — continue with what's available
