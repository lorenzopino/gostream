---
id: mem_58601e3cedfa
created: "2026-04-16T22:00:00.651Z"
last_retrieved: "2026-04-16T22:00:00.651Z"
retrieval_count: 0
strength: 0.4332
half_life_days: 7
layer: episodic
tags: [claude-code-memory]
emotional_valence: neutral
schema_fit: 0.5
source: "claude-memory:gostream_architecture_current.md"
outcome_score: null
outcome_positive: 0
outcome_negative: 0
conflicts_with: []
pinned: false
confidence: observed
---

**Setup (April 2026):** GoStream (native binary) + Jellyfin (native macOS app) + Prowlarr/FlareSolverr (Docker). FUSE mount at ~/MediaCenter/gostream-fuse bridges GoStream to Jellyfin — Jellyfin reads virtual .mkv files directly from the mount. GoStream binary at ~/VSCodeWorkspace/gostream/gostream. Jellyfin app at /Applications/Jellyfin.app. Docker compose at ~/Webapps/media-center/ contains ONLY prowlarr and flaresolverr. Full details: `.claude/skills/gostream-architecture/SKILL.md`
