package native

import (
	"context"
	"errors"
	"sync"
	"time"
)

const probeLimit = 32 << 20

// ExecuteProbe preserves literal local-tool input and stdout while bounding both
// channels together. It uses the existing owned process supervisor and the
// original one-second probe cleanup allowance.
// docs/adr/0093-runtime-transition-guard.md:128.
func ExecuteProbe(ctx context.Context, root string, argv []string, environment map[string]string, allowance time.Duration, input []byte) (Execution, error) {
	if err := ctx.Err(); err != nil {
		return Execution{}, err
	}
	plan, variables, err := prepareCommand(root, argv, environment)
	if err != nil {
		return Execution{}, err
	}
	plan.Stdin = string(input)
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	quota := &probeQuota{cancel: cancel}
	ops := defaultProcessOps()
	ops.waitLimit = time.Second
	execution, err := supervise(child, plan, allowance,
		func(context.Context, int) error { return nil }, ops,
		runOptions{argv0: argv[0], limit: probeLimit, environment: variables, errorOutput: quota,
			consume: func(_ context.Context, data []byte) (bool, error) {
				_, err := quota.Write(data)
				return false, err
			}})
	if quota.exceeded() {
		execution.Outcome = "output_limit"
		err = errors.Join(err, errors.New("local probe output limit exceeded"))
	}
	return execution, err
}

// A single counter bounds combined output, while the supervisor retains stdout.
type probeQuota struct {
	mutex    sync.Mutex
	total    int
	overflow bool
	cancel   context.CancelFunc
}

func (q *probeQuota) Write(data []byte) (int, error) {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	count := min(len(data), probeLimit-q.total)
	q.total += count
	if count != len(data) {
		q.overflow = true
		q.cancel()
		return count, errors.New("local probe output limit exceeded")
	}
	return count, nil
}

func (q *probeQuota) exceeded() bool {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	return q.overflow
}
