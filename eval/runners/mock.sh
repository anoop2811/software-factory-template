#!/bin/sh
# eval/runners/mock.sh
# A deterministic mock runner for the eval harness's own self-test. It calls no
# model — it just exercises the scorer so the harness is provable in CI without
# credentials. FACTORY_MOCK_MODE selects the behaviour:
#   pass  (default) — solve the reference task (verify.sh will pass)
#   fail            — produce nothing (verify.sh will fail)
#   cheat           — solve, but tamper the oracle (integrity check must catch it)
#
# The runner contract — the same shape a real opencode / Claude / Codex runner
# implements: `runner <workdir>`. The runner reads <workdir>/task.md and writes
# its implementation into <workdir>. Its exit status is ignored; verify.sh scores.
WORKDIR="$1"
cd "$WORKDIR" || exit 0
case "${FACTORY_MOCK_MODE:-pass}" in
  pass)  printf 'FIXED\n' > answer.txt ;;
  fail)  : ;;
  cheat) printf 'FIXED\n' > answer.txt; : > verify.sh ;;
esac
exit 0
