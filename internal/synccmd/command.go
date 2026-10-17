// Package synccmd renders native harness adapters from the canonical assets.
package synccmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/anoop2811/software-factory-template/internal/config"
	"github.com/anoop2811/software-factory-template/internal/fileinput"
	"github.com/anoop2811/software-factory-template/internal/installationfs"
	"github.com/anoop2811/software-factory-template/internal/native"
	"github.com/anoop2811/software-factory-template/internal/output"
)

const inputLimit = 16 << 20

// ErrUnsafe distinguishes ownership and I/O refusals from advisory source errors
// for init and doctor. docs/adr/0099-native-factory-orchestration.md:45.
var ErrUnsafe = errors.New("unsafe synchronization operation")

var errUsage = errors.New("usage: factory sync [all|opencode|claude|codex]")

// Run implements the public selector contract. The caller owns signal handling.
// docs/adr/0099-native-factory-orchestration.md:26.
func Run(ctx context.Context, args []string, root string, environment map[string]string, stdout, stderr io.Writer) int {
	err := Execute(ctx, args, root, environment, stdout, stderr)
	if err == nil {
		return 0
	}
	_, _ = fmt.Fprintf(stderr, "factory sync: %v\n", err)
	if errors.Is(err, errUsage) {
		return 2
	}
	return 1
}

// Execute prepares the complete selection before publishing individual files.
// It never changes the process environment or cwd. Cancellation is preserved as
// a context error; unsafe ownership or publication failures wrap ErrUnsafe.
// docs/adr/0099-native-factory-orchestration.md:63.
func Execute(ctx context.Context, args []string, root string, environment map[string]string, stdout, _ io.Writer) (returned error) {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		return unsafe(output.WriteEvent(ctx, stdout, []byte(errUsage.Error()+"\n")))
	}
	selector := "all"
	if len(args) > 1 {
		return errUsage
	}
	if len(args) == 1 {
		selector = args[0]
	}
	if selector != "all" && selector != "opencode" && selector != "claude" && selector != "codex" {
		return errUsage
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	settings, active, err := configuration(ctx, environment)
	if err != nil {
		return err
	}
	for key, value := range settings {
		if strings.HasSuffix(key, "_MODEL") && (!utf8.ValidString(value) || strings.ContainsRune(value, 0)) {
			return errors.New("unsupported model text")
		}
	}
	if selector == "opencode" && !active {
		return unsafe(output.WriteEvent(ctx, stdout, []byte("sync-opencode: no harness settings configured — leaving opencode.json as-is\n")))
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return unsafe(err)
	}
	tree, err := installationfs.Open(ctx, root)
	if err != nil {
		return unsafe(err)
	}
	defer func() { returned = errors.Join(returned, unsafe(tree.Close())) }()
	source, err := readSource(ctx, tree)
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, source.close()) }()
	plan := &publication{tree: tree}
	defer func() { returned = errors.Join(returned, plan.close()) }()
	for _, harness := range []string{"opencode", "claude", "codex"} {
		if selector != "all" && selector != harness {
			continue
		}
		if err := render(ctx, plan, source, harness, settings, active); err != nil {
			return err
		}
	}
	if err := plan.inspect(ctx); err != nil {
		return err
	}
	if err := source.check(ctx); err != nil {
		return unsafe(err)
	}
	if err := plan.prepare(ctx); err != nil {
		return err
	}
	if err := source.check(ctx); err != nil {
		return unsafe(err)
	}
	if err := plan.publish(ctx); err != nil {
		return err
	}
	return unsafe(output.WriteEvent(ctx, stdout, []byte(strings.Join(plan.messages, "\n")+"\n")))
}

func unsafe(err error) error {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return fmt.Errorf("%w: %w", ErrUnsafe, err)
}

func configuration(ctx context.Context, environment map[string]string) (map[string]string, bool, error) {
	path := environment["FACTORY_CONFIG"]
	if path == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, false, unsafe(err)
		}
		result, commandErr := native.ExecuteCommand(ctx, cwd, []string{"git", "rev-parse", "--show-toplevel"}, environment, 5*time.Second)
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		if result.OwnershipUnconfirmed || result.Outcome == "timeout" || result.Outcome == "interrupted" || result.Outcome == "output_limit" {
			return nil, false, unsafe(errors.New("configuration discovery did not complete safely"))
		}
		base := "."
		if commandErr == nil && result.ExitCode != nil && *result.ExitCode == 0 {
			base = strings.TrimRight(string(result.Stdout), "\n")
		}
		path = config.ResolvePath("", base)
	}
	yaml, _, err := optionalInput(ctx, path)
	if err != nil {
		return nil, false, fmt.Errorf("read configuration: %w", err)
	}
	legacy, legacyExists, err := optionalInput(ctx, config.LegacyPath(path))
	if err != nil {
		return nil, false, fmt.Errorf("read legacy configuration: %w", err)
	}
	effective, err := config.EffectiveBytes(ctx, yaml, legacy, environment)
	if err != nil {
		return nil, false, err
	}
	active := legacyExists
	for _, key := range []string{"cost_profile", "model_provider", "opencode_frontier_model", "opencode_default_model", "opencode_economy_model"} {
		for _, line := range strings.Split(string(yaml), "\n") {
			active = active || strings.HasPrefix(line, key+":")
		}
	}
	return effective, active, nil
}

func optionalInput(ctx context.Context, path string) ([]byte, bool, error) {
	data, err := fileinput.Read(ctx, path, inputLimit)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	return data, err == nil, unsafe(err)
}
