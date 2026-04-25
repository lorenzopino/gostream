# Jellyfin Health Skill

**Purpose:** Diagnose Jellyfin-side issues and check subtitle availability.

## Configuration

Expected environment variables or `~/.hermes/config/gostream.json`:
```json
{
  "jellyfin_url": "http://localhost:8096",
  "jellyfin_api_key": "your-api-key"
}
```

## Tools

### Check Service Health
```bash
curl -s "$JELLYFIN_URL/System/Info?api_key=$JELLYFIN_API_KEY" | python3 -c "
import sys,json; d=json.load(sys.stdin)
print(f\"Jellyfin {d.get('Version','?')}, WS:{d.get('WebSocketPort','-')}, Ops:{d.get('OperatingSystem','-')}\")
"
```

### Check Active Playback Sessions
```bash
curl -s "$JELLYFIN_URL/Sessions?api_key=$JELLYFIN_API_KEY" | python3 -c "
import sys,json
for s in json.load(sys.stdin):
    np = s.get('NowPlayingItem',{})
    if np:
        print(f\"User:{s.get('UserName','?')} Playing:{np.get('Name','?')} Type:{np.get('Type','?')} ID:{np.get('Id','?')}\")
"
```

### Check Subtitle Availability for an Item
```bash
# Get item details including MediaStreams
curl -s "$JELLYFIN_URL/Items/<ITEM_ID>?api_key=$JELLYFIN_API_KEY" | python3 -c "
import sys,json
d=json.load(sys.stdin)
streams = d.get('MediaStreams',[])
subs = [s for s in streams if s.get('Type') == 'Subtitle']
if subs:
    for s in subs:
        print(f\"  {s.get('Language','?')} ({s.get('Codec','?')}) Default:{s.get('IsDefault',False)}\")
else:
    print('NO SUBTITLES')
"
```

### Check for English Subtitles
An item has English subtitles if any MediaStream has:
- `Type: "Subtitle"`
- `Language` is "en", "eng", or "english" (case-insensitive)
- OR `CodecTag` indicates English

### Download Subtitles (OpenSubtitles Plugin)
If Jellyfin has the OpenSubtitles plugin configured:
```bash
# Search for subtitles
curl -s "$JELLYFIN_URL/Subtitles/Search" \
  -H "Content-Type: application/json" \
  -d '{
    "ItemId": "<ITEM_ID>",
    "Language": "eng",
    "IsPerfectMatch": false
  }' \
  -X POST

# Download a subtitle (by SubtitleId from search results)
curl -s "$JELLYFIN_URL/Subtitles/<SUBTITLE_ID>/Stream/<ITEM_ID>/eng?api_key=$JELLYFIN_API_KEY" -X POST
```

### Trigger Library Refresh
```bash
curl -s -X POST "$JELLYFIN_URL/Library/Refresh?api_key=$JELLYFIN_API_KEY"
```

### Get Library Virtual Folders
```bash
curl -s "$JELLYFIN_URL/Library/VirtualFolders?api_key=$JELLYFIN_API_KEY"
```

## Diagnosis Flow

When investigating a playback/subtitle issue:

1. Check Jellyfin service health
2. Check if the item exists in Jellyfin
3. Check MediaStreams for subtitle availability
4. If English subtitles missing → check OpenSubtitles plugin status
5. If plugin not configured → report as Resource Request
6. If plugin configured → attempt download
