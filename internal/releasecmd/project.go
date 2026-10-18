package releasecmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	child, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := native.ExecuteCommandBounded(child, current, []string{"git", "rev-parse", "--show-toplevel"}, env, 64<<10, 64<<10)
	if err != nil || result.ExitCode == nil || !result.ExitConfirmed || result.OwnershipUnconfirmed {
		return "", errors.Join(err, errors.New("cannot resolve project Git boundary"))
	}
	if *result.ExitCode == 128 && strings.HasPrefix(string(result.Stderr), "fatal: not a git repository") {
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
