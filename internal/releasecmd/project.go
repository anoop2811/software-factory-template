package releasecmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anoop2811/software-factory-template/internal/installationfs"
	"github.com/anoop2811/software-factory-template/internal/native"
)

// The public installer upgrades the caller's repository, including invocation
// from its subdirectories. docs/adr/0101-go-release-handoff.md:35.
func projectRoot(ctx context.Context, environment map[string]string) (string, error) {
	current, err := os.Getwd()
	if err == nil {
		current, err = filepath.EvalSymlinks(current)
	}
	if err != nil {
		return "", err
	}
	env := make(map[string]string, len(environment))
	for key, value := range environment {
		if !strings.HasPrefix(key, "GIT_") {
			env[key] = value
		}
	}
	env["LC_ALL"] = "C"
	// A normal nonzero Git status is data, not a process-supervision failure.
	// docs/adr/0101-go-release-handoff.md:490.
	result, err := native.ExecuteCommand(ctx, current, []string{"git", "rev-parse", "--show-toplevel"}, env, 5*time.Second)
	if err != nil || result.ExitCode == nil || !result.ExitConfirmed || result.OwnershipUnconfirmed {
		return "", errors.Join(err, errors.New("cannot resolve project Git boundary"))
	}
	if *result.ExitCode == 128 && strings.HasPrefix(string(result.Stderr), "fatal: not a git repository") {
		if err := installationfs.RequireNoGitMetadata(ctx, current); err != nil {
			return "", err
		}
		return current, nil
	}
	if *result.ExitCode != 0 {
		return "", errors.New("cannot resolve project Git boundary")
	}
	root := strings.TrimSuffix(string(result.Stdout), "\n")
	if !filepath.IsAbs(root) || strings.ContainsAny(root, "\x00\r\n") {
		return "", errors.New("invalid project Git root")
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, current)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return "", errors.New("project Git root is outside the caller ancestry")
	}
	return root, nil
}
