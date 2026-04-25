# GoStream Action History Skill

## Purpose
Read, write, and query the persistent action history at `~/.hermes/gostream-action-history.json`. Used by the maintenance skill for pre-action checks and post-action logging.

## File Location
`~/.hermes/gostream-action-history.json`

## Schema
See `gostream-action-history-schema.json` in the same directory.

## Operations

### read_last(N)
Read the last N entries (default 20):
```bash
tail -c 50000 ~/.hermes/gostream-action-history.json | python3 -c "
import json, sys
data = json.load(sys.stdin)
actions = data.get('actions', [])
for a in actions[-N:]:
    print(json.dumps(a, indent=2))
"
```

### append(action)
Append a new action entry:
```bash
python3 -c "
import json, os
from datetime import datetime, timezone

path = os.path.expanduser('~/.hermes/gostream-action-history.json')
if os.path.exists(path):
    with open(path) as f:
        data = json.load(f)
else:
    data = {'actions': []}

action = {
    'ts': datetime.now(timezone.utc).isoformat(),
    'issue_type': '<type>',
    'file': '<filename>',
    'torrent_id': '<id>',
    'root_cause': '<diagnosis>',
    'action': '<what was done>',
    'result': 'success|partial|failed',
    'confidence': 0.95,
    'user_approved': true,
    'lessons': '<what was learned>'
}
data['actions'].append(action)

with open(path, 'w') as f:
    json.dump(data, f, indent=2)
print(f'Action recorded: {action[\"issue_type\"]} → {action[\"result\"]}')
"
```

### search(query_key, query_value)
Search for actions matching a criteria:
```bash
python3 -c "
import json, os
path = os.path.expanduser('~/.hermes/gostream-action-history.json')
with open(path) as f:
    data = json.load(f)
for a in data.get('actions', []):
    if a.get('<query_key>') == '<query_value>':
        print(json.dumps(a, indent=2))
"
```

### count_by_issue_type
Count actions grouped by issue_type:
```bash
python3 -c "
import json
from collections import Counter
path = os.path.expanduser('~/.hermes/gostream-action-history.json')
with open(path) as f:
    data = json.load(f)
types = Counter(a['issue_type'] for a in data.get('actions', []))
for t, c in types.most_common():
    print(f'{t}: {c}')
"
```

### prune_older_than(days)
Remove entries older than N days:
```bash
python3 -c "
import json, os
from datetime import datetime, timedelta, timezone

path = os.path.expanduser('~/.hermes/gostream-action-history.json')
with open(path) as f:
    data = json.load(f)

cutoff = datetime.now(timezone.utc) - timedelta(days=<N>)
before = len(data.get('actions', []))
data['actions'] = [
    a for a in data['actions']
    if datetime.fromisoformat(a['ts']) > cutoff
]
after = len(data['actions'])

with open(path, 'w') as f:
    json.dump(data, f, indent=2)
print(f'Pruned {before - after} entries older than {N} days')
"
```

## Pre-Action Check Protocol
Before taking any maintenance action:
1. `read_last(20)` — get recent 20 entries
2. Check if same `issue_type` + same `file` or `torrent_id` appears
3. If found and `ts` is < 30 min ago → SKIP (duplicate)
4. If found but `result` was "failed" → may retry with different approach
5. If not found → proceed with action
