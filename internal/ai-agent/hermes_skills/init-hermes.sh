#!/bin/bash
# init-hermes.sh — Initialize Hermes skills directory for GoStream AI Maintenance
# Run this once to set up the skills structure.
# Skills in this directory are then copied to ~/.hermes/skills/ for Hermes to use.

set -e

SKILLS_DIR="$HOME/.hermes/skills"
CONFIG_DIR="$HOME/.hermes/config"
STATE_DIR="$HOME/.hermes"

echo "=== GoStream AI Maintenance — Hermes Setup ==="

# Create directories
mkdir -p "$SKILLS_DIR"
mkdir -p "$CONFIG_DIR"

# Create empty action history
if [ ! -f "$STATE_DIR/gostream-action-history.json" ]; then
    echo '{"actions": []}' > "$STATE_DIR/gostream-action-history.json"
    echo "✓ Created action history: $STATE_DIR/gostream-action-history.json"
else
    echo "✓ Action history already exists"
fi

# Create empty queue
if [ ! -f "$STATE_DIR/gostream-queue.json" ]; then
    echo '{"entries": []}' > "$STATE_DIR/gostream-queue.json"
    echo "✓ Created queue: $STATE_DIR/gostream-queue.json"
else
    echo "✓ Queue already exists"
fi

# Create config template
if [ ! -f "$CONFIG_DIR/gostream.json" ]; then
    cat > "$CONFIG_DIR/gostream.json" << 'EOF'
{
  "jellyfin": {
    "url": "http://localhost:8096",
    "api_key": "YOUR_JELLYFIN_API_KEY"
  },
  "prowlarr": {
    "url": "http://localhost:9696",
    "api_key": "YOUR_PROWLARR_API_KEY"
  },
  "tmdb": {
    "api_key": "YOUR_TMDB_API_KEY"
  }
}
EOF
    echo "✓ Created config template: $CONFIG_DIR/gostream.json (edit with your API keys)"
else
    echo "✓ Config already exists"
fi

# Copy skills from staging area
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
for skill in "$SCRIPT_DIR"/*.md; do
    name=$(basename "$skill")
    cp "$skill" "$SKILLS_DIR/$name"
    echo "✓ Installed skill: $name"
done

echo ""
echo "=== Setup Complete ==="
echo "Skills installed to: $SKILLS_DIR"
echo "Config at: $CONFIG_DIR/gostream.json (edit with your API keys)"
echo ""
echo "Next steps:"
echo "1. Edit $CONFIG_DIR/gostream.json with your actual API keys"
echo "2. Configure Hermes webhook: hermes webhook subscribe --url http://localhost:9080/webhook/gostream"
echo "3. Configure Hermes cron: hermes cron add gostream-deep-scan '*/30 * * * *' 'activate gostream-deep-scan'"
echo "4. Enable AI agent in GoStream config.json: ai_agent.enabled = true"
