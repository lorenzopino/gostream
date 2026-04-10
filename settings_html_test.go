package main

import (
	"os"
	"strings"
	"testing"
)

func readSettingsHTML(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("settings.html")
	if err != nil {
		t.Fatalf("settings.html not found: %v", err)
	}
	return string(data)
}

func TestSettingsHTML_HasSyncFeedbackElements(t *testing.T) {
	html := readSettingsHTML(t)

	// Verify the three sync buttons exist
	buttons := []string{"sched-movies-btn", "sched-tv-btn", "sched-wl-btn"}
	for _, btn := range buttons {
		if !strings.Contains(html, `id="`+btn+`"`) {
			t.Errorf("settings.html missing button id=%q", btn)
		}
	}

	// Verify runSyncNow function exists
	if !strings.Contains(html, "function runSyncNow") {
		t.Error("settings.html missing runSyncNow function")
	}

	// Verify status message classes are defined in CSS
	for _, cls := range []string{"status-msg", "status-success", "status-error"} {
		if !strings.Contains(html, "."+cls) {
			t.Errorf("settings.html missing CSS class .%s", cls)
		}
	}
}

func TestSettingsHTML_RunSyncNowHandlesStates(t *testing.T) {
	html := readSettingsHTML(t)

	// Extract the runSyncNow function body
	start := strings.Index(html, "async function runSyncNow")
	if start == -1 {
		t.Fatal("runSyncNow function not found")
	}
	rest := html[start:]
	end := strings.Index(rest, "\n        async function ")
	if end == -1 {
		end = strings.Index(rest, "\n        function ")
	}
	if end == -1 {
		end = strings.Index(rest, "</script>")
	}
	if end == -1 {
		end = len(rest)
	}
	body := rest[:end]

	// Verify the function has feedback indicators
	checks := []struct {
		name    string
		pattern string
	}{
		{"disabled state", "disabled"},
		{"spinner/loading", "Loading"},
		{"success message", "status-success"},
		{"error message", "status-error"},
		{"already running handling", "409"},
	}
	for _, c := range checks {
		if !strings.Contains(body, c.pattern) {
			t.Errorf("runSyncNow missing %q indicator (pattern: %q)", c.name, c.pattern)
		}
	}
}

func TestSettingsHTML_StopSyncNowExists(t *testing.T) {
	html := readSettingsHTML(t)

	if !strings.Contains(html, "async function stopSyncNow") {
		t.Error("settings.html missing stopSyncNow function")
	}
}
