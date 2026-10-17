package installation

import (
	"context"
	"fmt"
	"strings"

	"github.com/anoop2811/software-factory-template/internal/staging"
)

// Legacy paths remain invocation adapters, never alternative implementations.
// docs/adr/0099-native-factory-orchestration.md:31.
func orchestrationAdapter(path string, image staging.Inspection) ([]byte, error) {
	commands := map[string]string{
		"scripts/sync-opencode.sh":           "sync opencode",
		"scripts/sync-claude.sh":             "sync claude",
		"scripts/sync-codex.sh":              "sync codex",
		"scripts/pre-push-check.sh":          "check",
		"scripts/prereq-check.sh":            "prereq",
		"scripts/harness-structural-eval.sh": "eval structural",
		"scripts/golden-task-eval.sh":        "eval golden",
		"scripts/adversarial-review.sh":      "review",
		"eval/runners/mock.sh":               "eval run mock",
		"eval/runners/example-harness.sh":    "eval run example-harness",
		"eval/runners/claude.sh":             "eval run claude",
		"eval/runners/codex.sh":              "eval run codex",
		"eval/runners/opencode.sh":           "eval run opencode",
	}
	command, ok := commands[path]
	if !ok {
		return nil, conflict("unrecognized orchestration adapter")
	}
	parent, arguments := "..", " \"$@\""
	if strings.HasPrefix(path, "eval/runners/") {
		parent = "../.."
	}
	if strings.HasPrefix(path, "scripts/sync-") {
		arguments = "" // Legacy synchronizers ignored every argument.
	}
	return []byte(fmt.Sprintf("#!/bin/bash\n# Factory installation %s %s %s\nset -e\n_factory_root=\"$(cd \"$(dirname \"${BASH_SOURCE[0]}\")/%s\" && pwd -P)\"\nexec \"$_factory_root/factory\" %s%s\n", image.Runtime.Version, image.Manifest.SourceRevision, image.Runtime.SHA256, parent, command, arguments)), nil
}

func helperBridge(ctx context.Context, path string, image staging.Inspection) ([]byte, error) {
	data, ok := image.Assets["runtime/shell/helpers.sh"]
	if !ok {
		return nil, conflict("required native helper bridge source is missing")
	}
	source := string(data)
	start := strings.Index(source, "_factory_helper_call() (")
	end := strings.Index(source, "\nfactory_log_event()")
	if start < 0 || end < start {
		return nil, conflict("unsupported native helper bridge source")
	}
	// BASH_SOURCE supplies relocation for Bash hosts. POSIX callers can supply
	// FACTORY_RUNTIME_BINARY; otherwise their caller repository selects factory.
	// No locator assigns caller-owned configuration or protocol variables.
	source = source[:start] + `_factory_helper_call() (
  if [ -n "${FACTORY_RUNTIME_BINARY-}" ]; then
    case "$FACTORY_RUNTIME_BINARY" in /*) ;; *) return 2 ;; esac
    [ -f "$FACTORY_RUNTIME_BINARY" ] && [ -x "$FACTORY_RUNTIME_BINARY" ] || return 2
    _factory_helper_runtime=$FACTORY_RUNTIME_BINARY
  elif [ -n "${BASH_VERSION-}" ]; then
    _factory_helper_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)" || return
    _factory_helper_runtime=$_factory_helper_root/factory
  else
    _factory_helper_root="$(git rev-parse --show-toplevel 2>/dev/null)" || return
    _factory_helper_runtime=$_factory_helper_root/factory
  fi
  /usr/bin/env "FACTORY_CONFIG=${FACTORY_CONFIG-}" "NO_COLOR=${NO_COLOR-}" \
    "FACTORY_EVENT_LOG=${FACTORY_EVENT_LOG-}" "FACTORY_EVENT_MAX_LINES=${FACTORY_EVENT_MAX_LINES-}" \
    FACTORY_BRIDGE_PROTOCOL=1 "$_factory_helper_runtime" helper "$@"
)
` + source[end:]
	switch path {
	case "scripts/lib/color.sh":
		source += "\nfactory_palette\n"
	case "scripts/lib/budget-config.sh":
		source += "\nfactory_budget_config\n"
	case "scripts/lib/events.sh", "scripts/lib/hookspath.sh", "scripts/lib/timing.sh":
	default:
		return nil, conflict("unrecognized sourceable helper adapter")
	}
	return []byte("# Factory installation " + image.Runtime.Version + " " + image.Manifest.SourceRevision + " " + image.Runtime.SHA256 + "\n" + source), ctx.Err()
}
