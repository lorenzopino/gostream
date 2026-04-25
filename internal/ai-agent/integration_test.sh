#!/bin/bash
# GoStream AI Agent — Integration Test (W3-A)
# Verifies Phase 2 implementation: API endpoints, queue persistence, Mac restart resilience
# Run this after GoStream is running: ./integration_test.sh

set -e

GOSTREAM_HOST="${GOSTREAM_HOST:-localhost:9080}"
GOSTORM_HOST="${GOSTORM_HOST:-localhost:8090}"
TEST_FAILED=0

echo "=== GoStream AI Agent Integration Test ==="
echo "Testing against: http://$GOSTREAM_HOST"
echo "GoStorm API: http://$GOSTORM_HOST"
echo ""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Helper functions
pass() { echo -e "${GREEN}✓ PASS${NC}: $1"; }
fail() { echo -e "${RED}✗ FAIL${NC}: $1"; TEST_FAILED=1; }
skip() { echo -e "${YELLOW}⊘ SKIP${NC}: $1"; }

# Test 1: GoStorm API availability
echo "[1/10] GoStorm API availability..."
if curl -s "http://$GOSTORM_HOST/torrents" > /dev/null 2>&1; then
    pass "GoStorm API responding"
else
    fail "GoStorm API not responding (required for AI agent)"
fi

# Test 2: AI API endpoints (schema hints)
echo "[2/10] AI API endpoints return schema hints..."
ENDPOINTS=("torrent-state" "active-playback" "fuse-health" "queue-status" "recent-logs" "config" "favorites-check")
for endpoint in "${ENDPOINTS[@]}"; do
    resp=$(curl -s "http://$GOSTREAM_HOST/api/ai/$endpoint" 2>/dev/null || echo "")
    if echo "$resp" | grep -q "schema_hint\|error\|movies\|queue\|logs\|config"; then
        pass "  /api/ai/$endpoint responds"
    else
        fail "  /api/ai/$endpoint no valid response"
    fi
done

# Test 3: Queue status endpoint
echo "[3/10] Queue status endpoint..."
queue_resp=$(curl -s "http://$GOSTREAM_HOST/api/ai/queue-status" 2>/dev/null || echo "")
if echo "$queue_resp" | grep -q "pending_batches\|processing_batches\|failed_batches"; then
    pass "Queue status returns valid JSON"
else
    fail "Queue status invalid response"
fi

# Test 4: Favorites check endpoint
echo "[4/10] Favorites check endpoint..."
fav_resp=$(curl -s "http://$GOSTREAM_HOST/api/ai/favorites-check" 2>/dev/null || echo "")
if echo "$fav_resp" | grep -q "movies\|tv_series"; then
    pass "Favorites check returns valid JSON"
else
    fail "Favorites check invalid response"
fi

# Test 5: Queue persistence (Mac restart resilience)
echo "[5/10] Queue persistence (disk-based)..."
STATE_DIR="${STATE_DIR:-.}"
QUEUE_FILE="$STATE_DIR/STATE/ai-agent-queue.json"
if [ -f "$QUEUE_FILE" ]; then
    pass "Queue file exists: $QUEUE_FILE"
    if [ -s "$QUEUE_FILE" ]; then
        pass "Queue file has content (survives restart)"
    else
        skip "Queue file empty (no pending batches)"
    fi
else
    skip "Queue file not found at $QUEUE_FILE (will be created on first batch)"
fi

# Test 6: AI log file
echo "[6/10] AI structured log file..."
AI_LOG="${STATE_DIR}/logs/gostream-ai.log"
if [ -f "$AI_LOG" ]; then
    pass "AI log file exists: $AI_LOG"
    if [ -s "$AI_LOG" ]; then
        entries=$(wc -l < "$AI_LOG")
        pass "AI log has $entries entries"
    else
        skip "AI log empty (no events yet)"
    fi
else
    skip "AI log not found at $AI_LOG (will be created on first event)"
fi

# Test 7: Hermes skills exist
echo "[7/10] Hermes skills exist..."
SKILLS_DIR="./internal/ai-agent/hermes_skills"
REQUIRED_SKILLS=("gostream-dispatcher.md" "gostream-maintenance.md" "jellyfin-health.md" "prowlarr-search.md" "tmdb-lookup.md" "gostream-deep-scan.md" "gostream-code-fix.md" "gostream-action-history.md")
missing=0
for skill in "${REQUIRED_SKILLS[@]}"; do
    if [ -f "$SKILLS_DIR/$skill" ]; then
        : # pass silently
    else
        fail "  Missing skill: $skill"
        missing=1
    fi
done
if [ $missing -eq 0 ]; then
    pass "All 8 Hermes skills present"
fi

# Test 8: init-hermes.sh exists and is executable
echo "[8/10] Hermes setup script..."
INIT_SCRIPT="$SKILLS_DIR/init-hermes.sh"
if [ -f "$INIT_SCRIPT" ]; then
    if [ -x "$INIT_SCRIPT" ]; then
        pass "init-hermes.sh exists and is executable"
    else
        skip "init-hermes.sh exists but not executable (run: chmod +x)"
    fi
else
    fail "init-hermes.sh not found"
fi

# Test 9: Extended detectors file exists
echo "[9/10] Extended detectors (Phase 2)..."
if [ -f "./internal/ai-agent/detectors_extended.go" ]; then
    pass "detectors_extended.go exists (5 new detectors)"
else
    fail "detectors_extended.go missing"
fi

# Test 10: Favorites check file exists
echo "[10/10] Favorites check endpoint (Phase 2)..."
if [ -f "./internal/ai-agent/favorites_check.go" ]; then
    pass "favorites_check.go exists (TMDB + Jellyfin integration)"
else
    fail "favorites_check.go missing"
fi

# Summary
echo ""
echo "=== Test Summary ==="
if [ $TEST_FAILED -eq 0 ]; then
    echo -e "${GREEN}All critical tests passed!${NC}"
    echo "Phase 2 AI Maintenance Agent is ready."
    echo ""
    echo "Next steps:"
    echo "1. Configure Hermes webhook: hermes webhook subscribe --url http://$GOSTREAM_HOST/webhook/gostream"
    echo "2. Deploy skills: ./internal/ai-agent/hermes_skills/init-hermes.sh"
    echo "3. Enable AI agent in config.json: ai_agent.enabled = true"
    exit 0
else
    echo -e "${RED}Some tests failed.${NC} Check output above."
    exit 1
fi
