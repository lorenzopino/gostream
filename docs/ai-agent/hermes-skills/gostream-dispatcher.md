# GoStream Dispatcher

**Trigger:** Webhook from GoStream OR cron job (~30 min deep scan)

## Webhook Handler

When a webhook POST arrives with an IssueBatch:

1. Parse the JSON payload — validate it has `id`, `issues` (non-empty array), `created`, `source`
2. Check: "Is a GoStream maintenance subagent already active?"
   - If yes → enqueue batch to `~/.hermes/gostream-queue.json`
   - If no → spawn isolated subagent with the maintenance skill and the batch as context

## Queue Management

Queue file: `~/.hermes/gostream-queue.json`

Format:
```json
{
  "batches": [
    {
      "batch": { "id": "...", "issues": [...], "created": "...", "source": "realtime" },
      "status": "pending|processing|failed",
      "priority_rank": 1,
      "retry_count": 0
    }
  ]
}
```

Priority ranks: B=1, A=2, C=3, D=4

**Dequeue rule:** Select the pending batch with lowest `priority_rank`, mark as "processing", spawn subagent.

**On completion:** Mark as "complete". Prune completed batches older than 24 hours.

**On failure:** Increment `retry_count`. If < 3, mark as "pending" for retry. If >= 3, mark as "failed" and notify user via Telegram.

## Cron Deep Scan

Every ~30 minutes (configured via Hermes cron):

1. Activate the `gostream-deep-scan` skill
2. Deep scan checks: torrent health, favorites completeness, subtitle availability, system health
3. If issues found → creates IssueBatch → routes through normal dispatch flow
4. If all clear → logs "deep scan: all healthy" to `~/.hermes/gostream-scan.log`

## User Preferences (hardcoded)

- **Torrent selection:** smallest file size + most seeders (within same quality tier)
- **Issue priority:** B (won't start) > A (wrong film) > C (incomplete series) > D (missing subtitles)
- **Favorites:** Movies must be 100% pre-downloaded. TV series must have ALL seasons/episodes.
- **Communication:** Telegram via Hermes gateway (already configured — use gateway for notifications)
