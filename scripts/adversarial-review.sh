#!/bin/bash
set -euo pipefail

# scripts/adversarial-review.sh
# Reviews a diff with a model and prints findings as markdown. Advisory: it never
# gates a merge — a model's opinion is not a computational control, and the
# factory does not pretend otherwise. The gates block; this one advises.
#
# It calls provider HTTP APIs directly without a harness CLI. The legacy default
# uses curl and jq; OpenRouter can explicitly select a prebuilt Go client. This
# repository's trusted-base CI builds that client before supplying its API key.
#
# Usage:
#   ./scripts/adversarial-review.sh <diff-file>          # writes markdown to stdout
#   REVIEW_MODEL=... REVIEW_API_KEY=... ./scripts/adversarial-review.sh diff.patch
#
# Reads from factory.yaml (or the environment, which wins):
#   MODEL_PROVIDER   openrouter | anthropic | openai
#   REVIEW_MODEL     model id; falls back to the frontier tier for the provider
#   REVIEW_API_KEY   the key itself, supplied by CI from a repository secret
#   REVIEW_REASONING_EFFORT  optional OpenRouter effort; empty keeps provider defaults
#   REVIEW_OPENROUTER_PROVIDER  optional hosting slug; empty keeps gateway routing
#   REVIEW_MAX_TOKENS  optional OpenRouter completion cap; defaults to 8192
# Environment only:
#   REVIEW_TIMEOUT_SECONDS  total HTTP deadline, 1..1200; defaults to 1200
#   REVIEW_GO_CLIENT  optional absolute executable path for OpenRouter only
#   REVIEW_HTTP_RETRIES  Go client retry allowance, 0 or 1; defaults to 0
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

# Settings are parsed from factory.yaml, never sourced (Decision 41). That
# matters most here: this script runs in CI against pull requests, and reading a
# repository file must never mean executing it. Older repos keep theirs in
# factory.config and are still read.
# shellcheck source=lib/config.sh
. "$ROOT_DIR/scripts/lib/config.sh"
factory_config_export

# Normalize before arithmetic, keeping the transport deadline inside the CI job.
# docs/DECISION_LOG.md:2246.
TIMEOUT="${REVIEW_TIMEOUT_SECONDS:-1200}"
if ! [[ "$TIMEOUT" =~ ^[0-9]+$ ]]; then
  echo "adversarial-review: invalid REVIEW_TIMEOUT_SECONDS; use a decimal value from 1 through 1200." >&2
  exit 1
fi
TIMEOUT="${TIMEOUT#"${TIMEOUT%%[!0]*}"}"
[ -n "$TIMEOUT" ] || TIMEOUT=0
if [ "${#TIMEOUT}" -gt 4 ] || [ "$TIMEOUT" -lt 1 ] || [ "$TIMEOUT" -gt 1200 ]; then
  echo "adversarial-review: invalid REVIEW_TIMEOUT_SECONDS; use a decimal value from 1 through 1200." >&2
  exit 1
fi

PROVIDER="${MODEL_PROVIDER:-openrouter}"
API_KEY="${REVIEW_API_KEY:-}"
if [ -z "$API_KEY" ]; then
  echo "adversarial-review: no REVIEW_API_KEY in the environment." >&2
  echo "  CI supplies it from the repository secret named in factory.yaml." >&2
  exit 1
fi

# Model: explicit REVIEW_MODEL wins, else this provider's frontier tier.
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

# Keep partial bodies private and classify transfer failure before extraction.
# docs/adr/0065-adversarial-review-transport-deadline.md:36.
RESPONSE_FILE="$(umask 077; mktemp "${TMPDIR:-/tmp}/factory-review.XXXXXX")"
trap 'rm -f -- "$RESPONSE_FILE"' EXIT

request_review() {
  local curl_status=0 transport http_status total_seconds first_byte_seconds received_bytes _extra
  transport="$(curl -sS --connect-timeout 15 --max-time "$TIMEOUT" \
    --output "$RESPONSE_FILE" \
    --write-out '%{http_code}\t%{time_total}\t%{time_starttransfer}\t%{size_download}' \
    "$@" 2>/dev/null)" || curl_status=$?
  IFS=$'\t' read -r http_status total_seconds first_byte_seconds received_bytes _extra <<< "$transport" || true
  # Metadata is never interpolated into diagnostics unless it is numeric.
  [[ "$http_status" =~ ^[0-9]{3}$ ]] || http_status=unknown
  [[ "$total_seconds" =~ ^[0-9]+([.][0-9]+)?$ ]] || total_seconds=unknown
  [[ "$first_byte_seconds" =~ ^[0-9]+([.][0-9]+)?$ ]] || first_byte_seconds=unknown
  [[ "$received_bytes" =~ ^[0-9]+([.][0-9]+)?$ ]] || received_bytes=unknown
  if [ "$curl_status" -eq 28 ]; then
    echo "adversarial-review: request timed out." >&2
  elif [ "$curl_status" -ne 0 ]; then
    echo "adversarial-review: transport failure." >&2
  elif ! [[ "$http_status" =~ ^2[0-9]{2}$ ]]; then
    echo "adversarial-review: HTTP request failed." >&2
  else
    RESPONSE="$(cat "$RESPONSE_FILE")"
    return 0
  fi
  printf 'adversarial-review: curl_status=%s http_status=%s total_seconds=%s first_byte_seconds=%s received_bytes=%s\n' \
    "$curl_status" "$http_status" "$total_seconds" "$first_byte_seconds" "$received_bytes" >&2
  return 1
}

# Two request shapes cover the three providers: Anthropic has its own Messages
# API; OpenRouter and OpenAI are both OpenAI-compatible chat completions.
case "$PROVIDER" in
  anthropic)
    ENDPOINT="https://api.anthropic.com/v1/messages"
    BODY="$(jq -n --arg m "$MODEL" --arg s "$SYSTEM_PROMPT" --arg u "$USER_PROMPT" \
      '{model:$m, max_tokens:4096, system:$s, messages:[{role:"user",content:$u}]}')"
    request_review "$ENDPOINT" \
      -H "x-api-key: $API_KEY" -H "anthropic-version: 2023-06-01" \
      -H "content-type: application/json" -d "$BODY" || exit 1
    TEXT="$(printf '%s' "$RESPONSE" | jq -r '.content[0].text // empty' 2>/dev/null || true)"
    ;;
  *)
    if [ "$PROVIDER" = "openai" ]; then
      ENDPOINT="https://api.openai.com/v1/chat/completions"
    else
      ENDPOINT="https://openrouter.ai/api/v1/chat/completions"
    fi
    # Reasoning shares the output allowance; keep adopter defaults unless set.
    # docs/adr/0054-explicit-review-reasoning-effort.md:17.
    EFFORT="${REVIEW_REASONING_EFFORT:-}"
    ROUTE="${REVIEW_OPENROUTER_PROVIDER:-}"
    if [ "$PROVIDER" != "openai" ]; then
      case "$EFFORT" in
        ''|none|minimal|low|medium|high|xhigh|max) ;;
        *)
          echo "adversarial-review: invalid REVIEW_REASONING_EFFORT; use none, minimal, low, medium, high, xhigh, max, or empty." >&2
          exit 1 ;;
      esac
      # Validate a single hosting slug before any request; never evaluate it.
      # docs/adr/0055-pin-deepseek-review-provider.md:27.
      if [ -n "$ROUTE" ] && ! [[ "$ROUTE" =~ ^[a-z0-9]+([-/][a-z0-9]+)*$ ]]; then
        echo "adversarial-review: invalid REVIEW_OPENROUTER_PROVIDER; use a lowercase provider or endpoint slug, or empty." >&2
        exit 1
      fi
      MAX_TOKENS="${REVIEW_MAX_TOKENS:-8192}"
      if ! [[ "$MAX_TOKENS" =~ ^[0-9]+$ ]]; then
        echo "adversarial-review: invalid REVIEW_MAX_TOKENS; use a decimal value from 1024 through 32768." >&2
        exit 1
      fi
      # Normalize leading zeroes before bounded arithmetic, so an arbitrarily
      # long decimal cannot overflow Bash's integer evaluator.
      MAX_TOKENS="${MAX_TOKENS#"${MAX_TOKENS%%[!0]*}"}"
      [ -n "$MAX_TOKENS" ] || MAX_TOKENS=0
      MAX_TOKENS_DIGITS=${#MAX_TOKENS}
      if [ "$MAX_TOKENS_DIGITS" -gt 5 ] ||
        { [ "$MAX_TOKENS_DIGITS" -eq 5 ] && [ "$MAX_TOKENS" -gt 32768 ]; } ||
        { [ "$MAX_TOKENS_DIGITS" -lt 4 ] ||
          { [ "$MAX_TOKENS_DIGITS" -eq 4 ] && [ "$MAX_TOKENS" -lt 1024 ]; }; }; then
        echo "adversarial-review: invalid REVIEW_MAX_TOKENS; use a decimal value from 1024 through 32768." >&2
        exit 1
      fi
    fi
    # Bound OpenRouter output while preserving the OpenAI request contract.
    # docs/adr/0057-configurable-review-output-cap.md:8.
    # A selected host is pinned without fallback and must support the parameters.
    # docs/adr/0055-pin-deepseek-review-provider.md:20.
    BODY="$(jq -n --arg m "$MODEL" --arg s "$SYSTEM_PROMPT" --arg u "$USER_PROMPT" --arg p "$PROVIDER" --arg effort "$EFFORT" --arg route "$ROUTE" --argjson max_tokens "${MAX_TOKENS:-8192}" \
      '{model:$m, messages:[{role:"system",content:$s},{role:"user",content:$u}]} +
       (if $p == "openai" then {} else
         {max_tokens:$max_tokens} + (if $effort == "" then {} else {reasoning:{effort:$effort}} end) +
         (if $route == "" then {} else {provider:{order:[$route],allow_fallbacks:false,require_parameters:true}} end)
       end)')"
    # Explicit selection never falls back after a client failure.
    # docs/adr/0067-streaming-adversarial-review-client.md:32.
    if [ "$PROVIDER" != openai ] && [ -n "${REVIEW_GO_CLIENT:-}" ]; then
      case "$REVIEW_GO_CLIENT" in
        /*) ;;
        *) echo "adversarial-review: REVIEW_GO_CLIENT must name an absolute executable file." >&2; exit 1 ;;
      esac
      if [ ! -f "$REVIEW_GO_CLIENT" ] || [ ! -x "$REVIEW_GO_CLIENT" ]; then
        echo "adversarial-review: REVIEW_GO_CLIENT must name an absolute executable file." >&2
        exit 1
      fi
      printf '%s' "$BODY" | /usr/bin/env FACTORY_BRIDGE_PROTOCOL=1 \
        "REVIEW_API_KEY=$API_KEY" "REVIEW_TIMEOUT_SECONDS=$TIMEOUT" \
        "REVIEW_HTTP_RETRIES=${REVIEW_HTTP_RETRIES:-0}" \
        "$REVIEW_GO_CLIENT" review openrouter
      exit "$?"
    fi
    request_review "$ENDPOINT" \
      -H "Authorization: Bearer $API_KEY" -H "content-type: application/json" \
      -d "$BODY" || exit 1
    if [ "$PROVIDER" != "openai" ] && \
        printf '%s' "$RESPONSE" | jq -e '.choices[0].finish_reason == "length"' >/dev/null 2>&1; then
      echo "adversarial-review: incomplete review: OpenRouter reached the response token limit (finish_reason=length)." >&2
      exit 1
    fi
    TEXT="$(printf '%s' "$RESPONSE" | jq -r '.choices[0].message.content // empty' 2>/dev/null || true)"
    ;;
esac

if [ -z "$TEXT" ]; then
  ERR="$(printf '%s' "$RESPONSE" | jq -r '.error.message // empty' 2>/dev/null || true)"
  echo "adversarial-review: the model returned no review${ERR:+ ($ERR)}" >&2
  exit 1
fi

printf '%s\n' "$TEXT"
