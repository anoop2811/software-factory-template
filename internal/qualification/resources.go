package qualification

import (
	"os"
	"runtime"
	"strconv"
	"strings"
)

// Detect is used by the short-lived private helper, never by the factory runtime.
// Refresh the quota-aware default even when GOMAXPROCS was explicitly raised;
// preserve a lower user budget and never exceed the CPU-affinity count.
// docs/adr/0104-adaptive-qualification-workers.md:42.
func Detect() Resources {
	previous := runtime.GOMAXPROCS(0)
	runtime.SetDefaultGOMAXPROCS()
	cpus := min(previous, runtime.GOMAXPROCS(0), runtime.NumCPU())
	// SetDefaultGOMAXPROCS still honors the cgroup opt-out GODEBUG setting.
	// If disabled, the runtime cannot establish our automatic container budget.
	if runtime.GOOS == "linux" {
		quotaEnabled := true
		for entry := range strings.SplitSeq(os.Getenv("GODEBUG"), ",") {
			if key, value, ok := strings.Cut(entry, "="); ok && key == "containermaxprocs" {
				n, err := strconv.ParseInt(value, 10, 32)
				quotaEnabled = err == nil && n > 0
			}
		}
		if !quotaEnabled {
			cpus = 1
		}
	}
	memory, err := platformMemory()
	if err != nil {
		return Resources{CPUs: cpus, Note: "memory probe unavailable; conservative fallback"}
	}
	return Resources{CPUs: cpus, MemoryBytes: memory}
}
