package loop

import (
	"context"
	"os"
	"time"

	"github.com/anoop2811/software-factory-template/internal/native"
)

// Snapshot probes preserve caller cwd, literal argv, complete environment and
// binary stdin through the shared owned process supervisor.
// docs/adr/0093-runtime-transition-guard.md:128.
func probe(ctx context.Context, environment map[string]string, allowance time.Duration, args []string, input []byte) ([]byte, int, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, 0, loopError()
	}
	execution, err := native.ExecuteProbe(ctx, cwd, args, environment, allowance, input)
	if err != nil || !execution.ExitConfirmed || execution.OwnershipUnconfirmed || execution.ExitCode == nil || (execution.Outcome != "completed" && execution.Outcome != "failed") {
		return nil, 0, loopError()
	}
	return execution.Stdout, *execution.ExitCode, nil
}
