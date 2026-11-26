package contextcmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/anoop2811/software-factory-template/internal/configuredcmd"
	"github.com/anoop2811/software-factory-template/internal/fileinput"
	"github.com/anoop2811/software-factory-template/internal/native"
	"golang.org/x/sys/unix"
)

type sourceFile struct {
	path string
	data []byte
}

func normalizePath(raw string) (string, error) {
	if raw == "" || filepath.IsAbs(raw) || !utf8.ValidString(raw) || strings.IndexByte(raw, 0) >= 0 {
		return "", invalid("invalid worktree-relative file %s", safeName(raw))
	}
	for part := range strings.SplitSeq(raw, "/") {
		if part == ".." || strings.EqualFold(part, ".git") || strings.EqualFold(part, ".factory") {
			return "", invalid("traversal or private path %s", safeName(raw))
		}
	}
	if filepath.Clean(raw) == "." || strings.HasSuffix(raw, "/") || strings.HasSuffix(raw, "/.") {
		return "", invalid("expected a file path: %s", safeName(raw))
	}
	return filepath.Clean(raw), nil
}

type worktree struct {
	root    *os.File
	private []os.FileInfo
}

// Inspect Git's normal ignore configuration, with repository identity taken from
// the caller's physical worktree. Suppress optional writes and executable fsmonitor hooks.
func queryGit(ctx context.Context, cwd string, env map[string]string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	argv := append([]string{"git", "--no-optional-locks", "--literal-pathspecs", "-c", "core.fsmonitor=false", "-c", "core.untrackedCache=false"}, args...)
	result, err := native.ExecuteCommandBounded(ctx, cwd, argv, env, 1<<20, 4096)
	// Git can warn about unreadable ignore rules and still exit successfully.
	if args[0] == "ls-files" && len(result.Stderr) != 0 {
		return nil, errors.New("cannot establish Git file eligibility")
	}
	if err != nil || result.ExitCode == nil || *result.ExitCode != 0 {
		outside := bytes.HasPrefix(result.Stderr, []byte("fatal: not a git repository")) ||
			bytes.HasPrefix(result.Stderr, []byte("fatal: this operation must be run in a work tree"))
		if args[0] == "rev-parse" && args[len(args)-1] == "--show-toplevel" && outside {
			return nil, invalid("expected a Git worktree")
		}
		return nil, errors.New("cannot inspect the Git worktree")
	}
	return result.Stdout, nil
}

func openWorktree(ctx context.Context, env map[string]string) (*worktree, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, errors.New("cannot locate the current directory")
	}
	// Read paths individually: a worktree or a separately located Git directory
	// can itself contain a newline.
	var dirs []string
	for _, flag := range []string{"--show-toplevel", "--absolute-git-dir", "--git-common-dir"} {
		args := []string{"rev-parse", "--path-format=absolute"}
		if flag == "--show-toplevel" {
			args = append(args, "--is-inside-work-tree")
		}
		raw, err := queryGit(ctx, cwd, env, append(args, flag)...)
		if err != nil {
			return nil, err
		}
		if flag == "--show-toplevel" {
			var inside bool
			raw, inside = bytes.CutPrefix(raw, []byte("true\n"))
			if !inside {
				return nil, invalid("expected a directory inside the Git worktree")
			}
		}
		path, ok := strings.CutSuffix(string(raw), "\n")
		if !ok || !filepath.IsAbs(path) {
			return nil, invalid("expected a Git worktree")
		}
		dirs = append(dirs, path)
	}
	tree := &worktree{}
	for _, path := range dirs[1:] {
		info, err := os.Stat(path)
		if err != nil {
			return nil, errors.New("cannot identify private worktree directories")
		}
		tree.private = append(tree.private, info)
	}
	if info, err := os.Stat(filepath.Join(dirs[0], ".factory")); err == nil {
		tree.private = append(tree.private, info)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("cannot identify private factory state")
	}
	tree.root, err = os.Open(dirs[0]) // #nosec G304 -- Git worktree root; every selected component is opened relative to this descriptor.
	if err != nil {
		return nil, errors.New("cannot open the Git worktree")
	}
	return tree, nil
}

func (tree *worktree) checkRoot() error {
	opened, err := tree.root.Stat()
	if err != nil {
		return errors.New("cannot inspect the Git worktree")
	}
	if slices.ContainsFunc(tree.private, func(private os.FileInfo) bool { return os.SameFile(opened, private) }) {
		return invalid("worktree root is private")
	}
	named, err := os.Stat(tree.root.Name())
	if err != nil || !os.SameFile(named, opened) {
		return invalid("worktree changed during preview")
	}
	return nil
}

func selectionError(name string, err error) error {
	if errors.Is(err, unix.ENOENT) || errors.Is(err, unix.ELOOP) || errors.Is(err, unix.ENOTDIR) || errors.Is(err, unix.ENAMETOOLONG) {
		return invalid("missing or unsafe file %s", safeName(name))
	}
	return fmt.Errorf("cannot inspect file %s", safeName(name))
}

// Walk using directory descriptors. A changed intermediate component cannot
// redirect a subsequent open through a symlink. specs/003-task-context-and-skills.md:122.
func (tree *worktree) open(ctx context.Context, name string) (*os.File, error) {
	dir := tree.root
	defer func() {
		if dir != tree.root {
			_ = dir.Close()
		}
	}()
	parts := strings.Split(name, "/")
	for idx, part := range parts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		flags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_CLOEXEC | unix.O_NONBLOCK | unix.O_NOCTTY
		if idx < len(parts)-1 {
			flags |= unix.O_DIRECTORY
		} else {
			// Avoid opening a known device, socket or FIFO even nonblocking.
			var before unix.Stat_t
			if err := unix.Fstatat(int(dir.Fd()), part, &before, unix.AT_SYMLINK_NOFOLLOW); err != nil {
				return nil, selectionError(name, err)
			}
			if before.Mode&unix.S_IFMT != unix.S_IFREG || before.Nlink != 1 {
				return nil, invalid("expected a regular single-link file %s", safeName(name))
			}
		}
		fd, err := unix.Openat(int(dir.Fd()), part, flags, 0)
		if err != nil {
			return nil, selectionError(name, err)
		}
		child := os.NewFile(uintptr(fd), part)
		info, err := child.Stat()
		if err != nil {
			_ = child.Close()
			return nil, selectionError(name, err)
		}
		if slices.ContainsFunc(tree.private, func(private os.FileInfo) bool { return os.SameFile(info, private) }) {
			_ = child.Close()
			return nil, invalid("cannot select private path %s", safeName(name))
		}
		if idx == len(parts)-1 {
			return child, nil
		}
		if dir != tree.root {
			_ = dir.Close()
		}
		dir = child
	}
	return nil, invalid("empty file path")
}

func sourceStat(f *os.File, name string) (unix.Stat_t, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(int(f.Fd()), &stat); err != nil {
		return stat, selectionError(name, err)
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 {
		return stat, invalid("expected a regular single-link file %s", safeName(name))
	}
	if stat.Size < 0 || stat.Size > fileLimit {
		return stat, invalid("file byte limit (%d) exceeded by %s", fileLimit, safeName(name))
	}
	return stat, nil
}

type pinnedSource struct {
	name string
	file *os.File
	stat unix.Stat_t
}

// Compare the current eligible name with the retained single-link descriptor.
// ctime also detects in-place changes and changes to link count between checks.
func (p pinnedSource) check(ctx context.Context, tree *worktree) error {
	current, err := tree.open(ctx, p.name)
	if err != nil {
		return err
	}
	defer func() { _ = current.Close() }()
	stat, err := sourceStat(current, p.name)
	if err != nil {
		return err
	}
	if stat.Dev != p.stat.Dev || stat.Ino != p.stat.Ino || stat.Ctim != p.stat.Ctim || stat.Size != p.stat.Size {
		return invalid("file changed during preview: %s", safeName(p.name))
	}
	return nil
}

func readSources(ctx context.Context, paths []string) ([]sourceFile, error) {
	env := configuredcmd.CaptureEnvironment()
	for key := range env {
		if strings.HasPrefix(key, "GIT_") && key != "GIT_CONFIG_GLOBAL" && key != "GIT_CONFIG_SYSTEM" && key != "GIT_CONFIG_NOSYSTEM" {
			delete(env, key)
		}
	}
	env["GIT_NO_LAZY_FETCH"] = "1"
	env["LC_ALL"] = "C"
	tree, err := openWorktree(ctx, env)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tree.root.Close() }()
	var pinned []pinnedSource
	defer func() {
		for _, p := range pinned {
			_ = p.file.Close()
		}
	}()
	seen := make(map[string]bool)
	total := int64(0)
	args := []string{"ls-files", "--cached", "--others", "--exclude-standard", "--deduplicate", "-z", "--"}
	for _, name := range paths {
		if seen[name] {
			continue
		}
		seen[name] = true
		f, err := tree.open(ctx, name)
		if err != nil {
			return nil, err
		}
		stat, err := sourceStat(f, name)
		pinned = append(pinned, pinnedSource{name, f, stat})
		if err != nil {
			return nil, err
		}
		total += stat.Size
		if total > totalLimit {
			return nil, invalid("total read limit (%d) exceeded by %s", totalLimit, safeName(name))
		}
		args = append(args, name)
	}
	if err := tree.checkRoot(); err != nil {
		return nil, err
	}
	raw, err := queryGit(ctx, tree.root.Name(), env, args...)
	if err != nil {
		return nil, err
	}
	eligible := make(map[string]bool)
	for name := range strings.SplitSeq(string(raw), "\x00") {
		eligible[name] = true
	}
	var sources []sourceFile
	for _, p := range pinned {
		if !eligible[p.name] {
			return nil, invalid("ignored or ineligible file %s", safeName(p.name))
		}
		if err := p.check(ctx, tree); err != nil {
			return nil, err
		}
		data, readErr := fileinput.ReadFile(ctx, p.file, max(1, p.stat.Size))
		if err := p.check(ctx, tree); err != nil {
			return nil, err
		}
		if readErr != nil || int64(len(data)) != p.stat.Size {
			return nil, fmt.Errorf("cannot read file %s", safeName(p.name))
		}
		if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
			return nil, invalid("expected UTF-8 text without NUL in %s", safeName(p.name))
		}
		sources = append(sources, sourceFile{p.name, data})
	}
	return sources, tree.checkRoot()
}
