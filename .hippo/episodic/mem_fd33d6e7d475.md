---
id: mem_fd33d6e7d475
created: "2026-04-16T22:00:00.668Z"
last_retrieved: "2026-04-16T22:00:00.668Z"
retrieval_count: 0
strength: 0.4332
half_life_days: 7
layer: episodic
tags: [claude-code-memory]
emotional_valence: neutral
schema_fit: 0.5
source: "claude-memory:project_architecture_decisions.md"
outcome_score: null
outcome_positive: 0
outcome_negative: 0
conflicts_with: []
pinned: false
confidence: observed
---

## Decisioni confermate

1. **Frontend Vue.js eliminato** — Jellyfin è la UI esclusiva, il backend serve Jellyfin via plugin C# e file .strm

2. **Pre-Cacher = Library Builder permanente** (non cache con TTL):
   - File .strm permanenti per Jellyfin
   - Torrent validati multipli (top 3) per fallback, con scoring PTN
   - Sottotitoli multipli per lingua, salvati localmente

3. **Sottotitoli: approccio ibrido a 2 fasi**:
   - Fase 1 (background): Pre-scarica top 3 EN + top 3 IT per IMDB ID, classificati per download count. Salva metadati PTN-parsed del release name.
   - Fase 2 (al Play): Match release name torrent ↔ sottotitoli pre-cachati. Fallback: ricerca per hash file scaricato.

4. **Priorità lingue sottotitoli**: EN prima, IT come fallback

5. **./media contiene .strm proxy** — creati da scripts/create_strm_catalog.sh, Jellyfin li vede come media virtuali

6. **Categorie catalogo TMDB**:
   - Film IT nuove uscite: 6 mesi, vote_avg>6, original_language=it, vote_count≥2
   - Film IT archivio: tutto TMDB, vote_avg>6, original_language=it, vote_count≥50
   - Film internazionali nuove uscite: 6 mesi, vote_avg>6, tutte le lingue, vote_count≥50
   - Film internazionali archivio: tutto TMDB, vote_avg>7, tutte le lingue, vote_count≥100
   - Serie TV: vote_avg>6, vote_count≥100, tutte le lingue, esclusi Kids/Soap/Talk/War&Politics

7. **Strategia retry torrent non trovati**:
   - Se torrent non trovato → nascondi, marca "da riprovare"
   - Max 3 tentativi a distanza ≥1 giorno
    [truncated]
