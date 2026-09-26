// Package configuredcmd shares native command preparation across entrypoints.
package configuredcmd

import (
	"context"
	"errors"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/anoop2811/software-factory-template/internal/budgetcmd"
	"github.com/anoop2811/software-factory-template/internal/commandenv"
	"github.com/anoop2811/software-factory-template/internal/loopcmd"
)

// Run prepares and runs a configured command without a script fallback.
// Preparation errors belong to the boundary; adapter diagnostics are emitted once.
// docs/adr/0078-go-public-budget-loop.md:56.
func Run(parent context.Context, kind string, args []string, environment map[string]string, stdout, stderr io.Writer) (int, error) {
	if kind != "budget" && kind != "loop" {
		return 2, errors.New("unsupported configured command")
	}
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	var err error
	if kind == "budget" {
		environment, err = commandenv.Budget(ctx, args, environment)
	} else {
		environment, err = commandenv.Loop(ctx, environment)
	}
	if err != nil {
		return 2, err
	}
	if kind == "budget" {
		return budgetcmd.Run(ctx, args, environment, stdout, stderr), nil
	}
	return loopcmd.Run(ctx, args, environment, stdout, stderr), nil
}

// CaptureEnvironment returns a detached snapshot without changing process state.
// docs/adr/0078-go-public-budget-loop.md:58.
func CaptureEnvironment() map[string]string {
	environment := make(map[string]string)
	for _, entry := range os.Environ() {
		key, value, found := strings.Cut(entry, "=")
		if found {
			environment[key] = value
		}
	}
	return environment
}
