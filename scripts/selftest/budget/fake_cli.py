#!/usr/bin/env python3
"""Native-shaped fixtures, independent of the production usage normalizers."""
import json
import os
from pathlib import Path
import subprocess
import sys
import time


name = Path(sys.argv[0]).name
args = sys.argv[1:]
mode = os.environ.get("BUDGET_FIXTURE_MODE", "normal")
if "--version" in args:
    print(name + " fixture-1.0")
    sys.exit(0)
if "--help" in args or "-h" in args:
    if mode == "unsupported":
        print("Unsupported old CLI")
    else:
        print("exec run --json --format --output-format --agent --model -m -p --print --cd "
              "--sandbox --config -c --permission-mode --verbose --allowedTools "
              "--no-session-persistence --disable-slash-commands --ephemeral")
    sys.exit(0)

if name == "codex" and args and args[0] == "app-server":
    for line in sys.stdin:
        request = json.loads(line)
        if "id" not in request:
            continue
        result = {}
        if request["method"] == "config/read":
            result = {"config": {"features": {"hooks": mode != "hooks_disabled"}}}
        elif request["method"] == "configRequirements/read":
            result = {"requirements": {"featureRequirements": {"hooks": False}}
                      if mode == "hooks_managed_off" else None}
        elif request["method"] == "hooks/list":
            result = {"data": [{"cwd": os.getcwd(), "errors": [], "warnings": [], "hooks": [{
                "eventName": "preToolUse", "handlerType": "command",
                "command": 'FACTORY_AGENT_ROLE=implementer "$(git rev-parse --show-toplevel)/scripts/hooks/test-edit-denial.sh"',
                "matcher": "^(apply_patch|Edit|Write)$", "enabled": True,
                "trustStatus": "untrusted" if mode == "hook_untrusted" else "trusted", "async": False}]}]}
        print(json.dumps({"id": request["id"], "result": result}), flush=True)
    sys.exit(0)

prompt = sys.stdin.read()
with open(os.environ["BUDGET_FIXTURE_CALLS"], "a") as log:
    log.write(json.dumps({"name": name, "args": args, "stdin": prompt,
                          "role": os.environ.get("FACTORY_AGENT_ROLE"),
                          "opencode_config": os.environ.get("OPENCODE_CONFIG_CONTENT")}) + "\n")

if mode == "hold":
    child = subprocess.Popen([sys.executable, "-c", "import time; time.sleep(120)"])
    Path(os.environ["BUDGET_FIXTURE_CHILD"]).write_text(str(child.pid))
    time.sleep(120)
if mode == "malformed":
    print("this is not JSON and includes private-response-sentinel")
    sys.exit(0)
if mode == "empty":
    sys.exit(0)

def emit(value):
    print(json.dumps(value), flush=True)

bad = -1 if mode == "negative" else float("nan") if mode == "nonfinite" else None
if name == "codex":
    emit({"type": "thread.started", "thread_id": "fake-thread"})
    emit({"type": "item.completed", "item": {"id": "text-1", "type": "agent_message",
                                                "text": "private-response-sentinel"}})
    if mode == "error":
        emit({"type": "turn.failed", "error": {"message": "private-error-sentinel"}})
    else:
        usage = {"input_tokens": 100, "cached_input_tokens": 25, "output_tokens": 10}
        if bad is not None:
            usage["input_tokens"] = bad
        if mode == "partial":
            usage.pop("output_tokens")
        emit({"type": "turn.completed", "usage": usage})
elif name == "claude":
    event = {"type": "result", "subtype": "success", "is_error": False,
             "result": "private-response-sentinel", "session_id": "fake-session",
             "total_cost_usd": 0.25, "usage": {"input_tokens": 100, "output_tokens": 10,
                 "cache_read_input_tokens": 25, "cache_creation_input_tokens": 0}}
    if bad is not None:
        event["total_cost_usd"] = bad
        event["usage"]["input_tokens"] = bad
    if mode == "partial":
        event.pop("total_cost_usd")
    if mode == "error":
        event.update(subtype="error_during_execution", is_error=True,
                     errors=["private-error-sentinel"])
    emit(event)
elif name == "opencode":
    emit({"type": "text", "part": {"id": "text-1", "sessionID": "fake-session", "messageID": "message-1",
                                    "type": "text", "text": "private-response-sentinel"}})
    for index, cost in enumerate((0.1, 0.15)):
        event = {"type": "step_finish", "sessionID": "fake-session", "part": {
            "id": "part-" + str(index), "sessionID": "fake-session", "messageID": "message-1",
            "type": "step-finish", "reason": "stop", "cost": cost,
            "tokens": {"input": 50, "output": 5, "reasoning": 0,
                       "cache": {"read": 10, "write": 0}}}}
        if bad is not None:
            event["part"]["cost"] = bad
            event["part"]["tokens"]["input"] = bad
        if mode == "partial" and index == 1:
            event["part"].pop("cost")
        emit(event)
        if mode == "duplicate":
            emit(event)
    if mode == "error":
        emit({"type": "error", "error": {"name": "UnknownError",
                                            "data": {"message": "private-error-sentinel"}}})
else:
    raise SystemExit("unexpected fake harness name " + name)
