"""Subprocess acceptance for docs/BUDGETS.md:121; no real CLI invocation."""
import json
from concurrent.futures import ThreadPoolExecutor
from contextlib import contextmanager, ExitStack, redirect_stderr
import importlib.util
import io
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
       if not key.startswith(("FACTORY_", "GIT_", "CODEX_", "CLAUDE_", "OPENCODE_"))
       and key not in ("MODEL_PROVIDER", "COST_PROFILE")}
ENV.update(GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_GLOBAL=os.devnull,
           PYTHONDONTWRITEBYTECODE="1")
CHECKS = []


def test(name):
    def register(function):
        CHECKS.append((name, function))
        return function
    return register


def require(condition, message):
    if not condition:
        raise AssertionError(message)


def json_lines(process):
    objects = []
    for line in process.stdout.splitlines():
        try:
            objects.append(json.loads(line))
        except ValueError:
            pass
    require(objects, "no JSON result: " + process.stdout + process.stderr)
    return objects


class Fixture:
    def __init__(self, directory, name):
        self.root = directory / name
        self.root.mkdir()
        for relative in ("factory", "scripts/factory-budget.sh", "scripts/lib",
                         ".opencode/agent", ".claude/agents", ".codex/agents", "opencode.json",
                         "scripts/hooks/test-edit-denial.sh"):
            origin, target = SOURCE / relative, self.root / relative
            target.parent.mkdir(parents=True, exist_ok=True)
            if origin.is_dir():
                shutil.copytree(origin, target)
            elif origin.is_file():
                shutil.copy2(origin, target)
        self.bin = self.root / "fixture-bin"
        self.bin.mkdir()
        for name in ("codex", "claude", "opencode"):
            shutil.copy2(SOURCE / "scripts/selftest/budget/fake_cli.py", self.bin / name)
            (self.bin / name).chmod(0o755)
        self.calls = self.root / "fixture-calls.jsonl"
        self.child = self.root / "fixture-child.pid"
        self.prompt = self.root / "fixture-prompt.txt"
        self.prompt.write_text("private-prompt-sentinel $(touch should-never-exist)\n")
        self.environment = dict(ENV, PATH=str(self.bin) + os.pathsep + ENV["PATH"],
                                FACTORY_CONFIG=str(self.root / "factory.yaml"),
                                BUDGET_FIXTURE_CALLS=str(self.calls),
                                BUDGET_FIXTURE_CHILD=str(self.child))
        self.settings = {}
        self.config(budget_enabled="true", budget_max_attempts="8",
                    budget_max_session_runs="15")

    def config(self, **settings):
        self.settings.update(settings)
        (self.root / "factory.yaml").write_text("".join(
            key + ': "' + str(value) + '"\n' for key, value in self.settings.items()))

    def command(self, verb="run", harness="claude", session="session", task="task", extra=()):
        argv = [str(self.root / "factory"), "budget", verb]
        if verb != "report":
            argv += ["--harness", harness, "--session", session, "--task", task]
        else:
            argv += ["--session", session]
        if verb == "run":
            argv += ["--prompt-file", str(self.prompt)]
        return argv + ["--json"] + list(extra)

    def run(self, verb="run", mode="normal", expected=0, **options):
        result = subprocess.run(self.command(verb, **options), cwd=self.root,
                                env=dict(self.environment, BUDGET_FIXTURE_MODE=mode),
                                capture_output=True, text=True, timeout=20)
        if expected is None:
            require(result.returncode != 0, "unexpected success: " + result.stdout)
        else:
            require(result.returncode == expected,
                    "exit {} expected {}: {}{}".format(result.returncode, expected,
                                                       result.stdout, result.stderr))
        return result

    def start(self, **options):
        return subprocess.Popen(self.command(**options), cwd=self.root,
                                env=dict(self.environment, BUDGET_FIXTURE_MODE="hold"),
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)

    def history(self):
        return json.loads((self.root / ".factory/budget.json").read_text())["runs"]

    def invoked(self):
        return [json.loads(line) for line in self.calls.read_text().splitlines()] if self.calls.exists() else []

    def wait_for_child(self):
        until = time.monotonic() + 8
        while time.monotonic() < until:
            if self.child.exists():
                return int(self.child.read_text())
            time.sleep(0.03)
        raise AssertionError("fake invocation never reached the child process")


def stopped(pid):
    result = subprocess.run(["ps", "-o", "stat=", "-p", str(pid)], capture_output=True, text=True)
    return result.returncode != 0 or not result.stdout.strip() or result.stdout.strip().startswith("Z")


def cleanup(process, child=None):
    if process.poll() is None:
        process.send_signal(signal.SIGTERM)
    try:
        process.communicate(timeout=5)
    except subprocess.TimeoutExpired:
        process.kill()
        process.communicate()
    if child and not stopped(child):
        os.kill(child, signal.SIGKILL)


@contextmanager
def runtime_modules(f):
    """Load the fixture's runtime for deterministic OS-failure injection."""
    modules = []
    for name in ("budget_adapters", "budget"):
        spec = importlib.util.spec_from_file_location(name, f.root / "scripts/lib" / (name + ".py"))
        module = importlib.util.module_from_spec(spec)
        with mock.patch.dict(sys.modules, {"budget_adapters": modules[0]} if modules else {}):
            spec.loader.exec_module(module)
        modules.append(module)
    yield modules[1], modules[0]


@contextmanager
def unreapable_process(module):
    """An owned process can remain unreapable even after its group is killed."""
    process = SimpleNamespace(pid=424242, poll=lambda: 0)
    process.stdin, process.stdout, process.stderr = [tempfile.TemporaryFile() for _ in range(3)]
    waits = []

    def wait(timeout=None):
        waits.append(timeout)
        raise subprocess.TimeoutExpired("fixture-owned-process", timeout)

    process.wait = wait
    selector = mock.MagicMock()
    selector.__enter__.return_value = selector
    selector.get_map.return_value = {}
    with ExitStack() as stack:
        stack.enter_context(mock.patch.object(module.subprocess, "Popen", return_value=process))
        stack.enter_context(mock.patch.object(module.selectors, "DefaultSelector", return_value=selector))
        stack.enter_context(mock.patch.object(module.os, "killpg"))
        try:
            yield process, waits, selector
        finally:
            for stream in (process.stdin, process.stdout, process.stderr):
                stream.close()


def bounded_waits(waits):
    require(waits and all(isinstance(timeout, (int, float)) and 0 < timeout <= 5 for timeout in waits),
            "owned-process cleanup performed an unbounded wait: " + repr(waits))


def restored_execution_state(process, handlers):
    require(all(stream.closed for stream in (process.stdin, process.stdout, process.stderr)),
            "reaping timeout skipped pipe closure")
    require(all(signal.getsignal(signum) == handler for signum, handler in handlers.items()),
            "reaping timeout left supervisor signal handlers installed")


@test("unconfirmed harness exit remains timed out and blocks admission with its reservation")
def unreaped_run(f):
    with runtime_modules(f) as (budget, adapters):
        config = dict(enabled=True, action="stop", max_attempts=8, max_session_runs=15,
                      timeout_seconds=30, session_seconds=90, max_concurrent=3, estimated_usd=None)
        args = SimpleNamespace(harness="claude", role="implementer", session="session", task="task",
                               max_cost_usd=None, prompt_file=str(f.prompt), json=True)
        handlers = {signum: signal.getsignal(signum) for signum in (signal.SIGINT, signal.SIGTERM, signal.SIGHUP)}
        ledger = budget.Ledger(f.root)
        with unreapable_process(budget) as (process, waits, _selector), \
                mock.patch.object(adapters, "preflight"), mock.patch.object(budget, "emit"):
            try:
                result = budget.run(args, config, ledger, f.root)
            finally:
                restored_execution_state(process, handlers)
            bounded_waits(waits)
            require(result == 124, "unconfirmed process exit did not return timeout")
        record = ledger.read()["runs"][-1]
        require(record["status"] == "active" and record["outcome"] == "timeout" and
                record["exit_code"] is None and record["ended_at"] is None,
                "unconfirmed exit was marked completed: " + repr(record))
        require(record["estimated_usd"] is None and record["tokens"] is None and not record["complete"],
                "unconfirmed exit claimed complete usage")
        require(record["reserved_seconds"] == 30 and record["process_pid"] == process.pid,
                "unconfirmed process lost its charged reservation or recovery identity")
        plan = budget.make_plan(args, config, ledger.read())
        require(plan["blockers"] and plan["remaining_session_seconds"] == 60,
                "unconfirmed process freed admission or reserved session time")


@test("cleanup reaping timeout preserves the original error and restores execution state")
def unreaped_error(f):
    with runtime_modules(f) as (budget, _adapters):
        handlers = {signum: signal.getsignal(signum) for signum in (signal.SIGINT, signal.SIGTERM, signal.SIGHUP)}
        original = ValueError("fixture admission callback failed")

        def fail_after_spawn(_pid):
            raise original

        with unreapable_process(budget) as (process, waits, _selector):
            caught = None
            try:
                budget.execute(["fixture"], "", f.root, {}, 30, fail_after_spawn)
            except Exception as error:
                caught = error
            finally:
                restored_execution_state(process, handlers)
            bounded_waits(waits)
            require(caught is original, "cleanup reaping timeout replaced the original execution error: " + repr(caught))


@test("spawn-ledger failure retains an unreaped process identity and blocks admission")
def unreaped_spawn_ledger(f):
    with runtime_modules(f) as (budget, adapters):
        config = dict(enabled=True, action="stop", max_attempts=8, max_session_runs=15,
                      timeout_seconds=30, session_seconds=90, max_concurrent=3, estimated_usd=None)
        args = SimpleNamespace(harness="claude", role="implementer", session="session", task="task",
                               max_cost_usd=None, prompt_file=str(f.prompt), json=True)
        ledger = budget.Ledger(f.root)
        locked = ledger.locked
        calls = []
        original_message = "fixture spawn-ledger write unavailable"

        @contextmanager
        def fail_spawn_lock():
            calls.append(True)
            if len(calls) == 2:
                raise budget.BudgetError(original_message)
            with locked():
                yield

        error_output = io.StringIO()
        handlers = {signum: signal.getsignal(signum) for signum in (signal.SIGINT, signal.SIGTERM, signal.SIGHUP)}
        with unreapable_process(budget) as (process, waits, _selector), \
                mock.patch.object(adapters, "preflight"), mock.patch.object(budget, "emit"), \
                mock.patch.object(ledger, "locked", side_effect=fail_spawn_lock), redirect_stderr(error_output):
            try:
                result = budget.run(args, config, ledger, f.root)
            finally:
                restored_execution_state(process, handlers)
            bounded_waits(waits)
        require(result != 0 and original_message in error_output.getvalue(),
                "spawn failure was hidden or replaced by cleanup failure")
        record = ledger.read()["runs"][-1]
        require(record["status"] == "active" and record["process_pid"] == process.pid and
                record["exit_code"] is None and record["ended_at"] is None,
                "spawn-ledger failure freed the still-unconfirmed process: " + repr(record))
        require(record["reserved_seconds"] == 30 and record["estimated_usd"] is None and
                record["tokens"] is None and not record["complete"],
                "spawn-ledger failure lost its reservation or claimed usage")
        plan = budget.make_plan(args, config, ledger.read())
        require(plan["blockers"] and plan["remaining_session_seconds"] == 60,
                "spawn-ledger failure admitted another run or freed reserved time")


@test("Codex hook probe cleanup is bounded and refuses launch when exit is unconfirmed")
def unreaped_probe(f):
    with runtime_modules(f) as (_budget, adapters):
        responses = [
            {"id": 2, "result": {"config": {"features": {"hooks": True}}}},
            {"id": 3, "result": {"data": [{"cwd": str(f.root), "errors": [], "hooks": [{
                "eventName": "preToolUse", "handlerType": "command", "command": adapters.CODEX_COMMAND,
                "matcher": adapters.CODEX_MATCHER, "enabled": True, "trustStatus": "trusted", "async": False}]}]}},
            {"id": 4, "result": {"requirements": None}},
        ]
        raw = b"".join((json.dumps(row) + "\n").encode() for row in responses)
        with unreapable_process(adapters) as (process, waits, selector), \
                mock.patch.object(adapters.os, "read", return_value=raw):
            selector.select.return_value = [True]
            caught = None
            try:
                adapters._codex_hook_preflight("fixture-codex", str(f.root), [])
            except Exception as error:
                caught = error
            bounded_waits(waits)
            require(process.stdin.closed and process.stdout.closed, "Codex probe timeout leaked pipes")
            require(isinstance(caught, ValueError), "unconfirmed Codex probe exit did not refuse launch: " + repr(caught))
            require(selector.close.called, "Codex probe timeout leaked its selector")


@test("plan and empty report are local and read-only")
def local(f):
    f.config(budget_enabled="false")
    plan = json_lines(f.run("plan", expected=2))[-1]
    require(plan["blockers"] and plan["model_source"] == "inherited", "disabled/inherited plan missing")
    report = json_lines(f.run("report"))[-1]
    require(report["totals"]["runs"] == 0, "empty history claims runs")
    require(not f.invoked() and not (f.root / ".factory").exists(), "read-only commands wrote state or invoked CLI")


@test("disabled feature and strict USD never launch")
def disabled(f):
    f.config(budget_enabled="false")
    f.run(expected=None)
    f.config(budget_enabled="true")
    for harness in ("codex", "claude", "opencode"):
        result = f.run(harness=harness, extra=("--max-cost-usd", "1"), expected=None)
        require("cost" in (result.stdout + result.stderr).lower(), "strict USD error is not actionable")
    require(not f.invoked(), "disabled/strict USD invoked a model")


@test("invalid limits, identifiers, and roles fail before launch")
def malformed(f):
    for key, value in (("budget_max_attempts", "0"), ("budget_timeout_seconds", "-1"),
                       ("budget_session_seconds", "nan"), ("budget_max_concurrent", "inf"),
                       ("budget_estimated_usd", "-0.1"), ("budget_enabled", "maybe"),
                       ("budget_action", "allow")):
        original = dict(f.settings)
        f.config(**{key: value})
        f.run(expected=None)
        f.settings = original
        f.config()
    f.run(session="../outside", expected=None)
    f.run(extra=("--role", "arbitrary"), expected=None)
    require(not f.invoked(), "invalid configuration reached a CLI")


@test("missing CLI, capabilities, roles, and prompt fail before launch")
def preflight(f):
    f.run(mode="unsupported", expected=None)
    role = f.root / ".opencode/agent/implementer.md"
    role.rename(role.with_suffix(".saved"))
    f.run(harness="codex", expected=None)
    role.with_suffix(".saved").rename(role)
    f.prompt.unlink()
    f.run(expected=None)
    require(not f.invoked(), "missing prerequisite reached a CLI")
    f.prompt.write_text("restored fixture prompt")
    # Remove every fallback PATH entry before removing the fake CLI: never
    # accidentally discover an installed, authenticated real Claude binary.
    for executable in ("bash", "python3", "git", "sed", "head", "tr", "dirname"):
        (f.bin / executable).symlink_to(shutil.which(executable))
    f.environment["PATH"] = str(f.bin)
    (f.bin / "claude").unlink()
    f.run(expected=None)
    require(not f.invoked(), "missing CLI reached a fallback executable")


@test("all three adapters preserve argv, role, model and report real fixture usage")
def native(f):
    model = "fixture/model;$(touch injection-marker)"
    f.config(codex_default_model=model, claude_default_model=model, opencode_default_model=model)
    for harness in ("codex", "claude", "opencode"):
        f.run(harness=harness, task=harness)
        call = f.invoked()[-1]
        require(call["name"] == harness and call["role"] == "implementer", "wrong harness or enforcement role")
        require(call["args"][call["args"].index("--model") + 1] == model, "model was split or rewritten")
        require("private-prompt-sentinel" in call["stdin"], "prompt was not delivered on stdin")
        require(not any("private-prompt-sentinel" in arg for arg in call["args"]), "prompt exposed in argv")
        require(not any(arg in call["args"] for arg in (
            "--dangerously-skip-permissions", "--dangerously-bypass-approvals-and-sandbox", "--yolo")),
            "adapter bypassed permission enforcement")
        if harness == "codex":
            require(call["args"][:2] == ["exec", "--json"] and
                    call["args"][call["args"].index("--sandbox") + 1] == "workspace-write",
                    "Codex native sandbox contract missing")
            require(Path(call["args"][call["args"].index("--cd") + 1]).resolve() == f.root.resolve() and
                    call["args"][-1] == "-", "Codex lost the owned cwd or stdin prompt marker")
            require(len(call["stdin"]) > len(f.prompt.read_text()), "canonical role instructions not delivered to Codex")
        else:
            expected = (["-p", "--output-format", "json", "--agent", "implementer",
                         "--permission-mode", "acceptEdits", "--model", model] if harness == "claude"
                        else ["run", "--format", "json", "--agent", "implementer", "--model", model])
            require(call["args"] == expected, harness + " native argv contract changed: " + repr(call["args"]))
        record = f.history()[-1]
        require(record["outcome"] == "completed" and record["tokens"], "valid usage missing: " + repr(record))
        if harness == "codex":
            require(record["estimated_usd"] is None, "Codex invented a cost")
        else:
            require(abs(record["estimated_usd"] - 0.25) < 1e-9, "incorrect cost aggregation")
    require(not (f.root / "injection-marker").exists(), "model string executed as shell code")
    totals = json_lines(f.run("report"))[-1]["totals"]
    require(totals["runs"] == 3 and totals["unknown_cost_runs"] == 1 and not totals["cost_complete"],
            "mixed known/unknown total was presented as complete")
    require(abs(totals["known_estimated_usd"] - 0.5) < 1e-9, "known subtotal wrong")


@test("role-tier model resolution honors the economy profile without lowering reviewers")
def roles(f):
    f.config(cost_profile="economy", claude_default_model="default-fixture",
             claude_economy_model="economy-fixture", claude_frontier_model="frontier-fixture")
    for role, model in (("refactorer", "economy-fixture"), ("reviewer", "frontier-fixture")):
        f.run(task=role, extra=("--role", role))
        call = f.invoked()[-1]
        require(call["role"] == role and call["args"][call["args"].index("--model") + 1] == model,
                "role tier resolution weakened the quality policy")


@test("Codex enforces native hook trust and preserves a read-only reviewer sandbox")
def codex_permissions(f):
    for mode in ("hook_untrusted", "hooks_disabled", "hooks_managed_off"):
        f.run(harness="codex", mode=mode, expected=None)
    require(not f.invoked(), "untrusted or disabled Codex hook permitted an implementer launch")
    f.run(harness="codex", extra=("--role", "reviewer"))
    args = f.invoked()[-1]["args"]
    require(args[args.index("--sandbox") + 1] == "read-only", "reviewer received write access")
    f.run(harness="codex", task="trusted")
    args = f.invoked()[-1]["args"]
    require("hooks.PreToolUse=" in args[args.index("-c") + 1] and
            "FACTORY_AGENT_ROLE=implementer" in args[args.index("-c") + 1],
            "implementer native hook did not inject the enforcement role")


@test("Claude reviewer retains the canonical plan permission mode")
def claude_permissions(f):
    f.run(harness="claude", extra=("--role", "reviewer"))
    require(f.invoked()[-1]["args"] == ["-p", "--output-format", "json", "--agent", "reviewer",
                                      "--permission-mode", "plan"],
            "Claude reviewer received broader permissions than its generated plan role")


@test("OpenCode duplicate step events do not double-count usage")
def duplicates(f):
    f.run(harness="opencode", mode="duplicate")
    record = f.history()[-1]
    require(abs(record["estimated_usd"] - 0.25) < 1e-9, "duplicate incremental events were billed twice")
    require(record["tokens"]["input_tokens"] == 100, "duplicate incremental tokens counted twice")


@test("OpenCode role overlay preserves existing project and tool permissions")
def overlay(f):
    original = {"agent": {"implementer": {"permission": {"bash": "ask"}},
                           "custom": {"mode": "subagent"}}, "permission": {"webfetch": "deny"}}
    f.environment["OPENCODE_CONFIG_CONTENT"] = json.dumps(original)
    original_file = (f.root / "opencode.json").read_bytes()
    f.run(harness="opencode")
    result = json.loads(f.invoked()[-1]["opencode_config"])
    require(result["agent"]["implementer"]["mode"] == "all", "native run would fall back from subagent role")
    result["agent"]["implementer"].pop("mode")
    require(result == original, "role activation changed unrelated native permissions")
    require((f.root / "opencode.json").read_bytes() == original_file, "run rewrote native project config")
    f.environment["OPENCODE_CONFIG_CONTENT"] = "{broken"
    f.run(harness="opencode", task="invalid", expected=None)
    require(len(f.invoked()) == 1, "invalid native configuration was ignored")


@test("error events are failures even when the harness exits zero")
def errors(f):
    for harness in ("codex", "claude", "opencode"):
        f.run(harness=harness, task=harness, mode="error", expected=None)
        require(f.history()[-1]["outcome"] == "failed", "zero-exit error event counted as completion")


@test("malformed, incomplete and invalid numbers never become known costs")
def usage(f):
    f.config(budget_max_attempts="30", budget_max_session_runs="30")
    for harness in ("codex", "claude", "opencode"):
        for mode in ("empty", "malformed", "partial", "negative", "nonfinite"):
            result = subprocess.run(f.command(harness=harness), cwd=f.root,
                                    env=dict(f.environment, BUDGET_FIXTURE_MODE=mode),
                                    capture_output=True, text=True, timeout=20)
            require(result.returncode in (0, 1), "usage fixture was not launched: " + result.stderr)
            record = f.history()[-1]
            require(record["estimated_usd"] is None and not record["complete"],
                    harness + "/" + mode + " manufactured complete cost evidence")


@test("attempt and cross-task session-run limits are hard even in warn mode")
def attempts(f):
    f.config(budget_max_attempts="1", budget_max_session_runs="2", budget_action="warn")
    f.run()
    f.run(expected=None)
    f.run(task="second")
    f.run(task="third", expected=None)
    require(len(f.invoked()) == 2 and len(f.history()) == 2, "limit consumed extra invocation or free history")


@test("failed invocations consume attempts and are never retried automatically")
def failed_attempt(f):
    f.config(budget_max_attempts="1")
    f.run(mode="error", expected=None)
    f.run(expected=None)
    require(len(f.invoked()) == 1 and f.history()[0]["attempt"] == 1, "failure retried or escaped attempt accounting")


@test("simultaneous admissions reserve the last session launch atomically")
def racing(f):
    f.config(budget_max_concurrent="2", budget_max_session_runs="1")
    def launch(task):
        return subprocess.run(f.command(task=task), cwd=f.root, env=f.environment,
                              capture_output=True, text=True, timeout=20)
    with ThreadPoolExecutor(max_workers=2) as executor:
        results = list(executor.map(launch, ("left", "right")))
    require(sorted(result.returncode for result in results) == [0, 2],
            "concurrent last-slot admission did not produce one run and one refusal")
    require(len(f.invoked()) == 1 and len(f.history()) == 1, "atomic launch reservation admitted two calls")


@test("estimated USD stop and warn remain distinct from a strict financial ceiling")
def estimates(f):
    f.config(budget_estimated_usd="0.2", budget_action="stop")
    f.run(harness="codex", expected=None)
    f.run()
    result = f.run(task="second", expected=None)
    require("estimat" in (result.stdout + result.stderr).lower(), "estimate stop lacked an explanation")
    f.config(budget_action="warn")
    result = f.run(task="third")
    require("warn" in (result.stdout + result.stderr).lower(), "warn threshold silently ignored")
    require(len(f.invoked()) == 2, "estimate admission did not follow stop/warn policy")


@test("unknown previous cost blocks estimate-stop admission")
def unknown_cost(f):
    f.run(harness="codex")
    f.config(budget_estimated_usd="1", budget_action="stop")
    f.run(task="second", expected=None)
    require(len(f.invoked()) == 1, "unknown previous cost treated as zero")


@test("private metadata excludes prompts, responses, errors and unrelated secrets")
def privacy(f):
    f.environment["UNRELATED_API_KEY"] = "private-secret-sentinel"
    f.run()
    f.run(task="error", mode="error", expected=None)
    for path in (f.root / ".factory").rglob("*"):
        if path.is_file():
            content = path.read_text()
            require(not any(value in content for value in (
                "private-prompt-sentinel", "private-response-sentinel", "private-error-sentinel",
                "private-secret-sentinel")), "private payload leaked into " + str(path))
            require(stat.S_IMODE(path.stat().st_mode) & 0o077 == 0, "metadata file readable by other users")
    require(stat.S_IMODE((f.root / ".factory").stat().st_mode) & 0o077 == 0,
            "metadata directory has public permissions")


@test("human run output shows the final answer while JSON and ledger stay metadata-only")
def final_output(f):
    for harness in ("codex", "claude", "opencode"):
        command = f.command(harness=harness, task=harness)
        command.remove("--json")
        result = subprocess.run(command, cwd=f.root, env=f.environment,
                                capture_output=True, text=True, timeout=20)
        require(result.returncode == 0 and "private-response-sentinel" in result.stdout,
                harness + " human run omitted final answer: " + result.stdout + result.stderr)
    report = f.run("report")
    require("private-response-sentinel" not in report.stdout + (f.root / ".factory/budget.json").read_text(),
            "human response persisted into metadata")
    result = f.run(task="json")
    require("private-response-sentinel" not in result.stdout, "JSON output mixed response text into metadata")


@test("corrupt or unknown-schema history is preserved and blocks launches")
def corrupt(f):
    directory = f.root / ".factory"
    directory.mkdir(mode=0o700)
    ledger = directory / "budget.json"
    for contents in ("{broken", '{"schema":999,"runs":[]}', '{"schema":1,"runs":[{}]}'):
        ledger.write_text(contents)
        ledger.chmod(0o600)
        f.run(expected=None)
        require(ledger.read_text() == contents, "corruption silently reset")
    require(not f.invoked(), "corrupt ledger reached a CLI")


@test("symlinked storage is refused without writing through it")
def symlinks(f):
    outside = f.root / "outside"
    outside.mkdir()
    (f.root / ".factory").symlink_to(outside, target_is_directory=True)
    f.run(expected=None)
    require(not list(outside.iterdir()) and not f.invoked(), "symlink storage used")
    (f.root / ".factory").unlink()
    (f.root / ".factory").mkdir(mode=0o700)
    target = outside / "untouched.json"
    target.write_text('{"schema":1,"runs":[]}')
    (f.root / ".factory/budget.json").symlink_to(target)
    f.run(expected=None)
    require(target.read_text() == '{"schema":1,"runs":[]}' and not f.invoked(), "symlink ledger used")


@test("timeout terminates descendants, charges an attempt and records unknown cost")
def timeout(f):
    f.config(budget_timeout_seconds="1", budget_session_seconds="1")
    started = time.monotonic()
    f.run(mode="hold", expected=124)
    child = int(f.child.read_text())
    try:
        require(time.monotonic() - started < 8 and stopped(child), "timeout left a child process alive")
        record = f.history()[-1]
        require(record["outcome"] == "timeout" and record["estimated_usd"] is None,
                "timeout recorded fabricated completion/cost")
        f.run(task="second", expected=None)
        require(len(f.invoked()) == 1, "spent session time admitted another run")
    finally:
        if not stopped(child):
            os.kill(child, signal.SIGKILL)


@test("termination signal cleans owned process group and records interruption")
def interrupted(f):
    process = f.start()
    child = None
    try:
        child = f.wait_for_child()
        process.send_signal(signal.SIGTERM)
        process.communicate(timeout=8)
        require(process.returncode != 0 and stopped(child), "signal left an owned process running")
        record = f.history()[-1]
        require(record["outcome"] == "interrupted" and record["estimated_usd"] is None,
                "interrupted invocation disappeared from budget history")
    finally:
        cleanup(process, child)


@test("checkout concurrency limit covers different named sessions")
def concurrent(f):
    f.config(budget_max_concurrent="1", budget_timeout_seconds="5")
    process = f.start()
    child = None
    try:
        child = f.wait_for_child()
        f.run(session="other", expected=None)
        require(len(f.invoked()) == 1, "second session bypassed checkout concurrency")
    finally:
        cleanup(process, child)


@test("active reservations prevent parallel overspending of session wall time")
def reservation(f):
    f.config(budget_max_concurrent="2", budget_timeout_seconds="5", budget_session_seconds="5")
    process = f.start()
    child = None
    try:
        child = f.wait_for_child()
        plan = json_lines(f.run("plan", task="other", expected=2))[-1]
        require(plan["remaining_session_seconds"] <= 0, "active run time was not reserved")
        f.run(task="other", expected=None)
        require(len(f.invoked()) == 1, "session admitted two full time allowances")
    finally:
        cleanup(process, child)


@test("active unknown-cost invocation blocks concurrent estimate-stop admission")
def pending_cost(f):
    f.config(budget_max_concurrent="2", budget_estimated_usd="1")
    process = f.start()
    child = None
    try:
        child = f.wait_for_child()
        f.run(task="other", expected=None)
        require(len(f.invoked()) == 1, "unfinished cost was silently treated as zero")
    finally:
        cleanup(process, child)


@test("unclean supervisor death remains a charged fail-closed reservation")
def orphan(f):
    process = f.start()
    child = None
    owned = None
    try:
        child = f.wait_for_child()
        owned = f.history()[-1]["process_pid"]
        process.kill()
        process.wait(timeout=5)
        f.run(session="other", expected=None)
        require(len(f.invoked()) == 1 and f.history()[0]["status"] == "active",
                "stale active reservation became a free attempt")
    finally:
        if owned:
            try:
                os.killpg(owned, signal.SIGKILL)
            except ProcessLookupError:
                pass
        cleanup(process, child)


@test("installer and upgrade deliver working local budget commands without enabling paid runs")
def delivery(f):
    # Real installer/upgrade artifact flow. Ancillary npm/prerequisite/selftest
    # work is stubbed here; gate correctness is exercised by the ordinary suite.
    template = f.root / "template"
    shutil.copytree(SOURCE, template, ignore=shutil.ignore_patterns(
        ".git", ".factory", "node_modules", "__pycache__", ".venv"))
    for relative in ("scripts/selftest/run.sh", "scripts/prereq-check.sh"):
        stub = template / relative
        stub.write_text("#!/bin/bash\nexit 0\n")
        stub.chmod(0o755)
    npm = f.bin / "npm"
    npm.write_text("#!/bin/bash\nexit 0\n")
    npm.chmod(0o755)
    target = f.root / "adopter"
    target.mkdir()
    (target / "README.md").write_text("Adopter-owned content.\n")
    environment = dict(f.environment)
    environment.pop("FACTORY_CONFIG")
    answers = "\n".join(["Budget", "budget", "@example", "example", "", "", "",
                           "inherit", "standard", "n", "y", ""])
    result = subprocess.run(["bash", str(template / "scripts/factory-init.sh"), str(target),
                             "--pack", "none"], cwd=f.root, env=environment, input=answers,
                            capture_output=True, text=True, timeout=60)
    require(result.returncode == 0, "installer failed: " + result.stdout + result.stderr)
    require((target / "README.md").read_text() == "Adopter-owned content.\n", "installer rewrote adopter content")
    for workflow in (target / ".github/workflows").glob("*.yml"):
        require("scripts/selftest/budget.sh" not in workflow.read_text() or
                (target / "scripts/selftest/budget.sh").is_file(),
                "installed CI invokes missing template-only budget.sh: " + workflow.name)
    assets = ("scripts/factory-budget.sh", "scripts/lib/budget.py", "scripts/lib/budget_adapters.py",
              "docs/BUDGETS.md")
    for relative in assets:
        require((target / relative).read_bytes() == (template / relative).read_bytes(),
                "installer omitted/changed budget asset: " + relative)
    command = [str(target / "factory"), "budget", "plan", "--harness", "claude",
               "--session", "installed", "--task", "local", "--json"]
    result = subprocess.run(command, cwd=target, env=environment, capture_output=True, text=True, timeout=20)
    plan = json_lines(result)[-1]
    require(not plan["configuration"]["enabled"] and plan["blockers"], "installation enabled paid runs")
    require(".factory/" in (target / ".gitignore").read_text(), "installed ledger is not gitignored")
    original_config = (target / "factory.yaml").read_bytes()
    # Missing framework files must be repaired, preserving adopter choices.
    for relative in assets:
        (target / relative).unlink()
    result = subprocess.run([str(target / "factory"), "upgrade", "--source", str(template)],
                            cwd=target, env=dict(environment, FACTORY_UPGRADE_ACTIVE="1"),
                            stdin=subprocess.DEVNULL, capture_output=True, text=True, timeout=60)
    require(result.returncode == 0, "upgrade failed: " + result.stdout + result.stderr)
    require((target / "factory.yaml").read_bytes() == original_config, "upgrade rewrote budget preferences")
    for relative in assets:
        require((target / relative).read_bytes() == (template / relative).read_bytes(),
                "upgrade did not repair budget asset: " + relative)
    result = subprocess.run(command, cwd=target, env=environment, capture_output=True, text=True, timeout=20)
    require(json_lines(result)[-1]["blockers"] and not f.invoked(), "upgraded plan launched a model")


def main():
    failures = []
    with tempfile.TemporaryDirectory(prefix="factory-budget-acceptance-") as directory:
        for index, (name, function) in enumerate(CHECKS):
            try:
                function(Fixture(Path(directory), str(index)))
                print("  ok: " + name, flush=True)
            except Exception as error:
                failures.append(name)
                print("  FAIL: " + name + ": " + str(error), flush=True)
    print("budget: {} passed, {} failed".format(len(CHECKS) - len(failures), len(failures)))
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main())
