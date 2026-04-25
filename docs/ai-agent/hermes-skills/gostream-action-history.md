# GoStream Action History Skill

**Purpose:** Track all AI actions for dedup, auditing, and self-learning.

## File Location

`~/.hermes/gostream-action-history.json`

Initial empty state:
```json
{ "actions": [] }
```

## Operations

### Read Last N Actions
```bash
cat ~/.hermes/gostream-action-history.json | python3 -c "
import sys,json
data = json.load(sys.stdin)
actions = data.get('actions',[])
for a in actions[-10:]:
    print(f\"{a['ts'][:19]} | {a['issue_type']:20s} | {a['action']:20s} | {a['result']:8s} | {a.get('file','')}\")
"
```

### Append Action
```python
import json, datetime

def record_action(history_path, action):
    with open(history_path, 'r') as f:
        data = json.load(f)
    action['ts'] = datetime.datetime.utcnow().isoformat() + 'Z'
    data['actions'].append(action)
    with open(history_path, 'w') as f:
        json.dump(data, f, indent=2)
```

### Check for Recent Similar Action
Before taking any action, check:
```bash
cat ~/.hermes/gostream-action-history.json | python3 -c "
import sys,json,datetime
data = json.load(sys.stdin)
actions = data.get('actions',[])
cutoff = datetime.datetime.utcnow() - datetime.timedelta(minutes=30)
for a in actions:
    ts = datetime.datetime.fromisoformat(a['ts'].replace('Z',''))
    if ts > cutoff:
        print(f'RECENT: {a[\"ts\"]} {a[\"issue_type\"]} {a[\"action\"]} {a[\"result\"]}')
"
```

If same `issue_type` + `file` was addressed < 30 min ago → skip.

### Prune Old Entries
Remove entries older than 7 days:
```python
import json, datetime

def prune_old(history_path, days=7):
    with open(history_path, 'r') as f:
        data = json.load(f)
    cutoff = datetime.datetime.utcnow() - datetime.timedelta(days=days)
    data['actions'] = [
        a for a in data['actions']
        if datetime.datetime.fromisoformat(a['ts'].replace('Z','')) > cutoff
    ]
    with open(history_path, 'w') as f:
        json.dump(data, f, indent=2)
```

### Search by Issue Type
```bash
cat ~/.hermes/gostream-action-history.json | python3 -c "
import sys,json
data = json.load(sys.stdin)
issue_type = sys.argv[1] if len(sys.argv) > 1 else ''
for a in data.get('actions',[]):
    if issue_type in a.get('issue_type',''):
        print(json.dumps(a, indent=2))
" "dead_torrent"
```
