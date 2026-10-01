// Package migrateconfigcmd migrates legacy configuration as inert local data.
package migrateconfigcmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/anoop2811/software-factory-template/internal/config"
	"github.com/anoop2811/software-factory-template/internal/native"
	"github.com/anoop2811/software-factory-template/internal/output"
)

type commandError struct {
	message string
	status  int
}

func (e *commandError) Error() string { return e.message }

// Run owns signal handling and bounded Git discovery; it never changes cwd.
// docs/adr/0090-go-native-config-migration.md:18.
func Run(parent context.Context, args []string, environment map[string]string, stdout, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	// Keep SIGPIPE from terminating checked writes to stdout/stderr. The channel
	// is intentionally unread: WriteEvent reports the write failure synchronously.
	broken := make(chan os.Signal, 1)
	signal.Notify(broken, syscall.SIGPIPE)
	defer signal.Stop(broken)
	root, err := discover(ctx, environment)
	if err == nil {
		err = Execute(ctx, args, root, environment, stdout)
	}
	if err == nil {
		return 0
	}
	status := 1
	var failure *commandError
	if errors.As(err, &failure) {
		status = failure.status
	}
	diagnostic, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()
	_ = output.WriteEvent(diagnostic, stderr, []byte(err.Error()+"\n"))
	return status
}

func discover(ctx context.Context, environment map[string]string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	result, err := native.ExecuteCommand(ctx, cwd, []string{"git", "rev-parse", "--show-toplevel"}, environment, 10*time.Second)
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if result.OwnershipUnconfirmed || result.Outcome == "timeout" || result.Outcome == "interrupted" || result.Outcome == "output_limit" {
		return "", errors.New("factory migrate-config: root discovery did not complete safely")
	}
	if err != nil || result.ExitCode == nil || *result.ExitCode != 0 {
		//nolint:nilerr // Ordinary Git failure uses cwd: docs/adr/0090-go-native-config-migration.md:20.
		return cwd, nil
	}
	root := strings.TrimRight(string(result.Stdout), "\n")
	if root == "" || strings.ContainsRune(root, 0) {
		return "", errors.New("factory migrate-config: invalid Git root")
	}
	return root, nil
}

// Execute prepares the complete change before publishing YAML or renaming input.
// docs/adr/0090-go-native-config-migration.md:65.
func Execute(ctx context.Context, args []string, root string, environment map[string]string, stdout io.Writer) (returned error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	dry := false
	for _, arg := range args {
		if arg != "--dry-run" {
			return &commandError{fmt.Sprintf("factory migrate-config: unknown argument '%s'", arg), 2}
		}
		dry = true
	}
	project, err := openProject(ctx, root)
	if err != nil {
		return fmt.Errorf("factory migrate-config: %w", err)
	}
	defer func() { returned = errors.Join(returned, project.directory.Close()) }()
	yaml, err := project.read(ctx, "factory.yaml")
	if errors.Is(err, os.ErrNotExist) {
		//nolint:revive,staticcheck // Preserve the frozen diagnostic punctuation: docs/adr/0090-go-native-config-migration.md:23.
		return errors.New("factory migrate-config: no factory.yaml here — run 'factory init' first.")
	}
	if err != nil {
		return fmt.Errorf("factory migrate-config: YAML: %w", err)
	}
	legacy, err := project.read(ctx, "factory.config")
	if errors.Is(err, os.ErrNotExist) {
		return output.WriteEvent(ctx, stdout, []byte("factory migrate-config: no factory.config — nothing to migrate.\n"))
	}
	if err != nil {
		return fmt.Errorf("factory migrate-config: legacy: %w", err)
	}
	if selected := environment["FACTORY_CONFIG"]; selected != "" {
		if !filepath.IsAbs(selected) {
			selected = root + "/" + selected
		}
		info, err := os.Stat(selected)
		if err != nil || !os.SameFile(yaml.info, info) {
			return errors.New("factory migrate-config: FACTORY_CONFIG must identify the project factory.yaml")
		}
	}
	if err := project.noBackup(); err != nil {
		return err
	}
	change, err := buildPlan(ctx, yaml.data, legacy.data, dry)
	if err != nil {
		return err
	}
	guard := func(ctx context.Context) error {
		if err := project.check(ctx); err != nil {
			return err
		}
		if err := project.unchanged(ctx, "factory.config", legacy); err != nil {
			return err
		}
		return project.noBackup()
	}
	if dry {
		if err := guard(ctx); err != nil {
			return err
		}
		if err := project.unchanged(ctx, "factory.yaml", yaml); err != nil {
			return err
		}
		return output.WriteEvent(ctx, stdout, change.output)
	}
	if yaml.info.Mode().Perm()&0200 == 0 {
		return errors.New("factory migrate-config: YAML must be writable")
	}
	if project.identity.Mode().Perm()&0300 != 0300 {
		return errors.New("factory migrate-config: project directory must be writable and searchable")
	}
	if err := guard(ctx); err != nil {
		return err
	}
	err = config.Transform(ctx, root+"/factory.yaml", func(ctx context.Context, original []byte) ([]byte, error) {
		if err := project.unchanged(ctx, "factory.yaml", yaml); err != nil {
			return nil, err
		}
		if !bytes.Equal(original, yaml.data) {
			return nil, errors.New("factory migrate-config: YAML changed before transformation")
		}
		return change.yaml, ctx.Err()
	}, guard)
	if err != nil {
		return err
	}
	if err := guard(ctx); err != nil {
		return partial(err)
	}
	if err := project.renameLegacy(ctx, legacy); err != nil {
		return partial(err)
	}
	return output.WriteEvent(ctx, stdout, change.output)
}

func partial(err error) error {
	return fmt.Errorf("factory migrate-config: YAML was updated, but legacy rename did not complete; inspect factory.config and factory.config.migrated before retrying: %w", err)
}
