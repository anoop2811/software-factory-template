package initcmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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

// PrepareCI reuses confined initializer publication for exact disposable originals.
// Authentication belongs to the release owner. docs/adr/0143-ci-checkout-preparation.md:43.
func PrepareCI(ctx context.Context, root string, origin, host staging.Inspection, runtime string, environment map[string]string) (returned error) {
	path, err := physicalPath(root)
	if err != nil {
		return err
	}
	target, err := openTree(path)
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, target.root.Close()) }()
	if _, err := ciPlan(ctx, target, origin, host, runtime, environment); err != nil {
		return err
	}
	lease, err := transition.RootExclusive(ctx, path)
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, lease.Close(context.WithoutCancel(ctx))) }()
	opened, err := target.root.Stat()
	if err != nil {
		return err
	}
	current, err := os.Lstat(path)
	if err != nil || !os.SameFile(opened, current) {
		return errors.Join(err, errors.New("CI checkout root changed"))
	}
	plan, err := ciPlan(ctx, target, origin, host, runtime, environment)
	if err != nil {
		return err
	}
	if err := lease.Check(ctx); err != nil {
		return err
	}
	// Barrier publication can fail after its rename; retain recovery guidance.
	// docs/adr/0143-ci-checkout-preparation.md:115.
	defer func() {
		if returned != nil {
			returned = errors.Join(returned, errors.New("CI publication may be incomplete; preserve any pending evidence and use a fresh disposable checkout"))
		}
	}()
	if err := target.publish(ctx, asset{path: ".factory/runtime-publication.pending", mode: 0600}); err != nil {
		return err
	}
	for _, entry := range plan {
		if entry.path == ".factory-version" {
			continue
		}
		if err := lease.Check(ctx); err != nil {
			return err
		}
		if err := target.publishWithBackup(ctx, entry, false); err != nil {
			return err
		}
	}
	if err := lease.Check(ctx); err != nil {
		return err
	}
	return activateCI(ctx, target, host, plan, environment, lease.Check)
}

func ciPlan(ctx context.Context, target *tree, origin, host staging.Inspection, runtime string, environment map[string]string) ([]asset, error) {
	// Existing custody, including empty roots, is never a preparation target.
	// docs/adr/0143-ci-checkout-preparation.md:75.
	for _, path := range []string{".factory/bin", ".factory/assets", ".factory/backups", ".factory/installation-transactions", ".factory/installation.current", ".factory/runtime-publication.pending"} {
		parent, name, err := target.parent(ctx, path, false)
		if errors.Is(err, unix.ENOENT) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var stat unix.Stat_t
		err = unix.Fstatat(int(parent.Fd()), name, &stat, unix.AT_SYMLINK_NOFOLLOW)
		closeErr := parent.Close()
		if err == nil || !errors.Is(err, unix.ENOENT) || closeErr != nil {
			return nil, errors.Join(closeErr, errors.New("CI runtime custody already exists or is unsafe; use a fresh disposable checkout"))
		}
	}
	if origin.Runtime.Version != host.Runtime.Version || origin.Manifest.SourceRevision != host.Manifest.SourceRevision {
		return nil, errors.New("CI release identity differs")
	}
	inputs, err := installation.ProjectInputs(ctx, target.path, nil)
	if err != nil {
		return nil, err
	}
	before, err := installation.Selection(ctx, origin, inputs)
	if err != nil {
		return nil, err
	}
	after, err := installation.Selection(ctx, host, inputs)
	if err != nil {
		return nil, err
	}
	originals := make(map[string]installation.SelectedAsset, len(before))
	for _, entry := range before {
		originals[entry.Path] = entry
	}
	if len(originals) != len(after) {
		return nil, errors.New("CI origin and host selections differ")
	}
	var plan []asset
	var public, private []string
	for _, entry := range after {
		original, found := originals[entry.Path]
		if !found || original.Runtime != entry.Runtime {
			return nil, errors.New("CI origin and host selections differ")
		}
		data, mode, exists, identity, err := target.observe(ctx, entry.Path)
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(entry.Path, ".factory/") {
			private = append(private, entry.Path)
			if exists {
				return nil, errors.New("CI runtime custody already exists; use a fresh disposable checkout")
			}
		} else {
			public = append(public, entry.Path)
			if !exists || mode != original.Mode || !bytes.Equal(data, original.Data) {
				return nil, fmt.Errorf("CI public asset does not match authenticated origin: %s", entry.Path)
			}
		}
		a := asset{path: entry.Path, data: entry.Data, mode: entry.Mode, previous: data, previousMode: mode, previousIdentity: identity, existed: exists}
		if entry.Runtime {
			a.runtime, a.runtimeSHA256, a.runtimeBytes = runtime, host.Runtime.SHA256, host.BinaryBytes
		}
		plan = append(plan, a)
	}
	if _, _, found, err := target.read(ctx, ".factory/runtime-publication.pending"); err != nil || found {
		return nil, errors.Join(err, errors.New("CI publication is pending; use a fresh disposable checkout"))
	}
	git, err := ciGit(ctx, target.path, environment)
	if err != nil {
		return nil, err
	}
	remaining := 128
	if err := ciCleanGit(ctx, target, git, 0, &remaining); err != nil {
		return nil, err
	}
	tracked, err := git.run(ctx, append([]string{"ls-files", "--cached", "-z", "--"}, public...)...)
	if err != nil || !ciPathsEqual(tracked, public) {
		return nil, errors.Join(err, errors.New("CI public assets must all be tracked"))
	}
	if err := ciPrivateIgnore(ctx, git, private); err != nil {
		return nil, err
	}
	ignore, _, exists, err := target.read(ctx, ".gitignore")
	if err != nil || !exists {
		return nil, errors.Join(err, errors.New("CI checkout ignore rules are unavailable"))
	}
	if err := proveIgnoreRules(ctx, ignore, environment); err != nil {
		return nil, err
	}
	priority := func(path string) int {
		switch path {
		case ".factory/installation.current":
			return 1
		case "factory":
			return 2
		case ".factory-version":
			return 3
		default:
			return 0
		}
	}
	sort.Slice(plan, func(i, j int) bool {
		if priority(plan[i].path) != priority(plan[j].path) {
			return priority(plan[i].path) < priority(plan[j].path)
		}
		return plan[i].path < plan[j].path
	})
	return plan, ctx.Err()
}

// Worktree diffs must not delegate inspection to unhardened submodule readers.
// docs/adr/0143-ci-checkout-preparation.md:105.
func ciCleanGit(ctx context.Context, target *tree, git *ciGitReader, depth int, remaining *int) error {
	opened, err := target.root.Stat()
	if err != nil {
		return err
	}
	current, err := os.Lstat(target.path)
	if err != nil || !os.SameFile(opened, current) {
		return errors.Join(err, errors.New("CI Git worktree root changed"))
	}
	top, err := git.run(ctx, "rev-parse", "--show-toplevel")
	if err != nil || strings.TrimSuffix(string(top), "\n") != target.path {
		return errors.Join(err, errors.New("CI preparation requires the checkout root"))
	}
	for _, args := range [][]string{
		{"diff", "--cached", "--quiet", "--no-ext-diff", "--no-textconv", "--ignore-submodules=none", "HEAD", "--"},
		{"diff", "--quiet", "--no-ext-diff", "--no-textconv", "--ignore-submodules=all", "--"},
	} {
		if _, err := git.run(ctx, args...); err != nil {
			return errors.Join(err, errors.New("CI preparation requires a clean tracked index and worktree"))
		}
	}
	flags, err := git.run(ctx, "ls-files", "-v", "-z")
	if err != nil {
		return err
	}
	for _, entry := range bytes.Split(flags, []byte{0}) {
		if len(entry) != 0 && (entry[0] == 'S' || entry[0] >= 'a' && entry[0] <= 'z') {
			return errors.New("CI tracked files cannot use assume-unchanged or skip-worktree")
		}
	}
	entries, err := git.run(ctx, "ls-files", "--stage", "-z")
	if err != nil {
		return err
	}
	for _, entry := range bytes.Split(entries, []byte{0}) {
		if len(entry) == 0 {
			continue
		}
		header, path, found := strings.Cut(string(entry), "\t")
		fields := strings.Fields(header)
		if !found || len(fields) != 3 || fields[2] != "0" {
			return errors.New("CI index is ambiguous")
		}
		if fields[0] != "160000" {
			continue
		}
		if depth >= 16 || *remaining <= 0 {
			return errors.New("CI submodule inspection limit exceeded")
		}
		*remaining--
		if err := ciGitlink(ctx, target, path, fields[1], git.environment, depth+1, remaining); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func ciGitlink(ctx context.Context, target *tree, path, oid string, environment map[string]string, depth int, remaining *int) (returned error) {
	directory, name, err := target.parent(ctx, path+"/.git", false)
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, directory.Close()) }()
	entries, err := directory.ReadDir(1)
	if len(entries) == 0 && errors.Is(err, io.EOF) {
		return ctx.Err()
	}
	if err != nil {
		return err
	}
	var metadata unix.Stat_t
	if err := unix.Fstatat(int(directory.Fd()), name, &metadata, unix.AT_SYMLINK_NOFOLLOW); err != nil || (!safeDirectory(metadata) && !safeFile(metadata)) {
		return errors.Join(err, errors.New("CI submodule metadata is unsafe or ambiguous"))
	}
	child := &tree{path: filepath.Join(target.path, path), root: directory}
	git, err := ciGit(ctx, child.path, environment)
	if err != nil {
		return err
	}
	head, err := git.run(ctx, "rev-parse", "--verify", "HEAD")
	if err != nil || strings.TrimSuffix(string(head), "\n") != oid {
		return errors.Join(err, errors.New("CI submodule commit differs from its recorded gitlink"))
	}
	return ciCleanGit(ctx, child, git, depth, remaining)
}

// CI keeps frozen initializer sequencing intact while using its validation and
// confined custody primitives with hardened exhaustive Git inspection.
// docs/adr/0143-ci-checkout-preparation.md:97.
func activateCI(ctx context.Context, target *tree, image staging.Inspection, plan []asset, environment map[string]string, checkLease func(context.Context) error) (returned error) {
	tree, err := installationfs.Open(ctx, target.path)
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, tree.Close()) }()
	if _, err := installedlayout.ValidateAssets(ctx, tree, []string{image.Runtime.Version, image.Manifest.SourceRevision, image.Runtime.SHA256}); err != nil {
		return err
	}
	git, err := ciGit(ctx, target.path, environment)
	if err != nil {
		return err
	}
	var private []string
	for _, entry := range plan {
		if strings.HasPrefix(entry.path, ".factory/") {
			private = append(private, entry.path)
		}
	}
	if err := ciPrivateIgnore(ctx, git, private); err != nil {
		return err
	}
	// The same release marker is retained byte-for-byte across platform changes.
	if _, err := installedlayout.Validate(ctx, tree, nil); err != nil {
		return err
	}
	pending, err := tree.OpenFile(ctx, ".factory/runtime-publication.pending", 0)
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, pending.Close()) }()
	if err := checkLease(ctx); err != nil {
		return err
	}
	if err := pending.Check(ctx); err != nil {
		return err
	}
	if err := pending.Parent.Root.Remove(pending.Name); err != nil {
		return err
	}
	return pending.Parent.Sync(ctx)
}

func ciPrivateIgnore(ctx context.Context, git *ciGitReader, private []string) error {
	tracked, err := git.run(ctx, append([]string{"ls-files", "--cached", "-z", "--"}, private...)...)
	if err != nil || len(tracked) != 0 {
		return errors.Join(err, errors.New("CI private assets must be untracked"))
	}
	ignored, err := git.runInput(ctx, strings.Join(private, "\x00")+"\x00", "check-ignore", "--no-index", "--stdin", "-z")
	if err != nil || !ciPathsEqual(ignored, private) {
		return errors.Join(err, errors.New("CI private assets must all be effectively ignored"))
	}
	return nil
}

func ciPathsEqual(data []byte, expected []string) bool {
	paths := bytes.Split(data, []byte{0})
	if len(paths) != len(expected)+1 || len(paths[len(paths)-1]) != 0 {
		return false
	}
	want := make(map[string]bool, len(expected))
	for _, path := range expected {
		want[path] = true
	}
	for _, path := range paths[:len(paths)-1] {
		if !want[string(path)] {
			return false
		}
		delete(want, string(path))
	}
	return len(want) == 0
}

type ciGitReader struct {
	root        string
	environment map[string]string
	options     []string
}

func ciGit(ctx context.Context, root string, environment map[string]string) (*ciGitReader, error) {
	env := ignoreEnvironment(environment, root)
	env["GIT_OPTIONAL_LOCKS"] = "0"
	reader := &ciGitReader{root: root, environment: env, options: []string{"--no-optional-locks", "-c", "core.fsmonitor=false", "-c", "core.untrackedCache=false", "-c", "core.hooksPath=/dev/null"}}
	// Worktree comparisons must not execute repository-configured filters.
	result, err := reader.command(ctx, "", "config", "--null", "--name-only", "--list")
	if err != nil {
		return nil, err
	}
	if result.ExitCode == nil || *result.ExitCode != 0 {
		return nil, errors.New("CI filter configuration cannot be inspected")
	}
	for _, raw := range bytes.Split(result.Stdout, []byte{0}) {
		key := string(raw)
		_, field, _ := strings.Cut(key, "filter.")
		if !strings.HasPrefix(key, "filter.") || (!strings.HasSuffix(field, ".clean") && !strings.HasSuffix(field, ".smudge") && !strings.HasSuffix(field, ".process") && !strings.HasSuffix(field, ".required")) {
			continue
		}
		if strings.ContainsAny(key, "=\r\n") {
			return nil, errors.New("CI filter configuration is unsupported")
		}
		value := ""
		if strings.HasSuffix(key, ".required") {
			value = "false"
		}
		reader.options = append(reader.options, "-c", key+"="+value)
	}
	return reader, nil
}

func (git *ciGitReader) command(ctx context.Context, input string, args ...string) (native.CommandResult, error) {
	child, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	argv := append([]string{"git"}, git.options...)
	// check-ignore consumes literal filenames and rejects pathspec magic.
	if len(args) != 0 && args[0] != "check-ignore" {
		argv = append(argv, "--literal-pathspecs")
	}
	result, err := native.ExecuteCommandWithOptions(child, git.root, append(argv, args...), git.environment, native.CommandOptions{Input: input, StdoutLimit: 1 << 20, StderrLimit: 64 << 10})
	if err != nil || !result.ExitConfirmed || result.OwnershipUnconfirmed || (result.Outcome != "completed" && result.Outcome != "failed") {
		return result, errors.Join(err, errors.New("CI Git inspection did not complete safely"))
	}
	return result, ctx.Err()
}

func (git *ciGitReader) run(ctx context.Context, args ...string) ([]byte, error) {
	return git.runInput(ctx, "", args...)
}

func (git *ciGitReader) runInput(ctx context.Context, input string, args ...string) ([]byte, error) {
	if len(input) > 1<<20 {
		return nil, errors.New("CI Git input limit exceeded")
	}
	result, err := git.command(ctx, input, args...)
	if err != nil || result.ExitCode == nil || *result.ExitCode != 0 {
		return nil, errors.Join(err, errors.New("CI Git inspection failed"))
	}
	return result.Stdout, nil
}
