#!/usr/bin/env python3
"""Local admission control and metadata for explicit factory invocations.

docs/BUDGETS.md:5 defines the execution boundary and accounting limitations.
Only the explicit run path launches a harness; plans and reports are read-only.
"""

import argparse
import contextlib
import datetime
import fcntl
import json
import math
import os
from pathlib import Path
import re
import selectors
import signal
import stat
import subprocess
import sys
import time
import uuid

import budget_adapters


SCHEMA = 1
ROLES = ("spec-writer", "implementer", "refactorer", "reviewer", "wiki-maintainer")
HARNESS = ("codex", "claude", "opencode")
ID_PATTERN = re.compile(r"[A-Za-z0-9][A-Za-z0-9_-]{0,63}\Z")
OUTPUT_LIMIT = 16 * 1024 * 1024
HISTORY_LIMIT = 32 * 1024 * 1024


class BudgetError(Exception):
    pass


def now():
    return datetime.datetime.now(datetime.timezone.utc).isoformat()


def number(value, label, integer=False, zero=False):
    try:
        if isinstance(value, bool):
            raise ValueError()
        if integer and not re.fullmatch(r"[0-9]+", str(value)):
            raise ValueError()
        result = int(value) if integer else float(value)
        if not math.isfinite(result) or result < 0 or (not zero and result == 0):
            raise ValueError()
        return result
    except (ValueError, TypeError, OverflowError):
        raise BudgetError("{} must be a {}finite {}number".format(
            label, "nonnegative " if zero else "positive ", "integer " if integer else ""))


def configuration():
    def get(key, default):
        return os.environ.get("FACTORY_BUDGET_" + key, default)

    enabled = get("ENABLED", "false")
    if enabled not in ("true", "false"):
        raise BudgetError("budget_enabled must be true or false")
    action = get("ACTION", "stop")
    if action not in ("stop", "warn"):
        raise BudgetError("budget_action must be stop or warn")
    result = {"enabled": enabled == "true", "action": action}
    for key, default, integer in (
        ("max_attempts", "1", True), ("max_session_runs", "5", True),
        ("timeout_seconds", "300", False), ("session_seconds", "900", False),
        ("max_concurrent", "1", True),
    ):
        result[key] = number(get(key.upper(), default), "budget_" + key, integer)
    threshold = get("ESTIMATED_USD", "")
    result["estimated_usd"] = (number(threshold, "budget_estimated_usd")
                               if threshold != "" else None)
    return result


def safe_path(path, directory=False):
    """Never follow storage symlinks, including broken links."""
    try:
        info = path.lstat()
    except FileNotFoundError:
        return False
    expected = stat.S_ISDIR(info.st_mode) if directory else stat.S_ISREG(info.st_mode)
    if stat.S_ISLNK(info.st_mode) or not expected:
        raise BudgetError("unsafe budget storage: {} must be a {} (no symlinks)".format(
            path, "directory" if directory else "regular file"))
    if not directory and info.st_nlink != 1:
        raise BudgetError("unsafe budget storage: hard-linked file {}".format(path))
    return True


def validate_history(data):
    if not isinstance(data, dict) or type(data.get("schema")) is not int or data.get("schema") != SCHEMA:
        raise BudgetError("unsupported or malformed budget history schema; preserve and inspect .factory/budget.json")
    runs = data.get("runs")
    if not isinstance(runs, list):
        raise BudgetError("malformed budget history: runs must be an array")
    seen = set()
    for run in runs:
        if not isinstance(run, dict):
            raise BudgetError("malformed budget history: run must be an object")
        for key in ("id", "session", "task"):
            if not isinstance(run.get(key), str) or not ID_PATTERN.fullmatch(run[key]):
                raise BudgetError("malformed budget history: invalid " + key)
        if run["id"] in seen:
            raise BudgetError("malformed budget history: duplicate run id")
        seen.add(run["id"])
        if run.get("harness") not in HARNESS or run.get("role") not in ROLES:
            raise BudgetError("malformed budget history: invalid harness or role")
        if run.get("status") not in ("active", "completed"):
            raise BudgetError("malformed budget history: invalid status")
        if run.get("outcome") not in ("running", "completed", "failed", "timeout", "interrupted", "output_limit", "launch_error"):
            raise BudgetError("malformed budget history: invalid outcome")
        for key in ("elapsed_seconds", "reserved_seconds"):
            if type(run.get(key)) not in (int, float):
                raise BudgetError("malformed budget history: invalid " + key)
            number(run.get(key), "history " + key, zero=True)
        for key in ("attempt", "owner_pid"):
            if type(run.get(key)) is not int:
                raise BudgetError("malformed budget history: invalid " + key)
            number(run.get(key), "history " + key, integer=True)
        if not isinstance(run.get("complete"), bool):
            raise BudgetError("malformed budget history: invalid completeness")
        if run.get("estimated_usd") is not None:
            if type(run["estimated_usd"]) not in (int, float):
                raise BudgetError("malformed budget history: invalid estimated_usd")
            number(run["estimated_usd"], "history estimated_usd", zero=True)
        if not isinstance(run.get("warnings"), list) or not all(isinstance(w, str) for w in run["warnings"]):
            raise BudgetError("malformed budget history: invalid warnings")
        if run.get("tokens") is not None:
            if not isinstance(run["tokens"], dict):
                raise BudgetError("malformed budget history: invalid tokens")
            for value in run["tokens"].values():
                if type(value) is not int:
                    raise BudgetError("malformed budget history: invalid token count")
                number(value, "history tokens", integer=True, zero=True)
        if run.get("process_pid") is not None:
            if type(run["process_pid"]) is not int:
                raise BudgetError("malformed budget history: invalid process_pid")
            number(run["process_pid"], "history process_pid", integer=True)
        if run.get("exit_code") is not None and type(run["exit_code"]) is not int:
            raise BudgetError("malformed budget history: invalid exit_code")
        for key in ("model", "started_at", "source"):
            if not isinstance(run.get(key), str):
                raise BudgetError("malformed budget history: invalid " + key)
        if run["status"] == "active":
            if run["outcome"] not in ("running", "timeout", "launch_error") or run.get("ended_at") is not None:
                raise BudgetError("malformed budget history: inconsistent active run")
            if run["outcome"] != "running" and (
                    run.get("process_pid") is None or run.get("exit_code") is not None
                    or run["complete"] or run["estimated_usd"] is not None or run["tokens"] is not None):
                raise BudgetError("malformed budget history: inconsistent unconfirmed exit")
        elif not isinstance(run.get("ended_at"), str) or run["outcome"] == "running":
            raise BudgetError("malformed budget history: inconsistent completed run")
    return data


class Ledger:
    def __init__(self, root):
        self.directory = root / ".factory"
        self.path = self.directory / "budget.json"
        self.lock_path = self.directory / "budget.lock"

    def check(self):
        if safe_path(self.directory, directory=True):
            safe_path(self.path)
            safe_path(self.lock_path)

    def read(self):
        self.check()
        if not self.path.exists():
            return {"schema": SCHEMA, "runs": []}
        descriptor = os.open(str(self.path), os.O_RDONLY | os.O_NOFOLLOW)
        with os.fdopen(descriptor, "r", encoding="utf-8") as handle:
            if os.fstat(handle.fileno()).st_size > HISTORY_LIMIT:
                raise BudgetError("budget history exceeds 32 MiB; archive completed sessions before continuing")
            try:
                data = json.load(handle)
            except (ValueError, UnicodeError) as exc:
                raise BudgetError("malformed budget history; preserve and inspect .factory/budget.json") from exc
        return validate_history(data)

    @contextlib.contextmanager
    def locked(self):
        self.check()
        self.directory.mkdir(mode=0o700, exist_ok=True)
        self.check()
        os.chmod(str(self.directory), 0o700)
        descriptor = os.open(str(self.lock_path), os.O_RDWR | os.O_CREAT | os.O_NOFOLLOW, 0o600)
        try:
            os.fchmod(descriptor, 0o600)
            fcntl.flock(descriptor, fcntl.LOCK_EX)
            self.check()
            yield
        finally:
            os.close(descriptor)

    def write(self, data):
        validate_history(data)
        self.check()
        temp = self.directory / ("budget-" + uuid.uuid4().hex + ".tmp")
        descriptor = os.open(str(temp), os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
        try:
            with os.fdopen(descriptor, "w", encoding="utf-8") as handle:
                json.dump(data, handle, sort_keys=True, allow_nan=False)
                handle.write("\n")
                handle.flush()
                os.fsync(handle.fileno())
            os.replace(str(temp), str(self.path))
            directory_fd = os.open(str(self.directory), os.O_RDONLY)
            try:
                os.fsync(directory_fd)
            finally:
                os.close(directory_fd)
        finally:
            if temp.exists():
                temp.unlink()


def alive(pid):
    try:
        os.kill(pid, 0)
        return True
    except ProcessLookupError:
        return False
    except PermissionError:
        return True


def make_plan(args, config, history):
    runs = history["runs"]
    session = [r for r in runs if r["session"] == args.session]
    task = [r for r in session if r["task"] == args.task]
    active = [r for r in runs if r["status"] == "active"]
    charge = sum(r["reserved_seconds"] if r["status"] == "active" else r["elapsed_seconds"] for r in session)
    plan = {
        "configuration": config, "session": args.session, "task": args.task,
        "harness": args.harness, "role": args.role,
        "model": os.environ.get("FACTORY_BUDGET_MODEL", ""),
        "model_source": "selected" if os.environ.get("FACTORY_BUDGET_MODEL", "") else "inherited",
        "services": {"model_provider": "inherited; usage may incur provider charges",
                     "tools": "unknown; inherited from harness/project configuration"},
        "cost_reporting": "unknown" if args.harness == "codex" else "client-reported estimate, not billing",
        "remaining_attempts": max(0, config["max_attempts"] - len(task)),
        "remaining_session_runs": max(0, config["max_session_runs"] - len(session)),
        "remaining_session_seconds": max(0, config["session_seconds"] - charge),
        "active_runs": len(active), "blockers": [], "warnings": [],
    }
    blockers = plan["blockers"]
    if not config["enabled"]:
        blockers.append("budget feature is disabled; set budget_enabled: true to permit explicit runs")
    if args.max_cost_usd is not None:
        number(args.max_cost_usd, "--max-cost-usd")
        blockers.append("strict USD ceilings are unsupported for all harnesses; estimates are not billing limits")
    for record in active:
        if record["outcome"] != "running":
            blockers.append("unconfirmed process exit for run {}: confirm its process group is stopped, then recover its metadata using docs/BUDGETS.md".format(record["id"]))
        elif not alive(record["owner_pid"]):
            blockers.append("stale active run {}: confirm its process group is stopped, then recover its metadata using docs/BUDGETS.md; do not delete the attempt".format(record["id"]))
    if plan["remaining_attempts"] == 0:
        blockers.append("task attempt limit reached")
    if plan["remaining_session_runs"] == 0:
        blockers.append("session run limit reached")
    if plan["remaining_session_seconds"] <= 0:
        blockers.append("session time limit reached")
    if len(active) >= config["max_concurrent"]:
        blockers.append("checkout concurrency limit reached")
    threshold = config["estimated_usd"]
    if threshold is not None:
        concerns = []
        if args.harness == "codex":
            concerns.append("Codex does not report a usable USD estimate")
        if any(r["status"] == "active" for r in session):
            concerns.append("another session invocation has not reported its cost")
        if any(r["status"] != "active" and r["estimated_usd"] is None for r in session):
            concerns.append("previous session cost is unknown")
        known = sum(r["estimated_usd"] for r in session if r["estimated_usd"] is not None)
        if known >= threshold:
            concerns.append("session estimated USD threshold reached ({:.6f} >= {:.6f})".format(known, threshold))
        (blockers if config["action"] == "stop" else plan["warnings"]).extend(concerns)
    return plan


def emit(value, json_output):
    if json_output:
        print(json.dumps(value, sort_keys=True, allow_nan=False), flush=True)
        return
    if "configuration" in value:
        print("Budget plan: {harness}, role {role}, session {session}, task {task}".format(**value))
        print("Model: {} ({})".format(value["model"] or "harness default", value["model_source"]))
        print("Remaining: {} task attempts; {} session runs; {:.3f}s session time; {} active runs".format(
            value["remaining_attempts"], value["remaining_session_runs"],
            value["remaining_session_seconds"], value["active_runs"]))
        print("Per-run timeout: {}s; checkout concurrency: {}".format(
            value["configuration"]["timeout_seconds"], value["configuration"]["max_concurrent"]))
        print("Cost reporting: " + value["cost_reporting"])
        print("Services: model provider inherited; tool services unknown (harness/project configuration)")
        for blocker in value["blockers"]:
            print("BLOCKED: " + blocker)
        for warning in value["warnings"]:
            print("WARNING: " + warning)
    elif "totals" in value:
        totals = value["totals"]
        print("Budget report: {} runs, {} active, {:.3f}s charged".format(
            totals["runs"], totals["active"], totals["elapsed_seconds"]))
        print("Known client-estimated USD: {:.6f}; unknown-cost runs: {}{}".format(
            totals["known_estimated_usd"], totals["unknown_cost_runs"],
            " (partial total)" if not totals["cost_complete"] else ""))
        for record in value["runs"]:
            print("{} {}/{} {} {} estimated USD {}".format(
                record["id"], record["session"], record["task"], record["harness"], record["outcome"],
                "unknown" if record["estimated_usd"] is None else record["estimated_usd"]))
    else:
        print("Run {}: {}; elapsed {:.3f}s; estimated USD {}".format(
            value["id"], value["outcome"], value["elapsed_seconds"],
            "unknown" if value["estimated_usd"] is None else value["estimated_usd"]))
        for warning in value["warnings"]:
            print("WARNING: " + warning)
    sys.stdout.flush()


def report(history, session=None):
    runs = [r for r in history["runs"] if session is None or r["session"] == session]
    unknown = sum(r["estimated_usd"] is None for r in runs)
    return {"schema": SCHEMA, "runs": runs, "totals": {
        "runs": len(runs), "active": sum(r["status"] == "active" for r in runs),
        "elapsed_seconds": sum(r["elapsed_seconds"] for r in runs),
        "reserved_seconds": sum(r["reserved_seconds"] for r in runs if r["status"] == "active"),
        "known_estimated_usd": sum(r["estimated_usd"] for r in runs if r["estimated_usd"] is not None),
        "unknown_cost_runs": unknown, "cost_complete": unknown == 0,
    }}


def parse_events(raw):
    try:
        text = raw.decode("utf-8")
    except UnicodeError:
        return [None]
    try:
        whole = json.loads(text)
        return [whole] if isinstance(whole, dict) else [None]
    except ValueError:
        events = []
        for line in text.splitlines():
            if line.strip():
                try:
                    events.append(json.loads(line))
                except ValueError:
                    events.append(None)
        return events or [None]


def terminate_group(proc):
    # Kill all descendants, including children holding stdout open after parent exit.
    try:
        os.killpg(proc.pid, signal.SIGKILL)
    except ProcessLookupError:
        pass


def execute(argv, stdin_text, root, overrides, allowance, on_spawn, deadline=None):
    start = time.monotonic()
    env = os.environ.copy()
    env.update(overrides)
    caught = []
    previous = {}
    proc = None
    output = bytearray()
    outcome = "completed"
    code = None

    def interrupted(signum, _frame):
        caught.append(signum)

    for signum in (signal.SIGINT, signal.SIGTERM, signal.SIGHUP):
        previous[signum] = signal.signal(signum, interrupted)
    try:
        if deadline is not None:
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                raise BudgetError("loop time limit reached before process launch")
            allowance = min(allowance, remaining)
            start = time.monotonic()
        proc = subprocess.Popen(argv, cwd=str(root), env=env, stdin=subprocess.PIPE,
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                start_new_session=True)
        on_spawn(proc.pid)
        pending = memoryview(stdin_text.encode("utf-8"))
        with selectors.DefaultSelector() as selector:
            for stream in (proc.stdout, proc.stderr):
                os.set_blocking(stream.fileno(), False)
                selector.register(stream, selectors.EVENT_READ)
            if pending:
                os.set_blocking(proc.stdin.fileno(), False)
                selector.register(proc.stdin, selectors.EVENT_WRITE)
            else:
                proc.stdin.close()
            while selector.get_map():
                if caught:
                    outcome = "interrupted"
                    break
                remaining = allowance - (time.monotonic() - start)
                if remaining <= 0:
                    outcome = "timeout"
                    break
                if proc.poll() is not None:
                    terminate_group(proc)
                for key, _mask in selector.select(min(remaining, 0.05)):
                    stream = key.fileobj
                    if stream is proc.stdin:
                        try:
                            written = os.write(stream.fileno(), pending[:65536])
                            pending = pending[written:]
                        except BrokenPipeError:
                            pending = memoryview(b"")
                        if not pending:
                            selector.unregister(stream)
                            stream.close()
                    else:
                        chunk = os.read(stream.fileno(), 65536)
                        if not chunk:
                            selector.unregister(stream)
                            stream.close()
                        elif stream is proc.stdout:
                            output.extend(chunk)
                            if len(output) > OUTPUT_LIMIT:
                                outcome = "output_limit"
                                break
                if outcome != "completed":
                    break
            # A CLI may close both output streams and keep running.
            while proc.poll() is None and outcome == "completed":
                if caught:
                    outcome = "interrupted"
                elif time.monotonic() - start >= allowance:
                    outcome = "timeout"
                else:
                    time.sleep(0.02)
    except OSError as exc:
        raise BudgetError("could not launch harness: {}".format(exc.strerror)) from exc
    finally:
        try:
            if proc is not None:
                try:
                    terminate_group(proc)
                    try:
                        code = proc.wait(timeout=5)
                    except subprocess.TimeoutExpired:
                        # Decision 46: docs/DECISION_LOG.md:1699. SIGKILL is a
                        # request, not proof of exit; keep admission reserved.
                        outcome = "timeout"
                finally:
                    for stream in (proc.stdin, proc.stdout, proc.stderr):
                        if stream is not None and not stream.closed:
                            try:
                                stream.close()
                            except OSError:
                                pass
        finally:
            for signum, handler in previous.items():
                signal.signal(signum, handler)
    if code != 0 and outcome == "completed":
        outcome = "failed"
    return outcome, code, time.monotonic() - start, bytes(output)


def run(args, config, ledger, root, prompt_text=None, response_callback=None, quiet=False, deadline=None):
    output = (lambda value, json_output: None) if quiet else emit
    # Read-only checks precede CLI checks and admission. Disabled commands do not
    # even invoke a harness --help or create storage.
    initial = make_plan(args, config, ledger.read())
    if initial["blockers"]:
        output(initial, args.json)
        return 2
    try:
        prompt = prompt_text if prompt_text is not None else Path(args.prompt_file).read_text(encoding="utf-8")
    except (OSError, UnicodeError) as exc:
        raise BudgetError("cannot read --prompt-file: {}".format(exc)) from exc
    budget_adapters.preflight(args.harness, args.role, root)
    overrides = budget_adapters.environment(args.harness, args.role)
    argv, stdin_text = budget_adapters.build_command(
        args.harness, args.role, initial["model"], root, prompt)
    if deadline is not None:
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            raise BudgetError("loop time limit reached during native preflight")
        config = dict(config, timeout_seconds=min(config["timeout_seconds"], remaining))
    with ledger.locked():
        if deadline is not None:
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                raise BudgetError("loop time limit reached while waiting for budget admission")
            config = dict(config, timeout_seconds=min(config["timeout_seconds"], remaining))
        history = ledger.read()
        plan = make_plan(args, config, history)
        output(plan, args.json)
        if plan["blockers"]:
            return 2
        record = {
            "id": uuid.uuid4().hex, "session": args.session, "task": args.task,
            "harness": args.harness, "role": args.role, "model": plan["model"],
            "started_at": now(), "ended_at": None, "elapsed_seconds": 0.0,
            "reserved_seconds": min(config["timeout_seconds"], plan["remaining_session_seconds"]),
            "attempt": config["max_attempts"] - plan["remaining_attempts"] + 1,
            "owner_pid": os.getpid(), "process_pid": None,
            "status": "active", "outcome": "running", "exit_code": None,
            "tokens": None, "estimated_usd": None, "complete": False,
            "source": "not yet reported", "warnings": list(plan["warnings"]),
        }
        history["runs"].append(record)
        ledger.write(history)

    def on_spawn(pid):
        # Retain ownership locally even when persisting the PID fails.
        record["process_pid"] = pid
        with ledger.locked():
            data = ledger.read()
            for current in data["runs"]:
                if current["id"] == record["id"]:
                    current["process_pid"] = pid
                    break
            else:
                raise BudgetError("active run disappeared from ledger; stopping invocation")
            ledger.write(data)

    started = time.monotonic()
    response = ""
    try:
        outcome, code, elapsed, raw = execute(argv, stdin_text, root, overrides,
                                              record["reserved_seconds"], on_spawn, deadline=deadline)
        record.update(outcome=outcome, exit_code=code, elapsed_seconds=elapsed)
        events = parse_events(raw)
        metadata = budget_adapters.normalize(args.harness, events)
        del raw
        if outcome == "completed" and (metadata.get("failed") or not metadata.get("complete")):
            outcome = "failed"
        record.update(outcome=outcome, exit_code=code, elapsed_seconds=elapsed,
                      complete=bool(metadata.get("complete")), source=metadata.get("source", "unknown"))
        # Preserve only known metadata fields, never responses or tool payloads.
        record["tokens"] = metadata.get("tokens")
        record["estimated_usd"] = metadata.get("estimated_usd")
        if outcome in ("timeout", "interrupted", "output_limit") or not record["complete"]:
            record["tokens"] = None
            record["estimated_usd"] = None
            record["complete"] = False
        if record["estimated_usd"] is None:
            record["warnings"].append("cost is unknown; no complete session cost total can be claimed")
        if (response_callback is not None or not args.json) and record["outcome"] == "completed" and record["complete"]:
            response = budget_adapters.response_text(args.harness, events)
        del events
    except (BudgetError, ValueError, OSError) as exc:
        record.update(outcome="launch_error", elapsed_seconds=time.monotonic() - started,
                      tokens=None, estimated_usd=None, complete=False)
        # Errors can contain argv or harness content; keep raw details out of storage.
        record["warnings"].append("invocation or metadata processing failed; cost is unknown")
        print("factory budget: " + str(exc), file=sys.stderr)
    finally:
        if record["process_pid"] is not None and record["exit_code"] is None:
            # Preserve the attempt, time and concurrency reservation whenever
            # process exit was not confirmed, including supervisor errors.
            record["warnings"].append(
                "process exit is unconfirmed; reservation retained; confirm the "
                "process group has stopped before recovery in docs/BUDGETS.md")
        else:
            record.update(status="completed", ended_at=now())
        with ledger.locked():
            history = ledger.read()
            for index, current in enumerate(history["runs"]):
                if current["id"] == record["id"]:
                    history["runs"][index] = record
                    break
            else:
                raise BudgetError("active run disappeared from ledger; preserve history and investigate")
            if config["estimated_usd"] is not None:
                post = make_plan(args, config, history)
                for warning in post["warnings"] + post["blockers"]:
                    if "cost" in warning or "USD" in warning:
                        record["warnings"].append(warning)
            ledger.write(history)
    output(record, args.json)
    if response_callback is not None:
        response_callback(dict(record), response)
    if response and not quiet:
        print(response, flush=True)
    return {"completed": 0, "timeout": 124, "interrupted": 130}.get(record["outcome"], 1)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    for command in ("plan", "run", "report"):
        sub = commands.add_parser(command)
        sub.add_argument("--json", action="store_true")
        sub.add_argument("--session", required=command != "report")
        if command != "report":
            sub.add_argument("--task", required=True)
            sub.add_argument("--harness", required=True, choices=HARNESS)
            sub.add_argument("--role", default="implementer", choices=ROLES)
            sub.add_argument("--max-cost-usd")
        if command == "run":
            sub.add_argument("--prompt-file", required=True)
    args = parser.parse_args()
    for key in ("session", "task"):
        value = getattr(args, key, None)
        if value is not None and not ID_PATTERN.fullmatch(value):
            parser.error("{} must match [A-Za-z0-9][A-Za-z0-9_-]{{0,63}}".format(key))
    root = Path(os.environ.get("FACTORY_BUDGET_ROOT", os.getcwd()))
    ledger = Ledger(root)
    if args.command == "report":
        emit(report(ledger.read(), args.session), args.json)
        return 0
    config = configuration()
    if args.command == "plan":
        plan = make_plan(args, config, ledger.read())
        emit(plan, args.json)
        return 2 if plan["blockers"] else 0
    return run(args, config, ledger, root)


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (BudgetError, ValueError, OSError) as error:
        print("factory budget: " + str(error), file=sys.stderr)
        sys.exit(2)
