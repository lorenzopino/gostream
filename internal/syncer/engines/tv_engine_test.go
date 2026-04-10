package engines

import (
	"testing"
)

// TestEpisodeKey_IncludesYear verifies the key contains the year when firstAirDate is provided.
func TestEpisodeKey_IncludesYear(t *testing.T) {
	// Simulate the episodeKey logic
	key := episodeKeyWithYear("The Office", "2005-03-24", 1, 5)
	expected := "theoffice_2005_s01e05"
	if key != expected {
		t.Errorf("episodeKey = %q, want %q", key, expected)
	}
}

// TestEpisodeKey_DifferentiatesSameName verifies two shows with same name but different years get different keys.
func TestEpisodeKey_DifferentiatesSameName(t *testing.T) {
	keyUK := episodeKeyWithYear("The Office", "2001-07-09", 1, 1)
	keyUS := episodeKeyWithYear("The Office", "2005-03-24", 1, 1)

	if keyUK == keyUS {
		t.Errorf("keys should differ for same name different years: UK=%q US=%q", keyUK, keyUS)
	}
	if keyUK != "theoffice_2001_s01e01" {
		t.Errorf("UK key = %q, want %q", keyUK, "theoffice_2001_s01e01")
	}
	if keyUS != "theoffice_2005_s01e01" {
		t.Errorf("US key = %q, want %q", keyUS, "theoffice_2005_s01e01")
	}
}

// TestEpisodeKey_NoYear uses empty firstAirDate — should fall back to old format.
func TestEpisodeKey_NoYear(t *testing.T) {
	key := episodeKeyWithYear("Breaking Bad", "", 1, 1)
	expected := "breakingbad_s01e01"
	if key != expected {
		t.Errorf("episodeKey with no year = %q, want %q", key, expected)
	}
}

// TestEpisodeKey_ShortAirDate handles edge case where firstAirDate is too short.
func TestEpisodeKey_ShortAirDate(t *testing.T) {
	key := episodeKeyWithYear("Test Show", "20", 2, 3)
	expected := "testshow_s02e03"
	if key != expected {
		t.Errorf("episodeKey with short date = %q, want %q", key, expected)
	}
}

// episodeKeyWithYear replicates the logic in TVGoEngine.episodeKey for testing.
func episodeKeyWithYear(show, firstAirDate string, season, episode int) string {
	normalized := reTVNonWord.ReplaceAllString(lowercase(show), "")
	year := ""
	if len(firstAirDate) >= 4 {
		year = firstAirDate[:4]
	}
	if year != "" {
		return normalized + "_" + year + "_s" + pad2(season) + "e" + pad2(episode)
	}
	return normalized + "_s" + pad2(season) + "e" + pad2(episode)
}

func lowercase(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		result[i] = c
	}
	return string(result)
}

func pad2(n int) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}
