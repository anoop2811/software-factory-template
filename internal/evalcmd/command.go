// Package evalcmd runs the deterministic and golden factory evaluations.
package evalcmd

import (
	"context"
	"fmt"
	"io"
)

// Run preserves the installed evaluation routes. docs/adr/0099-native-factory-orchestration.md:99.
func Run(ctx context.Context, args []string, root string, environment map[string]string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: factory eval structural|golden|run")
		return 2
	}
	switch args[0] {
	case "structural":
		return structural(ctx, args[1:], root, environment, stdout, stderr)
	case "golden":
		return golden(ctx, args[1:], root, environment, stdout, stderr)
	case "run":
		return runner(ctx, args[1:], root, environment, stderr)
	default:
		fmt.Fprintf(stderr, "factory eval: unknown operation %q\n", args[0])
		return 2
	}
}
