#!/bin/bash
set -euo pipefail

# scripts/adversarial-review.sh
# Reviews a diff with a model and prints findings as markdown. Advisory: it never
# gates a merge — a model's opinion is not a computational control, and the
# factory does not pretend otherwise. The gates block; this one advises.
#
# It calls the provider's HTTP API directly rather than driving a harness CLI, so
# CI needs only curl and jq — no agent runtime, no node, no toolchain.
#
# Usage:
#   ./scripts/adversarial-review.sh <diff-file>          # writes markdown to stdout
#   REVIEW_MODEL=... REVIEW_API_KEY=... ./scripts/adversarial-review.sh diff.patch
#
# Reads from factory.config (or the environment, which wins):
#   MODEL_PROVIDER   openrouter | anthropic | openai
#   REVIEW_MODEL     model id; falls back to the frontier tier for the provider
#   REVIEW_API_KEY   the key itself, supplied by CI from a repository secret
#
# Exit 0 = a review was produced (findings or not). Exit 1 = it could not run.
# A failure here must never look like an approval, so the caller prints the
# reason rather than silently posting nothing.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIFF_FILE="${1:-}"

if [ -z "$DIFF_FILE" ] || [ ! -f "$DIFF_FILE" ]; then
  echo "adversarial-review: usage: $0 <diff-file>" >&2
  exit 1
fi
for tool in curl jq; do
  command -v "$tool" >/dev/null 2>&1 || { echo "adversarial-review: $tool is required" >&2; exit 1; }
done

# shellcheck source=/dev/null
[ -f "$ROOT_DIR/factory.config" ] && . "$ROOT_DIR/factory.config"

PROVIDER="${MODEL_PROVIDER:-openrouter}"
API_KEY="${REVIEW_API_KEY:-}"
if [ -z "$API_KEY" ]; then
  echo "adversarial-review: no REVIEW_API_KEY in the environment." >&2
  echo "  CI supplies it from the repository secret named in factory.config." >&2
  exit 1
fi

# Model: explicit REVIEW_MODEL wins, else this provider's frontier tier — the
# reviewer is the one role never routed to a cheap model (docs/COST_AND_TOKENS.md).
MODEL="${REVIEW_MODEL:-}"
if [ -z "$MODEL" ]; then
  case "$PROVIDER" in
    anthropic) MODEL="${CLAUDE_FRONTIER_MODEL:-claude-opus-4-8}" ;;
    openai)    MODEL="${CODEX_FRONTIER_MODEL:-gpt-5.6-sol}" ;;
    *)         MODEL="${OPENCODE_FRONTIER_MODEL:-openrouter/z-ai/glm-5.2}" ;;
  esac
  # OpenRouter model ids are the slug without the harness's provider prefix.
  MODEL="${MODEL#openrouter/}"
fi

DIFF="$(cat "$DIFF_FILE")"
if [ -z "${DIFF//[[:space:]]/}" ]; then
  echo "_No reviewable change in this diff._"
  exit 0
fi

# The prompt. It asks the model to REFUTE rather than assess, because a reviewer
# that summarises is a reviewer that approves. "No findings" is an allowed and
# expected answer — inventing a finding to look useful is the failure mode.
read -r -d '' SYSTEM_PROMPT <<'PROMPT' || true
You are an adversarial code reviewer. Your job is to find what is wrong with a
change before a human spends attention on it. You are not the model that wrote
this code, and you are not here to be agreeable.

Review the diff for, in priority order:
1. Correctness — logic errors, off-by-one, wrong operator, unhandled nil/empty,
   a case the change silently breaks.
2. Security — injection, unsafe input handling, secrets in code, permissions
   widened, a path that trusts what it should not.
3. Invariants — behaviour the change breaks that callers depend on:
   idempotency, immutability, ordering, error contracts.
4. Confabulation — a comment, citation, or doc reference asserting something the
   diff does not support, or pointing at a file or line that may not exist.
5. Over-engineering — abstraction with one caller, generalisation for a future
   that has not arrived, code that a stdlib call would replace.

Rules you must follow:
- Report ONLY what you can point at in this diff. Cite `file:line`.
- Do not summarise what the change does. The author knows. Findings only.
- Do not praise. "Looks good" is not a finding and wastes the reader.
- If a concern is speculative, label it **speculative** and say what would
  confirm it. Do not present a guess as a defect.
- If you find nothing worth a human's time, say exactly: "No findings." That is
  a valid, useful answer. Inventing a finding to appear thorough is the failure
  mode this review exists to avoid.
- You see only a diff, not the whole repository. Say so when it limits you
  rather than assuming the worst.

Format each finding as:
### <severity: critical | major | minor> — <file>:<line>
<what is wrong, and what would go wrong because of it>
**Fix:** <the smallest change that resolves it>
PROMPT

USER_PROMPT="Review this diff.

\`\`\`diff
$DIFF
\`\`\`"

# Two request shapes cover the three providers: Anthropic has its own Messages
# API; OpenRouter and OpenAI are both OpenAI-compatible chat completions.
case "$PROVIDER" in
  anthropic)
    ENDPOINT="https://api.anthropic.com/v1/messages"
    BODY="$(jq -n --arg m "$MODEL" --arg s "$SYSTEM_PROMPT" --arg u "$USER_PROMPT" \
      '{model:$m, max_tokens:4096, system:$s, messages:[{role:"user",content:$u}]}')"
    RESPONSE="$(curl -sS --max-time 180 "$ENDPOINT" \
      -H "x-api-key: $API_KEY" -H "anthropic-version: 2023-06-01" \
      -H "content-type: application/json" -d "$BODY")" || RESPONSE=""
    TEXT="$(printf '%s' "$RESPONSE" | jq -r '.content[0].text // empty' 2>/dev/null || true)"
    ;;
  *)
    if [ "$PROVIDER" = "openai" ]; then
      ENDPOINT="https://api.openai.com/v1/chat/completions"
    else
      ENDPOINT="https://openrouter.ai/api/v1/chat/completions"
    fi
    BODY="$(jq -n --arg m "$MODEL" --arg s "$SYSTEM_PROMPT" --arg u "$USER_PROMPT" \
      '{model:$m, messages:[{role:"system",content:$s},{role:"user",content:$u}]}')"
    RESPONSE="$(curl -sS --max-time 180 "$ENDPOINT" \
      -H "Authorization: Bearer $API_KEY" -H "content-type: application/json" \
      -d "$BODY")" || RESPONSE=""
    TEXT="$(printf '%s' "$RESPONSE" | jq -r '.choices[0].message.content // empty' 2>/dev/null || true)"
    ;;
esac

if [ -z "$TEXT" ]; then
  ERR="$(printf '%s' "$RESPONSE" | jq -r '.error.message // empty' 2>/dev/null || true)"
  echo "adversarial-review: the model returned no review${ERR:+ ($ERR)}" >&2
  exit 1
fi

printf '%s\n' "$TEXT"
