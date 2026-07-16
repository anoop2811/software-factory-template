#!/bin/bash
set -euo pipefail

# scripts/sync-claude.sh
# Generates .claude/settings.json, .mcp.json, and .claude/agents/*.md from
# the canonical opencode.json. Never hand-edit the Claude config — run this script.
#
# This script reads opencode.json and translates:
#   - permission block → Claude permissions allow/deny arrays
#   - mcp block → .mcp.json mcpServers
#   - agent definitions → .claude/agents/<name>.md
#   - hook registrations → Claude PreToolUse/PostToolUse hooks
#
# Prerequisites: jq must be installed.

if ! command -v jq &> /dev/null; then
  echo "ERROR: jq is required. Install with 'brew install jq'." >&2
  exit 1
fi

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
OPENCODE_JSON="$ROOT_DIR/opencode.json"

if [ ! -f "$OPENCODE_JSON" ]; then
  echo "ERROR: opencode.json not found at $OPENCODE_JSON" >&2
  exit 1
fi

mkdir -p "$ROOT_DIR/.claude/agents"
mkdir -p "$ROOT_DIR/.claude/hooks"

# --- Generate .claude/settings.json ---
SETTINGS=$(jq '{
  "$schema": "https://json.schemastore.org/claude-code-settings.json",
  permissions: {
    allow: [],
    deny: []
  },
  hooks: {
    PreToolUse: [
      {
        matcher: "Edit|Write",
        hooks: [
          {
            type: "command",
            command: "${CLAUDE_PROJECT_DIR}/scripts/hooks/test-edit-denial.sh",
            args: []
          }
        ]
      }
    ]
  }
}' "$OPENCODE_JSON")

# Translate opencode permission bash rules → Claude permission strings
ALLOW_RULES=$(jq -r '.permission.bash // {} | to_entries[] | select(.value == "allow") | "Bash(\(.key))"' "$OPENCODE_JSON")
DENY_RULES=$(jq -r '.permission.bash // {} | to_entries[] | select(.value == "deny") | "Bash(\(.key))"' "$OPENCODE_JSON")

EDIT_PERM=$(jq -r '.permission.edit // "ask"' "$OPENCODE_JSON")
if [ "$EDIT_PERM" = "deny" ]; then
  DENY_RULES="$DENY_RULES
Edit"
fi

SETTINGS=$(jq --arg allow "$ALLOW_RULES" --arg deny "$DENY_RULES" '
  .permissions.allow = ($allow | split("\n") | map(select(length > 0))) |
  .permissions.deny = ($deny | split("\n") | map(select(length > 0)))
' <<< "$SETTINGS")

echo "$SETTINGS" | jq '.' > "$ROOT_DIR/.claude/settings.json"
echo "sync-claude: wrote .claude/settings.json"

# --- Generate .mcp.json ---
MCP_JSON=$(jq '{
  mcpServers: (.mcp // {} | to_entries | map(select(.value.enabled != false) | {
    key: .key,
    value: {
      command: (.value.command | if type == "array" then .[0] else . end),
      args: (.value.command | if type == "array" then .[1:] else [] end),
      env: (.value.env // {})
    }
  }) | from_entries)
}' "$OPENCODE_JSON")

echo "$MCP_JSON" | jq '.' > "$ROOT_DIR/.mcp.json"
echo "sync-claude: wrote .mcp.json"

# --- Generate .claude/agents/ from opencode agent definitions ---
AGENTS=$(jq -r '.agent // {} | keys[]' "$OPENCODE_JSON")

for AGENT_NAME in $AGENTS; do
  DESCRIPTION=$(jq -r --arg name "$AGENT_NAME" '.agent[$name].description // ""' "$OPENCODE_JSON")
  MODEL=$(jq -r --arg name "$AGENT_NAME" '.agent[$name].model // "inherit"' "$OPENCODE_JSON")
  EDIT_PERM=$(jq -r --arg name "$AGENT_NAME" '.agent[$name].permission.edit // "ask"' "$OPENCODE_JSON")
  ROLE_FILE="$ROOT_DIR/.opencode/agent/${AGENT_NAME}.md"
  if [ ! -f "$ROLE_FILE" ]; then
    echo "ERROR: canonical role file not found: $ROLE_FILE" >&2
    exit 1
  fi
  BODY=$(awk '
    /^---$/ { frontmatter_delimiters++; next }
    frontmatter_delimiters >= 2 { print }
  ' "$ROLE_FILE")

  # Translate model: strip openrouter/anthropic/ prefix; convert dots to dashes
  # (OpenRouter slugs use dots: claude-sonnet-4.6; Anthropic API IDs use dashes: claude-sonnet-4-6)
  # Non-Anthropic models use "inherit" (Claude can't route them natively)
  CLAUDE_MODEL="inherit"
  if echo "$MODEL" | grep -q "anthropic/"; then
    CLAUDE_MODEL=$(echo "$MODEL" | sed 's|openrouter/anthropic/||' | tr '.' '-')
  fi

  # Translate permission → permissionMode
  PERMISSION_MODE="default"
  if [ "$EDIT_PERM" = "deny" ]; then
    PERMISSION_MODE="plan"
  fi

  cat > "$ROOT_DIR/.claude/agents/${AGENT_NAME}.md" <<EOF
---
name: ${AGENT_NAME}
description: ${DESCRIPTION}
model: ${CLAUDE_MODEL}
permissionMode: ${PERMISSION_MODE}
EOF

  if [ "$AGENT_NAME" = "implementer" ]; then
    cat >> "$ROOT_DIR/.claude/agents/${AGENT_NAME}.md" <<'EOF'
hooks:
  PreToolUse:
    - matcher: "Edit|Write"
      hooks:
        - type: command
          command: "FACTORY_AGENT_ROLE=implementer ${CLAUDE_PROJECT_DIR}/scripts/hooks/test-edit-denial.sh"
EOF
  fi

  cat >> "$ROOT_DIR/.claude/agents/${AGENT_NAME}.md" <<EOF
---

${BODY}
EOF

  echo "sync-claude: wrote .claude/agents/${AGENT_NAME}.md"
done

# --- Symlink shared format directories ---
if [ ! -L "$ROOT_DIR/CLAUDE.md" ]; then
  rm -f "$ROOT_DIR/CLAUDE.md"
  ln -s AGENTS.md "$ROOT_DIR/CLAUDE.md"
  echo "sync-claude: symlinked CLAUDE.md → AGENTS.md"
fi

echo "sync-claude: done"
