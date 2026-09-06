#!/usr/bin/env python3
"""Independent native-shaped CLI fixture; all changes stay in temporary repos."""
import json
import os
from pathlib import Path
import subprocess
import sys
import time

name = Path(sys.argv[0]).name
args = sys.argv[1:]
if "--version" in args:
    print(name + " fixture-1.0")
    sys.exit(0)
if "--help" in args or "-h" in args:
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
            result = {"config": {"features": {"hooks": True}}}
        elif request["method"] == "configRequirements/read":
            result = {"requirements": None}
        elif request["method"] == "hooks/list":
            result = {"data": [{"cwd": os.getcwd(), "errors": [], "warnings": [], "hooks": [{
                "eventName": "preToolUse", "handlerType": "command",
                "command": 'FACTORY_AGENT_ROLE=implementer "$(git rev-parse --show-toplevel)/scripts/hooks/test-edit-denial.sh"',
                "matcher": "^(apply_patch|Edit|Write)$", "enabled": True,
                "trustStatus": "trusted", "async": False}]}]}
        print(json.dumps({"id": request["id"], "result": result}), flush=True)
    sys.exit(0)

prompt = sys.stdin.read()
role = os.environ.get("FACTORY_AGENT_ROLE")
mode = os.environ.get("LOOP_FIXTURE_MODE", "normal")
calls = Path(os.environ["LOOP_FIXTURE_CALLS"])
previous = [json.loads(line) for line in calls.read_text().splitlines()] if calls.exists() else []
number = 1 + sum(item.get("role") == role for item in previous)
with calls.open("a") as log:
    log.write(json.dumps({"role": role, "name": name, "args": args, "prompt": prompt}) + "\n")

failed = mode == "implement_fail" and role == "implementer"
if role == "implementer":
    if mode == "hold":
        child = subprocess.Popen([sys.executable, "-c", "import time; time.sleep(120)"])
        Path(os.environ["LOOP_FIXTURE_CHILD"]).write_text(str(child.pid))
        time.sleep(120)
    if mode != "no_change":
        value = "bad" if mode in ("repeat_bad", "check_repair") and number == 1 else "good"
        if mode == "repeat_bad":
            value = "bad"
        if mode == "review_repair" and number > 1:
            value = "good repaired"
        Path("product.txt").write_text(value + "\n")
    if mode == "test_mutation":
        Path(os.environ.get("LOOP_FIXTURE_MUTATION_PATH", "product_test.py")).write_text("# altered tests\n")
    if mode == "protected_mutation":
        Path("governance.txt").write_text("altered governance\n")
    if mode == "config_mutation":
        with Path("factory.yaml").open("a") as output:
            output.write("# changed by model\n")
    if mode == "role_mutation":
        with Path(".opencode/agent/reviewer.md").open("a") as output:
            output.write("\nChanged reviewer instructions.\n")
    if mode == "ignored_policy_mutation":
        Path(".claude/settings.local.json").write_text('{"permissions":{"allow":["Bash(*)"]}}\n')
    response = "private-agent-response-sentinel"
elif role == "reviewer":
    if mode == "review_mutation":
        Path("product.txt").write_text("changed after passing verification\n")
    response = json.dumps({"verdict": "repair" if mode == "review_repair" and number == 1 else "approve",
                           "findings": ["private-review-finding-sentinel"]
                           if mode == "review_repair" and number == 1 else []})
    if mode == "review_invalid":
        response = "Looks good to me"
    response = os.environ.get("LOOP_FIXTURE_REVIEW", response)
    if mode == "review_fail":
        failed = True
else:
    raise SystemExit("Unexpected role " + str(role))

def emit(value):
    print(json.dumps(value), flush=True)

if name == "codex":
    emit({"type": "item.completed", "item": {"id": "answer", "type": "agent_message", "text": response}})
    emit({"type": "turn.failed", "error": {"message": "private-error-sentinel"}} if failed else
         {"type": "turn.completed", "usage": {"input_tokens": 100, "cached_input_tokens": 25, "output_tokens": 10}})
elif name == "claude":
    emit({"type": "result", "subtype": "error_during_execution" if failed else "success",
          "is_error": failed, "result": response, "session_id": "fixture-session",
          "total_cost_usd": 0.25, "usage": {"input_tokens": 100, "output_tokens": 10,
              "cache_read_input_tokens": 25, "cache_creation_input_tokens": 0}})
elif name == "opencode":
    emit({"type": "text", "part": {"id": "answer", "sessionID": "fixture-session",
                                    "messageID": "message", "type": "text", "text": response}})
    emit({"type": "step_finish", "sessionID": "fixture-session", "part": {
        "id": "step", "sessionID": "fixture-session", "messageID": "message", "type": "step-finish",
        "reason": "stop", "cost": 0.25, "tokens": {"input": 100, "output": 10, "reasoning": 0,
            "cache": {"read": 25, "write": 0}}}})
    if failed:
        emit({"type": "error", "error": {"name": "UnknownError", "data": {"message": "private-error-sentinel"}}})
