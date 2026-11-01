// Package copymanifest checks the initializer's base copy inputs without running
// initialization. docs/adr/0125-native-initializer-copy-inventory-gate.md:28.
package copymanifest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/anoop2811/software-factory-template/internal/eventlog"
	"github.com/anoop2811/software-factory-template/internal/initinventory"
	"github.com/anoop2811/software-factory-template/internal/installationfs"
	"github.com/anoop2811/software-factory-template/internal/native"
)

// OwnershipStatus preserves nested process uncertainty across the private hook
// boundary; it is never an ordinary policy refusal.
// docs/adr/0125-native-initializer-copy-inventory-gate.md:147.
const OwnershipStatus = 125

// Run receives installedRoot only from the physically admitted bridge context.
// No argument or environment variable can establish that authority.
// docs/adr/0125-native-initializer-copy-inventory-gate.md:135.
func Run(parent context.Context, subject, installedRoot string, environment map[string]string, stdout, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	root, err := filepath.Abs(subject)
	if err == nil {
		root, err = filepath.EvalSymlinks(root)
	}
	if err == nil && installedRoot != "" && root == installedRoot {
		_, _ = fmt.Fprintln(stdout, "copy-manifest-check: qualified installed inventory; source copy inputs skipped")
		return 0
	}
	if err == nil {
		err = check(ctx, root, environment)
	}
	if err == nil {
		_, _ = fmt.Fprintln(stdout, "copy-manifest-check: every native initializer copy input is safe and git-tracked")
		return 0
	}
	status := 1
	var ownership *native.OwnershipError
	if errors.As(err, &ownership) {
		status = OwnershipStatus
		_, _ = fmt.Fprintln(stderr, "copy-manifest-check: native ownership unconfirmed:", ownership.SafeDiagnostic())
	} else {
		_, _ = fmt.Fprintln(stderr, "COPY-MANIFEST FAIL:", err)
	}
	if environment["FACTORY_COPY_GATE_EVENT"] == "1" {
		_ = eventlog.Append(ctx, root, environment, "copy-manifest-check", "native initializer inventory refused")
	}
	return status
}

func check(ctx context.Context, root string, environment map[string]string) (returned error) {
	tree, err := installationfs.Open(ctx, root)
	if err != nil {
		return errors.New("source inventory root is unavailable or unsafe")
	}
	defer func() { returned = errors.Join(returned, tree.Close()) }()
	paths, err := selectedInputs(ctx, tree)
	if err != nil {
		return err
	}
	// Git receives literal argv, no caller-selected index/worktree or fsmonitor.
	// docs/adr/0125-native-initializer-copy-inventory-gate.md:142.
	env := maps.Clone(environment)
	if env == nil {
		env = map[string]string{}
	}
	for key := range env {
		if strings.HasPrefix(key, "GIT_") {
			delete(env, key)
		}
	}
	env["GIT_CONFIG_NOSYSTEM"], env["GIT_CONFIG_GLOBAL"], env["GIT_OPTIONAL_LOCKS"] = "1", os.DevNull, "0"
	args := append([]string{"git", "--literal-pathspecs", "-c", "core.fsmonitor=false", "ls-files", "-z", "--error-unmatch", "--"}, paths...)
	result, err := native.ExecuteCommand(ctx, root, args, env, 5*time.Second)
	if err != nil {
		var ownership *native.OwnershipError
		if errors.As(err, &ownership) {
			return err
		}
		return errors.New("native Git inventory check did not complete")
	}
	if result.OwnershipUnconfirmed {
		return &native.OwnershipError{}
	}
	if ctx.Err() != nil || result.Outcome != "completed" || !result.ExitConfirmed || result.ExitCode == nil || *result.ExitCode != 0 {
		return errors.New("initializer copy inputs are not all git-tracked or Git did not complete")
	}
	// Even a literal Git pathspec can select descendants. Only an exact returned
	// name proves this file is tracked. NUL framing preserves newline filenames.
	// docs/adr/0125-native-initializer-copy-inventory-gate.md:160.
	tracked := make(map[string]bool, len(paths))
	for _, path := range strings.Split(string(result.Stdout), "\x00") {
		tracked[path] = true
	}
	for _, path := range paths {
		if !tracked[path] {
			return fmt.Errorf("initializer input %q is not exactly git-tracked", path)
		}
	}
	// Directory identity does not prove unchanged membership. Reuse selection to
	// discover new optional/group inputs before reporting success.
	// docs/adr/0125-native-initializer-copy-inventory-gate.md:190.
	after, err := selectedInputs(ctx, tree)
	if err != nil {
		return err
	}
	if !slices.Equal(paths, after) {
		return errors.New("initializer copy inventory changed during validation")
	}
	return tree.Check(ctx)
}

func selectedInputs(ctx context.Context, tree *installationfs.Tree) ([]string, error) {
	paths := append(initinventory.Required(), initinventory.Readme)
	for _, path := range paths {
		if err := inspect(ctx, tree, path); err != nil {
			return nil, inputFailure("required initializer input", path, err)
		}
	}
	for _, path := range initinventory.Optional() {
		err := inspect(ctx, tree, path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, inputFailure("optional initializer input", path, err)
		}
		paths = append(paths, path)
	}
	for _, group := range initinventory.Groups() {
		members, err := groupMembers(ctx, tree, group)
		if err != nil {
			return nil, inputFailure("initializer group", group.Directory, err)
		}
		for _, path := range members {
			if err := inspect(ctx, tree, path); err != nil {
				return nil, inputFailure("initializer group input", path, err)
			}
		}
		paths = append(paths, members...)
	}
	slices.Sort(paths)
	return paths, tree.Check(ctx)
}

// Preserve useful error categories without exposing raw path-bearing errors.
// docs/adr/0125-native-initializer-copy-inventory-gate.md:206.
func inputFailure(kind, path string, cause error) error {
	var pathError *os.PathError
	switch {
	case errors.Is(cause, os.ErrNotExist):
		cause = os.ErrNotExist
	case errors.Is(cause, installationfs.ErrUnsafe):
		cause = installationfs.ErrUnsafe
	case errors.Is(cause, os.ErrPermission):
		cause = os.ErrPermission
	case errors.Is(cause, context.Canceled):
		cause = context.Canceled
	case errors.Is(cause, context.DeadlineExceeded):
		cause = context.DeadlineExceeded
	case errors.As(cause, &pathError):
		cause = errors.New("input I/O unavailable")
	}
	return fmt.Errorf("%s %q: %w", kind, path, cause)
}

func inspect(ctx context.Context, tree *installationfs.Tree, path string) error {
	parent, name := tree.Root, path
	if index := strings.LastIndexByte(path, '/'); index >= 0 {
		// Fixed inventory parents are canonical; a selected group's final leaf
		// remains literal Unix data, including backslashes.
		// docs/adr/0125-native-initializer-copy-inventory-gate.md:180.
		directory := path[:index]
		name = path[index+1:]
		var err error
		parent, _, _, err = tree.Parent(ctx, directory+"/.inventory", false)
		if err != nil {
			return err
		}
	}
	file, err := parent.OpenFile(ctx, name, 16<<20)
	if err != nil {
		return err
	}
	return errors.Join(file.Check(ctx), file.Close())
}

func groupMembers(ctx context.Context, tree *installationfs.Tree, group initinventory.Group) (paths []string, returned error) {
	parent, _, _, err := tree.Parent(ctx, group.Directory+"/.inventory", false)
	if err != nil {
		return nil, err
	}
	directory, err := parent.Root.Open(".")
	if err != nil {
		return nil, err
	}
	defer func() { returned = errors.Join(returned, directory.Close()) }()
	entries, err := directory.ReadDir(4097)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(entries) > 4096 {
		return nil, errors.New("too many initializer group entries")
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), group.Suffix) {
			paths = append(paths, group.Directory+"/"+entry.Name())
		}
	}
	if len(paths) == 0 {
		return nil, errors.New("required initializer group is empty")
	}
	return paths, tree.Check(ctx)
}
