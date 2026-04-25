# GoStream Action History Skill

## Purpose
Track all actions taken by the AI maintenance agent. Enables:
- **Pre-action duplicate check**: Skip if same issue was addressed recently
- **Post-action audit**: Review what was done and when
- **Self-learning**: Identify patterns and improve over time

## File Location
`~/.hermes/gostream-action-history.json`

## Schema
```json
{
  "actions": [
    {
      "ts": "2026-04-25T09:00:00Z",
      "issue_type": "dead_torrent",
      "file": "The.Matrix.1999.REMUX.4K.mkv",
      "torrent_id": "abc123def456",
      "root_cause": "Original torrent had 0 seeders for 48h",
      "action": "replaced",
      "details": "Found alternative with 150 seeders, same quality, 12GB (was 28GB)",
      "result": "success",
      "confidence": 0.95,
      "user_approved": true,
      "lessons": "For REMUX 4K, prefer torrents with >50 seeders and size within 5% of original"
    }
  ]
}
```

## Fields
| Field | Type | Required | Description |
|---|---|---|---|
| `ts` | string (ISO 8601) | Yes | When the action was taken |
| `issue_type` | string | Yes | Issue type from batch (dead_torrent, slow_startup, etc.) |
| `file` | string | Yes | Filename affected |
| `torrent_id` | string | Optional | GoStorm torrent ID |
| `root_cause` | string | Yes | What was found to be the cause |
| `action` | string | Yes | What was done (replaced, downloaded, removed, etc.) |
| `details` | string | Optional | Additional context about the action |
| `result` | string | Yes | "success", "partial", or "failed" |
| `confidence` | float (0-1) | Yes | How confident the agent is in the fix |
| `user_approved` | bool | Yes | Whether user approved (for high-risk actions) |
| `lessons` | string | Optional | What was learned for next time |

## Operations

### Append Action
After completing an action, append to the file:
```bash
# Read current, append new, write back
python3 -c "
import json, sys
with open('$HOME/.hermes/gostream-action-history.json', 'r') as f:
    data = json.load(f)
data['actions'].append(json.loads(sys.stdin.read()))
with open('$HOME/.hermes/gostream-action-history.json', 'w') as f:
    json.dump(data, f, indent=2)
"
```

### Check Recent Actions (Pre-Action Check)
Before taking any action, check if the same issue was addressed recently:
```bash
# Show last 20 actions
python3 -c "
import json
with open('$HOME/.hermes/gostream-action-history.json', 'r') as f:
    data = json.load(f)
for a in data['actions'][-20:]:
    print(f'{a[\"ts\"]} | {a[\"issue_type\"]:20} | {a[\"file\"]:30} | {a[\"result\"]}')
"
```

### Search by Issue Type
```bash
# Find all actions for a specific issue type
python3 -c "
import json
with open('$HOME/.hermes/gostream-action-history.json', 'r') as f:
    data = json.load(f)
for a in data['actions']:
    if 'dead_torrent' in a['issue_type']:
        print(json.dumps(a, indent=2))
"
```

### Prune Old Entries
Remove entries older than 7 days to keep file manageable:
```bash
python3 -c "
import json
from datetime import datetime, timedelta
with open('$HOME/.hermes/gostream-action-history.json', 'r') as f:
    data = json.load(f)
cutoff = datetime.now() - timedelta(days=7)
data['actions'] = [a for a in data['actions'] if datetime.fromisoformat(a['ts']) > cutoff]
with open('$HOME/.hermes/gostream-action-history.json', 'w') as f:
    json.dump(data, f, indent=2)
print(f'Pruned to {len(data[\"actions\"])} entries')
"
```

## Initialization
If the file doesn't exist, create it:
```bash
mkdir -p ~/.hermes
echo '{"actions": []}' > ~/.hermes/gostream-action-history.json
```

## Usage in Maintenance Skill
1. Before action: Check last 20 entries for duplicates (same issue_type + file within 30 min)
2. After action: Append new entry with result
3. Periodically: Prune entries older than 7 days
