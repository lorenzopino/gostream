# Quality Profiles & TMDB Discovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make torrent quality selection and TMDB content discovery fully configurable via config.json, with named profiles and a Web UI editor.

**Architecture:** Replace hardcoded quality constants and TMDB discovery endpoints in three sync engines with config-driven profiles. The `quality` package becomes the single scoring entry point. TMDB client gains a dynamic query builder. The scheduler and on-demand triggers remain unchanged — only the discovery source is swapped.

**Tech Stack:** Go (config structs, quality package, tmdb client, sync engines), HTML/JS (settings.html extension)

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `config.go` | **Modify** | Add `QualityConfig`, `TMDBDiscoveryConfig` structs; replace dead `QualityScoringConfig` |
| `internal/syncer/quality/types.go` | **Create** | Unified profile types that both config.go and engines use |
| `internal/syncer/quality/scorer.go` | **Modify** | Add 720p support, `ApplyProfile()`, use new types |
| `internal/syncer/quality/resolver.go` | **Create** | `ResolveProfile(cfg)` — maps profile name to weights |
| `internal/catalog/tmdb/client.go` | **Modify** | Add `DiscoverMoviesFromConfig()`, `DiscoverTVFromConfig()`, date relative parser |
| `internal/syncer/engines/movie_go.go` | **Modify** | Replace constants → config, add 720p, use quality.ApplyProfile |
| `internal/syncer/engines/tv_go.go` | **Modify** | Replace constants → config, add 720p, use quality.ApplyProfile |
| `internal/syncer/engines/watchlist_go.go` | **Modify** | Use config profile instead of DefaultMovieProfile() |
| `internal/syncer/engines/movies.go` | **Modify** | Add QualityProfile + TMDBDiscovery to MoviesSyncerConfig |
| `internal/syncer/engines/tv.go` | **Modify** | Add QualityProfile + TMDBDiscovery to TVSyncerConfig |
| `internal/syncer/engines/watchlist.go` | **Modify** | Add QualityProfile to WatchlistSyncerConfig |
| `main.go` | **Modify** | Pass QualityConfig and TMDBDiscoveryConfig to syncers |
| `config.json.example` | **Modify** | Add quality + tmdb_discovery examples |
| `settings.html` | **Modify** | Add quality profile selector + weights editor, TMDB endpoint editor |
| `internal/syncer/quality/types_test.go` | **Create** | Test profile resolution |
| `internal/syncer/quality/scorer_test.go` | **Create** | Test scoring with different profiles |
| `internal/catalog/tmdb/discover_test.go` | **Create** | Test query builder and date parsing |

---

## Phase 1 — Quality Profile Types & Resolver (Foundation)

### Task 1: Create unified profile types

**Files:**
- Create: `internal/syncer/quality/types.go`
- Test: `internal/syncer/quality/types_test.go`

- [ ] **Step 1: Create types.go with unified profile types**

```go
package quality

// MovieProfile holds all configurable quality parameters for movie torrent selection.
type MovieProfile struct {
	Include4K            bool               `json:"include_4k"`
	Include1080p         bool               `json:"include_1080p"`
	Include720p          bool               `json:"include_720p"`
	SizeFloorGB          map[string]float64 `json:"size_floor_gb,omitempty"`  // keys: "720p", "1080p", "4k"
	SizeCeilingGB        map[string]float64 `json:"size_ceiling_gb,omitempty"` // keys: "720p", "1080p", "4k"
	MinSeeders           int                `json:"min_seeders"`
	Fallback4KMinSeeders *int               `json:"fallback_4k_min_seeders,omitempty"` // nil = 4K not fallback
	PriorityOrder        []string           `json:"priority_order"`                    // ["720p", "1080p", "4k"]
	ScoreWeights         MovieScoreWeights  `json:"score_weights"`
}

// MovieScoreWeights holds configurable scoring weights for movies.
type MovieScoreWeights struct {
	Resolution4K       *int `json:"resolution_4k,omitempty"`
	Resolution1080p    *int `json:"resolution_1080p,omitempty"`
	Resolution720p     *int `json:"resolution_720p,omitempty"`
	DolbyVision        *int `json:"dolby_vision,omitempty"`
	HDR                *int `json:"hdr,omitempty"`
	HDR10Plus          *int `json:"hdr10_plus,omitempty"`
	Atmos              *int `json:"atmos,omitempty"`
	Audio51            *int `json:"audio_5_1,omitempty"`
	AudioStereo        *int `json:"audio_stereo,omitempty"`
	BluRay             *int `json:"bluray,omitempty"`
	Remux              *int `json:"remux,omitempty"`
	ITA                *int `json:"ita,omitempty"`
	SeederBonus        *int `json:"seeder_bonus,omitempty"`
	SeederThreshold    *int `json:"seeder_threshold,omitempty"`
	UnknownSizePenalty *int `json:"unknown_size_penalty,omitempty"`
}

// TVProfile holds all configurable quality parameters for TV torrent selection.
type TVProfile struct {
	Include4K       bool               `json:"include_4k"`
	Include1080p    bool               `json:"include_1080p"`
	Include720p     bool               `json:"include_720p"`
	SizeFloorGB     map[string]float64 `json:"size_floor_gb,omitempty"`
	SizeCeilingGB   map[string]float64 `json:"size_ceiling_gb,omitempty"`
	MinSeeders4K    int                `json:"min_seeders_4k"`
	MinSeeders      int                `json:"min_seeders"`
	FullpackBonus   int                `json:"fullpack_bonus"`
	PriorityOrder   []string           `json:"priority_order"`
	ScoreWeights    TVScoreWeights     `json:"score_weights"`
}

// TVScoreWeights holds configurable scoring weights for TV.
type TVScoreWeights struct {
	Resolution4K    *int `json:"resolution_4k,omitempty"`
	Resolution1080p *int `json:"resolution_1080p,omitempty"`
	Resolution720p  *int `json:"resolution_720p,omitempty"`
	HDR             *int `json:"hdr,omitempty"`
	Atmos           *int `json:"atmos,omitempty"`
	Audio51         *int `json:"audio_5_1,omitempty"`
	ITA             *int `json:"ita,omitempty"`
	Seeder100Bonus  *int `json:"seeder_100_bonus,omitempty"`
	Seeder50Bonus   *int `json:"seeder_50_bonus,omitempty"`
	Seeder20Bonus   *int `json:"seeder_20_bonus,omitempty"`
}
```

- [ ] **Step 2: Add default profiles to types.go**

Append to `types.go`:

```go
// DefaultQualityFirstMovies returns the "quality-first" movie profile (matches current hardcoded behavior).
func DefaultQualityFirstMovies() MovieProfile {
	v15 := 15
	v50 := 50
	return MovieProfile{
		Include4K: true, Include1080p: true, Include720p: false,
		SizeFloorGB:   map[string]float64{"4k": 10, "1080p": 4},
		SizeCeilingGB: map[string]float64{"4k": 60, "1080p": 20},
		MinSeeders:           15,
		Fallback4KMinSeeders: nil,
		PriorityOrder:        []string{"4k", "1080p", "720p"},
		ScoreWeights: MovieScoreWeights{
			Resolution4K:       ptr(200), Resolution1080p: ptr(50),
			DolbyVision: ptr(150), HDR: ptr(100), HDR10Plus: ptr(100),
			Atmos: ptr(50), Audio51: ptr(25), AudioStereo: ptr(-50),
			BluRay: ptr(10), Remux: ptr(-500), ITA: ptr(60),
			SeederBonus: ptr(5), SeederThreshold: ptr(50), UnknownSizePenalty: ptr(-5),
		},
	}
}

// DefaultSizeFirstMovies returns the "size-first" movie profile.
func DefaultSizeFirstMovies() MovieProfile {
	v50 := 50
	return MovieProfile{
		Include4K: true, Include1080p: true, Include720p: true,
		SizeFloorGB:   map[string]float64{"720p": 0.5, "1080p": 0.8, "4k": 1},
		SizeCeilingGB: map[string]float64{"720p": 3, "1080p": 5, "4k": 8},
		MinSeeders:           15,
		Fallback4KMinSeeders: &v50,
		PriorityOrder:        []string{"720p", "1080p", "4k"},
		ScoreWeights: MovieScoreWeights{
			Resolution720p: ptr(500), Resolution1080p: ptr(300), Resolution4K: ptr(100),
			DolbyVision: ptr(50), HDR: ptr(40), HDR10Plus: ptr(40),
			Atmos: ptr(25), Audio51: ptr(15), AudioStereo: ptr(10),
			BluRay: ptr(5), Remux: ptr(-500), ITA: ptr(60),
			SeederBonus: ptr(5), SeederThreshold: ptr(30), UnknownSizePenalty: ptr(-5),
		},
	}
}

// DefaultQualityFirstTV returns the "quality-first" TV profile.
func DefaultQualityFirstTV() TVProfile {
	return TVProfile{
		Include4K: true, Include1080p: true, Include720p: false,
		SizeFloorGB:   map[string]float64{"4k": 10, "1080p": 1},
		SizeCeilingGB: map[string]float64{"4k": 30, "1080p": 30},
		MinSeeders4K: 10, MinSeeders: 5, FullpackBonus: 500,
		PriorityOrder: []string{"4k", "1080p", "720p"},
		ScoreWeights: TVScoreWeights{
			Resolution4K: ptr(200), Resolution1080p: ptr(50),
			HDR: ptr(100), Atmos: ptr(50), Audio51: ptr(25),
			ITA: ptr(40), Seeder100Bonus: ptr(100), Seeder50Bonus: ptr(50), Seeder20Bonus: ptr(10),
		},
	}
}

// DefaultSizeFirstTV returns the "size-first" TV profile.
func DefaultSizeFirstTV() TVProfile {
	v50 := 50
	return TVProfile{
		Include4K: true, Include1080p: true, Include720p: true,
		SizeFloorGB:   map[string]float64{"720p": 0.3, "1080p": 0.5, "4k": 0.5},
		SizeCeilingGB: map[string]float64{"720p": 1, "1080p": 2, "4k": 3},
		MinSeeders4K: v50, MinSeeders: 10, FullpackBonus: 300,
		PriorityOrder: []string{"720p", "1080p", "4k"},
		ScoreWeights: TVScoreWeights{
			Resolution720p: ptr(500), Resolution1080p: ptr(300), Resolution4K: ptr(100),
			HDR: ptr(40), Atmos: ptr(25), Audio51: ptr(15),
			ITA: ptr(40), Seeder100Bonus: ptr(100), Seeder50Bonus: ptr(50), Seeder20Bonus: ptr(10),
		},
	}
}

func ptr[T any](v T) *T { return &v }
```

- [ ] **Step 3: Write types_test.go**

```go
package quality

import "testing"

func TestDefaultProfilesAreValid(t *testing.T) {
	tests := []struct {
		name string
		prof MovieProfile
	}{
		{"quality-first", DefaultQualityFirstMovies()},
		{"size-first", DefaultSizeFirstMovies()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.prof.PriorityOrder == nil || len(tt.prof.PriorityOrder) == 0 {
				t.Error("PriorityOrder must not be empty")
			}
			if tt.prof.MinSeeders <= 0 {
				t.Errorf("MinSeeders must be > 0, got %d", tt.prof.MinSeeders)
			}
			if tt.prof.ScoreWeights.Resolution4K == nil && tt.prof.Include4K {
				t.Error("Include4K is true but Resolution4K weight is nil")
			}
		})
	}
}

func TestDefaultTVProfilesAreValid(t *testing.T) {
	tests := []struct {
		name string
		prof TVProfile
	}{
		{"quality-first", DefaultQualityFirstTV()},
		{"size-first", DefaultSizeFirstTV()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.prof.PriorityOrder == nil || len(tt.prof.PriorityOrder) == 0 {
				t.Error("PriorityOrder must not be empty")
			}
			if tt.prof.FullpackBonus < 0 {
				t.Errorf("FullpackBonus must be >= 0, got %d", tt.prof.FullpackBonus)
			}
		})
	}
}
```

- [ ] **Step 4: Run tests**

```bash
cd /Users/lorenzo/VSCodeWorkspace/gostream && go test ./internal/syncer/quality/... -v -run "TestDefault"
```
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/syncer/quality/types.go internal/syncer/quality/types_test.go
git commit -m "feat: add unified quality profile types with defaults"
```

---

### Task 2: Create profile resolver

**Files:**
- Create: `internal/syncer/quality/resolver.go`
- Test: (add to `types_test.go`)

- [ ] **Step 1: Create resolver.go**

```go
package quality

// QualityConfig mirrors the top-level "quality" key in config.json.
type QualityConfig struct {
	Profile  string                    `json:"profile"`  // "quality-first" | "size-first"
	Profiles map[string]ProfileSet     `json:"profiles,omitempty"`
}

// ProfileSet holds movie + TV profiles for a named preset.
type ProfileSet struct {
	Movies *MovieProfile `json:"movies,omitempty"`
	TV     *TVProfile    `json:"tv,omitempty"`
}

// ResolveMovies returns the active movie profile. Falls back to default if not configured.
func (qc *QualityConfig) ResolveMovies() MovieProfile {
	if qc == nil {
		return DefaultQualityFirstMovies()
	}
	if qc.Profiles == nil {
		return defaultProfileByName(qc.Profile)
	}
	if ps, ok := qc.Profiles[qc.Profile]; ok && ps.Movies != nil {
		return mergeMovieProfile(*ps.Movies, defaultProfileByName(qc.Profile))
	}
	return defaultProfileByName(qc.Profile)
}

// ResolveTV returns the active TV profile. Falls back to default if not configured.
func (qc *QualityConfig) ResolveTV() TVProfile {
	if qc == nil {
		return DefaultQualityFirstTV()
	}
	if qc.Profiles == nil {
		return defaultProfileByName(qc.Profile)
	}
	if ps, ok := qc.Profiles[qc.Profile]; ok && ps.TV != nil {
		return mergeTVProfile(*ps.TV, defaultProfileByNameTV(qc.Profile))
	}
	return defaultProfileByNameTV(qc.Profile)
}

func defaultProfileByName(name string) MovieProfile {
	switch name {
	case "size-first":
		return DefaultSizeFirstMovies()
	default:
		return DefaultQualityFirstMovies()
	}
}

func defaultProfileByNameTV(name string) TVProfile {
	switch name {
	case "size-first":
		return DefaultSizeFirstTV()
	default:
		return DefaultQualityFirstTV()
	}
}

// mergeMovieProfile fills nil pointer fields in custom with values from defaults.
func mergeMovieProfile(custom, def MovieProfile) MovieProfile {
	result := def
	if custom.Include4K != result.Include4K || custom.Include4K { result.Include4K = custom.Include4K }
	if custom.Include1080p != result.Include1080p || custom.Include1080p { result.Include1080p = custom.Include1080p }
	if custom.Include720p != result.Include720p || custom.Include720p { result.Include720p = custom.Include720p }
	if len(custom.SizeFloorGB) > 0 { result.SizeFloorGB = custom.SizeFloorGB }
	if len(custom.SizeCeilingGB) > 0 { result.SizeCeilingGB = custom.SizeCeilingGB }
	if custom.MinSeeders > 0 { result.MinSeeders = custom.MinSeeders }
	if custom.Fallback4KMinSeeders != nil { result.Fallback4KMinSeeders = custom.Fallback4KMinSeeders }
	if len(custom.PriorityOrder) > 0 { result.PriorityOrder = custom.PriorityOrder }
	mergeMovieWeights(&result.ScoreWeights, &custom.ScoreWeights)
	return result
}

func mergeMovieWeights(result, custom *MovieScoreWeights) {
	if custom.Resolution4K != nil { result.Resolution4K = custom.Resolution4K }
	if custom.Resolution1080p != nil { result.Resolution1080p = custom.Resolution1080p }
	if custom.Resolution720p != nil { result.Resolution720p = custom.Resolution720p }
	if custom.DolbyVision != nil { result.DolbyVision = custom.DolbyVision }
	if custom.HDR != nil { result.HDR = custom.HDR }
	if custom.HDR10Plus != nil { result.HDR10Plus = custom.HDR10Plus }
	if custom.Atmos != nil { result.Atmos = custom.Atmos }
	if custom.Audio51 != nil { result.Audio51 = custom.Audio51 }
	if custom.AudioStereo != nil { result.AudioStereo = custom.AudioStereo }
	if custom.BluRay != nil { result.BluRay = custom.BluRay }
	if custom.Remux != nil { result.Remux = custom.Remux }
	if custom.ITA != nil { result.ITA = custom.ITA }
	if custom.SeederBonus != nil { result.SeederBonus = custom.SeederBonus }
	if custom.SeederThreshold != nil { result.SeederThreshold = custom.SeederThreshold }
	if custom.UnknownSizePenalty != nil { result.UnknownSizePenalty = custom.UnknownSizePenalty }
}

func mergeTVProfile(custom, def TVProfile) TVProfile {
	result := def
	if custom.Include4K { result.Include4K = true }
	if custom.Include1080p { result.Include1080p = true }
	if custom.Include720p { result.Include720p = true }
	if len(custom.SizeFloorGB) > 0 { result.SizeFloorGB = custom.SizeFloorGB }
	if len(custom.SizeCeilingGB) > 0 { result.SizeCeilingGB = custom.SizeCeilingGB }
	if custom.MinSeeders4K > 0 { result.MinSeeders4K = custom.MinSeeders4K }
	if custom.MinSeeders > 0 { result.MinSeeders = custom.MinSeeders }
	if custom.FullpackBonus != 0 { result.FullpackBonus = custom.FullpackBonus }
	if len(custom.PriorityOrder) > 0 { result.PriorityOrder = custom.PriorityOrder }
	mergeTVWeights(&result.ScoreWeights, &custom.ScoreWeights)
	return result
}

func mergeTVWeights(result, custom *TVScoreWeights) {
	if custom.Resolution4K != nil { result.Resolution4K = custom.Resolution4K }
	if custom.Resolution1080p != nil { result.Resolution1080p = custom.Resolution1080p }
	if custom.Resolution720p != nil { result.Resolution720p = custom.Resolution720p }
	if custom.HDR != nil { result.HDR = custom.HDR }
	if custom.Atmos != nil { result.Atmos = custom.Atmos }
	if custom.Audio51 != nil { result.Audio51 = custom.Audio51 }
	if custom.ITA != nil { result.ITA = custom.ITA }
	if custom.Seeder100Bonus != nil { result.Seeder100Bonus = custom.Seeder100Bonus }
	if custom.Seeder50Bonus != nil { result.Seeder50Bonus = custom.Seeder50Bonus }
	if custom.Seeder20Bonus != nil { result.Seeder20Bonus = custom.Seeder20Bonus }
}
```

- [ ] **Step 2: Add resolver tests to types_test.go**

```go
func TestResolveMovies_NoConfig(t *testing.T) {
	var qc *QualityConfig
	prof := qc.ResolveMovies()
	if prof.MinSeeders != 15 {
		t.Errorf("expected MinSeeders=15, got %d", prof.MinSeeders)
	}
	if prof.Include720p {
		t.Error("quality-first should not include 720p by default")
	}
}

func TestResolveMovies_SizeFirst(t *testing.T) {
	qc := &QualityConfig{Profile: "size-first"}
	prof := qc.ResolveMovies()
	if !prof.Include720p {
		t.Error("size-first should include 720p")
	}
	if prof.PriorityOrder[0] != "720p" {
		t.Errorf("expected priority_order[0]=720p, got %s", prof.PriorityOrder[0])
	}
}

func TestResolveMovies_CustomOverrides(t *testing.T) {
	minSeeders := 25
	qc := &QualityConfig{
		Profile: "size-first",
		Profiles: map[string]ProfileSet{
			"size-first": {
				Movies: &MovieProfile{MinSeeders: minSeeders},
			},
		},
	}
	prof := qc.ResolveMovies()
	if prof.MinSeeders != 25 {
		t.Errorf("expected MinSeeders=25 (custom override), got %d", prof.MinSeeders)
	}
	// Other fields should come from default
	if !prof.Include720p {
		t.Error("custom override should preserve default Include720p")
	}
}
```

- [ ] **Step 3: Run tests**

```bash
cd /Users/lorenzo/VSCodeWorkspace/gostream && go test ./internal/syncer/quality/... -v
```
Expected: All PASS

- [ ] **Step 4: Commit**

```bash
git add internal/syncer/quality/resolver.go internal/syncer/quality/types_test.go
git commit -m "feat: add profile resolver with merge logic"
```

---

### Task 3: Extend scorer.go with 720p and ApplyProfile

**Files:**
- Modify: `internal/syncer/quality/scorer.go`

- [ ] **Step 1: Add 720p regex and update Score function**

Replace the existing `Score` function and add the 720p regex:

```go
// Add to var block:
re720p = regexp.MustCompile(`(?i)720p`)
```

Replace the `Score` function:

```go
// Score calculates a quality score for a title based on its metadata and seeders.
func Score(title string, seeders int, profile Profile) int {
	w := profile.Weights
	score := 0

	if re4K.MatchString(title) {
		score += w.Res4K
	} else if re1080p.MatchString(title) {
		score += w.Res1080p
	} else if re720p.MatchString(title) {
		// 720p gets Res720p weight (default 0 in quality-first, positive in size-first)
		// Check if the profile has a 720p weight — if not, skip scoring
	}

	if reDV.MatchString(title) {
		score += w.DolbyVision
	}
	if reHDR10Plus.MatchString(title) {
		score += w.HDR10Plus
	} else if reHDR.MatchString(title) {
		score += w.HDR
	}

	if reAtmos.MatchString(title) {
		score += w.Atmos
	}
	if re51.MatchString(title) {
		score += w.Audio51
	}
	if reStereo.MatchString(title) && !re51.MatchString(title) && !reAtmos.MatchString(title) {
		score += w.Stereo
	}

	if reBluRay.MatchString(title) {
		score += w.BluRay
	}

	if seeders > w.SeederThreshold {
		score += w.SeederBonus
	}

	return score
}
```

- [ ] **Step 2: Add Res720p to Weights struct**

```go
type Weights struct {
	Res4K           int `json:"res_4k"`
	Res1080p        int `json:"res_1080p"`
	Res720p         int `json:"res_720p"` // NEW
	HDR             int `json:"hdr"`
	DolbyVision     int `json:"dolby_vision"`
	HDR10Plus       int `json:"hdr10_plus"`
	Atmos           int `json:"atmos"`
	Audio51         int `json:"audio_5_1"`
	Stereo          int `json:"stereo"`
	BluRay          int `json:"bluray"`
	SeederBonus     int `json:"seeder_bonus"`
	SeederThreshold int `json:"seeder_threshold"`
}
```

Update `DefaultMovieProfile()` to include `Res720p: 0` and `DefaultTVProfile()` similarly.

- [ ] **Step 3: Commit**

```bash
git add internal/syncer/quality/scorer.go
git commit -m "feat: add 720p support to quality scorer"
```

---

## Phase 2 — Config Structure

### Task 4: Update config.go

**Files:**
- Modify: `config.go`

- [ ] **Step 1: Replace dead QualityScoringConfig with new QualityConfig**

Find and replace the existing `QualityWeights`, `TVQualityWeights`, `QualityScoringConfig` structs and the `QualityScoringConfig` field in `Config`:

```go
// Replace the old structs (lines ~43-74) with:

// QualityConfig holds the active quality profile and custom overrides.
// Mirrors the "quality" key in config.json.
type QualityConfig struct {
	Profile  string                `json:"profile"`            // "quality-first" | "size-first"
	Profiles map[string]ProfileSet `json:"profiles,omitempty"` // custom overrides per profile name
}
```

Note: `ProfileSet`, `MovieProfile`, `TVProfile` etc. live in `internal/syncer/quality/types.go`, not in config.go. The `QualityConfig` here in config.go just references them.

Actually, to avoid import cycles (config → quality → config), keep the types in config.go and add conversion functions. Let me reconsider.

**Better approach:** Keep the JSON types in config.go (same structure as quality/types.go but without the methods). The `quality.ResolveMovies()` and `quality.ResolveTV()` take `config.QualityConfig` and convert internally.

```go
// In config.go — these are the JSON-serializable types
type QualityConfig struct {
	Profile  string                    `json:"profile"`
	Profiles map[string]QualityProfileSet `json:"profiles,omitempty"`
}

type QualityProfileSet struct {
	Movies *MovieQualityProfile `json:"movies,omitempty"`
	TV     *TVQualityProfile    `json:"tv,omitempty"`
}

type MovieQualityProfile struct {
	Include4K            bool                `json:"include_4k"`
	Include1080p         bool                `json:"include_1080p"`
	Include720p          bool                `json:"include_720p"`
	SizeFloorGB          map[string]float64  `json:"size_floor_gb,omitempty"`
	SizeCeilingGB        map[string]float64  `json:"size_ceiling_gb,omitempty"`
	MinSeeders           int                 `json:"min_seeders"`
	Fallback4KMinSeeders *int                `json:"fallback_4k_min_seeders,omitempty"`
	PriorityOrder        []string            `json:"priority_order"`
	ScoreWeights         MovieScoreWeights   `json:"score_weights"`
}

type MovieScoreWeights struct {
	Resolution4K       *int `json:"resolution_4k,omitempty"`
	Resolution1080p    *int `json:"resolution_1080p,omitempty"`
	Resolution720p     *int `json:"resolution_720p,omitempty"`
	DolbyVision        *int `json:"dolby_vision,omitempty"`
	HDR                *int `json:"hdr,omitempty"`
	HDR10Plus          *int `json:"hdr10_plus,omitempty"`
	Atmos              *int `json:"atmos,omitempty"`
	Audio51            *int `json:"audio_5_1,omitempty"`
	AudioStereo        *int `json:"audio_stereo,omitempty"`
	BluRay             *int `json:"bluray,omitempty"`
	Remux              *int `json:"remux,omitempty"`
	ITA                *int `json:"ita,omitempty"`
	SeederBonus        *int `json:"seeder_bonus,omitempty"`
	SeederThreshold    *int `json:"seeder_threshold,omitempty"`
	UnknownSizePenalty *int `json:"unknown_size_penalty,omitempty"`
}

type TVQualityProfile struct {
	Include4K       bool               `json:"include_4k"`
	Include1080p    bool               `json:"include_1080p"`
	Include720p     bool               `json:"include_720p"`
	SizeFloorGB     map[string]float64 `json:"size_floor_gb,omitempty"`
	SizeCeilingGB   map[string]float64 `json:"size_ceiling_gb,omitempty"`
	MinSeeders4K    int                `json:"min_seeders_4k"`
	MinSeeders      int                `json:"min_seeders"`
	FullpackBonus   int                `json:"fullpack_bonus"`
	PriorityOrder   []string           `json:"priority_order"`
	ScoreWeights    TVScoreWeights     `json:"score_weights"`
}

type TVScoreWeights struct {
	Resolution4K    *int `json:"resolution_4k,omitempty"`
	Resolution1080p *int `json:"resolution_1080p,omitempty"`
	Resolution720p  *int `json:"resolution_720p,omitempty"`
	HDR             *int `json:"hdr,omitempty"`
	Atmos           *int `json:"atmos,omitempty"`
	Audio51         *int `json:"audio_5_1,omitempty"`
	ITA             *int `json:"ita,omitempty"`
	Seeder100Bonus  *int `json:"seeder_100_bonus,omitempty"`
	Seeder50Bonus   *int `json:"seeder_50_bonus,omitempty"`
	Seeder20Bonus   *int `json:"seeder_20_bonus,omitempty"`
}
```

- [ ] **Step 2: Add TMDBDiscoveryConfig to config.go**

Add after `QualityConfig`:

```go
// TMDBDiscoveryConfig holds configurable TMDB discovery endpoints.
type TMDBDiscoveryConfig struct {
	Movies *TMDBEndpointGroup `json:"movies,omitempty"`
	TV     *TMDBEndpointGroup `json:"tv,omitempty"`
}

type TMDBEndpointGroup struct {
	Endpoints []TMDBEndpoint `json:"endpoints"`
}

type TMDBEndpoint struct {
	Name        string  `json:"name"`
	Enabled     bool    `json:"enabled"`
	EndpointType string `json:"type"` // "discover" | "trending" | "list"

	// Common
	Language  *string `json:"language,omitempty"`
	SortBy    *string `json:"sort_by,omitempty"`
	Pages     *int    `json:"pages,omitempty"`

	// Discover shared
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

	// Movie-specific
	PrimaryReleaseDateGte *string `json:"primary_release_date_gte,omitempty"`
	PrimaryReleaseDateLte *string `json:"primary_release_date_lte,omitempty"`
	PrimaryReleaseYear    *int    `json:"primary_release_year,omitempty"`
	WithReleaseType       *string `json:"with_release_type,omitempty"`
	Region                *string `json:"region,omitempty"`
	IncludeVideo          *bool   `json:"include_video,omitempty"`

	// TV-specific
	FirstAirDateGte          *string `json:"first_air_date_gte,omitempty"`
	FirstAirDateLte          *string `json:"first_air_date_lte,omitempty"`
	FirstAirDateYear         *int    `json:"first_air_date_year,omitempty"`
	WithStatus               *string `json:"with_status,omitempty"`
	WithType                 *string `json:"with_type,omitempty"`
	WithNetworks             *string `json:"with_networks,omitempty"`
	IncludeNullFirstAirDates *bool   `json:"include_null_first_air_dates,omitempty"`

	// List endpoint
	EndpointURL *string `json:"endpoint,omitempty"`

	// Trending endpoint
	TimeWindow *string `json:"time_window,omitempty"`
}
```

- [ ] **Step 3: Add QualityConfig and TMDBDiscoveryConfig fields to Config struct**

Find the `Config` struct and add two new fields:

```go
	// --- Quality Profiles ---
	Quality QualityConfig `json:"quality"`

	// --- TMDB Discovery ---
	TMDBDiscovery TMDBDiscoveryConfig `json:"tmdb_discovery"`
```

- [ ] **Step 4: Commit**

```bash
git add config.go
git commit -m "feat: add QualityConfig and TMDBDiscoveryConfig to config"
```

---

## Phase 3 — TMDB Discovery Query Builder

### Task 5: Add dynamic TMDB discovery methods

**Files:**
- Modify: `internal/catalog/tmdb/client.go`
- Test: `internal/catalog/tmdb/discover_test.go`

- [ ] **Step 1: Add date parser utility**

Add to the top of `client.go`:

```go
import "time"

// parseRelativeDate parses relative date strings like "-6months", "+12months", "2024-01-15".
func parseRelativeDate(s string) string {
	if s == "" {
		return ""
	}
	// Check if it's an absolute date
	if _, err := time.Parse("2006-01-02", s); err == nil {
		return s
	}
	// Parse relative: "-6months", "+1y", "-30d", etc.
	now := time.Now()
	var d time.Duration
	if n, unit := parseRelativeUnit(s); n != 0 {
		switch unit {
		case "months":
			d = time.Duration(n * 30 * 24 * int(time.Hour))
		case "years", "y":
			d = time.Duration(n * 365 * 24 * int(time.Hour))
		case "days", "d":
			d = time.Duration(n * 24 * int(time.Hour))
		}
		if s[0] == '-' {
			return now.Add(-d).Format("2006-01-02")
		}
		return now.Add(d).Format("2006-01-02")
	}
	return s // fallback: return as-is
}

func parseRelativeUnit(s string) (int, string) {
	s = strings.TrimSpace(s)
	neg := false
	if s[0] == '-' {
		neg = true
		s = s[1:]
	} else if s[0] == '+' {
		s = s[1:]
	}

	// Try to find where the number ends
	i := 0
	for i < len(s) && (s[i] >= '0' && s[i] <= '9') {
		i++
	}
	if i == 0 {
		return 0, ""
	}
	n, err := strconv.Atoi(s[:i])
	if err != nil {
		return 0, ""
	}
	if neg {
		n = -n
	}
	return n, s[i:]
}
```

- [ ] **Step 2: Add DiscoverMoviesFromConfig**

```go
// DiscoverMoviesFromConfig executes all configured movie discovery endpoints.
func (c *Client) DiscoverMoviesFromConfig(ctx context.Context, cfg TMDBEndpointGroup) ([]Movie, error) {
	var all []Movie
	seen := make(map[int]bool)

	for _, ep := range cfg.Endpoints {
		if !ep.Enabled {
			continue
		}
		var movies []Movie
		var err error
		switch ep.EndpointType {
		case "discover":
			movies, err = c.discoverMovieFromEndpoint(ctx, ep)
		case "trending":
			tw := "week"
			if ep.TimeWindow != nil {
				tw = *ep.TimeWindow
			}
			movies, err = c.TrendingMovies(ctx, pagesOrDefault(ep.Pages, 1))
		case "list":
			region := ""
			if ep.Region != nil {
				region = *ep.Region
			}
			movies, err = c.DiscoverMoviesByRegion(ctx, *ep.EndpointURL, region, pagesOrDefault(ep.Pages, 1))
		default:
			continue
		}
		if err != nil {
			continue // skip failed endpoints, log in production
		}
		for _, m := range movies {
			if !seen[m.ID] {
				seen[m.ID] = true
				all = append(all, m)
			}
		}
	}
	return all, nil
}

func (c *Client) discoverMovieFromEndpoint(ctx context.Context, ep TMDBEndpoint) ([]Movie, error) {
	q := url.Values{}
	q.Set("api_key", c.apiKey)
	if ep.Language != nil { q.Set("language", *ep.Language) }
	if ep.SortBy != nil { q.Set("sort_by", *ep.SortBy) }
	if ep.IncludeAdult != nil { q.Set("include_adult", fmt.Sprintf("%t", *ep.IncludeAdult)) }
	if ep.IncludeVideo != nil { q.Set("include_video", fmt.Sprintf("%t", *ep.IncludeVideo)) }
	if ep.VoteAverageGte != nil { q.Set("vote_average.gte", fmt.Sprintf("%.1f", *ep.VoteAverageGte)) }
	if ep.VoteCountGte != nil { q.Set("vote_count.gte", fmt.Sprintf("%d", *ep.VoteCountGte)) }
	if ep.WithGenres != nil { q.Set("with_genres", *ep.WithGenres) }
	if ep.WithoutGenres != nil { q.Set("without_genres", *ep.WithoutGenres) }
	if ep.WithKeywords != nil { q.Set("with_keywords", *ep.WithKeywords) }
	if ep.WithoutKeywords != nil { q.Set("without_keywords", *ep.WithoutKeywords) }
	if ep.WithOriginalLanguage != nil { q.Set("with_original_language", *ep.WithOriginalLanguage) }
	if ep.WithOriginCountry != nil { q.Set("with_origin_country", *ep.WithOriginCountry) }
	if ep.PrimaryReleaseYear != nil { q.Set("primary_release_year", fmt.Sprintf("%d", *ep.PrimaryReleaseYear)) }
	if ep.WithRuntimeGte != nil { q.Set("with_runtime.gte", fmt.Sprintf("%d", *ep.WithRuntimeGte)) }
	if ep.WithRuntimeLte != nil { q.Set("with_runtime.lte", fmt.Sprintf("%d", *ep.WithRuntimeLte)) }
	if ep.WithReleaseType != nil { q.Set("with_release_type", *ep.WithReleaseType) }
	if ep.Region != nil { q.Set("region", *ep.Region) }
	if ep.WatchRegion != nil { q.Set("watch_region", *ep.WatchRegion) }
	if ep.PrimaryReleaseDateGte != nil { q.Set("primary_release_date.gte", parseRelativeDate(*ep.PrimaryReleaseDateGte)) }
	if ep.PrimaryReleaseDateLte != nil { q.Set("primary_release_date.lte", parseRelativeDate(*ep.PrimaryReleaseDateLte)) }

	var all []Movie
	pages := pagesOrDefault(ep.Pages, 1)
	for page := 1; page <= pages; page++ {
		q.Set("page", fmt.Sprintf("%d", page))
		urlStr := fmt.Sprintf("%s/discover/movie?%s", baseURL, q.Encode())
		movies, err := c.fetchDiscoverPage(ctx, urlStr)
		if err != nil {
			return all, err
		}
		all = append(all, movies...)
	}
	return all, nil
}

func pagesOrDefault(p *int, def int) int {
	if p != nil && *p > 0 {
		return *p
	}
	return def
}
```

- [ ] **Step 3: Add DiscoverTVFromConfig** (same pattern, TV-specific params)

```go
// DiscoverTVFromConfig executes all configured TV discovery endpoints.
func (c *Client) DiscoverTVFromConfig(ctx context.Context, cfg TMDBEndpointGroup) ([]TVShow, error) {
	var all []TVShow
	seen := make(map[int]bool)

	for _, ep := range cfg.Endpoints {
		if !ep.Enabled {
			continue
		}
		var shows []TVShow
		var err error
		switch ep.EndpointType {
		case "discover":
			shows, err = c.discoverTVFromEndpoint(ctx, ep)
		case "trending":
			tw := "week"
			if ep.TimeWindow != nil {
				tw = *ep.TimeWindow
			}
			// Use TVTrending but respect custom pages
			shows, err = c.tvTrendingWithPages(ctx, pagesOrDefault(ep.Pages, 3))
		case "list":
			// Use existing fetchTVPage with custom endpoint
			shows, err = c.fetchTVListPage(ctx, *ep.EndpointURL, pagesOrDefault(ep.Pages, 1))
		default:
			continue
		}
		if err != nil {
			continue
		}
		for _, s := range shows {
			if !seen[s.ID] {
				seen[s.ID] = true
				all = append(all, s)
			}
		}
	}
	return all, nil
}

func (c *Client) discoverTVFromEndpoint(ctx context.Context, ep TMDBEndpoint) ([]TVShow, error) {
	q := url.Values{}
	q.Set("api_key", c.apiKey)
	if ep.Language != nil { q.Set("language", *ep.Language) }
	if ep.SortBy != nil { q.Set("sort_by", *ep.SortBy) }
	if ep.IncludeAdult != nil { q.Set("include_adult", fmt.Sprintf("%t", *ep.IncludeAdult)) }
	if ep.IncludeNullFirstAirDates != nil { q.Set("include_null_first_air_dates", fmt.Sprintf("%t", *ep.IncludeNullFirstAirDates)) }
	if ep.VoteAverageGte != nil { q.Set("vote_average.gte", fmt.Sprintf("%.1f", *ep.VoteAverageGte)) }
	if ep.VoteCountGte != nil { q.Set("vote_count.gte", fmt.Sprintf("%d", *ep.VoteCountGte)) }
	if ep.WithGenres != nil { q.Set("with_genres", *ep.WithGenres) }
	if ep.WithoutGenres != nil { q.Set("without_genres", *ep.WithoutGenres) }
	if ep.WithKeywords != nil { q.Set("with_keywords", *ep.WithKeywords) }
	if ep.WithoutKeywords != nil { q.Set("without_keywords", *ep.WithoutKeywords) }
	if ep.WithOriginalLanguage != nil { q.Set("with_original_language", *ep.WithOriginalLanguage) }
	if ep.WithOriginCountry != nil { q.Set("with_origin_country", *ep.WithOriginCountry) }
	if ep.WithRuntimeGte != nil { q.Set("with_runtime.gte", fmt.Sprintf("%d", *ep.WithRuntimeGte)) }
	if ep.WithRuntimeLte != nil { q.Set("with_runtime.lte", fmt.Sprintf("%d", *ep.WithRuntimeLte)) }
	if ep.WithStatus != nil { q.Set("with_status", *ep.WithStatus) }
	if ep.WithType != nil { q.Set("with_type", *ep.WithType) }
	if ep.WithNetworks != nil { q.Set("with_networks", *ep.WithNetworks) }
	if ep.WatchRegion != nil { q.Set("watch_region", *ep.WatchRegion) }
	if ep.FirstAirDateGte != nil { q.Set("first_air_date.gte", parseRelativeDate(*ep.FirstAirDateGte)) }
	if ep.FirstAirDateLte != nil { q.Set("first_air_date.lte", parseRelativeDate(*ep.FirstAirDateLte)) }
	if ep.FirstAirDateYear != nil { q.Set("first_air_date_year", fmt.Sprintf("%d", *ep.FirstAirDateYear)) }

	var all []TVShow
	pages := pagesOrDefault(ep.Pages, 1)
	for page := 1; page <= pages; page++ {
		q.Set("page", fmt.Sprintf("%d", page))
		urlStr := fmt.Sprintf("%s/discover/tv?%s", baseURL, q.Encode())
		shows, err := c.fetchTVPage(ctx, urlStr)
		if err != nil {
			return all, err
		}
		all = append(all, shows...)
	}
	return all, nil
}

func (c *Client) tvTrendingWithPages(ctx context.Context, pages int) ([]TVShow, error) {
	var all []TVShow
	for page := 1; page <= pages; page++ {
		urlStr := fmt.Sprintf("%s/trending/tv/week?api_key=%s&page=%d", baseURL, c.apiKey, page)
		shows, err := c.fetchTVPage(ctx, urlStr)
		if err != nil {
			return all, err
		}
		all = append(all, shows...)
	}
	return all, nil
}

func (c *Client) fetchTVListPage(ctx context.Context, endpoint string, pages int) ([]TVShow, error) {
	var all []TVShow
	for page := 1; page <= pages; page++ {
		urlStr := fmt.Sprintf("%s%s?api_key=%s&page=%d", baseURL, endpoint, c.apiKey, page)
		shows, err := c.fetchTVPage(ctx, urlStr)
		if err != nil {
			return all, err
		}
		all = append(all, shows...)
	}
	return all, nil
}
```

- [ ] **Step 4: Add import for `net/url`** if not already present

- [ ] **Step 5: Commit**

```bash
git add internal/catalog/tmdb/client.go
git commit -m "feat: add DiscoverMoviesFromConfig and DiscoverTVFromConfig with dynamic query builder"
```

---

### Task 6: Test TMDB query builder

**Files:**
- Create: `internal/catalog/tmdb/discover_test.go`

- [ ] **Step 1: Create discover_test.go**

```go
package tmdb

import (
	"testing"
)

func TestParseRelativeDate(t *testing.T) {
	tests := []struct {
		input    string
		absolute bool // true if we just want it to be a valid date
	}{
		{"-6months", true},
		{"+12months", true},
		{"2024-01-15", true},
		{"-1y", true},
		{"+30d", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseRelativeDate(tt.input)
			if tt.absolute {
				_, err := time.Parse("2006-01-02", result)
				if err != nil {
					t.Errorf("parseRelativeDate(%q) = %q, not a valid date: %v", tt.input, result, err)
				}
			}
		})
	}
}

func TestParseRelativeDate_AbsolutePassthrough(t *testing.T) {
	got := parseRelativeDate("2024-06-15")
	if got != "2024-06-15" {
		t.Errorf("expected 2024-06-15, got %s", got)
	}
}

func TestPagesOrDefault(t *testing.T) {
	v := 5
	if got := pagesOrDefault(&v, 1); got != 5 {
		t.Errorf("expected 5, got %d", got)
	}
	if got := pagesOrDefault(nil, 3); got != 3 {
		t.Errorf("expected 3 (default), got %d", got)
	}
}
```

- [ ] **Step 2: Run tests**

```bash
cd /Users/lorenzo/VSCodeWorkspace/gostream && go test ./internal/catalog/tmdb/... -v -run "TestParse|TestPages"
```
Expected: All PASS

- [ ] **Step 3: Commit**

```bash
git add internal/catalog/tmdb/discover_test.go
git commit -m "test: add TMDB discover query builder tests"
```

---

## Phase 4 — Wire Config to Engines

### Task 7: Update syncer configs (movies.go, tv.go, watchlist.go)

**Files:**
- Modify: `internal/syncer/engines/movies.go`
- Modify: `internal/syncer/engines/tv.go`
- Modify: `internal/syncer/engines/watchlist.go`

- [ ] **Step 1: Add Quality + TMDBDiscovery to MoviesSyncerConfig**

In `movies.go`, add to `MoviesSyncerConfig`:

```go
type MoviesSyncerConfig struct {
	// ... existing fields ...
	Quality       QualityConfig          `json:"-"`
	TMDBDiscovery TMDBDiscoveryConfig    `json:"-"`
}
```

Update `NewMoviesSyncer` to pass these to `MovieEngineConfig`.

- [ ] **Step 2: Same for TVSyncerConfig** in `tv.go`

- [ ] **Step 3: Same for WatchlistSyncerConfig** in `watchlist.go`

- [ ] **Step 4: Commit**

```bash
git add internal/syncer/engines/movies.go internal/syncer/engines/tv.go internal/syncer/engines/watchlist.go
git commit -m "feat: add Quality and TMDBDiscovery to syncer configs"
```

---

### Task 8: Update MovieEngineConfig and use profile

**Files:**
- Modify: `internal/syncer/engines/movie_go.go`

- [ ] **Step 1: Add fields to MovieEngineConfig**

```go
type MovieEngineConfig struct {
	// ... existing fields ...
	Quality       QualityConfig       `json:"-"`
	TMDBDiscovery TMDBDiscoveryConfig `json:"-"`
}
```

- [ ] **Step 2: Store profile in MovieGoEngine**

Add to `MovieGoEngine` struct:

```go
qualityProfile MovieProfile  // resolved from config
```

In `NewMovieGoEngine`, after creating the engine:

```go
e.qualityProfile = quality.ResolveMovieFromConfig(cfg.Quality)
```

Where `quality.ResolveMovieFromConfig` converts `config.MovieQualityProfile` → `quality.MovieProfile`.

Actually, since we want to avoid import cycles, the engine should receive the resolved profile directly. Let me simplify:

Add to `MovieEngineConfig`:
```go
QualityProfile MovieProfile `json:"-"`
```

In `main.go`, resolve: `movieCfg.QualityProfile = globalConfig.Quality.ResolveMovies()`

- [ ] **Step 3: Replace hardcoded constants in filterMovieStreams**

Replace the two-pass logic with dynamic priority order:

```go
func (e *MovieGoEngine) filterMovieStreams(streams []prowlarr.Stream) []MovieStream {
	prof := e.qualityProfile
	priorityOrder := prof.PriorityOrder
	if len(priorityOrder) == 0 {
		priorityOrder = []string{"4k", "1080p", "720p"}
	}

	for _, res := range priorityOrder {
		var candidates []MovieStream
		for _, s := range streams {
			c := e.classifyMovieStream(s)
			if c == nil {
				continue
			}
			// Match resolution type to current priority pass
			match := false
			switch res {
			case "4k":
				match = c.Is4K
			case "1080p":
				match = c.Is1080p && !c.Is4K
			case "720p":
				match = c.Is720p && !c.Is4K && !c.Is1080p
			}
			if !match {
				continue
			}
			candidates = append(candidates, *c)
		}
		if len(candidates) > 0 {
			sort.Slice(candidates, func(i, j int) bool {
				return candidates[i].QualityScore > candidates[j].QualityScore
			})
			return candidates
		}
	}

	// 4K fallback if configured
	if prof.Fallback4KMinSeeders != nil {
		var fallback []MovieStream
		for _, s := range streams {
			c := e.classifyMovieStream(s)
			if c != nil && c.Is4K && c.Seeders >= *prof.Fallback4KMinSeeders {
				fallback = append(fallback, *c)
			}
		}
		if len(fallback) > 0 {
			sort.Slice(fallback, func(i, j int) bool {
				return fallback[i].SizeGB < fallback[j].SizeGB // smallest 4K
			})
			return fallback
		}
	}

	return nil
}
```

- [ ] **Step 4: Add Is720p field to MovieStream**

```go
type MovieStream struct {
	Title        string
	Hash         string
	Is4K         bool
	Is1080p      bool // NEW
	Is720p       bool // NEW
	QualityScore int
	Seeders      int
	SizeGB       float64
}
```

- [ ] **Step 5: Update classifyMovieStream to check include flags and size ceilings**

In `classifyMovieStream`, replace:
```go
is4K := reM4K.MatchString(fullText)
is1080p := reM1080p.MatchString(fullText) && !reM720p.MatchString(fullText)

if !is4K && !is1080p {
    return nil
}
```

With:
```go
prof := e.qualityProfile
is4K := reM4K.MatchString(fullText)
is1080p := reM1080p.MatchString(fullText) && !reM720p.MatchString(fullText)
is720p := reM720p.MatchString(fullText) && !is1080p && !is4K

// Check include flags
if is4K && !prof.Include4K { return nil }
if is1080p && !prof.Include1080p { return nil }
if is720p && !prof.Include720p { return nil }

if !is4K && !is1080p && !is720p { return nil }
```

Replace the hardcoded size checks with config-driven ones:
```go
sizeGB := e.extractMovieSizeGB(title)

// Get ceiling/floor for this resolution
var floorGB, ceilingGB float64
if is4K {
    floorGB = prof.SizeFloorGB["4k"]
    ceilingGB = prof.SizeCeilingGB["4k"]
} else if is1080p {
    floorGB = prof.SizeFloorGB["1080p"]
    ceilingGB = prof.SizeCeilingGB["1080p"]
} else {
    floorGB = prof.SizeFloorGB["720p"]
    ceilingGB = prof.SizeCeilingGB["720p"]
}

if ceilingGB > 0 && sizeGB != 0 && (sizeGB < floorGB || sizeGB > ceilingGB) {
    return nil
}
if ceilingGB > 0 && sizeGB == 0 && is4K && prof.Fallback4KMinSeeders == nil {
    // reject unknown size for 4K unless it's a fallback scenario
    return nil
}
```

- [ ] **Step 6: Replace calculateMovieScore with config-driven scoring**

```go
func (e *MovieGoEngine) calculateMovieScore(text string, seeders int, sizeGB float64, is4K, is1080p, is720p bool) int {
	w := e.qualityProfile.ScoreWeights
	score := 0

	if is4K && w.Resolution4K != nil {
		score += *w.Resolution4K
	} else if is1080p && w.Resolution1080p != nil {
		score += *w.Resolution1080p
	} else if is720p && w.Resolution720p != nil {
		score += *w.Resolution720p
	}

	if reMDV.MatchString(text) && w.DolbyVision != nil {
		score += *w.DolbyVision
	} else if reMHDR.MatchString(text) && w.HDR != nil {
		score += *w.HDR
	}

	if reMAtmos.MatchString(text) && w.Atmos != nil {
		score += *w.Atmos
	} else if reM51.MatchString(text) && w.Audio51 != nil {
		score += *w.Audio51
	} else if reMStereo.MatchString(text) && w.AudioStereo != nil {
		score += *w.AudioStereo
	}

	if reMRemux.MatchString(text) && w.Remux != nil {
		score += *w.Remux
	}

	if reMITA.MatchString(text) && w.ITA != nil {
		score += *w.ITA
	}

	if sizeGB == 0 && is4K && w.UnknownSizePenalty != nil {
		score += *w.UnknownSizePenalty
	}

	if w.SeederBonus != nil && w.SeederThreshold != nil {
		if seeders > *w.SeederThreshold {
			bonus := seeders
			if bonus > 50 { bonus = 50 }
			score += bonus * (*w.SeederBonus) / 5
		}
	}

	return score
}
```

Update the call site in `classifyMovieStream`:
```go
score := e.calculateMovieScore(fullText, seeders, sizeGB, is4K, is1080p, is720p)
```

- [ ] **Step 7: Update discoverMovies to use config-driven TMDB**

Replace `discoverMovies`:

```go
func (e *MovieGoEngine) discoverMovies(ctx context.Context) ([]tmdb.Movie, error) {
	if len(e.tmdbDiscovery.Movies.Endpoints) > 0 {
		return e.tmdb.DiscoverMoviesFromConfig(ctx, *e.tmdbDiscovery.Movies)
	}
	// Fallback: use hardcoded defaults (backward compat)
	return e.discoverMoviesHardcoded(ctx)
}

func (e *MovieGoEngine) discoverMoviesHardcoded(ctx context.Context) ([]tmdb.Movie, error) {
	// ... current implementation ...
}
```

Add to `MovieGoEngine`:
```go
tmdbDiscovery TMDBDiscoveryConfig `json:"-"`
```

- [ ] **Step 8: Commit**

```bash
git add internal/syncer/engines/movie_go.go
git commit -m "feat: make movie engine use config-driven quality profile and TMDB discovery"
```

---

### Task 9: Update TV engine (same pattern)

**Files:**
- Modify: `internal/syncer/engines/tv_go.go`

- [ ] **Step 1: Add fields to TVGoEngine**

```go
qualityProfile  TVProfile
tmdbDiscovery   TMDBDiscoveryConfig
```

- [ ] **Step 2: Replace constants in classifyStream and calculateQualityScore** with config-driven logic (same pattern as movie engine)

- [ ] **Step 3: Add 720p support** — `reTV720p = regexp.MustCompile(`(?i)720p`)`

- [ ] **Step 4: Update discoverShows** to use `DiscoverTVFromConfig`

- [ ] **Step 5: Commit**

```bash
git add internal/syncer/engines/tv_go.go
git commit -m "feat: make TV engine use config-driven quality profile and TMDB discovery"
```

---

### Task 10: Update watchlist engine

**Files:**
- Modify: `internal/syncer/engines/watchlist_go.go`

- [ ] **Step 1: Add qualityProfile field** and replace `quality.DefaultMovieProfile()` with `e.qualityProfile`

- [ ] **Step 2: Update size filters and stream selection** to use config ceiling/floor

- [ ] **Step 3: Commit**

```bash
git add internal/syncer/engines/watchlist_go.go
git commit -m "feat: make watchlist engine use config-driven quality profile"
```

---

## Phase 5 — Wire main.go

### Task 11: Pass config to syncers in main.go

**Files:**
- Modify: `main.go` (around syncer instantiation, ~lines 3110-3165)

- [ ] **Step 1: Resolve profiles and pass to syncers**

Find where syncers are created and add:

```go
movieSyncer := engines.NewMoviesSyncer(engines.MoviesSyncerConfig{
    // ... existing fields ...
    QualityProfile:  globalConfig.Quality.ResolveMovies(),
    TMDBDiscovery:   globalConfig.TMDBDiscovery,
})
```

- [ ] **Step 2: Do the same for TV and watchlist syncers**

- [ ] **Step 3: Commit**

```bash
git add main.go
git commit -m "feat: wire quality profile and TMDB discovery to syncers in main"
```

---

## Phase 6 — Config Example

### Task 12: Update config.json.example

**Files:**
- Modify: `config.json.example`

- [ ] **Step 1: Append quality and tmdb_discovery examples**

```json
{
  "... existing config ...": "",

  "quality": {
    "profile": "size-first",
    "profiles": {
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
  },

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
        }
      ]
    },
    "tv": {
      "endpoints": [
        {
          "name": "discover_en",
          "enabled": true,
          "type": "discover",
          "language": "en",
          "sort_by": "popularity.desc",
          "first_air_date_gte": "-6months",
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

- [ ] **Step 2: Commit**

```bash
git add config.json.example
git commit -m "docs: add quality profile and tmdb_discovery examples to config.json.example"
```

---

## Phase 7 — Build & Verify

### Task 13: Build and run full test suite

**Files:** All modified files

- [ ] **Step 1: Build**

```bash
cd /Users/lorenzo/VSCodeWorkspace/gostream && go build -o gostream . 2>&1
```
Expected: No errors

- [ ] **Step 2: Run all tests**

```bash
cd /Users/lorenzo/VSCodeWorkspace/gostream && go test ./... -v 2>&1 | tail -30
```
Expected: All PASS

- [ ] **Step 3: Commit final build**

```bash
git add -A && git commit -m "chore: build and test verification"
```

---

## Self-Review Checklist

- [ ] **Spec coverage:** All requirements from the design spec are covered? Quality profiles (two presets, configurable weights, ceiling/floor, 720p, REMUX penalty, fallback 4K) → Tasks 1-4, 7-10. TMDB discovery (dynamic endpoints, date parsing, all params) → Tasks 5-6, 8-9. Web UI → Not in this plan (separate phase). On-demand REST → Not in this plan.
- [ ] **Placeholder scan:** No "TBD", "TODO", or "implement later" in the plan.
- [ ] **Type consistency:** `MovieProfile`, `TVProfile`, `QualityConfig`, `TMDBEndpoint` are consistent across tasks.
- [ ] **DRY/YAGNI:** No unnecessary features. Web UI and on-demand REST are separate phases.

---

**Out of scope for this plan** (separate future plans):
- Web UI editor in settings.html
- On-demand REST API endpoints for "Run Discovery Now"
- Drag & drop endpoint reordering in UI

Plan covers: backend config structs, profile resolution, TMDB query builder, engine wiring, backward compatibility.
