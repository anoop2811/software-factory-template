"""Native invocations and metadata-only accounting; docs/DECISION_LOG.md:1681."""

import json
import math
import os
import pathlib
import re
import selectors
import shlex
import shutil
import signal
import subprocess
import time


ROLES = {"spec-writer", "implementer", "refactorer", "reviewer", "wiki-maintainer"}
HELP = {
    "codex": (["exec", "--help"], ("--json", "--sandbox", "--cd", "--model", "--config")),
    "claude": (["--help"], ("--print", "--output-format", "--agent", "--permission-mode", "--model")),
    "opencode": (["run", "--help"], ("--format", "--agent", "--model")),
}
CODEX_MATCHER = "^(apply_patch|Edit|Write)$"
CODEX_COMMAND = 'FACTORY_AGENT_ROLE=implementer "$(git rev-parse --show-toplevel)/scripts/hooks/test-edit-denial.sh"'


def _role_edit_permission(root, role):
    try:
        config = json.loads((pathlib.Path(root) / "opencode.json").read_text(encoding="utf-8"))
        policy = config["agent"][role]["permission"]
        return policy.get("edit", "ask")
    except (OSError, UnicodeError, ValueError, KeyError, TypeError, AttributeError):
        raise ValueError("budget run requires canonical role permissions in opencode.json") from None


def _codex_settings(root, role):
    edit = _role_edit_permission(root, role)
    sandbox = "read-only" if edit == "deny" else "workspace-write"
    if role != "implementer":
        return sandbox, []
    hook = (pathlib.Path(root) / "scripts/hooks/test-edit-denial.sh")
    if not hook.is_file() or not os.access(str(hook), os.X_OK):
        raise ValueError("missing executable implementer test-edit-denial hook")
    # JSON string encoding is also valid for these TOML basic strings.
    override = ('hooks.PreToolUse=[{matcher=' + json.dumps(CODEX_MATCHER)
                + ',hooks=[{type="command",command=' + json.dumps(CODEX_COMMAND)
                + ',timeout=30,statusMessage="Enforcing implementer test-file boundary"}]}]')
    return sandbox, ["-c", override]


def _codex_hook_preflight(binary, root, flags):
    """Inspect native hook trust through a private, short-lived local server."""
    process = None
    poller = selectors.DefaultSelector()
    responses = {}
    cleanup_timed_out = False
    try:
        process = subprocess.Popen([binary, "app-server"] + flags, cwd=root,
            stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
            start_new_session=True)
        requests = [
            {"id": 1, "method": "initialize", "params": {
                "clientInfo": {"name": "factory-budget-preflight", "version": "1"},
                "capabilities": {"experimentalApi": True}}},
            {"method": "initialized"},
            {"id": 2, "method": "config/read", "params": {"cwd": str(root), "includeLayers": False}},
            {"id": 3, "method": "hooks/list", "params": {"cwds": [str(root)]}},
            {"id": 4, "method": "configRequirements/read", "params": {}},
        ]
        process.stdin.write(b"".join((json.dumps(request) + "\n").encode() for request in requests))
        process.stdin.flush()
        poller.register(process.stdout, selectors.EVENT_READ)
        pending = b""
        received = 0
        deadline = time.monotonic() + 10
        while time.monotonic() < deadline and not {2, 3, 4}.issubset(responses):
            if not poller.select(min(0.2, max(0, deadline - time.monotonic()))):
                continue
            chunk = os.read(process.stdout.fileno(), 65536)
            if not chunk:
                break
            received += len(chunk)
            if received > 2 * 1024 * 1024:
                raise ValueError("Codex hook capability response exceeded its limit")
            pending += chunk
            while b"\n" in pending:
                line, pending = pending.split(b"\n", 1)
                row = json.loads(line)
                if isinstance(row, dict) and row.get("id") in (2, 3, 4):
                    responses[row["id"]] = row
    except (OSError, ValueError):
        raise ValueError("Codex hook capability probe failed; repair or update the CLI") from None
    finally:
        poller.close()
        if process is not None:
            try:
                os.killpg(process.pid, signal.SIGTERM)
            except ProcessLookupError:
                pass
            try:
                process.wait(timeout=1)
            except subprocess.TimeoutExpired:
                pass
            # Terminate descendants even when the server has already exited.
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            try:
                process.wait(timeout=1)
            except subprocess.TimeoutExpired:
                cleanup_timed_out = True
            finally:
                for stream in (process.stdin, process.stdout):
                    try:
                        stream.close()
                    except OSError:
                        pass
    if cleanup_timed_out:
        raise ValueError("Codex hook capability probe exit is unconfirmed; "
                         "confirm process group {} has stopped before retrying".format(process.pid))
    try:
        effective = responses[2]["result"]["config"]
        features = effective.get("features", {}) or {}
        if features.get("hooks", features.get("codex_hooks", True)) is False:
            raise ValueError("Codex hooks are disabled; enable hooks before budget run")
        requirements = responses[4]["result"]["requirements"] or {}
        required_features = requirements.get("featureRequirements", {}) or {}
        if required_features.get("hooks", required_features.get("codex_hooks", True)) is False:
            raise ValueError("Codex administrator policy disables required hooks; budget run is unavailable")
        entries = responses[3]["result"]["data"]
        candidates = [hook for entry in entries if entry.get("cwd") == str(root)
                      and not entry.get("errors") for hook in entry["hooks"]]
        allowed = any(hook.get("eventName") == "preToolUse"
            and hook.get("handlerType") == "command" and hook.get("command") == CODEX_COMMAND
            and hook.get("matcher") == CODEX_MATCHER and hook.get("enabled") is True
            and hook.get("trustStatus") in ("trusted", "managed")
            and not hook.get("async", False) for hook in candidates)
    except (KeyError, TypeError, AttributeError):
        raise ValueError("Codex CLI cannot establish required hook trust; update it before budget run") from None
    if not allowed:
        setup = " ".join(shlex.quote(arg) for arg in ["codex", "--cd", str(root)] + flags)
        raise ValueError("Codex implementer hook must be enabled and trusted. Run " + setup
                         + ", then use /hooks to review and trust that exact test-edit-denial hook; retry budget run")


def _role_text(root, role, directory=".opencode/agent"):
    if role not in ROLES:
        raise ValueError("unsupported factory role")
    path = pathlib.Path(root) / directory / (role + ".md")
    try:
        text = path.read_text(encoding="utf-8")
    except (OSError, UnicodeError):
        raise ValueError("missing or unreadable role configuration: " + str(path)) from None
    if text.startswith("---\n"):
        parts = text.split("\n---", 1)
        if len(parts) != 2:
            raise ValueError("invalid role frontmatter: " + str(path))
        text = parts[1].strip()
    if not text.strip():
        raise ValueError("empty role instructions: " + str(path))
    return text


def preflight(harness, role, root):
    """Read local roles and CLI help only; no provider or agent invocation."""
    if harness not in HELP:
        raise ValueError("unsupported harness")
    _role_text(root, role)
    environment(harness, role)
    if harness == "claude":
        _role_text(root, role, ".claude/agents")
        _role_edit_permission(root, role)
    binary = shutil.which(harness)
    if not binary:
        raise ValueError(harness + " CLI is missing; install it before budget run")
    args, flags = HELP[harness]
    try:
        result = subprocess.run(
            [binary] + args, cwd=root, stdin=subprocess.DEVNULL,
            stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
            timeout=10, check=False, encoding="utf-8", errors="replace",
        )
    except (OSError, subprocess.TimeoutExpired):
        raise ValueError(harness + " CLI help failed; repair the local installation") from None
    if result.returncode != 0:
        raise ValueError(harness + " CLI help failed; repair the local installation")
    missing = [flag for flag in flags if not re.search(re.escape(flag) + r"\b", result.stdout)]
    if missing:
        raise ValueError(harness + " CLI lacks required flags: " + ", ".join(missing))
    if harness == "codex":
        _, flags = _codex_settings(root, role)
        if flags:
            _codex_hook_preflight(binary, str(pathlib.Path(root).resolve()), flags)


def environment(harness, role, environ=None):
    """Overlay only role mode so OpenCode does not silently select its default.

    Canonical factory roles are subagents. OpenCode run requires all/primary;
    its config-content layer merges with the existing native configuration.
    """
    if harness not in HELP or role not in ROLES:
        raise ValueError("unsupported harness or factory role")
    result = {"FACTORY_AGENT_ROLE": role}
    if harness != "opencode":
        return result
    existing = (os.environ if environ is None else environ).get("OPENCODE_CONFIG_CONTENT", "")
    try:
        overlay = json.loads(existing) if existing else {}
    except (TypeError, ValueError):
        raise ValueError("OPENCODE_CONFIG_CONTENT must be a JSON object for budget run") from None
    if not isinstance(overlay, dict):
        raise ValueError("OPENCODE_CONFIG_CONTENT must be a JSON object for budget run")
    agents = overlay.setdefault("agent", {})
    if not isinstance(agents, dict):
        raise ValueError("OPENCODE_CONFIG_CONTENT.agent must be an object")
    selected = agents.setdefault(role, {})
    if not isinstance(selected, dict):
        raise ValueError("OPENCODE_CONFIG_CONTENT agent role must be an object")
    if selected.get("disable") is True:
        raise ValueError("selected OpenCode role is disabled in OPENCODE_CONFIG_CONTENT")
    selected["mode"] = "all"
    try:
        result["OPENCODE_CONFIG_CONTENT"] = json.dumps(overlay, allow_nan=False)
    except (TypeError, ValueError):
        raise ValueError("OPENCODE_CONFIG_CONTENT contains invalid JSON values") from None
    return result


def build_command(harness, role, model, root, prompt):
    """Preserve native project config; supply prompts via stdin, never argv."""
    instructions = _role_text(root, role)
    if harness == "codex":
        sandbox, flags = _codex_settings(root, role)
        argv = ["codex", "exec", "--json", "--sandbox", sandbox, "--cd", str(root)] + flags
        prompt = "Factory role: " + role + "\n\n" + instructions + "\n\nTask:\n" + prompt
    elif harness == "claude":
        permission_mode = "plan" if _role_edit_permission(root, role) == "deny" else "acceptEdits"
        argv = ["claude", "-p", "--output-format", "json", "--agent", role,
                "--permission-mode", permission_mode]
    elif harness == "opencode":
        argv = ["opencode", "run", "--format", "json", "--agent", role]
    else:
        raise ValueError("unsupported harness")
    if model:
        argv.extend(["--model", model])
    if harness == "codex":
        argv.append("-")
    return argv, prompt


def _number(value, integer=False):
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        return False
    try:
        return math.isfinite(value) and value >= 0 and (not integer or isinstance(value, int))
    except OverflowError:
        return False


def _tokens(value, required, optional=()):
    if not isinstance(value, dict):
        return None
    keys = list(required) + [key for key in optional if key in value]
    if any(not _number(value.get(key), integer=True) for key in keys):
        return None
    return {key: value[key] for key in keys}


def normalize(harness, events):
    """Return bounded metadata, never transcript content or provider error text.

    Completeness concerns reported usage, not correctness or billing authority.
    The supervisor must also invalidate completeness on timeout/parse truncation.
    """
    if harness not in HELP:
        raise ValueError("unsupported harness")
    malformed = any(not isinstance(event, dict) for event in events)
    rows = [event for event in events if isinstance(event, dict)]
    result = {"tokens": None, "estimated_usd": None, "complete": False,
              "failed": False, "source": "unavailable"}
    if harness == "codex":
        result["source"] = "codex.turn.completed.usage; cost unavailable"
        result["failed"] = any(row.get("type") in ("turn.failed", "error") for row in rows)
        finals = [row for row in rows if row.get("type") == "turn.completed"]
        if finals:
            result["tokens"] = _tokens(finals[-1].get("usage"),
                ("input_tokens", "cached_input_tokens", "output_tokens"), ("reasoning_output_tokens",))
            result["complete"] = result["tokens"] is not None and not result["failed"] and not malformed
    elif harness == "claude":
        result["source"] = "claude.result.usage (main agent); total_cost_usd (client estimate)"
        finals = [row for row in rows if row.get("type") == "result"]
        result["failed"] = any(row.get("type") == "error" for row in rows)
        if finals:
            final = finals[-1]
            subtype = final.get("subtype")
            result["failed"] = result["failed"] or subtype != "success" or final.get("is_error") is True
            result["tokens"] = _tokens(final.get("usage"), ("input_tokens", "output_tokens",
                "cache_creation_input_tokens", "cache_read_input_tokens"))
            cost = final.get("total_cost_usd")
            # A crash result can contain zeroed totals despite earlier spending.
            if _number(cost) and subtype != "error_during_execution" and not malformed:
                result["estimated_usd"] = cost
            result["complete"] = result["tokens"] is not None and result["estimated_usd"] is not None
    else:
        result["source"] = "opencode.step_finish.part (deduplicated incremental client estimate)"
        result["failed"] = any(row.get("type") in ("error", "session.error") for row in rows)
        seen = {}
        total_cost = 0.0
        totals = {key: 0 for key in ("input_tokens", "output_tokens", "reasoning_output_tokens",
                                    "cache_read_input_tokens", "cache_creation_input_tokens")}
        last_reason = None
        valid = not malformed
        count = 0
        for row in rows:
            if row.get("type") != "step_finish":
                continue
            part = row.get("part")
            if not isinstance(part, dict):
                valid = False
                continue
            identity = tuple(part.get(key) for key in ("sessionID", "messageID", "id"))
            if any(not isinstance(key, str) or not key for key in identity):
                valid = False
                continue
            values = part.get("tokens")
            tokens = _tokens(values, ("input", "output", "reasoning"), ("total",))
            cache = _tokens(values.get("cache") if isinstance(values, dict) else None, ("read", "write"))
            cost = part.get("cost")
            signature = (tokens, cache, cost, part.get("reason"))
            if identity in seen:
                if seen[identity] != signature:
                    valid = False
                continue
            seen[identity] = signature
            count += 1
            last_reason = part.get("reason")
            if tokens is None or cache is None or not _number(cost):
                valid = False
                continue
            for target, source in (("input_tokens", "input"), ("output_tokens", "output"),
                                   ("reasoning_output_tokens", "reasoning")):
                totals[target] += tokens[source]
            totals["cache_read_input_tokens"] += cache["read"]
            totals["cache_creation_input_tokens"] += cache["write"]
            total_cost += cost
        if count and valid and not result["failed"] and last_reason == "stop" and _number(total_cost):
            result.update(tokens=totals, estimated_usd=total_cost, complete=True)
    if malformed:
        result["complete"] = False
        result["estimated_usd"] = None
    return result


def response_text(harness, events):
    """Select only assistant answer text for the terminal; never ledger input."""
    rows = [row for row in events if isinstance(row, dict)]
    if harness == "codex":
        answers = [row.get("item", {}).get("text") for row in rows
                   if row.get("type") == "item.completed" and isinstance(row.get("item"), dict)
                   and row["item"].get("type") == "agent_message"]
        return next((answer for answer in reversed(answers) if isinstance(answer, str)), "")
    if harness == "claude":
        answers = [row.get("result") for row in rows if row.get("type") == "result"
                   and row.get("subtype") == "success" and row.get("is_error") is not True]
        return next((answer for answer in reversed(answers) if isinstance(answer, str)), "")
    if harness == "opencode":
        answers = []
        seen = set()
        for row in rows:
            part = row.get("part")
            if row.get("type") != "text" or not isinstance(part, dict):
                continue
            text = part.get("text")
            identity = tuple(part.get(key) for key in ("sessionID", "messageID", "id"))
            if isinstance(text, str) and all(isinstance(key, str) for key in identity) and identity not in seen:
                seen.add(identity)
                answers.append(text)
        return "\n".join(answers)
    raise ValueError("unsupported harness")
