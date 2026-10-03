package assessment

import (
	"bytes"
	"context"
	"errors"
	"io"
	"maps"
	"strings"
	"time"

	"github.com/anoop2811/software-factory-template/internal/native"
	"golang.org/x/sys/unix"
)

type recoveryGit struct {
	writer      *recoveryWriter
	root        string
	environment map[string]string
	info        *writePin
	exclude     *writePin
}

func (g *recoveryGit) command(ctx context.Context, arguments ...string) (native.CommandResult, error) {
	if err := g.writer.check(ctx); err != nil {
		return native.CommandResult{}, err
	}
	argv := append([]string{"git", "-c", "core.fsmonitor=false", "-c", "core.untrackedCache=false", "-c", "core.excludesFile=/dev/null"}, arguments...)
	result, err := native.ExecuteCommand(ctx, g.root, argv, g.environment, 10*time.Second)
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	if result.OwnershipUnconfirmed || result.Outcome == "timeout" || result.Outcome == "interrupted" || result.Outcome == "output_limit" {
		return result, failure(1, "local Git query did not complete safely")
	}
	if err != nil || result.ExitCode == nil {
		return result, failure(2, "a normal local Git installation is required")
	}
	if err := g.writer.check(ctx); err != nil {
		return result, err
	}
	return result, nil
}

// Git control is confined to ordinary physical metadata in this installation.
// Fixed overrides disable executable fsmonitor and external ignore configuration.
// docs/adr/0091-durable-local-recovery-creation.md:63.
// docs/adr/0091-durable-local-recovery-creation.md:154.
func (w *recoveryWriter) localGit(ctx context.Context, root string, environment map[string]string) (*recoveryGit, error) {
	git, err := w.directory(ctx, w.root, ".git", false, false)
	if err != nil {
		return nil, failure(2, "linked or external Git directories are not supported")
	}
	if _, err := named(git.file, "commondir"); !errors.Is(err, unix.ENOENT) {
		return nil, failure(2, "shared Git directories are not supported")
	}
	info, err := w.directory(ctx, git, "info", false, false)
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"config", "HEAD", "index"} {
		stat, err := named(git.file, name)
		if errors.Is(err, unix.ENOENT) && name == "index" {
			continue
		}
		if err != nil || !ownedFile(stat, 1<<62) || stat.Mode&0022 != 0 {
			return nil, failure(2, "unsafe Git control file")
		}
		if _, err := w.existingFile(ctx, git, name, unix.O_RDONLY, 1<<62, false); err != nil {
			return nil, err
		}
	}
	effective := maps.Clone(environment)
	if effective == nil {
		effective = map[string]string{"PATH": "/usr/bin:/bin"}
	}
	for key := range effective {
		if strings.HasPrefix(key, "GIT_") {
			delete(effective, key)
		}
	}
	effective["GIT_CONFIG_NOSYSTEM"] = "1"
	effective["GIT_CONFIG_GLOBAL"] = "/dev/null"
	effective["GIT_OPTIONAL_LOCKS"] = "0"
	g := &recoveryGit{writer: w, root: root, environment: effective, info: info}
	result, err := g.command(ctx, "rev-parse", "--show-toplevel", "--absolute-git-dir", "--is-bare-repository")
	if err != nil {
		return nil, err
	}
	if *result.ExitCode != 0 || string(result.Stdout) != root+"\n"+root+"/.git\nfalse\n" {
		return nil, failure(2, "Git installation must be rooted at the current directory")
	}
	return g, nil
}

// Keep one persistent flock inode for the entire operation, covering exclusion,
// bounded inventory and exclusive reservation; never unlink or replace it.
func (g *recoveryGit) acquire(ctx context.Context) error {
	const name = "factory-recovery.lock"
	var lock *writePin
	var err error
	if _, inspectErr := named(g.info.file, name); errors.Is(inspectErr, unix.ENOENT) {
		lock, err = g.writer.createFile(ctx, g.info, name)
	} else {
		lock, err = g.writer.existingFile(ctx, g.info, name, unix.O_RDWR, 0, true)
	}
	if err != nil {
		return err
	}
	if err := unix.Flock(int(lock.file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return failure(2, "another recovery creation is running")
		}
		return failure(1, "cannot lock local recovery creation")
	}
	return g.writer.check(ctx)
}

func (g *recoveryGit) untracked(ctx context.Context) error {
	result, err := g.command(ctx, "ls-files", "--cached", "-z", "--", ".factory/backups")
	if err != nil {
		return err
	}
	if *result.ExitCode != 0 {
		return failure(1, "cannot assess tracked recovery paths")
	}
	if len(result.Stdout) != 0 {
		return failure(2, "tracked recovery paths must be preserved")
	}
	return nil
}

func (g *recoveryGit) ignored(ctx context.Context, paths []string) (bool, error) {
	for _, path := range paths {
		// Quiet non-verbose status evaluates actual ignoring; verbose output can
		// report a matched negation without establishing that a path is ignored.
		result, err := g.command(ctx, "check-ignore", "--no-index", "-q", "--", path)
		if err != nil {
			return false, err
		}
		if *result.ExitCode == 1 {
			return false, nil
		}
		if *result.ExitCode != 0 {
			return false, failure(1, "cannot assess effective recovery ignoring")
		}
	}
	return true, nil
}

func (g *recoveryGit) excludeFile(ctx context.Context) ([]byte, error) {
	stat, err := named(g.info.file, "exclude")
	if errors.Is(err, unix.ENOENT) {
		return nil, nil
	}
	if err != nil || !ownedFile(stat, 64<<10) || stat.Mode&0022 != 0 || stat.Mode&0111 != 0 {
		return nil, failure(2, "unsafe local Git exclude file")
	}
	g.exclude, err = g.writer.existingFile(ctx, g.info, "exclude", unix.O_RDWR, 64<<10, false)
	if err != nil {
		return nil, err
	}
	return g.writer.read(ctx, g.exclude, 64<<10)
}

func (g *recoveryGit) ensureIgnored(ctx context.Context, paths []string) error {
	if err := g.untracked(ctx); err != nil {
		return err
	}
	original, err := g.excludeFile(ctx)
	if err != nil {
		return err
	}
	rule := []byte("/.factory/backups/")
	present := false
	for _, line := range bytes.Split(original, []byte{'\n'}) {
		if bytes.Equal(line, rule) {
			present = true
			break
		}
	}
	expected := original
	if !present {
		addition := append(bytes.Clone(rule), '\n')
		if len(original) > 0 && original[len(original)-1] != '\n' {
			addition = append([]byte{'\n'}, addition...)
		}
		if len(original)+len(addition) > 64<<10 {
			return failure(2, "local Git exclude file exceeds 64 KiB")
		}
		if g.exclude == nil {
			g.exclude, err = g.writer.createFile(ctx, g.info, "exclude")
			if err != nil {
				return err
			}
		}
		// Every partial append is inert. Only a fully synced and read-back
		// comment can become the full narrow rule through one positional byte.
		// docs/adr/0091-durable-local-recovery-creation.md:164.
		stage := bytes.Clone(addition)
		leading := len(stage) - len(rule) - 1
		stage[leading] = '#'
		if _, err := g.exclude.file.Seek(int64(len(original)), io.SeekStart); err != nil {
			return failure(1, "cannot position local Git exclusion")
		}
		g.writer.changed = true
		if err := g.writer.write(ctx, g.exclude, stage); err != nil {
			return err
		}
		if err := g.writer.sync(ctx, g.exclude); err != nil {
			return err
		}
		readback, err := g.writer.read(ctx, g.exclude, 64<<10)
		if err != nil {
			return err
		}
		if !bytes.Equal(readback, append(bytes.Clone(original), stage...)) {
			return failure(1, "local Git exclusion readback did not match")
		}
		if err := g.writer.writeAt(ctx, g.exclude, []byte{'/'}, int64(len(original)+leading)); err != nil {
			return err
		}
		expected = append(bytes.Clone(original), addition...)
	}
	// A visible rule is not evidence of durability after an earlier sync failure.
	// Re-sync and read back existing rules before fresh creation or exact reuse.
	// docs/adr/0091-durable-local-recovery-creation.md:185.
	if err := g.writer.sync(ctx, g.exclude); err != nil {
		return err
	}
	readback, err := g.writer.read(ctx, g.exclude, 64<<10)
	if err != nil {
		return err
	}
	if !bytes.Equal(readback, expected) {
		return failure(1, "local Git exclusion activation readback did not match")
	}
	if err := g.writer.sync(ctx, g.info); err != nil {
		return err
	}
	ignored, err := g.ignored(ctx, paths)
	if err != nil {
		return err
	}
	if !ignored {
		return failure(2, "project ignore rules expose recovery paths")
	}
	return g.untracked(ctx)
}
