"""Contract acceptance for docs/LOOPS.md:5 through real CLI and filesystem paths."""
import json
from contextlib import contextmanager
import importlib.util
import os
from pathlib import Path
import shutil
import signal
import stat
import subprocess
import sys
import tempfile
import time
from types import SimpleNamespace
from unittest import mock

SOURCE = Path(sys.argv[1]).resolve()
ENV = {key: value for key, value in os.environ.items()
       if not key.startswith(("FACTORY_", "GIT_", "CODEX_", "CLAUDE_", "OPENCODE_", "LOOP_"))
       and key not in ("MODEL_PROVIDER", "COST_PROFILE")}
ENV.update(GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_GLOBAL=os.devnull, PYTHONDONTWRITEBYTECODE="1")
CHECKS = []


def test(name):
    def register(function):
        CHECKS.append((name, function))
        return function
    return register


def require(condition, message):
    if not condition:
        raise AssertionError(message)


def objects(result):
    values = []
    for line in result.stdout.splitlines():
        try:
            values.append(json.loads(line))
        except ValueError:
            pass
    require(values, "missing JSON output: " + result.stdout + result.stderr)
    return values


class Fixture:
    def __init__(self, parent, name):
        self.root = parent / name
        self.root.mkdir()
        for relative in ("factory", "scripts/factory-loop.sh", "scripts/factory-budget.sh", "scripts/lib",
                         ".opencode/agent", ".claude/agents", ".codex/agents", "opencode.json",
                         "scripts/hooks/test-edit-denial.sh"):
            origin, target = SOURCE / relative, self.root / relative
            target.parent.mkdir(parents=True, exist_ok=True)
            if origin.is_dir():
                shutil.copytree(origin, target, ignore=shutil.ignore_patterns("__pycache__"))
            elif origin.is_file():
                shutil.copy2(origin, target)
        self.bin = self.root / "fixture-bin"
        self.bin.mkdir()
        for harness in ("codex", "claude", "opencode"):
            shutil.copy2(SOURCE / "scripts/selftest/loop/fake_cli.py", self.bin / harness)
            (self.bin / harness).chmod(0o755)
        shutil.copy2(SOURCE / "scripts/selftest/loop/check.py", self.root / "check.py")
        (self.root / ".gitignore").write_text(".factory/\nfixture-*\ntemplate/\nadopter/\noutside/\n.claude/settings.local.json\n")
        (self.root / ".claude/settings.local.json").write_text('{}\n')
        (self.root / "product.txt").write_text("initial\n")
        (self.root / "product_test.py").write_text("# human-owned tests\n")
        (self.root / "governance.txt").write_text("human-reviewed governance\n")
        self.calls = self.root / "fixture-calls.jsonl"
        self.child = self.root / "fixture-child.pid"
        self.prompt = self.root / "fixture-prompt.txt"
        self.prompt.write_text("private-prompt-sentinel: implement the requested change\n")
        self.environment = dict(ENV, PATH=str(self.bin) + os.pathsep + ENV["PATH"],
                                FACTORY_CONFIG=str(self.root / "factory.yaml"),
                                LOOP_FIXTURE_CALLS=str(self.calls), LOOP_FIXTURE_CHILD=str(self.child))
        self.settings = {}
        self.config(loop_enabled="true", loop_max_attempts="3", loop_timeout_seconds="30",
                    loop_check_timeout_seconds="3", loop_check_command="python3 check.py",
                    loop_no_progress_limit="1", check_command="python3 check.py",
                    budget_enabled="true", budget_max_attempts="8", budget_max_session_runs="12",
                    budget_timeout_seconds="5", budget_session_seconds="30", budget_max_concurrent="1",
                    protected_paths="governance.txt scripts/lib", test_file_patterns=r"_test\.py$")
        self.git("init", "-q")
        self.git("config", "user.email", "fixture@example.invalid")
        self.git("config", "user.name", "Loop Fixture")
        self.git("add", ".")
        self.git("commit", "-qm", "fixture")

    def git(self, *args):
        result = subprocess.run(["git"] + list(args), cwd=self.root, env=self.environment,
                                capture_output=True, text=True, timeout=10)
        require(result.returncode == 0, "git fixture failed: " + result.stderr)
        return result.stdout.strip()

    def config(self, **settings):
        self.settings.update(settings)
        (self.root / "factory.yaml").write_text("".join(
            key + ': "' + str(value) + '"\n' for key, value in self.settings.items()))

    def command(self, verb="run", harness="claude", session="session", task="task", mode="bounded", extra=()):
        args = [str(self.root / "factory"), "loop", verb, "--session", session, "--task", task, "--json"]
        if verb != "status":
            args += ["--harness", harness]
            if mode is not None:
                args += ["--mode", mode]
            if verb in ("run", "resume") and mode == "bounded":
                args += ["--prompt-file", str(self.prompt)]
        return args + list(extra)

    def run(self, verb="run", fixture="normal", expected=0, **options):
        result = subprocess.run(self.command(verb, **options), cwd=self.root,
                                env=dict(self.environment, LOOP_FIXTURE_MODE=fixture),
                                capture_output=True, text=True, timeout=25)
        require(result.returncode != 0 if expected is None else result.returncode == expected,
                "exit {} expected {}: {}{}".format(result.returncode, expected, result.stdout, result.stderr))
        return result

    def start(self, fixture="hold", **options):
        return subprocess.Popen(self.command(**options), cwd=self.root,
                                env=dict(self.environment, LOOP_FIXTURE_MODE=fixture),
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)

    def invoked(self):
        return [json.loads(line) for line in self.calls.read_text().splitlines()] if self.calls.exists() else []

    def roles(self):
        return [call["role"] for call in self.invoked()]

    def history(self):
        return json.loads((self.root / ".factory/budget.json").read_text())["runs"]

    def checkpoint(self):
        return objects(self.run("status"))[-1]

    def wait_for_child(self):
        until = time.monotonic() + 8
        while time.monotonic() < until:
            if self.child.exists():
                return int(self.child.read_text())
            time.sleep(0.03)
        raise AssertionError("fixture never launched child")


def stopped(pid):
    result = subprocess.run(["ps", "-o", "stat=", "-p", str(pid)], capture_output=True, text=True)
    return result.returncode != 0 or not result.stdout.strip() or result.stdout.strip().startswith("Z")


def cleanup(process, child=None):
    if process.poll() is None:
        process.send_signal(signal.SIGTERM)
    try:
        process.communicate(timeout=8)
    except subprocess.TimeoutExpired:
        process.kill()
        process.communicate(timeout=3)
    if child and not stopped(child):
        os.kill(child, signal.SIGKILL)


@test("default manual plan is local, read-only and never inherits enabled budgets")
def manual_plan(f):
    plan = objects(f.run("plan", mode=None))[-1]
    require("manual" in json.dumps(plan), "default did not select manual mode")
    require(not f.invoked() and not (f.root / ".factory").exists(), "plan performed work or wrote storage")


@test("manual checks distinguish passing evidence from implemented and reviewed completion")
def manual(f):
    (f.root / "product.txt").write_text("good\n")
    result = f.run(mode=None)
    require(f.roles() == ["check"], "manual mode called a model")
    require(not (f.root / ".factory/budget.json").exists(), "manual mode consumed model budget")
    checkpoint = f.checkpoint()
    require(checkpoint["status"] == "stopped" and checkpoint["outcome"] == "manual_passed",
            "manual check falsely claimed implemented and reviewed completion")
    evidence = checkpoint["evidence"]
    require(len(evidence) == 1 and evidence[0]["kind"] == "check", "manual evidence fabricated model work")
    require(evidence[0]["command"] == "python3 check.py" and evidence[0]["exit_code"] == 0,
            "checkpoint lacks exact check command/result")
    require(evidence[0]["snapshot"]["head"] == f.git("rev-parse", "HEAD")
            and evidence[0]["snapshot"]["source"] == checkpoint["snapshot"]["source"]
            and evidence[0]["duration_seconds"] >= 0 and checkpoint["elapsed_seconds"] >= evidence[0]["duration_seconds"],
            "check evidence lacks tested content/HEAD identity or elapsed time")
    require(objects(result), "manual output omitted structured result")


@test("disabled bounded execution refuses before checks or model calls")
def disabled(f):
    for settings in ({"loop_enabled": "false"}, {"loop_enabled": "true", "budget_enabled": "false"}):
        f.config(**settings)
        f.run(expected=None)
        require(not f.invoked(), "disabled configuration performed work")


@test("malformed modes, identities, limits and missing commands fail without side effects")
def invalid(f):
    for options in ({"mode": "automatic"}, {"session": "../escape"}, {"task": "bad/name"}):
        f.run(expected=None, **options)
    for key, value in (("loop_max_attempts", "0"), ("loop_timeout_seconds", "nan"),
                       ("loop_check_timeout_seconds", "-1"), ("loop_no_progress_limit", "0"),
                       ("loop_enabled", "sometimes")):
        original = f.settings[key]
        f.config(**{key: value})
        f.run(expected=None)
        f.config(**{key: original})
    f.config(loop_check_command="", check_command="")
    f.run(mode="manual", expected=None)
    require(not f.invoked() and not (f.root / ".factory").exists(), "invalid inputs had side effects")


@test("all three harnesses implement then check then independently review with shared accounting")
def harnesses(f):
    for harness in ("codex", "claude", "opencode"):
        (f.root / "product.txt").write_text("initial\n")
        before = len(f.invoked())
        result = f.run(harness=harness, task=harness)
        calls = f.invoked()[before:]
        require([call["role"] for call in calls] == ["implementer", "check", "reviewer"],
                harness + " did not preserve implement/check/review order")
        require([row["role"] for row in f.history() if row["task"] == harness] == ["implementer", "reviewer"],
                harness + " reviewer escaped the shared task budget")
        require(all(call["name"] == harness for call in calls if call["role"] != "check"),
                "harness silently switched")
        require("completed" in json.dumps(objects(result)[-1]).lower(), "successful bounded run lacked completed state")


@test("failed checks feed bounded diagnostics to a repair before review")
def check_repair(f):
    f.run(fixture="check_repair")
    require(f.roles() == ["implementer", "check", "implementer", "check", "reviewer"], "incorrect check repair sequence")
    repair = [call for call in f.invoked() if call["role"] == "implementer"][1]["prompt"]
    require("private-check-log-sentinel" in repair and "untrusted" in repair.lower(),
            "repair did not receive explicitly untrusted diagnostic context")


@test("review findings drive a separately checked repair and a second reviewer")
def review_repair(f):
    f.run(fixture="review_repair")
    require(f.roles() == ["implementer", "check", "reviewer", "implementer", "check", "reviewer"],
            "review repair skipped deterministic recheck or independent rereview")
    repair = [call for call in f.invoked() if call["role"] == "implementer"][1]["prompt"]
    require("private-review-finding-sentinel" in repair, "review findings missing from repair")
    require(len(f.history()) == 4, "reviewer and repair calls were not all charged")


@test("one budget attempt permits implementation but refuses the reviewer without raising limits")
def budget_limit(f):
    f.config(budget_max_attempts="1")
    before = (f.root / "factory.yaml").read_bytes()
    f.run(expected=None)
    require(f.roles() == ["implementer", "check"] and len(f.history()) == 1,
            "reviewer bypassed exhausted task budget")
    require((f.root / "factory.yaml").read_bytes() == before, "controller raised user budget")


@test("implementation failures never trigger automatic paid retries")
def implementation_failure(f):
    f.run(fixture="implement_fail", expected=None)
    require(f.roles() == ["implementer"], "failed process triggered checks or paid retry")
    require((f.root / "product.txt").read_text() == "good\n", "stop discarded useful user changes")


@test("malformed and failed reviewer results stop without accepting a passing check alone")
def invalid_review(f):
    for mode in ("review_invalid", "review_fail"):
        before = len(f.invoked())
        f.run(fixture=mode, task=mode, expected=None)
        require(f.roles()[before:] == ["implementer", "check", "reviewer"], "invalid reviewer auto-retried")


@test("strict reviewer contract rejects duplicate keys, extra fields and contradictory findings")
def strict_review(f):
    responses = ['{"verdict":"approve","findings":["must fix"]}',
                 '{"verdict":"repair","findings":[]}',
                 '{"verdict":"approve","findings":[],"extra":true}',
                 '{"verdict":"repair","verdict":"approve","findings":[]}']
    for index, response in enumerate(responses):
        f.environment["LOOP_FIXTURE_REVIEW"] = response
        before = len(f.invoked())
        f.run(task="strict" + str(index), expected=None)
        require(f.roles()[before:] == ["implementer", "check", "reviewer"], "malformed verdict triggered repair")


@test("unchanged failed source stops repeated repair before exhausting the generous budget")
def no_progress(f):
    f.config(loop_max_attempts="5")
    result = f.run(fixture="repeat_bad", expected=None)
    require(f.roles().count("implementer") == 2 and "reviewer" not in f.roles(),
            "unchanged failed source consumed excess repairs")
    require(any(word in (result.stdout + result.stderr).lower() for word in ("progress", "repeat")),
            "no-progress handoff reason missing")


@test("repeated identical check and source state stops independently of no-progress allowance")
def repeated_failure(f):
    f.config(loop_max_attempts="5", loop_no_progress_limit="4")
    f.run(fixture="repeat_bad", expected=None)
    require(f.roles() == ["implementer", "check", "implementer", "check"], "identical failure kept repeating")
    require(f.checkpoint()["outcome"] == "repeated_failure", "repeated check failure not identified")


@test("loop attempt limit counts the first implementation and preserves failed changes")
def attempts(f):
    f.config(loop_max_attempts="1")
    f.run(fixture="check_repair", expected=None)
    require(f.roles() == ["implementer", "check"], "loop initial attempt not counted")
    require((f.root / "product.txt").read_text() == "bad\n", "failure discarded working tree")


@test("test, governing config and protected-path mutations require human handoff")
def boundaries(f):
    for mode, path in (("test_mutation", "product_test.py"), ("protected_mutation", "governance.txt"),
                       ("config_mutation", "factory.yaml"), ("role_mutation", ".opencode/agent/reviewer.md")):
        before = (f.root / path).read_bytes()
        offset = len(f.invoked())
        f.run(fixture=mode, task=mode, expected=None)
        require(f.roles()[offset:] == ["implementer"], "forbidden mutation continued into checks or review")
        require((f.root / path).read_bytes() != before, "controller reverted mutation instead of preserving handoff")
        count = len(f.invoked())
        f.run("resume", task=mode, expected=None)
        require(len(f.invoked()) == count, "resume silently accepted forbidden mutation")


@test("ignored native harness permission files remain governing policy inputs")
def ignored_policy(f):
    f.run(fixture="ignored_policy_mutation", expected=None)
    require(f.roles() == ["implementer"], "ignored harness policy mutation continued into checks or review")


@test("Go and Java pack POSIX test patterns protect their actual test filenames")
def pack_patterns(f):
    for index, (pattern, filename) in enumerate(((r"_test\.go([^[:alnum:]_]|$)", "product_test.go"),
                                                (r"(Test|Tests|IT|ITCase)\.java$", "ProductIT.java"))):
        f.config(test_file_patterns=pattern)
        (f.root / filename).write_text("human-owned tests\n")
        f.environment["LOOP_FIXTURE_MUTATION_PATH"] = filename
        before = len(f.invoked())
        f.run(fixture="test_mutation", task="pack" + str(index), expected=None)
        require(f.roles()[before:] == ["implementer"], "pack test mutation escaped human handoff")


@test("budget admission deadline is checked after waiting for ledger lock before native spawn")
def admission_deadline(f):
    modules = {}
    for name in ("budget_adapters", "budget"):
        spec = importlib.util.spec_from_file_location(name, f.root / "scripts/lib" / (name + ".py"))
        module = importlib.util.module_from_spec(spec)
        with mock.patch.dict(sys.modules, modules):
            spec.loader.exec_module(module)
        modules[name] = module
    runtime = modules["budget"]
    ledger = runtime.Ledger(f.root)
    original_locked = ledger.locked
    delays = [True]

    @contextmanager
    def delayed_lock():
        with original_locked():
            if delays:
                delays.pop()
                time.sleep(0.12)
            yield

    args = SimpleNamespace(session="deadline", task="task", harness="claude", role="implementer",
                           max_cost_usd=None, json=True, prompt_file=None)
    # Native readiness is unrelated to the admission race; skip its help probes
    # while retaining the real command builder, ledger and subprocess executor.
    with mock.patch.dict(os.environ, f.environment, clear=True), mock.patch.object(ledger, "locked", delayed_lock), \
            mock.patch.object(modules["budget_adapters"], "preflight", return_value=None), \
            mock.patch.object(runtime.subprocess, "Popen", wraps=subprocess.Popen) as spawn:
        config = runtime.configuration()
        config["enabled"] = True
        try:
            runtime.run(args, config, ledger, f.root, prompt_text="deadline fixture", quiet=True,
                        response_callback=lambda *_: None, deadline=time.monotonic() + 0.05)
        except runtime.BudgetError:
            pass
    require(spawn.call_count == 0 and not f.invoked(),
            "expired admission deadline still launched native child after ledger wait")


@test("unconfirmed model exit plus failed completion write blocks subsequent manual loops")
def uncertain_completion(f):
    # Start from a valid actual checkpoint; inject only the OS completion-write
    # failure and unconfirmed subprocess result, without creating an orphan.
    f.run(fixture="implement_fail", expected=None)
    modules = {}
    for name in ("budget_adapters", "budget", "loop"):
        spec = importlib.util.spec_from_file_location(name, f.root / "scripts/lib" / (name + ".py"))
        module = importlib.util.module_from_spec(spec)
        with mock.patch.dict(sys.modules, modules):
            spec.loader.exec_module(module)
        modules[name] = module
    runtime, budget_runtime = modules["loop"], modules["budget"]
    store = runtime.Store(f.root)
    history = store.read()
    record = history["runs"][0]
    config = {"enabled": True, "check_command": "python3 check.py", "test_patterns": [r"_test\.py$"],
              "protected_paths": ["governance.txt", "scripts/lib"], "max_attempts": 3,
              "timeout_seconds": 30, "check_timeout_seconds": 3, "no_progress_limit": 1}
    args = SimpleNamespace(session="session", task="task", harness="claude", mode="bounded",
                           prompt_file=str(f.prompt), json=True)
    original_write = budget_runtime.Ledger.write
    writes = []

    def fail_completion(ledger, data):
        writes.append(True)
        if len(writes) == 3:
            raise OSError("fixture completion fsync failed")
        return original_write(ledger, data)

    def uncertain_execute(argv, prompt, root, overrides, timeout, on_spawn, **options):
        on_spawn(123456)
        return "timeout", None, 0.01, b""

    with mock.patch.dict(os.environ, f.environment, clear=True):
        budget_config = budget_runtime.configuration()
        budget_config.update(enabled=True, max_attempts=8)
        record.update(status="active", outcome="running", attempts=0, elapsed_seconds=0,
                      evidence=[], budget_runs=[], uncertain=False,
                      baseline=runtime.stable_snapshot(f.root, config), snapshot=runtime.stable_snapshot(f.root, config),
                      policy=runtime.policy(config, budget_config))
        with mock.patch.object(budget_runtime.Ledger, "write", fail_completion), \
                mock.patch.object(budget_runtime, "execute", uncertain_execute), \
                mock.patch.object(modules["budget_adapters"], "preflight", return_value=None):
            controller = runtime.Controller(args, config, budget_config, f.root, store, history, record,
                                            f.prompt.read_text())
            require(controller.execute() != 0, "uncertain model invocation reported completion")
        require(len(writes) == 3 and f.history()[-1]["status"] == "active", "fault injection missed completion write")
    result = f.run("plan", mode="manual", expected=None)
    require(objects(result)[-1]["blockers"], "manual plan bypassed an unconfirmed model child")
    count = len(f.invoked())
    f.run(mode="manual", session="other", expected=None)
    require(len(f.invoked()) == count, "manual check overlapped an unconfirmed model child")


@test("source changes during a check or reviewer invalidate the passing evidence")
def evidence_changes(f):
    for mode in ("check_mutation", "review_mutation"):
        offset = len(f.invoked())
        f.run(fixture=mode, task=mode, expected=None)
        expected = ["implementer", "check"] + (["reviewer"] if mode == "review_mutation" else [])
        require(f.roles()[offset:] == expected, "stale source evidence continued the loop")


@test("fresh terminal resume does not relaunch or reset accounting; changed source refuses")
def resume(f):
    f.run()
    before = (f.root / ".factory/budget.json").read_bytes()
    count = len(f.invoked())
    # A terminal checkpoint may report completed idempotently or ask for handoff.
    result = subprocess.run(f.command("resume"), cwd=f.root, env=f.environment,
                            capture_output=True, text=True, timeout=20)
    require(result.returncode in (0, 2), "unexpected terminal resume result: " + result.stdout + result.stderr)
    require(len(f.invoked()) == count and (f.root / ".factory/budget.json").read_bytes() == before,
            "resume relaunched or reset budget")
    (f.root / "new-untracked.txt").write_text("new relevant source\n")
    f.run("resume", expected=None)
    require(len(f.invoked()) == count, "untracked source did not invalidate checkpoint")


@test("prompt, policy, file modes and deletions invalidate checkpoint freshness")
def freshness(f):
    mutations = [lambda: f.prompt.write_text("changed instructions\n"),
                 lambda: (f.root / "product.txt").chmod(0o755),
                 lambda: (f.root / "product.txt").unlink(),
                 lambda: f.config(loop_max_attempts="4")]
    for index, mutate in enumerate(mutations):
        (f.root / "product.txt").write_text("initial\n")
        f.run(task="fresh" + str(index))
        count = len(f.invoked())
        mutate()
        result = f.run("resume", task="fresh" + str(index), expected=None)
        require("stale checkpoint" in (result.stdout + result.stderr).lower(),
                "changed input hit a terminal-state rejection instead of checkpoint freshness")
        require(len(f.invoked()) == count, "stale checkpoint triggered model execution")


@test("resumable manual checkpoints reject changed tracked, untracked, mode, deleted and policy inputs")
def manual_freshness(f):
    mutations = [lambda: (f.root / "product.txt").write_text("good changed\n"),
                 lambda: (f.root / "new-source.txt").write_text("new relevant source\n"),
                 lambda: (f.root / "product.txt").chmod(0o755),
                 lambda: (f.root / "product.txt").unlink(),
                 lambda: f.config(loop_max_attempts="4"),
                 lambda: (f.root / ".claude/settings.local.json").write_text('{"changed":true}\n')]
    for index, mutate in enumerate(mutations):
        (f.root / "product.txt").write_text("good\n")
        task = "manualfresh" + str(index)
        f.run(mode="manual", task=task)
        count = len(f.invoked())
        mutate()
        result = f.run("resume", mode="manual", task=task, expected=None)
        require("stale checkpoint" in (result.stdout + result.stderr).lower(), "manual freshness did not explain stale input")
        require(len(f.invoked()) == count, "stale manual checkpoint launched another check")


@test("manual checkpoint resume cannot silently enable bounded model calls")
def resume_manual(f):
    (f.root / "product.txt").write_text("good\n")
    f.run(mode="manual")
    f.run("resume", mode="bounded", expected=None)
    require(f.roles() == ["check"], "manual resume escalated to paid mode")


@test("manual resume retains elapsed allowance and cannot reset an exhausted checkpoint")
def resume_time(f):
    f.config(loop_timeout_seconds="2")
    f.environment["LOOP_FIXTURE_DELAY"] = "0.1"
    (f.root / "product.txt").write_text("good\n")
    f.run(mode="manual", fixture="check_delay")
    original = f.checkpoint()
    f.environment["LOOP_FIXTURE_DELAY"] = "3"
    result = f.run("resume", mode="manual", fixture="check_delay", expected=None)
    resumed = f.checkpoint()
    require(resumed["elapsed_seconds"] > original["elapsed_seconds"]
            and resumed["evidence"][:len(original["evidence"])] == original["evidence"]
            and resumed["attempts"] == 0,
            "manual resume discarded consumed time or evidence: " + result.stdout + result.stderr)
    count = len(f.invoked())
    f.run("resume", mode="manual", expected=None)
    require(len(f.invoked()) == count, "exhausted checkpoint restarted checks")


@test("unmerged index entries prevent any check, model or checkpoint write")
def unmerged(f):
    oid = f.git("rev-parse", "HEAD:product.txt")
    result = subprocess.run(["git", "update-index", "--index-info"], cwd=f.root, env=f.environment,
                            input="0 " + "0" * 40 + "\tproduct.txt\n100644 " + oid + " 1\tproduct.txt\n",
                            capture_output=True, text=True, timeout=10)
    require(result.returncode == 0, "cannot construct unmerged fixture: " + result.stderr)
    f.run(expected=None)
    require(not f.invoked() and not (f.root / ".factory").exists(), "unmerged index performed work")


@test("private checkpoints omit prompt, agent answers, raw logs and secrets")
def privacy(f):
    f.environment["UNRELATED_API_KEY"] = "private-secret-sentinel"
    f.run(fixture="check_repair")
    for path in (f.root / ".factory").rglob("*"):
        if path.is_file():
            content = path.read_text()
            require(not any(value in content for value in ("private-prompt-sentinel", "private-agent-response-sentinel",
                        "private-check-log-sentinel", "private-secret-sentinel")), "private data leaked into " + str(path))
            require(stat.S_IMODE(path.stat().st_mode) & 0o077 == 0, "checkpoint accessible to other users")


@test("corrupt checkpoint state is preserved and refuses further work")
def corrupt(f):
    directory = f.root / ".factory"
    directory.mkdir(mode=0o700)
    checkpoint = directory / "loops.json"
    for content in ("{broken", '{"schema":999,"runs":[]}', '{"schema":1,"runs":[{}]}'):
        checkpoint.write_text(content)
        checkpoint.chmod(0o600)
        f.run(expected=None)
        require(checkpoint.read_text() == content and not f.invoked(), "corruption silently reset or ran")


@test("symlinked storage and checkpoint files never write outside private storage")
def symlinks(f):
    outside = f.root / "outside"
    outside.mkdir()
    directory = f.root / ".factory"
    directory.symlink_to(outside, target_is_directory=True)
    f.run(expected=None)
    require(not list(outside.iterdir()) and not f.invoked(), "storage symlink followed")
    directory.unlink()
    directory.mkdir(mode=0o700)
    target = outside / "data.json"
    content = '{"schema":1,"runs":[]}'
    target.write_text(content)
    (directory / "loops.json").symlink_to(target)
    f.run(expected=None)
    require(target.read_text() == content and not f.invoked(), "checkpoint symlink followed")


@test("deterministic check timeout terminates descendants and records a stopped handoff")
def check_timeout(f):
    f.config(loop_check_timeout_seconds="1")
    started = time.monotonic()
    try:
        f.run(mode="manual", fixture="check_hold", expected=None)
        child = int(f.child.read_text())
        require(stopped(child) and time.monotonic() - started < 10, "check timeout left a process alive")
        require("timeout" in json.dumps(f.checkpoint()).lower(), "check timeout handoff absent")
    finally:
        if f.child.exists() and not stopped(int(f.child.read_text())):
            os.kill(int(f.child.read_text()), signal.SIGKILL)


@test("loop deadline also bounds a budgeted model invocation and kills descendants")
def loop_timeout(f):
    f.config(loop_timeout_seconds="1", budget_timeout_seconds="10")
    started = time.monotonic()
    try:
        f.run(fixture="hold", expected=None)
        child = int(f.child.read_text())
        require(stopped(child) and time.monotonic() - started < 10, "loop deadline did not bound model execution")
        require(len(f.history()) == 1 and f.history()[0]["status"] != "active",
                "loop timeout abandoned budget child accounting")
    finally:
        if f.child.exists() and not stopped(int(f.child.read_text())):
            os.kill(int(f.child.read_text()), signal.SIGKILL)


@test("checkout lock rejects concurrent sessions and signal cleans owned children")
def concurrency(f):
    process = f.start()
    child = None
    try:
        child = f.wait_for_child()
        f.run(session="other", expected=None)
        require(f.roles() == ["implementer"], "second session overlapped active controller")
        process.send_signal(signal.SIGTERM)
        process.communicate(timeout=10)
        require(process.returncode != 0 and stopped(child), "signal left model descendants alive")
    finally:
        cleanup(process, child)


@test("unclean controller death keeps a stale active checkpoint and blocks new work")
def stale_active(f):
    process = f.start()
    child = None
    try:
        child = f.wait_for_child()
        process.kill()
        process.wait(timeout=3)
        f.run(session="other", expected=None)
        require(f.roles() == ["implementer"], "stale checkpoint admitted overlapping model execution")
    finally:
        if (f.root / ".factory/budget.json").exists():
            for run in f.history():
                pid = run.get("process_pid")
                if pid:
                    try:
                        os.killpg(pid, signal.SIGKILL)
                    except ProcessLookupError:
                        pass
        cleanup(process, child)


@test("installer and upgrade deliver loop assets while preserving manual disabled defaults")
def delivery(f):
    template = f.root / "template"
    shutil.copytree(SOURCE, template, ignore=shutil.ignore_patterns(
        ".git", ".factory", "node_modules", "__pycache__", ".venv"))
    for relative in ("scripts/selftest/run.sh", "scripts/prereq-check.sh"):
        stub = template / relative
        stub.write_text("#!/bin/bash\nexit 0\n")
        stub.chmod(0o755)
    (f.bin / "npm").write_text("#!/bin/bash\nexit 0\n")
    (f.bin / "npm").chmod(0o755)
    target = f.root / "adopter"
    target.mkdir()
    environment = dict(f.environment)
    environment.pop("FACTORY_CONFIG")
    (target / "README.md").write_text("Adopter-owned repository.\n")
    for args in (["init", "-q"], ["config", "user.email", "fixture@example.invalid"],
                 ["config", "user.name", "Loop Fixture"], ["add", "README.md"], ["commit", "-qm", "fixture"]):
        result = subprocess.run(["git"] + args, cwd=target, env=environment,
                                capture_output=True, text=True, timeout=10)
        require(result.returncode == 0, "adopter git setup failed: " + result.stderr)
    answers = "\n".join(["Loop", "loop", "@example", "example", "", "", "", "inherit", "standard", "n", "y", ""])
    result = subprocess.run(["bash", str(template / "scripts/factory-init.sh"), str(target), "--pack", "none"],
                            cwd=f.root, env=environment, input=answers, capture_output=True, text=True, timeout=60)
    require(result.returncode == 0, "installer failed: " + result.stdout + result.stderr)
    assets = ("scripts/factory-loop.sh", "scripts/lib/loop.py", "scripts/lib/budget-config.sh", "docs/LOOPS.md")
    for relative in assets:
        require((target / relative).is_file(), "installer missing delivery asset " + relative)
        require((target / relative).read_bytes() == (template / relative).read_bytes(), "changed delivery asset " + relative)
    config = (target / "factory.yaml").read_bytes()
    command = [str(target / "factory"), "loop", "plan", "--harness", "claude", "--session", "installed",
               "--task", "local", "--json"]
    result = subprocess.run(command, cwd=target, env=environment, capture_output=True, text=True, timeout=20)
    require("manual" in json.dumps(objects(result)[-1]), "installer did not default to manual")
    require(b"loop_enabled: false" in config, "installer enabled bounded runs")
    for workflow in (target / ".github/workflows").glob("*.yml"):
        require("scripts/selftest/loop.sh" not in workflow.read_text() or (target / "scripts/selftest/loop.sh").is_file(),
                "installed CI references absent template fixture")
    for relative in assets:
        (target / relative).unlink()
    result = subprocess.run([str(target / "factory"), "upgrade", "--source", str(template)],
                            cwd=target, env=dict(environment, FACTORY_UPGRADE_ACTIVE="1"),
                            stdin=subprocess.DEVNULL, capture_output=True, text=True, timeout=60)
    require(result.returncode == 0, "upgrade failed: " + result.stdout + result.stderr)
    require((target / "factory.yaml").read_bytes() == config, "upgrade changed user policy")
    for relative in assets:
        require((target / relative).is_file(), "upgrade omitted " + relative + ": " + result.stdout + result.stderr)
        require((target / relative).read_bytes() == (template / relative).read_bytes(), "upgrade changed " + relative)
    require(not f.invoked(), "delivery validation called a model")


def main():
    selected = os.environ.get("LOOP_TEST_FILTER", "")
    checks = [(name, function) for name, function in CHECKS if not selected or selected in function.__name__]
    failures = []
    with tempfile.TemporaryDirectory(prefix="factory-loop-acceptance-") as parent:
        for index, (name, function) in enumerate(checks):
            try:
                function(Fixture(Path(parent), str(index)))
                print("  ok: " + name, flush=True)
            except Exception as error:
                failures.append(name)
                print("  FAIL: " + name + ": " + str(error), flush=True)
    print("loop: {} passed, {} failed".format(len(checks) - len(failures), len(failures)), flush=True)
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main())
