// Package reportcmd renders local gate facts and explicitly labeled estimates.
package reportcmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/anoop2811/software-factory-template/internal/config"
	"github.com/anoop2811/software-factory-template/internal/fileinput"
	"github.com/anoop2811/software-factory-template/internal/native"
	"github.com/anoop2811/software-factory-template/internal/output"
)

const fileLimit = 16 << 20

// Run uses the invoked dispatcher directory for assets and the caller cwd for
// configuration discovery and relative overrides. docs/adr/0087-go-native-report.md:22.
func Run(parent context.Context, args []string, root string, environment map[string]string, stdout, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	broken := make(chan os.Signal, 1)
	signal.Notify(broken, syscall.SIGPIPE)
	defer signal.Stop(broken)
	err := run(ctx, args, root, environment, stdout)
	if ctx.Err() != nil {
		err = fmt.Errorf("report canceled: %w", ctx.Err())
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "factory report: "+err.Error())
		return 1
	}
	return 0
}

func run(ctx context.Context, args []string, root string, environment map[string]string, stdout io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	log := environment["FACTORY_EVENT_LOG"]
	if log == "" {
		log = filepath.Join(root, ".factory/events.log")
	}
	if len(args) > 0 && args[0] == "--clear" {
		// Unlink cannot remove a directory and does not follow the final symlink.
		// All removal errors are deliberately best-effort. docs/adr/0087-go-native-report.md:31.
		_ = syscall.Unlink(log)
		return output.WriteEvent(ctx, stdout, []byte("factory report: event log cleared.\n"))
	}
	effective, err := configuration(ctx, environment)
	if err != nil {
		return err
	}
	gates, err := gateCount(ctx, filepath.Join(root, "scripts/hooks"))
	if err != nil {
		return fmt.Errorf("inspect installed gates: %w", err)
	}
	events, err := optionalInput(ctx, log)
	if err != nil {
		return fmt.Errorf("read event log: %w", err)
	}
	if strings.ContainsRune(string(events), 0) {
		return errors.New("unsafe NUL-bearing event log")
	}
	review, perReview, err := reviewEstimate(environment["FACTORY_REVIEW_TOKENS"])
	if err != nil {
		return err
	}
	blocks, err := blockCount(ctx, string(events))
	if err != nil {
		return err
	}
	if blocks != 0 && perReview > math.MaxUint64/blocks {
		return errors.New("review estimate multiplication overflow")
	}
	return render(ctx, stdout, effective, gates, blocks, string(events), review, blocks*perReview)
}

func configuration(ctx context.Context, environment map[string]string) (map[string]string, error) {
	path := environment["FACTORY_CONFIG"]
	if path == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		result, err := native.ExecuteCommand(ctx, cwd, []string{"git", "rev-parse", "--show-toplevel"}, environment, 5*time.Second)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if result.OwnershipUnconfirmed || result.Outcome == "timeout" || result.Outcome == "interrupted" || result.Outcome == "output_limit" {
			return nil, errors.New("git configuration discovery did not complete safely")
		}
		root := string(result.Stdout)
		if err != nil || result.ExitCode == nil || *result.ExitCode != 0 {
			root += ".\n"
		}
		path = config.ResolvePath("", strings.ReplaceAll(root, "\x00", ""))
	}
	yaml, err := optionalInput(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("read configuration: %w", err)
	}
	legacy, err := optionalInput(ctx, config.LegacyPath(path))
	if err != nil {
		return nil, fmt.Errorf("read legacy configuration: %w", err)
	}
	var preserved []string
	for key := range environment {
		if config.KnownExportKey(key) {
			preserved = append(preserved, key)
		}
	}
	actions, err := config.ExportPlanBytes(ctx, yaml, legacy, preserved)
	if err != nil {
		return nil, err
	}
	effective := maps.Clone(environment)
	if effective == nil {
		effective = map[string]string{}
	}
	for _, action := range actions {
		if !action.ExportOnly {
			effective[action.Key] = action.Value
		}
	}
	return effective, nil
}

func optionalInput(ctx context.Context, path string) ([]byte, error) {
	data, err := fileinput.Read(ctx, path, fileLimit)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return data, err
}

func reviewEstimate(value string) (string, uint64, error) {
	if value == "" || strings.IndexFunc(value, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		value = "3000"
	}
	amount, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return "", 0, errors.New("review estimate is outside the uint64 range")
	}
	return value, amount, nil
}
