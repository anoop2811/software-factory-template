// Package qualification selects bounded workers for repository-only Go checks.
// docs/adr/0104-adaptive-qualification-workers.md:22.
package qualification

import (
	"errors"
	"strconv"
)

const workerGiB = uint64(1 << 30)

// Resources is a read-only snapshot; zero values mean detection was uncertain.
type Resources struct {
	CPUs        int
	MemoryBytes uint64
	Note        string
}

// Plan contains explicit worker counts, never shell expressions.
type Plan struct {
	Base        int
	Handoff     int
	CPUs        int
	MemoryBytes uint64
	Note        string
}

// Select preserves explicit reproducible overrides and bounds automatic work.
// docs/adr/0104-adaptive-qualification-workers.md:22.
func Select(requested, maximum string, resources Resources) (Plan, error) {
	workerLimit, err := workerInteger(maximum)
	if err != nil {
		return Plan{}, errors.New("GO_RUNTIME_TEST_MAX_PROCS must be an integer from 1 through 32")
	}
	plan := Plan{CPUs: resources.CPUs, MemoryBytes: resources.MemoryBytes, Note: resources.Note}
	switch {
	case requested != "auto":
		workers, err := workerInteger(requested)
		if err != nil || workers > workerLimit {
			return Plan{}, errors.New("GO_RUNTIME_TEST_PROCS must be auto or an integer from 1 through GO_RUNTIME_TEST_MAX_PROCS")
		}
		plan.Base = workers
		plan.Note = "explicit override of automatic resource budgets"
	case resources.CPUs < 1 || resources.MemoryBytes == 0:
		plan.Base = 1
		if plan.Note == "" {
			plan.Note = "unknown resources; conservative fallback"
		}
	default:
		cpuBudget := max(1, resources.CPUs-1)
		memoryBudget := uint64(1)
		if resources.MemoryBytes > 2*workerGiB {
			memoryBudget = max(uint64(1), (resources.MemoryBytes-2*workerGiB)/(2*workerGiB))
		}
		// Clamp before converting the unsigned memory count, including on 32-bit hosts.
		memoryWorkers := int(min(uint64(32), memoryBudget)) // #nosec G115 -- the unsigned value is clamped to 32 before conversion.
		plan.Base = min(workerLimit, cpuBudget, memoryWorkers)
		if plan.Note == "" {
			plan.Note = "automatic CPU/memory budget"
		}
	}
	plan.Handoff = min(plan.Base, 2)
	return plan, nil
}

func workerInteger(value string) (int, error) {
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 || n > 32 || strconv.Itoa(n) != value {
		return 0, errors.New("invalid worker integer")
	}
	return n, nil
}
