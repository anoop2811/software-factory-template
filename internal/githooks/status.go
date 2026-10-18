// Package githooks classifies the hook path Git actually resolves.
package githooks

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anoop2811/software-factory-template/internal/native"
	"golang.org/x/sys/unix"
)

// Status separates configured hooks from executable hooks without changing Git
// configuration. docs/adr/0099-native-factory-orchestration.md:139.
func Status(ctx context.Context, root string, environment map[string]string) (state, path string, err error) {
	probe := func(args ...string) (string, error) {
		result, err := native.ExecuteCommand(ctx, root, append([]string{"git"}, args...), environment, 10*time.Second)
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if result.OwnershipUnconfirmed || result.Outcome == "timeout" || result.Outcome == "interrupted" || result.Outcome == "output_limit" {
			return "", errors.New("git hooksPath probe did not complete safely")
		}
		if err != nil || result.ExitCode == nil || *result.ExitCode != 0 {
			//nolint:nilerr // Unavailable Git retains the legacy warning classification: docs/adr/0099-native-factory-orchestration.md:139.
			return "", nil
		}
		return strings.TrimRight(string(result.Stdout), "\n"), nil
	}
	path, err = probe("rev-parse", "--git-path", "hooks/pre-push")
	if err != nil {
		return "", "", err
	}
	if !filepath.IsAbs(path) {
		path = root + "/" + path
	}
	configured, err := probe("config", "--get", "core.hooksPath")
	if err != nil {
		return "", "", err
	}
	if path == root+"/.githooks/pre-push" {
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() && unix.Access(path, unix.X_OK) == nil {
			return "armed", path, nil
		}
		return "inert", path, nil
	}
	if configured != "" {
		return "hijacked", path, nil
	}
	return "absent", path, nil
}
