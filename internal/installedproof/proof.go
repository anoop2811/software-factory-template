// Package installedproof proves the shipped gates in isolated native fixtures.
package installedproof

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anoop2811/software-factory-template/internal/filepublish"
	"github.com/anoop2811/software-factory-template/internal/installationfs"
	"github.com/anoop2811/software-factory-template/internal/native"
)

var gates = []string{"test-edit-denial", "commit-message-lint", "decision-log-gate", "diff-aware-check", "direct-main-push-block", "pending-lessons-push-block", "loop-close-check", "shared-script-enforcement", "hook-existence-check", "wiki-lint", "workflow-lint", "copy-manifest-check", "gate-instrumentation-check", "citation-lint"}

// Run executes every shipped required gate against a violation and its healthy sibling.
// It never executes the source selftest or saved recovery code.
// docs/adr/0098-whole-installation-upgrade-and-rollback.md:207.
func Run(ctx context.Context, root string, out io.Writer) (returned error) {
	source, err := installationfs.Open(ctx, root)
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, source.Close()) }()
	scratch, err := os.MkdirTemp("", "factory-installed-proof-")
	if err != nil {
		return err
	}
	scratch, err = filepath.EvalSymlinks(scratch)
	if err != nil {
		return err
	}
	identity, err := os.Lstat(scratch)
	if err != nil {
		return err
	}
	defer func() {
		current, err := os.Lstat(scratch)
		if err != nil || !os.SameFile(identity, current) {
			returned = errors.Join(returned, errors.New("proof scratch changed; cleanup refused"), err)
		} else {
			returned = errors.Join(returned, os.RemoveAll(scratch))
		}
	}()
	destination, err := os.OpenRoot(scratch)
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, destination.Close()) }()
	binary, err := source.OpenFile(ctx, ".factory/bin/factory-runtime", 256<<20)
	if err != nil {
		return err
	}
	digest, _, digestErr := binary.Digest(ctx, 256<<20)
	if digestErr == nil {
		digestErr = binary.Rewind(ctx)
	}
	if digestErr != nil {
		return errors.Join(digestErr, binary.Close())
	}
	stage, err := filepublish.PrepareReader(ctx, destination, ".proof-driver-", binary.File, binary.Metadata().Size, digest, 0700)
	if err == nil {
		err = errors.Join(binary.Check(ctx), stage.PublishNoReplace(ctx, "driver"), stage.Cleanup())
	}
	if err = errors.Join(err, binary.Close()); err != nil {
		return err
	}
	for _, name := range gates {
		if err := ctx.Err(); err != nil {
			return err
		}
		fixture := filepath.Join(scratch, name)
		if err := os.Mkdir(fixture, 0700); err != nil {
			return err
		}
		p := &proof{root: fixture, environment: environment(fixture)}
		for _, path := range sharedFiles {
			file, err := source.OpenFile(ctx, path, 1<<20)
			if err != nil {
				return err
			}
			_, data, readErr := file.Digest(ctx, 1<<20)
			mode := os.FileMode(file.Metadata().Mode & 0777)
			if err := errors.Join(readErr, file.Close()); err != nil {
				return err
			}
			if err := p.write(ctx, path, string(data), mode); err != nil {
				return err
			}
		}
		if err := p.write(ctx, "factory", "#!/bin/bash\n_root=\"$(cd \"$(dirname \"${BASH_SOURCE[0]}\")\" && pwd -P)\"\nexec \"$_root/../driver\" \"$@\"\n", 0700); err != nil {
			return err
		}
		if err := p.write(ctx, "factory.yaml", "project_name: Proof\n", 0600); err != nil {
			return err
		}
		for _, args := range [][]string{{"init", "-q"}, {"config", "user.name", "Factory proof"}, {"config", "user.email", "proof@example.invalid"}, {"add", "--all"}, {"commit", "-q", "-m", "chore: fixture base"}} {
			if err := p.git(ctx, args...); err != nil {
				return err
			}
		}
		if err := p.pair(ctx, name); err != nil {
			return fmt.Errorf("installed proof %s: %w", name, err)
		}
		if _, err := fmt.Fprintf(out, "proof: %s refused its violation and accepted its healthy control\n", name); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(out, "selftest: %d shipped gates proved, 0 failed\n", len(gates))
	return err
}

var sharedFiles = []string{".githooks/pre-push", "scripts/lib/config.sh", "scripts/lib/roles.sh", "scripts/lib/events.sh", "scripts/lib/color.sh", "scripts/lib/hookspath.sh", "scripts/lib/timing.sh", "scripts/citation-lint.sh", "scripts/selftest/run.sh", "scripts/factory-init.sh", "scripts/prereq-check.sh", "scripts/pre-push-check.sh", "scripts/sync-claude.sh", "scripts/sync-codex.sh", "scripts/harness-structural-eval.sh", "scripts/golden-task-eval.sh", "scripts/hooks/test-edit-denial.sh", "scripts/hooks/commit-message-lint.sh", "scripts/hooks/decision-log-gate.sh", "scripts/hooks/diff-aware-check.sh", "scripts/hooks/direct-main-push-block.sh", "scripts/hooks/pending-lessons-push-block.sh", "scripts/hooks/loop-close-check.sh", "scripts/hooks/shared-script-enforcement.sh", "scripts/hooks/hook-existence-check.sh", "scripts/hooks/wiki-lint.sh", "scripts/hooks/workflow-lint.sh", "scripts/hooks/copy-manifest-check.sh", "scripts/hooks/gate-instrumentation-check.sh"}

type proof struct {
	root        string
	environment map[string]string
}

func environment(root string) map[string]string {
	env := map[string]string{}
	for _, entry := range os.Environ() {
		key, value, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(key, "GIT_") {
			env[key] = value
		}
	}
	env["GIT_CONFIG_NOSYSTEM"] = "1"
	env["GIT_CONFIG_GLOBAL"] = "/dev/null"
	env["FACTORY_CONFIG"] = filepath.Join(root, "factory.yaml")
	env["FACTORY_BRIDGE_PROTOCOL"] = ""
	env["FACTORY_EVENT_LOG"] = filepath.Join(root, "events.log")
	env["REPO_ROOT"] = root
	env["FACTORY_AGENT_ROLE"] = "reviewer"
	return env
}
func (p *proof) write(ctx context.Context, path, contents string, mode os.FileMode) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path = filepath.Join(p.root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(contents), mode)
}
func (p *proof) run(ctx context.Context, expected int, args ...string) error {
	result, err := native.ExecuteCommand(ctx, p.root, args, p.environment, 10*time.Second)
	if err != nil {
		return err
	}
	if result.OwnershipUnconfirmed || !result.ExitConfirmed || result.ExitCode == nil || *result.ExitCode != expected {
		actual := "unavailable"
		if result.ExitCode != nil {
			actual = fmt.Sprint(*result.ExitCode)
		}
		return fmt.Errorf("expected status %d, got %s (%s): %s%s", expected, actual, result.Outcome, result.Stdout, result.Stderr)
	}
	return nil
}
func (p *proof) git(ctx context.Context, args ...string) error {
	return p.run(ctx, 0, append([]string{"git", "-c", "core.hooksPath=/dev/null", "-c", "commit.gpgSign=false"}, args...)...)
}
func (p *proof) hook(ctx context.Context, name string, expected int, args ...string) error {
	path := "./scripts/hooks/" + name + ".sh"
	if name == "citation-lint" {
		path = "./scripts/citation-lint.sh"
	}
	return p.run(ctx, expected, append([]string{path}, args...)...)
}
func (p *proof) pair(ctx context.Context, name string) error {
	write := func(path, body string) error { return p.write(ctx, path, body, 0600) }
	switch name {
	case "test-edit-denial":
		if err := write("factory.yaml", "test_file_patterns: \"_test\\.go$\"\n"); err != nil {
			return err
		}
		p.environment["FACTORY_AGENT_ROLE"] = "implementer"
		if err := p.hook(ctx, name, 2, "pkg/sample_test.go"); err != nil {
			return err
		}
		return p.hook(ctx, name, 0, "pkg/sample.go")
	case "direct-main-push-block":
		for _, pair := range []struct {
			ref    string
			status int
		}{{"main", 1}, {"feature", 0}} {
			result, err := native.ExecuteCheck(ctx, p.root, "printf 'refs/heads/source 123 refs/heads/"+pair.ref+" 456\\n' | ./scripts/hooks/direct-main-push-block.sh", p.environment, 10*time.Second, func(context.Context, int) error { return nil })
			if err != nil {
				return err
			}
			if result.ExitCode == nil || *result.ExitCode != pair.status || result.OwnershipUnconfirmed {
				return errors.New("direct push control did not hold")
			}
		}
		return nil
	case "pending-lessons-push-block":
		if err := write("memory/PENDING-LESSONS.md", "unaddressed proof reminder\n"); err != nil {
			return err
		}
		if err := p.hook(ctx, name, 1); err != nil {
			return err
		}
		if err := os.Remove(filepath.Join(p.root, "memory/PENDING-LESSONS.md")); err != nil {
			return err
		}
		return p.hook(ctx, name, 0)
	case "commit-message-lint":
		if err := p.git(ctx, "commit", "-q", "--allow-empty", "-m", "broken message"); err != nil {
			return err
		}
		if err := p.hook(ctx, name, 1, "HEAD"); err != nil {
			return err
		}
		if err := p.git(ctx, "commit", "-q", "--amend", "--allow-empty", "-m", "chore: healthy control"); err != nil {
			return err
		}
		return p.hook(ctx, name, 0, "HEAD")
	case "decision-log-gate":
		if err := write("factory.yaml", "protected_paths: core\ndecision_log: docs/DECISION_LOG.md\n"); err != nil {
			return err
		}
		if err := write("docs/DECISION_LOG.md", "## Decision 1: proof\n"); err != nil {
			return err
		}
		if err := write("core/entry", "changed\n"); err != nil {
			return err
		}
		if err := p.git(ctx, "add", "--all"); err != nil {
			return err
		}
		if err := p.git(ctx, "commit", "-q", "-m", "feat: protected change"); err != nil {
			return err
		}
		if err := p.hook(ctx, name, 1, "HEAD~1", "HEAD"); err != nil {
			return err
		}
		if err := p.git(ctx, "commit", "-q", "--amend", "--allow-empty", "-m", "feat: protected change\n\nImplements Decision 1."); err != nil {
			return err
		}
		return p.hook(ctx, name, 0, "HEAD~1", "HEAD")
	case "diff-aware-check":
		if err := write("factory.yaml", "protected_paths: core\ncheck_command: false\n"); err != nil {
			return err
		}
		if err := write("core/entry", "first\n"); err != nil {
			return err
		}
		if err := p.git(ctx, "add", "--all"); err != nil {
			return err
		}
		if err := p.git(ctx, "commit", "-q", "-m", "chore: controlled base"); err != nil {
			return err
		}
		if err := write("core/entry", "second\n"); err != nil {
			return err
		}
		if err := p.hook(ctx, name, 1); err != nil {
			return err
		}
		if err := write("factory.yaml", "protected_paths: core\ncheck_command: true\n"); err != nil {
			return err
		}
		return p.hook(ctx, name, 0)
	case "loop-close-check":
		p.environment["FACTORY_SESSION_START_HEAD"] = "HEAD"
		if err := write("new-entry", "work\n"); err != nil {
			return err
		}
		if err := p.hook(ctx, name, 1); err != nil {
			return err
		}
		if err := write("memory/lessons/001-proof.md", "observed 2026-10-09 via installed proof\n"); err != nil {
			return err
		}
		return p.hook(ctx, name, 0)
	case "shared-script-enforcement":
		if err := write(".codex/agents/implementer.toml", "no delegate\n"); err != nil {
			return err
		}
		if err := p.hook(ctx, name, 1); err != nil {
			return err
		}
		if err := write(".codex/agents/implementer.toml", "FACTORY_AGENT_ROLE=implementer scripts/hooks/test-edit-denial.sh\n"); err != nil {
			return err
		}
		return p.hook(ctx, name, 0)
	case "hook-existence-check":
		if err := write("factory.yaml", "local_hooks: scripts/hooks/local-proof.sh\n"); err != nil {
			return err
		}
		if err := p.hook(ctx, name, 1); err != nil {
			return err
		}
		if err := p.write(ctx, "scripts/hooks/local-proof.sh", "#!/bin/sh\nexit 0\n", 0700); err != nil {
			return err
		}
		if err := p.git(ctx, "add", "--all"); err != nil {
			return err
		}
		return p.hook(ctx, name, 0)
	case "wiki-lint":
		if err := write("wiki/page.md", "# Missing provenance\n"); err != nil {
			return err
		}
		if err := p.hook(ctx, name, 1); err != nil {
			return err
		}
		if err := write("wiki/page.md", "# Proof\n\nobserved 2026-10-09 via installed gate proof\n"); err != nil {
			return err
		}
		return p.hook(ctx, name, 0)
	case "workflow-lint":
		if err := write("workflows/proof.md", "## node\n- role: reviewer\n- kind: agent\n"); err != nil {
			return err
		}
		if err := p.hook(ctx, name, 1); err != nil {
			return err
		}
		if err := write("workflows/proof.md", "## node\n- role: reviewer\n- kind: verify\n"); err != nil {
			return err
		}
		return p.hook(ctx, name, 0)
	case "copy-manifest-check":
		if err := write("scripts/factory-init.sh", "cp \"$TEMPLATE_DIR/proof-input\" \"$OUTPUT\"\n"); err != nil {
			return err
		}
		if err := p.hook(ctx, name, 1); err != nil {
			return err
		}
		if err := write("proof-input", "qualified\n"); err != nil {
			return err
		}
		if err := p.git(ctx, "add", "proof-input"); err != nil {
			return err
		}
		return p.hook(ctx, name, 0)
	case "gate-instrumentation-check":
		if err := p.write(ctx, "scripts/hooks/proof-mute.sh", "#!/bin/sh\nexit 1\n", 0700); err != nil {
			return err
		}
		if err := p.hook(ctx, name, 1); err != nil {
			return err
		}
		if err := p.write(ctx, "scripts/hooks/proof-mute.sh", "#!/bin/sh\n. ./scripts/lib/events.sh\nfactory_log_event proof reason\nexit 1\n", 0700); err != nil {
			return err
		}
		return p.hook(ctx, name, 0)
	case "citation-lint":
		if err := write("factory.yaml", "docs_root: docs\ncitation_prefix: PROOF_\n"); err != nil {
			return err
		}
		if err := write("docs/PROOF_SOURCE.md", "line one\nline two\n"); err != nil {
			return err
		}
		if err := write("note.md", "PROOF_SOURCE.md:99\n"); err != nil {
			return err
		}
		if err := p.hook(ctx, name, 1); err != nil {
			return err
		}
		if err := write("note.md", "PROOF_SOURCE.md:2\n"); err != nil {
			return err
		}
		return p.hook(ctx, name, 0)
	}
	return errors.New("shipped gate has no proof control")
}
