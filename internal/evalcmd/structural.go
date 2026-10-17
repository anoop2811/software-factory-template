package evalcmd

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anoop2811/software-factory-template/internal/fileinput"
	"github.com/anoop2811/software-factory-template/internal/native"
)

const fileLimit = 16 << 20

func structural(ctx context.Context, args []string, root string, environment map[string]string, stdout, stderr io.Writer) int {
	harness := ""
	for _, arg := range args {
		if strings.HasPrefix(arg, "--harness=") {
			harness = strings.TrimPrefix(arg, "--harness=")
		}
	}
	path, needles := "", []string{"test-edit-denial.sh"}
	switch harness {
	case "opencode":
		path = ".opencode/plugin/factory-hooks.ts"
		needles = append(needles, "agentNameToRole")
	case "claude":
		path = ".claude/agents/implementer.md"
		needles = append(needles, "FACTORY_AGENT_ROLE=implementer")
	case "codex":
		path = ".codex/agents/implementer.toml"
		needles = append(needles, "FACTORY_AGENT_ROLE=implementer")
	default:
		fmt.Fprintln(stderr, "usage: harness-structural-eval.sh --harness=opencode|claude|codex")
		return 2
	}
	data, err := fileinput.Read(ctx, filepath.Join(root, path), fileLimit)
	if err != nil {
		return 1
	}
	for _, needle := range needles {
		if !strings.Contains(string(data), needle) {
			return 1
		}
	}
	configuration, err := os.CreateTemp("", "factory-eval-config-")
	if err != nil {
		fmt.Fprintln(stderr, "harness-structural-eval: cannot create private configuration")
		return 1
	}
	defer func() { _ = os.Remove(configuration.Name()) }()
	_, writeErr := io.WriteString(configuration, "test_file_patterns: \"_test\\.go([^[:alnum:]_]|$)\"\n")
	closeErr := configuration.Close()
	if writeErr != nil || closeErr != nil {
		return 1
	}
	probeArgs, payload := []string{filepath.Join(root, "scripts/hooks/test-edit-denial.sh")}, ""
	switch harness {
	case "opencode":
		probeArgs = append(probeArgs, "pkg/probe_test.go")
	case "claude":
		payload = `{"tool_name":"Write","tool_input":{"file_path":"pkg/probe_test.go"}}`
	case "codex":
		payload = `{"tool_name":"apply_patch","tool_input":{"command":"*** Begin Patch\n*** Update File: pkg/probe_test.go\n@@\n-old\n+new\n*** End Patch"}}`
	}
	for i, role := range []string{"implementer", "spec-writer"} {
		variables := maps.Clone(environment)
		if variables == nil {
			variables = map[string]string{}
		}
		variables["FACTORY_CONFIG"], variables["FACTORY_AGENT_ROLE"] = configuration.Name(), role
		probe, cancel := context.WithTimeout(ctx, 30*time.Second)
		result, err := native.ExecuteCommandWithOptions(probe, root, probeArgs, variables, native.CommandOptions{Input: payload, StdoutLimit: fileLimit, StderrLimit: fileLimit})
		cancel()
		if ctx.Err() != nil || result.OwnershipUnconfirmed || !result.ExitConfirmed || result.ExitCode == nil {
			fmt.Fprintln(stderr, "harness-structural-eval: probe did not complete")
			return 1
		}
		if result.Outcome != "completed" && result.Outcome != "failed" {
			fmt.Fprintln(stderr, "harness-structural-eval: probe did not complete")
			return 1
		}
		_ = err // An ordinary nonzero probe status is the denial contract.
		expected, label := 2, "implementer"
		if i == 1 {
			expected, label = 0, "non-implementer"
		}
		if *result.ExitCode != expected {
			fmt.Fprintf(stderr, "STRUCTURAL EVAL FAIL: %s %s edit returned %d, expected %d\n", harness, label, *result.ExitCode, expected)
			return 1
		}
	}
	fmt.Fprintf(stdout, "harness-structural-eval: PASS harness=%s\n", harness)
	fmt.Fprintln(stdout, "harness-structural-eval: live in-harness parity remains a separate OBSERVED gate")
	return 0
}
