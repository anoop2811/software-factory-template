package native

import (
	"context"
	"errors"
	"strings"
	"time"
)

// ExecuteCheck runs the explicitly configured deterministic shell check using
// shared process-group supervision and a complete captured environment.
// docs/adr/0075-go-manual-loop-controller.md:65.
func ExecuteCheck(ctx context.Context, root, check string, environment map[string]string, allowance time.Duration, onSpawn func(context.Context, int) error) (Execution, error) {
	if root == "" || strings.ContainsRune(root, 0) || strings.ContainsRune(check, 0) {
		return Execution{Outcome: "launch_error"}, errors.New("invalid deterministic check")
	}
	variables := make([]string, 0, len(environment)+1)
	for key, value := range environment {
		if key == "" || strings.ContainsAny(key, "=\x00") || strings.ContainsRune(value, 0) {
			return Execution{Outcome: "launch_error"}, errors.New("invalid deterministic check environment")
		}
		if key != "FACTORY_AGENT_ROLE" {
			variables = append(variables, key+"="+value)
		}
	}
	variables = append(variables, "FACTORY_AGENT_ROLE=reviewer")
	path, ok := environment["PATH"]
	if !ok {
		path = "/bin:/usr/bin"
	}
	binary, err := findExecutableOnPath("bash", root, path)
	if err != nil {
		return Execution{Outcome: "launch_error"}, errors.New("cannot resolve check shell")
	}
	plan := Plan{Root: root, Argv: []string{binary, "-c", "exec 2>&1\n" + check}}
	return supervise(ctx, plan, allowance, onSpawn, defaultProcessOps(), runOptions{limit: outputLimit, mergeStderr: true, environment: variables})
}
