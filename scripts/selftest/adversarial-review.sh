#!/bin/bash
set -euo pipefail

# Real provider-script boundary, fake HTTP only. docs/adr/0052-self-hosted-adversarial-review.md:30.
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
command -v jq >/dev/null 2>&1 || { echo "adversarial-review selftest: jq is required" >&2; exit 1; }
FIXTURE="$(mktemp -d "${TMPDIR:-/tmp}/factory-adversarial-review.XXXXXX")"
trap 'rm -rf "$FIXTURE"' EXIT
mkdir -p "$FIXTURE/template/scripts/lib" "$FIXTURE/bin"
cp "$ROOT/scripts/adversarial-review.sh" "$FIXTURE/template/scripts/"
cp "$ROOT/scripts/lib/config.sh" "$FIXTURE/template/scripts/lib/"

cat > "$FIXTURE/bin/curl" <<'CURL'
#!/bin/bash
set -euo pipefail
printf 'x' >> "$FIXTURE_CALLS"
printf '%s\n' "$@" > "$FIXTURE_ARGS"
output=""; format=""; limit=0
while [ "$#" -gt 0 ]; do
  case "$1" in
    -d|--data-binary) printf '%s' "$2" > "$FIXTURE_BODY"; shift 2 ;;
    -o|--output) output="$2"; shift 2 ;;
    -w|--write-out) format="$2"; shift 2 ;;
    --max-time) limit="$2"; shift 2 ;;
    *) shift ;;
  esac
done
status="${FIXTURE_CURL_STATUS:-0}"
if [ "${FIXTURE_REQUIRED_SECONDS:-0}" -gt "$limit" ]; then status=28; fi
if [ -n "$output" ]; then
  cat "$FIXTURE_RESPONSE" > "$output"
  printf '%s\n' "$output" > "$FIXTURE_OUTPUT_RECORD"
  ls -ld "$output" | cut -c 2-10 > "$FIXTURE_MODE_RECORD"
else
  cat "$FIXTURE_RESPONSE"
fi
if [ -n "$format" ]; then
  format="${format//'%{http_code}'/${FIXTURE_HTTP_STATUS:-200}}"
  format="${format//'%{response_code}'/${FIXTURE_HTTP_STATUS:-200}}"
  format="${format//'%{time_total}'/${FIXTURE_TIME_TOTAL:-240.000}}"
  format="${format//'%{time_starttransfer}'/0.250}"
  format="${format//'%{size_download}'/660}"
  printf '%b' "$format"
fi
if [ "$status" -ne 0 ]; then printf '%s\n' 'private-curl-error fixture-not-a-secret' >&2; fi
exit "$status"
CURL
chmod +x "$FIXTURE/bin/curl"

PASSED=0
FAILED=0
STATUS=0

reset_fixture() {
  STATUS="not run"
  rm -f "$FIXTURE/factory.config"
  cat > "$FIXTURE/factory.yaml" <<'CONFIG'
model_provider: openrouter
review_model: z-ai/glm-5.3-flash
CONFIG
  printf '%s\n' '+a real diff' > "$FIXTURE/diff.patch"
  printf '%s' '{"choices":[{"finish_reason":"stop","message":{"content":"### major — example.go:1\nA concrete finding."}}]}' > "$FIXTURE/response.json"
  : > "$FIXTURE/calls"
  : > "$FIXTURE/args"
  : > "$FIXTURE/body.json"
  : > "$FIXTURE/output-path"
  : > "$FIXTURE/output-mode"
  mkdir -p "$FIXTURE/http-temp"
  chmod 700 "$FIXTURE/http-temp"
  printf '%s' 'preserve sibling' > "$FIXTURE/http-temp/sentinel"
}

run_review() {
  STATUS=0
  env -i PATH="$FIXTURE/bin:$PATH" \
    FACTORY_CONFIG="$FIXTURE/factory.yaml" REVIEW_API_KEY=fixture-not-a-secret \
    FIXTURE_CALLS="$FIXTURE/calls" FIXTURE_ARGS="$FIXTURE/args" \
    FIXTURE_BODY="$FIXTURE/body.json" FIXTURE_RESPONSE="$FIXTURE/response.json" \
    FIXTURE_OUTPUT_RECORD="$FIXTURE/output-path" FIXTURE_MODE_RECORD="$FIXTURE/output-mode" \
    TMPDIR="$FIXTURE/http-temp" \
    "$@" bash "$FIXTURE/template/scripts/adversarial-review.sh" "$FIXTURE/diff.patch" \
    > "$FIXTURE/stdout" 2> "$FIXTURE/stderr" || STATUS=$?
}

one_request() { [ "$(wc -c < "$FIXTURE/calls")" -eq 1 ]; }
no_requests() { [ ! -s "$FIXTURE/calls" ]; }
failed_without_findings() { [ "$STATUS" -ne 0 ] && [ ! -s "$FIXTURE/stdout" ]; }

configured_success() {
  run_review
  [ "$STATUS" -eq 0 ] && [ ! -s "$FIXTURE/stderr" ] || return 1
  one_request || return 1
  grep -qFx 'https://openrouter.ai/api/v1/chat/completions' "$FIXTURE/args" || return 1
  jq -e '.model == "z-ai/glm-5.3-flash" and .messages[0].role == "system" and .messages[1].role == "user"' "$FIXTURE/body.json" >/dev/null || return 1
  printf '### major — example.go:1\nA concrete finding.\n' > "$FIXTURE/expected"
  cmp -s "$FIXTURE/expected" "$FIXTURE/stdout"
}

repository_default_provider() {
  local lane_settings
  cp "$ROOT/factory.yaml" "$FIXTURE/factory.yaml"
  ! grep -q '^model_provider:' "$FIXTURE/factory.yaml" || return 1
  lane_settings="$(env -i PATH="$PATH" FACTORY_CONFIG="$FIXTURE/factory.yaml" \
    bash -c '. "$1"; factory_config_get review_lane; printf "\n"; factory_config_get review_api_key_secret; printf "\n"; factory_config_get review_reasoning_effort; printf "\n"; factory_config_get review_openrouter_provider; printf "\n"; factory_config_get review_max_tokens' \
    config-reader "$ROOT/scripts/lib/config.sh")" || return 1
  [ "$lane_settings" = $'on\nOPENROUTER_API_KEY\nnone\ndeepinfra\n8192' ] || return 1
  run_review
  [ "$STATUS" -eq 0 ] && one_request || return 1
  grep -qFx 'https://openrouter.ai/api/v1/chat/completions' "$FIXTURE/args" || return 1
  jq -e '.model == "z-ai/glm-5.3-flash" and .max_tokens == 8192 and .reasoning == {effort:"none"} and .provider == {order:["deepinfra"],allow_fallbacks:false,require_parameters:true} and .messages[1].role == "user"' "$FIXTURE/body.json" >/dev/null
}

# These source constraints do not prove GitHub runtime execution.
trusted_base_guard() {
  local guard workflow checkout
  for workflow in "$ROOT/packs/review-lane/review-pr.yml" "$ROOT/.github/workflows/adversarial-review.yml"; do
    guard="$(sed -n '/^  review:$/,/^    runs-on:/s/^    if: //p' "$workflow")"
    if [ "$guard" != 'github.event.pull_request.head.repo.full_name == github.repository && github.event.pull_request.base.ref == github.event.repository.default_branch' ]; then
      printf 'unexpected privileged-job guard: %s\n' "$workflow" >&2
      return 1
    fi
    # Read this scaffold's explicit step layout; a changed layout must receive
    # review rather than making this bounded structural check pass vacuously.
    checkout="$(awk '
      function finish() {
        if (owns_checkout) saved = block
        block = ""; owns_checkout = 0
      }
      /^      - / { finish(); in_step = 1 }
      in_step && NF && $0 !~ /^      / { finish(); in_step = 0 }
      /^[[:space:]]*(-[[:space:]]*)?uses:[[:space:]]*actions\/checkout@/ {
        count++
        if (in_step) owns_checkout = 1
      }
      in_step { block = block $0 "\n" }
      END {
        finish()
        if (count != 1 || saved == "") exit 1
        printf "%s", saved
      }
    ' "$workflow")" || {
      printf 'expected exactly one explicit checkout step: %s\n' "$workflow" >&2
      return 1
    }
    if ! printf '%s\n' "$checkout" | grep -qE '^      (  uses:|- uses:) actions/checkout@[[:xdigit:]]{40}( #.*)?$' ||
       [ "$(printf '%s\n' "$checkout" | grep '^          ref:')" != '          ref: ${{ github.event.pull_request.base.sha }}' ] ||
       [ "$(printf '%s\n' "$checkout" | grep '^          persist-credentials:')" != '          persist-credentials: false' ] ||
       grep -qE 'github\.event\.pull_request\.head\.(sha|ref)|github\.head_ref' "$workflow"; then
      printf 'unexpected trusted checkout configuration: %s\n' "$workflow" >&2
      return 1
    fi
  done
}

openrouter_cap() {
  run_review
  [ "$STATUS" -eq 0 ] && one_request || return 1
  jq -e '.max_tokens == 8192' "$FIXTURE/body.json" >/dev/null
}

configured_max_tokens() {
  printf 'review_max_tokens: 12288\n' >> "$FIXTURE/factory.yaml"
  run_review
  [ "$STATUS" -eq 0 ] && one_request || return 1
  jq -e '.max_tokens == 12288' "$FIXTURE/body.json" >/dev/null
}

max_tokens_environment_override() {
  printf 'review_max_tokens: 12288\n' >> "$FIXTURE/factory.yaml"
  run_review REVIEW_MAX_TOKENS=16384
  [ "$STATUS" -eq 0 ] && one_request || return 1
  jq -e '.max_tokens == 16384' "$FIXTURE/body.json" >/dev/null
}

empty_max_tokens_uses_default() {
  printf 'review_max_tokens: 12288\n' >> "$FIXTURE/factory.yaml"
  run_review REVIEW_MAX_TOKENS=
  [ "$STATUS" -eq 0 ] && one_request || return 1
  jq -e '.max_tokens == 8192' "$FIXTURE/body.json" >/dev/null
}

invalid_max_tokens() {
  local value
  for value in 0 1023 32769 12.5 bad 999999999999999999999; do
    reset_fixture
    run_review "REVIEW_MAX_TOKENS=$value"
    failed_without_findings && no_requests || return 1
    grep -qi 'max_tokens' "$FIXTURE/stderr" || return 1
  done
}

truncation_refusal() {
  printf '%s' '{"choices":[{"finish_reason":"length","message":{"content":"### major — unfinished finding"}}]}' > "$FIXTURE/response.json"
  run_review
  failed_without_findings && one_request || return 1
  grep -qi 'incomplete' "$FIXTURE/stderr"
}

literal_diff() {
  printf '+literal $(touch "%s") and `touch "%s"` and "quoted" \\slashes\n' "$FIXTURE/dollar-marker" "$FIXTURE/backtick-marker" > "$FIXTURE/diff.patch"
  run_review
  [ "$STATUS" -eq 0 ] && one_request || return 1
  [ ! -e "$FIXTURE/dollar-marker" ] && [ ! -e "$FIXTURE/backtick-marker" ] || return 1
  jq -e --rawfile diff "$FIXTURE/diff.patch" '.messages[1].content | contains($diff | rtrimstr("\n"))' "$FIXTURE/body.json" >/dev/null
}

missing_key() {
  run_review REVIEW_API_KEY=
  failed_without_findings && no_requests || return 1
  grep -q 'no REVIEW_API_KEY' "$FIXTURE/stderr"
}

http_failure() {
  run_review FIXTURE_CURL_STATUS=28
  failed_without_findings && one_request || return 1
  grep -qFx -- '--max-time' "$FIXTURE/args" && grep -qFx '480' "$FIXTURE/args" || return 1
  ! grep -qE '^--retry([=-]|$)' "$FIXTURE/args"
}

provider_error() {
  printf '%s' '{"error":{"message":"fixture provider refusal"}}' > "$FIXTURE/response.json"
  run_review
  failed_without_findings && one_request || return 1
  grep -q 'fixture provider refusal' "$FIXTURE/stderr"
}

empty_diff() {
  printf ' \n\t\n' > "$FIXTURE/diff.patch"
  run_review
  [ "$STATUS" -eq 0 ] && no_requests || return 1
  grep -q 'No reviewable change' "$FIXTURE/stdout"
}

anthropic_contract() {
  printf '%s' '{"content":[{"text":"No findings."}]}' > "$FIXTURE/response.json"
  run_review MODEL_PROVIDER=anthropic REVIEW_MODEL=fixture-anthropic REVIEW_REASONING_EFFORT=unsupported-fixture-value REVIEW_OPENROUTER_PROVIDER=invalid,route
  [ "$STATUS" -eq 0 ] && one_request || return 1
  grep -qFx 'https://api.anthropic.com/v1/messages' "$FIXTURE/args" || return 1
  jq -e '.model == "fixture-anthropic" and .max_tokens == 4096 and (has("reasoning") | not) and (has("provider") | not) and (.system | type == "string") and .messages[0].role == "user"' "$FIXTURE/body.json" >/dev/null || return 1
  grep -qFx 'No findings.' "$FIXTURE/stdout"
}

openai_contract() {
  run_review MODEL_PROVIDER=openai REVIEW_MODEL=fixture-openai REVIEW_REASONING_EFFORT=unsupported-fixture-value REVIEW_OPENROUTER_PROVIDER=invalid,route
  [ "$STATUS" -eq 0 ] && one_request || return 1
  grep -qFx 'https://api.openai.com/v1/chat/completions' "$FIXTURE/args" || return 1
  jq -e '.model == "fixture-openai" and (keys | sort) == ["messages","model"] and (.messages | length) == 2' "$FIXTURE/body.json" >/dev/null
}

openai_response_unchanged() {
  printf '%s' '{"choices":[{"finish_reason":"length","message":{"content":"Legacy OpenAI partial response."}}]}' > "$FIXTURE/response.json"
  run_review MODEL_PROVIDER=openai REVIEW_MODEL=fixture-openai
  [ "$STATUS" -eq 0 ] && [ ! -s "$FIXTURE/stderr" ] && one_request || return 1
  grep -qFx 'Legacy OpenAI partial response.' "$FIXTURE/stdout"
}

environment_override() {
  run_review MODEL_PROVIDER=openrouter REVIEW_MODEL=fixture-explicit-model
  [ "$STATUS" -eq 0 ] && one_request || return 1
  jq -e '.model == "fixture-explicit-model"' "$FIXTURE/body.json" >/dev/null
}

malformed_response() {
  printf '%s' '{broken json' > "$FIXTURE/response.json"
  run_review
  failed_without_findings && one_request || return 1
  grep -q 'returned no review' "$FIXTURE/stderr"
}

missing_diff() {
  rm "$FIXTURE/diff.patch"
  run_review
  failed_without_findings && no_requests || return 1
  grep -q 'usage:' "$FIXTURE/stderr"
}

no_findings_success() {
  printf '%s' '{"choices":[{"finish_reason":"stop","message":{"content":"No findings."}}]}' > "$FIXTURE/response.json"
  run_review
  [ "$STATUS" -eq 0 ] && one_request || return 1
  grep -qFx 'No findings.' "$FIXTURE/stdout"
}

# Request configuration only: gateway vocabulary is not a claim that every
# model supports every effort. docs/adr/0054-explicit-review-reasoning-effort.md:20.
configured_reasoning() {
  printf 'review_reasoning_effort: low\n' >> "$FIXTURE/factory.yaml"
  run_review
  [ "$STATUS" -eq 0 ] && one_request || return 1
  jq -e '.reasoning == {effort:"low"} and .max_tokens == 8192' "$FIXTURE/body.json" >/dev/null
}

unconfigured_reasoning() {
  run_review
  [ "$STATUS" -eq 0 ] && one_request || return 1
  jq -e '(has("reasoning") | not) and .max_tokens == 8192' "$FIXTURE/body.json" >/dev/null
}

reasoning_environment_override() {
  printf 'review_reasoning_effort: low\n' >> "$FIXTURE/factory.yaml"
  run_review REVIEW_REASONING_EFFORT=high
  [ "$STATUS" -eq 0 ] && one_request || return 1
  jq -e '.reasoning == {effort:"high"}' "$FIXTURE/body.json" >/dev/null
}

reasoning_empty_override() {
  printf 'review_reasoning_effort: high\n' >> "$FIXTURE/factory.yaml"
  run_review REVIEW_REASONING_EFFORT=
  [ "$STATUS" -eq 0 ] && one_request || return 1
  jq -e 'has("reasoning") | not' "$FIXTURE/body.json" >/dev/null
}

legacy_reasoning_precedence() {
  printf 'REVIEW_REASONING_EFFORT=high\n' > "$FIXTURE/factory.config"
  run_review
  [ "$STATUS" -eq 0 ] && one_request || return 1
  jq -e '.reasoning == {effort:"high"}' "$FIXTURE/body.json" >/dev/null || return 1
  reset_fixture
  printf 'REVIEW_REASONING_EFFORT=high\n' > "$FIXTURE/factory.config"
  printf 'review_reasoning_effort: low\n' >> "$FIXTURE/factory.yaml"
  run_review
  [ "$STATUS" -eq 0 ] && one_request || return 1
  jq -e '.reasoning == {effort:"low"}' "$FIXTURE/body.json" >/dev/null
}

gateway_reasoning_values() {
  local effort
  for effort in none minimal low medium high xhigh max; do
    reset_fixture
    run_review "REVIEW_REASONING_EFFORT=$effort"
    [ "$STATUS" -eq 0 ] && one_request || return 1
    jq -e --arg effort "$effort" '.reasoning == {effort:$effort} and .max_tokens == 8192' "$FIXTURE/body.json" >/dev/null || return 1
  done
}

invalid_configured_reasoning() {
  printf 'review_reasoning_effort: unsupported-effort\n' >> "$FIXTURE/factory.yaml"
  run_review
  failed_without_findings && no_requests || return 1
  grep -qi 'reasoning' "$FIXTURE/stderr"
}

literal_invalid_reasoning() {
  local literal
  printf -v literal '$(touch "%s")' "$FIXTURE/reasoning-marker"
  run_review "REVIEW_REASONING_EFFORT=$literal"
  [ ! -e "$FIXTURE/reasoning-marker" ] || return 1
  failed_without_findings && no_requests || return 1
  grep -qi 'reasoning' "$FIXTURE/stderr"
}

configured_route() {
  printf 'review_openrouter_provider: deepinfra\n' >> "$FIXTURE/factory.yaml"
  run_review
  [ "$STATUS" -eq 0 ] && one_request || return 1
  jq -e '.provider == {order:["deepinfra"],allow_fallbacks:false,require_parameters:true} and .max_tokens == 8192' "$FIXTURE/body.json" >/dev/null
}

unconfigured_route() {
  run_review
  [ "$STATUS" -eq 0 ] && one_request || return 1
  jq -e 'has("provider") | not' "$FIXTURE/body.json" >/dev/null
}

route_environment_override() {
  printf 'review_openrouter_provider: deepinfra\n' >> "$FIXTURE/factory.yaml"
  run_review REVIEW_OPENROUTER_PROVIDER=deepinfra/fp8
  [ "$STATUS" -eq 0 ] && one_request || return 1
  jq -e '.provider == {order:["deepinfra/fp8"],allow_fallbacks:false,require_parameters:true}' "$FIXTURE/body.json" >/dev/null
}

route_empty_override() {
  printf 'review_openrouter_provider: deepinfra\n' >> "$FIXTURE/factory.yaml"
  run_review REVIEW_OPENROUTER_PROVIDER=
  [ "$STATUS" -eq 0 ] && one_request || return 1
  jq -e 'has("provider") | not' "$FIXTURE/body.json" >/dev/null
}

legacy_route_precedence() {
  printf 'REVIEW_OPENROUTER_PROVIDER=deepinfra/fp8\n' > "$FIXTURE/factory.config"
  run_review
  [ "$STATUS" -eq 0 ] && one_request || return 1
  jq -e '.provider == {order:["deepinfra/fp8"],allow_fallbacks:false,require_parameters:true}' "$FIXTURE/body.json" >/dev/null || return 1
  reset_fixture
  printf 'REVIEW_OPENROUTER_PROVIDER=deepinfra/fp8\n' > "$FIXTURE/factory.config"
  printf 'review_openrouter_provider: deepinfra\n' >> "$FIXTURE/factory.yaml"
  run_review
  [ "$STATUS" -eq 0 ] && one_request || return 1
  jq -e '.provider == {order:["deepinfra"],allow_fallbacks:false,require_parameters:true}' "$FIXTURE/body.json" >/dev/null
}

# Slug grammar is a local input contract, not proof that a provider exists.
accepted_route_slugs() {
  local route
  for route in deepinfra deepinfra/fp8 provider-name/endpoint-2; do
    reset_fixture
    run_review "REVIEW_OPENROUTER_PROVIDER=$route"
    [ "$STATUS" -eq 0 ] && one_request || return 1
    jq -e --arg route "$route" '.provider == {order:[$route],allow_fallbacks:false,require_parameters:true}' "$FIXTURE/body.json" >/dev/null || return 1
  done
}

invalid_configured_routes() {
  local route
  for route in DeepInfra deepinfra/ deepinfra//fp8 /deepinfra deepinfra_fp8 deepinfra,other 'deepinfra other'; do
    reset_fixture
    printf 'review_openrouter_provider: %s\n' "$route" >> "$FIXTURE/factory.yaml"
    run_review
    failed_without_findings && no_requests || return 1
    grep -qi 'provider' "$FIXTURE/stderr" || return 1
  done
}

literal_invalid_route() {
  local literal
  printf -v literal '$(touch "%s")' "$FIXTURE/route-marker"
  run_review "REVIEW_OPENROUTER_PROVIDER=$literal"
  [ ! -e "$FIXTURE/route-marker" ] || return 1
  failed_without_findings && no_requests || return 1
  grep -qi 'provider' "$FIXTURE/stderr"
}

timeout_delayed_success() {
  run_review FIXTURE_REQUIRED_SECONDS=240
  [ "$STATUS" -eq 0 ] && one_request && [ ! -s "$FIXTURE/stderr" ] || return 1
  grep -q 'A concrete finding' "$FIXTURE/stdout" || return 1
  jq -e '.max_tokens == 8192' "$FIXTURE/body.json" >/dev/null
}

timeout_overrides() {
  local value
  for value in 1 240 480 000240 ''; do
    : > "$FIXTURE/calls"
    run_review "REVIEW_TIMEOUT_SECONDS=$value"
    [ "$STATUS" -eq 0 ] && one_request || return 1
    case "$value" in ''|480) value=480 ;; 000240) value=240 ;; esac
    grep -qFx -- "$value" "$FIXTURE/args" || return 1
    grep -qFx -- '--connect-timeout' "$FIXTURE/args" || return 1
    grep -qFx -- '15' "$FIXTURE/args" || return 1
  done
}

timeout_invalid() {
  local value
  for value in 0 481 -1 1.5 1e2 ' 240' abc 999999999999999999999999999999999999; do
    : > "$FIXTURE/calls"
    run_review "REVIEW_TIMEOUT_SECONDS=$value"
    failed_without_findings && no_requests || return 1
    grep -q 'REVIEW_TIMEOUT_SECONDS' "$FIXTURE/stderr" || return 1
  done
}

timeout_diagnostics() {
  run_review FIXTURE_CURL_STATUS=28
  failed_without_findings && one_request || return 1
  grep -qi 'timed out' "$FIXTURE/stderr" || return 1
  grep -q '28' "$FIXTURE/stderr" && grep -q '200' "$FIXTURE/stderr" || return 1
  grep -q '240.000' "$FIXTURE/stderr" && grep -q '0.250' "$FIXTURE/stderr" && grep -q '660' "$FIXTURE/stderr" || return 1
  ! grep -qE 'private-curl-error|fixture-not-a-secret|A concrete finding' "$FIXTURE/stderr"
}

transport_diagnostics() {
  run_review FIXTURE_CURL_STATUS=7
  failed_without_findings && one_request || return 1
  grep -qi 'transport' "$FIXTURE/stderr" && grep -q '7' "$FIXTURE/stderr" || return 1
  ! grep -qE 'private-curl-error|fixture-not-a-secret|A concrete finding' "$FIXTURE/stderr"
}

http_status_refusal() {
  run_review FIXTURE_HTTP_STATUS=503
  failed_without_findings && one_request || return 1
  grep -q '503' "$FIXTURE/stderr" || return 1
  ! grep -q 'A concrete finding' "$FIXTURE/stderr"
}

timeout_all_providers() {
  local provider
  for provider in openrouter openai anthropic; do
    : > "$FIXTURE/calls"
    run_review "MODEL_PROVIDER=$provider" REVIEW_TIMEOUT_SECONDS=240 FIXTURE_CURL_STATUS=28
    failed_without_findings && one_request || return 1
    grep -qFx '240' "$FIXTURE/args" || return 1
    grep -qi 'timed out' "$FIXTURE/stderr" || return 1
  done
}

timeout_metric_sanitization() {
  run_review FIXTURE_CURL_STATUS=28 FIXTURE_TIME_TOTAL=$'private-metric\nfixture-not-a-secret'
  failed_without_findings && one_request || return 1
  grep -qi 'timed out' "$FIXTURE/stderr" || return 1
  ! grep -qE 'private-metric|fixture-not-a-secret|private-curl-error' "$FIXTURE/stderr"
}

timeout_private_cleanup() {
  local status path
  for status in 0 28 7; do
    : > "$FIXTURE/calls"
    run_review "FIXTURE_CURL_STATUS=$status"
    one_request || return 1
    [ -s "$FIXTURE/output-path" ] || return 1
    path="$(cat "$FIXTURE/output-path")"
    case "$path" in "$FIXTURE/http-temp/"*) ;; *) return 1 ;; esac
    [ "$(cat "$FIXTURE/output-mode")" = 'rw-------' ] || return 1
    [ ! -e "$path" ] || return 1
    [ "$(cat "$FIXTURE/http-temp/sentinel")" = 'preserve sibling' ] || return 1
    [ "$(find "$FIXTURE/http-temp" -mindepth 1 | wc -l | tr -d ' ')" = 1 ] || return 1
  done
}

timeout_workflow_wiring() {
  local file
  for file in "$ROOT/.github/workflows/adversarial-review.yml" "$ROOT/packs/review-lane/review-pr.yml"; do
    sed -n '/^      - name: Review$/,/^      - name: Post the review$/p' "$file" |
      grep -q '^          REVIEW_TIMEOUT_SECONDS:.*vars.REVIEW_TIMEOUT_SECONDS' || return 1
    grep -q 'timeout-minutes: 10' "$file" || return 1
  done
}

check() {
  local name="$1" scenario="$2"
  reset_fixture
  if "$scenario"; then
    PASSED=$((PASSED + 1))
    printf '  ok: %s\n' "$name"
  else
    FAILED=$((FAILED + 1))
    printf '  FAIL: %s (script status %s)\n' "$name" "$STATUS" >&2
    cat "$FIXTURE/stderr" >&2
  fi
}

check 'delayed completion gets bounded time without a larger token cap' timeout_delayed_success
check 'timeout overrides normalize and retain a separate connect bound' timeout_overrides
check 'invalid timeouts refuse before HTTP' timeout_invalid
check 'timeout diagnostics preserve numeric evidence without leaking partial data' timeout_diagnostics
check 'transport failures are distinct from absent review content' transport_diagnostics
check 'HTTP errors cannot turn valid-looking partial content into findings' http_status_refusal
check 'all providers share the same bounded timeout contract' timeout_all_providers
check 'malformed transport metrics cannot leak raw diagnostic data' timeout_metric_sanitization
check 'private response storage is cleaned without touching existing siblings' timeout_private_cleanup
check 'active and generated workflows expose the Actions timeout variable' timeout_workflow_wiring
check 'configured OpenRouter model and markdown'  configured_success
check 'repository config uses default OpenRouter without native provider selection' repository_default_provider
check 'structural: privileged job requires same repository AND default base branch' trusted_base_guard
check 'OpenRouter default 8192-token response cap' openrouter_cap
check 'OpenRouter YAML output cap is configurable' configured_max_tokens
check 'caller output cap overrides YAML' max_tokens_environment_override
check 'empty caller output cap uses the safe default' empty_max_tokens_uses_default
check 'invalid output caps refuse before HTTP' invalid_max_tokens
check 'OpenRouter truncated output is incomplete, not review evidence' truncation_refusal
check 'diff content reaches JSON literally without execution' literal_diff
check 'missing API key refuses before HTTP' missing_key
check 'HTTP failure makes one bounded request without retries' http_failure
check 'provider error refuses with diagnostic' provider_error
check 'empty diff performs no HTTP request' empty_diff
check 'Anthropic request retains existing 4096-token cap' anthropic_contract
check 'OpenAI request body remains unchanged' openai_contract
check 'OpenAI length response retains its existing behavior' openai_response_unchanged
check 'caller model overrides configured model' environment_override
check 'malformed response cannot become approval' malformed_response
check 'missing diff refuses before HTTP' missing_diff
check 'complete no-findings answer remains valid' no_findings_success
check 'configured reasoning effort reaches OpenRouter within existing cap' configured_reasoning
check 'unconfigured reasoning preserves provider defaults' unconfigured_reasoning
check 'caller reasoning effort overrides configuration' reasoning_environment_override
check 'explicit empty caller effort omits configured reasoning' reasoning_empty_override
check 'legacy reasoning fallback remains below YAML precedence' legacy_reasoning_precedence
check 'documented gateway reasoning vocabulary is forwarded literally' gateway_reasoning_values
check 'invalid configured reasoning refuses before HTTP' invalid_configured_reasoning
check 'invalid caller reasoning is inert data and refuses before HTTP' literal_invalid_reasoning
check 'configured OpenRouter provider excludes fallbacks and requires parameters' configured_route
check 'unconfigured OpenRouter routing preserves provider defaults' unconfigured_route
check 'caller OpenRouter endpoint overrides configured provider' route_environment_override
check 'explicit empty caller route omits provider restriction' route_empty_override
check 'legacy OpenRouter route fallback remains below YAML precedence' legacy_route_precedence
check 'base and endpoint route slugs are forwarded literally' accepted_route_slugs
check 'invalid configured provider slugs refuse before HTTP' invalid_configured_routes
check 'invalid caller route is inert data and refuses before HTTP' literal_invalid_route

printf 'adversarial-review: %s passed, %s failed\n' "$PASSED" "$FAILED"
[ "$FAILED" -eq 0 ]
