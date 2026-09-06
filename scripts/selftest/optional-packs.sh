#!/bin/bash
set -euo pipefail

# Template-only integration coverage for Decision 44 (docs/DECISION_LOG.md:1653).
# Run the adopted selftest in isolated working-tree copies; never install this
# wrapper into adopter selftests or recursively invoke it from run.sh.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEMPLATE_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

python3 - "$TEMPLATE_ROOT" <<'PYTHON'
import os
from pathlib import Path
import re
import shutil
import signal
import subprocess
import sys
import tempfile

source = Path(sys.argv[1])
required = [
    "scripts/selftest/run.sh",
    "packs/go/pack.yaml",
    "packs/java/pack.yaml",
    "packs/typescript/pack.yaml",
    "packs/go/hooks/ginkgo-only-check.sh",
    "packs/java/hooks/junit5-only-check.sh",
    "packs/typescript/hooks/vitest-only-check.sh",
    "packs/review-lane/review-pr.yml",
    "templates/metrics.html",
]
missing = [name for name in required if not (source / name).is_file()]
if missing:
    sys.exit("optional-packs: required template inputs missing: " + ", ".join(missing))

summary_pattern = re.compile(r"^selftest: (\d+) passed, (\d+) failed, (\d+) skipped$", re.M)
late_assertion = "ok: an odd gate name survives as data, not code"
environment = os.environ.copy()
# Fixture git commands must never inherit the caller's checkout or agent role.
for key in list(environment):
    if key.startswith("GIT_") or key in ("FACTORY_CONFIG", "FACTORY_EVENT_LOG"):
        environment.pop(key)
environment["FACTORY_AGENT_ROLE"] = ""
environment["GIT_CONFIG_NOSYSTEM"] = "1"
environment["GIT_CONFIG_GLOBAL"] = os.devnull


def require(condition, message, output):
    if not condition:
        print(output, file=sys.stderr)
        raise AssertionError(message)


with tempfile.TemporaryDirectory(prefix="factory-optional-packs-") as temporary:
    workspace = Path(temporary)
    fixture = workspace / "fixture"
    shutil.copytree(
        source, fixture,
        ignore=shutil.ignore_patterns(
            ".git", ".factory", "node_modules", ".venv", "__pycache__",
            "optional-packs.sh",
        ),
    )
    subprocess.run(["git", "init", "-q", str(fixture)], env=environment, check=True)
    packs = workspace / "packs"
    (fixture / "packs").rename(packs)

    def run_case(name):
        print("optional-packs: " + name, flush=True)
        log = workspace / (name + ".log")
        with log.open("w") as output_file:
            process = subprocess.Popen(
                ["bash", "scripts/selftest/run.sh"], cwd=fixture,
                env=environment, stdout=output_file, stderr=subprocess.STDOUT,
                start_new_session=True,
            )
            try:
                status = process.wait(timeout=240)
            except subprocess.TimeoutExpired:
                os.killpg(process.pid, signal.SIGKILL)
                process.wait()
                raise AssertionError(name + " exceeded 240 seconds") from None
        output = log.read_text()
        match = summary_pattern.search(output)
        require(match is not None, name + " did not finish with a coverage summary", output)
        require(late_assertion in output, name + " did not reach the later metrics checks", output)
        return status, tuple(map(int, match.groups())), output

    status, counts, output = run_case("no-packs")
    require(status == 0 and counts[1] == 0, "missing optional packs failed the suite", output)
    require(counts[2] > 0, "missing packs were not counted as skipped", output)
    require(re.search(r"^\s*skip:.*packs/\*/pack\.yaml", output, re.M),
            "missing pack catalog was not named", output)
    require(re.search(r"^\s*skip:.*packs/review-lane/review-pr\.yml", output, re.M),
            "missing review-lane template was not named", output)

    packs.rename(fixture / "packs")
    review_template = fixture / "packs/review-lane/review-pr.yml"
    saved_review = workspace / "review-pr.yml"
    review_template.rename(saved_review)
    status, counts, output = run_case("no-review-template")
    require(status == 0 and counts[1] == 0 and counts[2] > 0,
            "missing review template did not pass with reported skips", output)
    require(re.search(r"^\s*skip:.*packs/review-lane/review-pr\.yml", output, re.M),
            "missing review template was not named", output)
    require("ok: java gate rejects violations when run directly" in output,
            "available dialect fixtures did not run", output)

    saved_review.rename(review_template)
    broken_gate = fixture / "packs/java/hooks/junit5-only-check.sh"
    broken_gate.write_text("#!/bin/bash\nexit 0\n")
    broken_gate.chmod(0o755)
    status, counts, output = run_case("broken-present-gate")
    require(status == 1 and counts[1] > 0,
            "a broken present gate was allowed to pass", output)
    require("FAIL: junit5-only-check rejects JUnit 4 import" in output,
            "the broken gate was not exercised against a violation", output)
    require(not re.search(r"^\s*skip:.*packs/java/hooks/junit5-only-check\.sh", output, re.M),
            "a present gate was mislabeled as unavailable", output)

print("optional-packs: 3 passed, 0 failed")
PYTHON
