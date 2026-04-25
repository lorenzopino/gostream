---
id: mem_fa64cd489eae
created: "2026-04-16T22:00:00.722Z"
last_retrieved: "2026-04-16T22:00:00.722Z"
retrieval_count: 0
strength: 0.4332
half_life_days: 7
layer: episodic
tags: [claude-code-memory]
emotional_valence: neutral
schema_fit: 0.5
source: "claude-memory:project_streaming_bugs_fixed.md"
outcome_score: null
outcome_positive: 0
outcome_negative: 0
conflicts_with: []
pinned: false
confidence: observed
---

## 4 root cause identificate e fixate (2026-04-01)

Il sistema di streaming non funzionava a causa di 4 bug stacked che si neutralizzavano a vicenda.

### Bug 1: `watchdog.start()` mai chiamato (main.py)
`TorrentWatchdog` singleton non veniva avviato nel lifespan FastAPI.
Le sessioni di streaming restavano per sempre in stato "buffering".
**Fix:** aggiunto `await watchdog.start()` e `await watchdog.stop()` in main.py.

### Bug 2: Routing mismatch plugin C# vs backend
Il plugin chiama `/api/v1/streaming/movie/{id}` ma le route alias erano
montate a `/api/v1/stream/streaming/movie/{id}` (prefisso doppio `/stream/streaming/`).
Tutte le chiamate del plugin ricevevano 404.
**Fix:** creato `plugin_router = APIRouter()` in stream.py con i route alias
a path corretti (senza `/streaming/` prefix interno), poi montato su `/streaming`
in api.py. La struttura corretta è:
- `/api/v1/stream/...`     = endpoint HLS e .strm
- `/api/v1/streaming/...`  = endpoint compatibilità plugin C#

### Bug 3: Field name `hls_url` vs `stream_url`
`_create_stream_session` ritornava `{"hls_url": url}` ma il C# `StreamResponse`
ha `[JsonPropertyName("stream_url")]`. `StreamUrl` era sempre null nel plugin.
**Fix:** `_create_stream_session` ritorna ora entrambi `stream_url` e `hls_url`.

### Bug 4: Plugin creava sorgente non riproducibile durante buffering
Plugin controllava `status == "ready" && StreamUrl != null` prima di creare
una sorgente playable. Poiché lo stato è "buffering" fino a 10MB scaricati,
veni [truncated]
