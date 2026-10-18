package initcmd

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/anoop2811/software-factory-template/internal/installation"
	"github.com/anoop2811/software-factory-template/internal/installationfs"
	"github.com/anoop2811/software-factory-template/internal/installedlayout"
	"github.com/anoop2811/software-factory-template/internal/native"
	"github.com/anoop2811/software-factory-template/internal/staging"
	"github.com/anoop2811/software-factory-template/internal/transition"
	"golang.org/x/sys/unix"
)

type qualified struct {
	image   staging.Inspection
	runtime string
	target  string
	started bool
}

// RunQualified consumes an already authenticated, privately staged release image.
// No Bash installation is published as an intermediate selection.
// docs/adr/0101-go-release-handoff.md:148.
func RunQualified(ctx context.Context, args []string, source, runtime string, image staging.Inspection, environment map[string]string, stdin io.Reader, stdout, stderr io.Writer) int {
	return runCommand(ctx, args, source, environment, stdin, stdout, stderr, &qualified{image: image, runtime: runtime})
}

func requireFresh(ctx context.Context, target *tree) error {
	if target == nil {
		return nil
	}
	for _, path := range []string{"factory", "factory.yaml", "factory.config", ".factory", ".factory-version"} {
		var stat unix.Stat_t
		err := unix.Fstatat(int(target.root.Fd()), path, &stat, unix.AT_SYMLINK_NOFOLLOW)
		if !errors.Is(err, unix.ENOENT) {
			return errors.New("candidate init requires a fresh factory; use upgrade for an existing installation")
		}
	}
	return ctx.Err()
}

func (q *qualified) overlay(ctx context.Context, target *tree, plan []asset, inputs map[string]string) ([]asset, error) {
	selected, err := installation.Selection(ctx, q.image, inputs)
	if err != nil {
		return nil, err
	}
	assets := map[string]asset{}
	for _, entry := range plan {
		assets[entry.path] = entry
	}
	for _, retired := range []string{"scripts/lib/budget.py", "scripts/lib/budget_adapters.py", "scripts/lib/loop.py", "scripts/lib/runtime_transition.py"} {
		delete(assets, retired)
	}
	for _, entry := range selected {
		a := asset{path: entry.Path, data: entry.Data, mode: entry.Mode}
		if target != nil {
			_, _, exists, err := target.read(ctx, entry.Path)
			if err != nil {
				return nil, err
			}
			if exists {
				return nil, errors.New("candidate init preserves existing framework assets; use upgrade")
			}
		}
		if entry.Runtime {
			a.runtime = q.runtime
			a.runtimeSHA256 = q.image.Runtime.SHA256
			a.runtimeBytes = q.image.BinaryBytes
		}
		assets[entry.Path] = a
	}
	ignore := assets[".gitignore"]
	for _, rule := range installation.PrivateIgnoreRules() {
		if !strings.Contains("\n"+string(ignore.data)+"\n", "\n"+rule+"\n") {
			ignore.data = append(ignore.data, []byte("\n"+rule+"\n")...)
		}
	}
	assets[".gitignore"] = ignore
	result := make([]asset, 0, len(assets))
	for _, entry := range assets {
		result = append(result, entry)
	}
	priority := func(path string) int {
		switch path {
		case ".factory/installation.current":
			return 1
		case "factory":
			return 2
		case ".factory-version":
			return 3
		}
		return 0
	}
	sort.Slice(result, func(i, j int) bool {
		if priority(result[i].path) != priority(result[j].path) {
			return priority(result[i].path) < priority(result[j].path)
		}
		return result[i].path < result[j].path
	})
	return result, nil
}

func (q *qualified) begin(ctx context.Context, target *tree, plan []asset, environment map[string]string) (func(context.Context) error, error) {
	lease, err := transition.RootExclusive(ctx, target.path)
	if err != nil {
		return nil, err
	}
	if err := requireFresh(ctx, target); err != nil {
		return nil, errors.Join(err, lease.Close(context.WithoutCancel(ctx)))
	}
	var ignore asset
	for _, entry := range plan {
		if entry.path == ".gitignore" {
			ignore = entry
		}
	}
	if err := proveIgnoreRules(ctx, ignore.data, environment); err != nil {
		return nil, errors.Join(err, lease.Close(context.WithoutCancel(ctx)))
	}
	if err := target.publish(ctx, ignore); err != nil {
		return nil, errors.Join(err, lease.Close(context.WithoutCancel(ctx)))
	}
	if err := checkPrivateIgnore(ctx, target, environment); err != nil {
		return nil, errors.Join(err, lease.Close(context.WithoutCancel(ctx)))
	}
	if err := target.publish(ctx, asset{path: ".factory/runtime-publication.pending", mode: 0600}); err != nil {
		return nil, errors.Join(err, lease.Close(context.WithoutCancel(ctx)))
	}
	q.target, q.started = target.path, true
	return lease.Close, nil
}

func (q *qualified) activate(ctx context.Context, target *tree, plan []asset, environment map[string]string) (returned error) {
	tree, err := installationfs.Open(ctx, target.path)
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, tree.Close()) }()
	if _, err := installedlayout.ValidateAssets(ctx, tree, []string{q.image.Runtime.Version, q.image.Manifest.SourceRevision, q.image.Runtime.SHA256}); err != nil {
		return err
	}
	if err := checkPrivateIgnore(ctx, target, environment); err != nil {
		return err
	}
	for _, entry := range plan {
		if entry.path == ".factory-version" {
			if err := target.publish(ctx, entry); err != nil {
				return err
			}
		}
	}
	if _, err := installedlayout.Validate(ctx, tree, nil); err != nil {
		return err
	}
	pending, err := tree.OpenFile(ctx, ".factory/runtime-publication.pending", 0)
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, pending.Close()) }()
	if err := pending.Check(ctx); err != nil {
		return err
	}
	if err := pending.Parent.Root.Remove(pending.Name); err != nil {
		return err
	}
	return pending.Parent.Sync(ctx)
}

var privatePaths = []string{".factory/bin/factory-runtime", ".factory/assets/scaffold/factory", ".factory/installation.current", ".factory/installation-transactions/ignore-proof", ".factory/backups/ignore-proof"}

func ignoreEnvironment(environment map[string]string, root string) map[string]string {
	env := childEnvironment(environment, root)
	delete(env, "GIT_CEILING_DIRECTORIES")
	env["LC_ALL"] = "C"
	return env
}

func checkPrivateIgnore(ctx context.Context, target *tree, environment map[string]string) error {
	data, _, found, err := target.read(ctx, ".gitignore")
	if err != nil || !found {
		return errors.Join(err, errors.New("installation ignore rules are unavailable"))
	}
	result, err := native.ExecuteCommand(ctx, target.path, []string{"git", "rev-parse", "--is-inside-work-tree"}, ignoreEnvironment(environment, target.path), 5*time.Second)
	if err != nil || result.ExitCode == nil || !result.ExitConfirmed || result.OwnershipUnconfirmed {
		return errors.Join(err, errors.New("cannot inspect installation Git boundary"))
	}
	if *result.ExitCode == 128 && strings.HasPrefix(string(result.Stderr), "fatal: not a git repository") {
		if err := noGitMetadata(target.path); err != nil {
			return err
		}
		return proveIgnoreRules(ctx, data, environment)
	}
	if *result.ExitCode != 0 || string(result.Stdout) != "true\n" {
		return errors.New("installation Git boundary is unavailable")
	}
	return checkIgnoredPaths(ctx, target.path, environment)
}

// Exit 128 alone includes broken Git metadata and operational failures.
func noGitMetadata(path string) error {
	for {
		if _, err := os.Lstat(filepath.Join(path, ".git")); !errors.Is(err, os.ErrNotExist) {
			return errors.Join(err, errors.New("git metadata exists or cannot be inspected; detached ignore proof refused"))
		}
		parent := filepath.Dir(path)
		if parent == path {
			return nil
		}
		path = parent
	}
}

func checkIgnoredPaths(ctx context.Context, root string, environment map[string]string) error {
	for _, path := range privatePaths {
		for _, argv := range [][]string{{"git", "ls-files", "--", path}, {"git", "check-ignore", "--no-index", "-q", "--", path}} {
			result, err := native.ExecuteCommand(ctx, root, argv, ignoreEnvironment(environment, root), 5*time.Second)
			if err != nil || result.ExitCode == nil || *result.ExitCode != 0 || !result.ExitConfirmed || result.OwnershipUnconfirmed || len(result.Stdout) != 0 {
				return errors.Join(err, errors.New("private installation data must be untracked and effectively ignored"))
			}
		}
	}
	return ctx.Err()
}

// A new target need not acquire Git metadata just to prove its future ignores.
// docs/adr/0101-go-release-handoff.md:351.
func proveIgnoreRules(ctx context.Context, data []byte, environment map[string]string) (returned error) {
	root, err := os.MkdirTemp("", "factory-init-ignore-")
	if err != nil {
		return err
	}
	identity, err := os.Lstat(root)
	if err != nil {
		return err
	}
	defer func() {
		current, err := os.Lstat(root)
		if err != nil || !os.SameFile(identity, current) {
			returned = errors.Join(returned, err, errors.New("ignore proof scratch changed"))
		} else {
			returned = errors.Join(returned, os.RemoveAll(root))
		}
	}()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), data, 0600); err != nil {
		return err
	}
	result, err := native.ExecuteCommand(ctx, root, []string{"git", "init", "--quiet"}, ignoreEnvironment(environment, root), 5*time.Second)
	if err != nil || result.ExitCode == nil || *result.ExitCode != 0 {
		return errors.Join(err, errors.New("cannot prepare isolated ignore proof"))
	}
	return checkIgnoredPaths(ctx, root, environment)
}

// Only this fixed private binary may exceed the normal initializer file bound.
// Publication uses the same checked rename path as other initializer assets.
func runtimeReader(ctx context.Context, a asset) (*installationfs.Tree, *installationfs.File, error) {
	tree, err := installationfs.Open(ctx, filepath.Dir(a.runtime))
	if err != nil {
		return nil, nil, err
	}
	file, err := tree.OpenFile(ctx, filepath.Base(a.runtime), 256<<20)
	if err != nil {
		return nil, nil, errors.Join(err, tree.Close())
	}
	if a.path != ".factory/bin/factory-runtime" || file.Metadata().Size != a.runtimeBytes {
		return nil, nil, errors.Join(errors.New("invalid candidate runtime"), file.Close(), tree.Close())
	}
	return tree, file, nil
}
