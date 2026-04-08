# Design: Configurazione Qualità Profili + TMDB Discovery

**Data:** 2026-04-08
**Autore:** GoStorm team
**Stato:** Draft — in attesa di review

---

## Obiettivo

Rendere completamente configurabili due aree critiche del sistema:

1. **Quality Profile** — Come vengono selezionati i torrent (attualmente hardcoded verso qualità massima)
2. **TMDB Discovery** — Quali parametri usare per cercare film/serie (attualmente hardcoded)

Entrambi devono essere modificabili via `config.json` senza ricompilare.

---

## Parte 1 — Quality Profile System

### 1.1 Struttura Config

```jsonc
{
  "quality": {
    "profile": "size-first",  // "quality-first" | "size-first"
    "profiles": {
      "quality-first": {
        "movies": {
          "include_4k": true,
          "include_1080p": true,
          "include_720p": false,
          "size_floor_gb": { "4k": 10, "1080p": 4 },
          "size_ceiling_gb": { "4k": 60, "1080p": 20 },
          "min_seeders": 15,
          "fallback_4k_min_seeders": null,
          "priority_order": ["4k", "1080p", "720p"],
          "score_weights": {
            "resolution_4k": 200,
            "resolution_1080p": 50,
            "resolution_720p": 0,
            "dolby_vision": 150,
            "hdr": 100,
            "hdr10_plus": 100,
            "atmos": 50,
            "audio_5_1": 25,
            "audio_stereo": -50,
            "bluray": 10,
            "remux": -500,
            "ita": 60,
            "seeder_bonus": 5,
            "seeder_threshold": 50,
            "unknown_size_penalty": -5
          }
        },
        "tv": {
          "include_4k": true,
          "include_1080p": true,
          "include_720p": false,
          "size_floor_gb": { "4k": 10, "1080p": 1 },
          "size_ceiling_gb": { "4k": 30, "1080p": 30 },
          "min_seeders_4k": 10,
          "min_seeders": 5,
          "fullpack_bonus": 500,
          "priority_order": ["4k", "1080p", "720p"],
          "score_weights": {
            "resolution_4k": 200,
            "resolution_1080p": 50,
            "resolution_720p": 0,
            "hdr": 100,
            "atmos": 50,
            "audio_5_1": 25,
            "ita": 40,
            "seeder_100_bonus": 100,
            "seeder_50_bonus": 50,
            "seeder_20_bonus": 10
          }
        }
      },
      "size-first": {
        "movies": {
          "include_4k": true,
          "include_1080p": true,
          "include_720p": true,
          "size_floor_gb": { "720p": 0.5, "1080p": 0.8, "4k": 1 },
          "size_ceiling_gb": { "720p": 3, "1080p": 5, "4k": 8 },
          "min_seeders": 15,
          "fallback_4k_min_seeders": 50,
          "priority_order": ["720p", "1080p", "4k"],
          "score_weights": {
            "resolution_720p": 500,
            "resolution_1080p": 300,
            "resolution_4k": 100,
            "dolby_vision": 50,
            "hdr": 40,
            "hdr10_plus": 40,
            "atmos": 25,
            "audio_5_1": 15,
            "audio_stereo": 10,
            "bluray": 5,
            "remux": -500,
            "ita": 60,
            "seeder_bonus": 5,
            "seeder_threshold": 30,
            "unknown_size_penalty": -5
          }
        },
        "tv": {
          "include_4k": true,
          "include_1080p": true,
          "include_720p": true,
          "size_floor_gb": { "720p": 0.3, "1080p": 0.5, "4k": 0.5 },
          "size_ceiling_gb": { "720p": 1, "1080p": 2, "4k": 3 },
          "min_seeders_4k": 50,
          "min_seeders": 10,
          "fullpack_bonus": 300,
          "priority_order": ["720p", "1080p", "4k"],
          "score_weights": {
            "resolution_720p": 500,
            "resolution_1080p": 300,
            "resolution_4k": 100,
            "hdr": 40,
            "atmos": 25,
            "audio_5_1": 15,
            "ita": 40,
            "seeder_100_bonus": 100,
            "seeder_50_bonus": 50,
            "seeder_20_bonus": 10
          }
        }
      }
    }
  }
}
```

### 1.2 Struct Go (`config.go`)

```go
type QualityConfig struct {
    Profile  string                       `json:"profile"`            // "quality-first" | "size-first"
    Profiles map[string]QualityProfileSet  `json:"profiles,omitempty"`
}

type QualityProfileSet struct {
    Movies *MovieQualityProfile `json:"movies,omitempty"`
    TV     *TVQualityProfile    `json:"tv,omitempty"`
}

type MovieQualityProfile struct {
    Include4K               bool                `json:"include_4k"`
    Include1080p            bool                `json:"include_1080p"`
    Include720p             bool                `json:"include_720p"`
    SizeFloorGB             map[string]float64  `json:"size_floor_gb,omitempty"`    // keys: "720p", "1080p", "4k"
    SizeCeilingGB           map[string]float64  `json:"size_ceiling_gb,omitempty"` // keys: "720p", "1080p", "4k"
    MinSeeders              int                 `json:"min_seeders"`
    Fallback4KMinSeeders    *int                `json:"fallback_4k_min_seeders,omitempty"` // nil = 4K non è fallback
    PriorityOrder           []string            `json:"priority_order"`                    // es: ["720p", "1080p", "4k"]
    ScoreWeights            MovieScoreWeights   `json:"score_weights"`
}

type MovieScoreWeights struct {
    Resolution4K        *int `json:"resolution_4k,omitempty"`
    Resolution1080p     *int `json:"resolution_1080p,omitempty"`
    Resolution720p      *int `json:"resolution_720p,omitempty"`
    DolbyVision         *int `json:"dolby_vision,omitempty"`
    HDR                 *int `json:"hdr,omitempty"`
    HDR10Plus           *int `json:"hdr10_plus,omitempty"`
    Atmos               *int `json:"atmos,omitempty"`
    Audio51             *int `json:"audio_5_1,omitempty"`
    AudioStereo         *int `json:"audio_stereo,omitempty"`
    BluRay              *int `json:"bluray,omitempty"`
    Remux               *int `json:"remux,omitempty"`
    ITA                 *int `json:"ita,omitempty"`
    SeederBonus         *int `json:"seeder_bonus,omitempty"`
    SeederThreshold     *int `json:"seeder_threshold,omitempty"`
    UnknownSizePenalty  *int `json:"unknown_size_penalty,omitempty"`
}

type TVQualityProfile struct {
    Include4K            bool               `json:"include_4k"`
    Include1080p         bool               `json:"include_1080p"`
    Include720p          bool               `json:"include_720p"`
    SizeFloorGB          map[string]float64 `json:"size_floor_gb,omitempty"`
    SizeCeilingGB        map[string]float64 `json:"size_ceiling_gb,omitempty"`
    MinSeeders4K         int                `json:"min_seeders_4k"`
    MinSeeders           int                `json:"min_seeders"`
    FullpackBonus        int                `json:"fullpack_bonus"`
    PriorityOrder        []string           `json:"priority_order"`
    ScoreWeights         TVScoreWeights     `json:"score_weights"`
}

type TVScoreWeights struct {
    Resolution4K      *int `json:"resolution_4k,omitempty"`
    Resolution1080p   *int `json:"resolution_1080p,omitempty"`
    Resolution720p    *int `json:"resolution_720p,omitempty"`
    HDR               *int `json:"hdr,omitempty"`
    Atmos             *int `json:"atmos,omitempty"`
    Audio51           *int `json:"audio_5_1,omitempty"`
    ITA               *int `json:"ita,omitempty"`
    Seeder100Bonus    *int `json:"seeder_100_bonus,omitempty"`
    Seeder50Bonus     *int `json:"seeder_50_bonus,omitempty"`
    Seeder20Bonus     *int `json:"seeder_20_bonus,omitempty"`
}
```

### 1.3 Default Hardcoded (backward compatible)

Se `quality` non è presente nel config, il sistema usa `"quality-first"` con i valori attuali. Nessun cambiamento di comportamento per chi non configura nulla.

### 1.4 Modifiche agli Engine

#### `movie_go.go`
- **Constants → Config:** Rimuovere `const mMovie*` hardcoded
- **`filterMovieStreams`:** Legge `priority_order` dal profile attivo e itera nell'ordine configurato (non più hard 4K→1080p)
- **`classifyMovieStream`:** Controlla `include_4k/1080p/720p` e `size_ceiling_gb`/`size_floor_gb` dal profile
- **`calculateMovieScore`:** Legge i pesi da `ScoreWeights` invece che da costanti
- **4K fallback:** Se `fallback_4k_min_seeders` è impostato e nessun stream nelle priorità precedenti passa, prova 4K con soglia seeders più alta

#### `tv_go.go`
- Stesse modifiche: costanti → config, `priority_order`, `include_*`, size ceiling/floor, pesi dinamici

#### `watchlist_go.go`
- Stesse modifiche: usa il profile globale invece di `DefaultMovieProfile()`

#### `quality/scorer.go`
- **Nuovo entry point:** `ApplyProfile(streams []Stream, profile Profile) []ScoredStream`
- I tre engine chiamano questo invece di duplicare la logica
- Le funzioni `ProfileFromConfig()` e `ProfileFromTVConfig()` ora vengono effettivamente usate

### 1.5 Regex 720p

Aggiungere `reM720p` e `reTV720p` ai pattern:
```go
reM720p = regexp.MustCompile(`(?i)720p|hd`)
reTV720p = regexp.MustCompile(`(?i)720p`)
```

---

## Parte 2 — TMDB Discovery Configurabile

### 2.1 Struttura Config

```jsonc
{
  "tmdb_discovery": {
    "movies": {
      "endpoints": [
        {
          "name": "discover_en_recent",
          "enabled": true,
          "type": "discover",
          "language": "en",
          "sort_by": "popularity.desc",
          "primary_release_date_gte": "-6months",
          "primary_release_date_lte": "+12months",
          "pages": 12,
          "vote_average_gte": null,
          "vote_count_gte": null,
          "with_genres": null,
          "without_genres": "99",
          "with_keywords": null,
          "without_keywords": null,
          "with_original_language": null,
          "with_origin_country": null,
          "primary_release_year": null,
          "with_runtime_gte": null,
          "with_runtime_lte": null,
          "with_release_type": "3|4",
          "region": null,
          "watch_region": null,
          "include_adult": false,
          "include_video": false
        },
        {
          "name": "discover_it",
          "enabled": true,
          "type": "discover",
          "language": "it",
          "sort_by": "popularity.desc",
          "primary_release_date_gte": "-6months",
          "primary_release_date_lte": "+12months",
          "pages": 3,
          "vote_average_gte": null,
          "vote_count_gte": null,
          "with_genres": null,
          "without_genres": "99",
          "with_keywords": null,
          "without_keywords": null,
          "with_original_language": null,
          "with_origin_country": null,
          "primary_release_year": null,
          "with_runtime_gte": null,
          "with_runtime_lte": null,
          "with_release_type": "3|4",
          "region": null,
          "watch_region": null,
          "include_adult": false,
          "include_video": false
        },
        {
          "name": "trending",
          "enabled": true,
          "type": "trending",
          "time_window": "week",
          "pages": 1
        },
        {
          "name": "now_playing_us",
          "enabled": true,
          "type": "list",
          "endpoint": "/movie/now_playing",
          "region": "US",
          "pages": 1
        }
      ]
    },
    "tv": {
      "endpoints": [
        {
          "name": "on_the_air",
          "enabled": true,
          "type": "list",
          "endpoint": "/tv/on_the_air",
          "pages": 3
        },
        {
          "name": "airing_today",
          "enabled": true,
          "type": "list",
          "endpoint": "/tv/airing_today",
          "pages": 2
        },
        {
          "name": "trending",
          "enabled": true,
          "type": "trending",
          "time_window": "week",
          "pages": 3
        },
        {
          "name": "discover_en",
          "enabled": true,
          "type": "discover",
          "language": "en",
          "sort_by": "popularity.desc",
          "first_air_date_gte": "-6months",
          "first_air_date_lte": null,
          "pages": 5,
          "vote_average_gte": null,
          "vote_count_gte": null,
          "with_genres": null,
          "without_genres": "99,16,10763,10764,10767",
          "with_keywords": null,
          "without_keywords": null,
          "with_original_language": null,
          "with_origin_country": null,
          "first_air_date_year": null,
          "with_runtime_gte": null,
          "with_runtime_lte": null,
          "with_status": null,
          "with_type": null,
          "with_networks": null,
          "watch_region": null,
          "include_adult": false,
          "include_null_first_air_dates": false
        }
      ]
    }
  }
}
```

### 2.2 Struct Go (`config.go`)

**Parametri obbligatori per ogni endpoint:** `name`, `enabled`, `type`. Tutti gli altri campi sono `*T` (pointer) con tag `omitempty` — se omessi o null, il parametro **non viene aggiunto alla query URL** e TMDB applica i suoi default server-side.

```go
type TMDBDiscoveryConfig struct {
    Movies *TMDBEndpointGroup `json:"movies,omitempty"`
    TV     *TMDBEndpointGroup `json:"tv,omitempty"`
}

type TMDBEndpointGroup struct {
    Endpoints []TMDBEndpoint `json:"endpoints"`
}

type TMDBEndpoint struct {
    Name     string `json:"name"`
    Enabled  bool   `json:"enabled"`
    EndpointType string `json:"type"` // "discover" | "trending" | "list"

    // --- Common ---
    Language  *string `json:"language,omitempty"`
    SortBy    *string `json:"sort_by,omitempty"`
    Pages     *int    `json:"pages,omitempty"`

    // --- Discover Movie/TV shared ---
    VoteAverageGte       *float64 `json:"vote_average_gte,omitempty"`
    VoteCountGte         *int     `json:"vote_count_gte,omitempty"`
    WithGenres           *string  `json:"with_genres,omitempty"`
    WithoutGenres        *string  `json:"without_genres,omitempty"`
    WithKeywords         *string  `json:"with_keywords,omitempty"`
    WithoutKeywords      *string  `json:"without_keywords,omitempty"`
    WithOriginalLanguage *string  `json:"with_original_language,omitempty"`
    WithOriginCountry    *string  `json:"with_origin_country,omitempty"`
    WithRuntimeGte       *int     `json:"with_runtime_gte,omitempty"`
    WithRuntimeLte       *int     `json:"with_runtime_lte,omitempty"`
    WatchRegion          *string  `json:"watch_region,omitempty"`
    IncludeAdult         *bool    `json:"include_adult,omitempty"`

    // --- Movie-specific ---
    PrimaryReleaseDateGte *string `json:"primary_release_date_gte,omitempty"`
    PrimaryReleaseDateLte *string `json:"primary_release_date_lte,omitempty"`
    PrimaryReleaseYear    *int    `json:"primary_release_year,omitempty"`
    WithReleaseType       *string `json:"with_release_type,omitempty"`
    Region                *string `json:"region,omitempty"`
    IncludeVideo          *bool   `json:"include_video,omitempty"`

    // --- TV-specific ---
    FirstAirDateGte           *string `json:"first_air_date_gte,omitempty"`
    FirstAirDateLte           *string `json:"first_air_date_lte,omitempty"`
    FirstAirDateYear          *int    `json:"first_air_date_year,omitempty"`
    WithStatus                *string `json:"with_status,omitempty"`
    WithType                  *string `json:"with_type,omitempty"`
    WithNetworks              *string `json:"with_networks,omitempty"`
    IncludeNullFirstAirDates  *bool   `json:"include_null_first_air_dates,omitempty"`

    // --- List endpoint ---
    EndpointURL *string `json:"endpoint,omitempty"` // es: "/movie/now_playing"

    // --- Trending endpoint ---
    TimeWindow *string `json:"time_window,omitempty"` // "day" | "week"
}
```

### 2.3 Modifiche al TMDB Client (`tmdb/client.go`)

**Nuovi metodi:**

```go
// DiscoverMoviesFromConfig esegue tutti gli endpoint "discover" e "list" configurati per i film
func (c *Client) DiscoverMoviesFromConfig(ctx context.Context, cfg TMDBEndpointGroup) ([]Movie, error)

// DiscoverTVFromConfig esegue tutti gli endpoint configurati per le serie TV
func (c *Client) DiscoverTVFromConfig(ctx context.Context, cfg TMDBEndpointGroup) ([]TVShow, error)
```

**Logica di costruzione URL per endpoint type `discover`:**
- Itera tutti i campi `*T` non-nil e li aggiunge come query param
- Supporto date relative: `"-6months"`, `"+12months"` → calcolate rispetto a `time.Now()`
- Deduplicazione per TMDB ID (un movie/show può apparire in più endpoint)

**Logica per endpoint type `trending`:**
- Chiama `/trending/movie/{time_window}` o `/trending/tv/{time_window}`
- `time_window` default: `"week"`

**Logica per endpoint type `list`:**
- Chiama l'endpoint specifico (`/movie/now_playing`, `/tv/on_the_air`, ecc.)
- Passa `region` se configurato

### 2.4 Modifiche agli Engine

#### `movie_go.go:discoverMovies()`
- Rimuove l'hardcoded list di 6 funzioni
- Chiama `e.tmdb.DiscoverMoviesFromConfig(ctx, config.TMDBDiscovery.Movies)`
- Mantiene il loop di dedup e merge esistente

#### `tv_go.go:discoverShows()`
- Rimuove l'hardcoded list di endpoint
- Chiama `e.tmdb.DiscoverTVFromConfig(ctx, config.TMDBDiscovery.TV)`

### 2.5 Backward Compatibility

Se `tmdb_discovery` non è presente nel config:
- Il TMDB client usa i metodi attuali (comportamento invariato)
- Oppure: default equivalente agli endpoint attuali (stesso risultato)

### 2.6 Scheduling e On-Demand

**Scheduled:** Gli engine di discovery girano già all'interno del `Run()` dei sync engine, che sono orchestrati dallo scheduler esistente (`SchedulerConfig` in `config.go`). Nessun cambiamento al cron — la discovery configurabile viene semplicemente iniettata dentro `discoverMovies()` e `discoverShows()`.

**On-demand:** L'interfaccia Control Panel esistente a `:9080/dashboard` ha già bottoni per sync manuali. Aggiungiamo:
- "Run Movie Discovery Now" — esegue solo la fase discovery dei film (senza ri-processare i torrent già presenti)
- "Run TV Discovery Now" — esegue solo la fase discovery delle serie TV
- Entrambi triggerano `discoverMovies()` / `discoverShows()` via HTTP POST a nuovi endpoint REST

### 2.7 Web UI per Editing

La `settings.html` esistente va estesa con:

**Sezione Quality Profile:**
- Dropdown per selezionare il profilo attivo (`quality-first` / `size-first`)
- Tabella editabile dei pesi (`score_weights`) per movies e TV
- Checkbox per `include_4k`, `include_1080p`, `include_720p`
- Input numerici per `size_floor_gb`, `size_ceiling_gb`, `min_seeders`
- Pulsante "Save & Apply" → scrive `config.json` e hot-reload dei profile negli engine

**Sezione TMDB Discovery:**
- Lista di endpoint con toggle on/off
- Form espandibile per ogni endpoint con tutti i parametri
- Validazione client-side: range check su voti, date, enum validation per sort_by/release_type/status/type
- Pulsante "Add Endpoint" per aggiungere nuovi endpoint
- Drag & drop per riordinare la lista
- Pulsante "Save & Apply" → scrive `config.json`
- Pulsante "Run Discovery Now" per trigger on-demand

---

## Enumerazioni Valide

### Movie Genres
| ID | Nome | | ID | Nome |
|----|------|-|----|------|
| 28 | Action | | 10751 | Family |
| 12 | Adventure | | 14 | Fantasy |
| 16 | Animation | | 36 | History |
| 35 | Comedy | | 27 | Horror |
| 80 | Crime | | 10402 | Music |
| 99 | Documentary | | 9648 | Mystery |
| 18 | Drama | | 10749 | Romance |
| | | | 878 | Sci-Fi |
| | | | 10770 | TV Movie |
| | | | 53 | Thriller |
| | | | 10752 | War |
| | | | 37 | Western |

### TV Genres
| ID | Nome | | ID | Nome |
|----|------|-|----|------|
| 10759 | Action & Adventure | | 10762 | Kids |
| 16 | Animation | | 9648 | Mystery |
| 35 | Comedy | | 10763 | News |
| 80 | Crime | | 10764 | Reality |
| 99 | Documentary | | 10765 | Sci-Fi & Fantasy |
| 18 | Drama | | 10766 | Soap |
| | | | 10767 | Talk |
| | | | 10768 | War & Politics |
| | | | 37 | Western |

### Movie Release Type
| ID | Tipo |
|----|------|
| 1 | Premiere |
| 2 | Theatrical Limited |
| 3 | Theatrical |
| 4 | Digital |
| 5 | Physical |
| 6 | TV |

### TV Status
| ID | Stato |
|----|-------|
| 0 | Returning Series |
| 1 | Planned |
| 2 | In Production |
| 3 | Torn Off |
| 4 | Ended |
| 5 | Canceled |
| 6 | Pilot |

### TV Type
| ID | Tipo |
|----|------|
| 0 | Documentary |
| 1 | News |
| 2 | Miniseries |
| 3 | Reality |
| 4 | Scripted |
| 5 | Talk Show |
| 6 | Video |

### Sort By (Movie)
| Valore |
|--------|
| `popularity.desc`, `popularity.asc` |
| `vote_average.desc`, `vote_average.asc` |
| `vote_count.desc`, `vote_count.asc` |
| `primary_release_date.desc`, `primary_release_date.asc` |
| `revenue.desc`, `revenue.asc` |
| `title.desc`, `title.asc` |
| `original_title.desc`, `original_title.asc` |

### Sort By (TV)
| Valore |
|--------|
| `popularity.desc`, `popularity.asc` |
| `vote_average.desc`, `vote_average.asc` |
| `vote_count.desc`, `vote_count.asc` |
| `first_air_date.desc`, `first_air_date.asc` |
| `name.desc`, `name.asc` |
| `original_name.desc`, `original_name.asc` |

### Watch Monetization Types
| Valore | Significato |
|--------|-------------|
| `flatrate` | Abbonamento |
| `free` | Gratuito |
| `ads` | Con pubblicità |
| `rent` | Noleggio |
| `buy` | Acquisto |

### Region Codes (principali)
| Codice | Paese |
|--------|-------|
| `IT` | Italia |
| `US` | Stati Uniti |
| `GB` | Regno Unito |
| `FR` | Francia |
| `DE` | Germania |
| `ES` | Spagna |

### Language Codes (principali)
| Codice | Lingua |
|--------|--------|
| `en` | Inglese |
| `it` | Italiano |
| `fr` | Francese |
| `de` | Tedesco |
| `es` | Spagnolo |
| `pt` | Portoghese |
| `ja` | Giapponese |
| `ko` | Coreano |
| `zh` | Cinese |

---

## File da Modificare

| File | Modifica |
|------|----------|
| `config.go` | Aggiungere `QualityConfig`, `TMDBDiscoveryConfig`, struct annidate |
| `internal/syncer/quality/scorer.go` | Unificare scoring, nuovo `ApplyProfile()`, connettere `ProfileFromConfig` |
| `internal/syncer/engines/movie_go.go` | Rimuovere costanti hardcoded, leggere da config, supportare 720p |
| `internal/syncer/engines/tv_go.go` | Rimuovere costanti hardcoded, leggere da config, supportare 720p |
| `internal/syncer/engines/watchlist_go.go` | Leggere profile da config invece di `DefaultMovieProfile()` |
| `internal/catalog/tmdb/client.go` | Nuovi metodi `DiscoverMoviesFromConfig`, `DiscoverTVFromConfig`, date relative |
| `main.go` | Passare config agli engine |
| `config.json.example` | Aggiungere esempi completi |

---

## Testing

- **Unit:** Scoring con profile diversi (quality-first vs size-first)
- **Unit:** TMDB query builder con vari combinazioni di parametri
- **Unit:** Date relative (`-6months`, `+12months`)
- **Integration:** End-to-end con config personalizzato
- **Regression:** Default behavior senza config (deve essere identico)
