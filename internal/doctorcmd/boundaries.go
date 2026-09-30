package doctorcmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anoop2811/software-factory-template/internal/config"
	"github.com/anoop2811/software-factory-template/internal/native"
	"github.com/anoop2811/software-factory-template/internal/reviewlanecmd"
	"golang.org/x/sys/unix"
)

func (r *report) hooksPath(ctx context.Context) error {
	if !regular(r.path(".githooks/pre-push")) {
		return nil
	}
	probe := func(args ...string) (string, error) {
		result, err := native.ExecuteCommand(ctx, r.root, append([]string{"git"}, args...), r.environment, 5*time.Second)
		if unsafeResult(result) || ctx.Err() != nil {
			return "", errors.New("git hooksPath probe did not complete safely")
		}
		if err == nil && succeeded(result) {
			return strings.TrimRight(string(result.Stdout), "\n"), nil
		}
		// Ordinary unavailable or unsuccessful Git probes retain the warning
		// classification below; unsafe supervision failures were rejected above.
		return "", nil
	}
	resolved, err := probe("rev-parse", "--git-path", "hooks/pre-push")
	if err != nil {
		return err
	}
	value, err := probe("config", "--get", "core.hooksPath")
	if err != nil {
		return err
	}
	resolved = r.path(resolved)
	switch {
	case resolved == r.path(".githooks/pre-push") && executable(resolved):
		r.line("[ ok ]", "git resolves the pre-push hook to this repo's .githooks")
	case resolved == r.path(".githooks/pre-push"):
		r.line("[warn]", "the pre-push hook is not executable — git ignores it, so the push gate is INERT")
		r.line("", "  file:  "+resolved)
		r.line("", "  fix:   chmod +x .githooks/pre-push")
	case value != "":
		r.line("[warn]", "core.hooksPath redirects git away from .githooks — the push gate is INERT")
		r.line("", "  git runs: "+resolved)
		r.line("", "  fix: git config core.hooksPath .githooks")
	default:
		r.line("[warn]", "push gate not installed — run: git config core.hooksPath .githooks")
	}
	return r.err
}

// Export planning reads only private bounded copies, retaining the shared grammar.
func configurationSnapshot(ctx context.Context, path string, data []byte) (string, func() error, error) {
	legacy, err := readFile(ctx, config.LegacyPath(path))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", nil, err
	}
	root, err := os.MkdirTemp("", "factory-doctor-config-")
	if err != nil {
		return "", nil, err
	}
	owned, err := os.OpenRoot(root)
	if err != nil {
		return "", nil, errors.Join(err, os.RemoveAll(root))
	}
	identity, err := owned.Stat(".")
	if err != nil {
		return "", nil, errors.Join(err, owned.Close(), os.RemoveAll(root))
	}
	cleanup := func() error { return errors.Join(removeScratch(root, owned, identity), owned.Close()) }
	if err := os.WriteFile(filepath.Join(root, "factory.yaml"), data, 0600); err != nil {
		return "", nil, errors.Join(err, cleanup())
	}
	if legacy != nil {
		if err := os.WriteFile(filepath.Join(root, "factory.config"), legacy, 0600); err != nil {
			return "", nil, errors.Join(err, cleanup())
		}
	}
	return root, cleanup, nil
}
func removeScratch(root string, owned *os.Root, identity os.FileInfo) error {
	// Generated directory modes may be read-only. Restore removal access only
	// inside our private tree; WalkDir does not follow generated symbolic links.
	err := fs.WalkDir(owned.FS(), ".", func(path string, item fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if item.IsDir() {
			return owned.Chmod(path, 0700)
		}
		return nil
	})
	current, statErr := os.Lstat(root)
	if statErr != nil || !os.SameFile(identity, current) {
		return errors.Join(err, errors.New("scratch identity changed; cleanup refused"))
	}
	return errors.Join(err, os.RemoveAll(root))
}

func (r *report) review(ctx context.Context, path string, data []byte) error {
	legacy, err := readFile(ctx, config.LegacyPath(path))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		r.line("[warn]", "review lane status unverified: "+err.Error())
		return ctx.Err()
	}
	settings, err := reviewlanecmd.SettingsFromBytes(ctx, data, legacy, r.environment)
	if err != nil {
		return err
	}
	if !settings.Enabled() {
		r.line("[inert]", "review lane            off (opt-in; ./factory review-lane enable)")
		return r.err
	}
	status, err := reviewlanecmd.SecretStatus(ctx, r.root, settings)
	if err != nil {
		return err
	}
	if status == "set" {
		r.line("[ARMED]", "review lane            advisory PR review, secret present")
	} else {
		r.line("[warn]", "review lane is ON but its secret ("+settings.Secret()+") is missing or unverified")
		r.line("", "  add it: GitHub -> Settings -> Secrets and variables -> Actions")
	}
	return r.err
}

var adapterPaths = []string{".claude", ".codex", ".mcp.json", "CLAUDE.md"}
var inputPaths = []string{"opencode.json", "AGENTS.md", ".opencode/agent", "scripts/sync-claude.sh", "scripts/sync-codex.sh", "scripts/lib/config.sh", "scripts/lib/roles.sh"}

func scratchEnvironment(environment map[string]string, root string) map[string]string {
	result := map[string]string{}
	for key, value := range environment {
		if !strings.HasPrefix(key, "GIT_") {
			result[key] = value
		}
	}
	result["FACTORY_CONFIG"] = filepath.Join(root, "factory.yaml")
	result["GIT_CEILING_DIRECTORIES"] = filepath.Dir(root)
	result["PWD"] = root
	return result
}
func (r *report) adapters(ctx context.Context, path string, data []byte) (returned error) {
	if !executable(r.path("scripts/sync-claude.sh")) || !directory(r.path(".claude")) {
		r.line("[skip]", "adapter drift (sync scripts or .claude not present)")
		return r.err
	}
	before, err := capture(ctx, r.root, append(append([]string{}, inputPaths...), adapterPaths...), false)
	if err != nil {
		r.line("[warn]", "adapter drift skipped (unsafe or unreadable snapshot: "+err.Error()+")")
		return ctx.Err()
	}
	if canonical, ok := before.entries["opencode.json"]; ok {
		var doc struct {
			Agent map[string]json.RawMessage `json:"agent"`
		}
		if json.Unmarshal([]byte(canonical.data), &doc) != nil {
			r.line("[warn]", "adapter drift skipped (invalid opencode.json)")
			return r.err
		}
		for role := range doc.Agent {
			if role == "" || strings.ContainsAny(role, "/\\. \t\r\n\x00") {
				r.line("[warn]", "adapter drift skipped (unsafe canonical role name)")
				return r.err
			}
		}
	}
	scratch, cleanup, err := configurationSnapshot(ctx, path, data)
	if err != nil {
		r.line("[warn]", "adapter drift skipped (cannot snapshot configuration: "+err.Error()+")")
		return ctx.Err()
	}
	defer func() { returned = errors.Join(returned, cleanup()) }()
	// Combined data budget includes config and legacy data copied above.
	for _, name := range []string{"factory.yaml", "factory.config"} {
		if info, err := os.Stat(filepath.Join(scratch, name)); err == nil {
			before.size += int(info.Size())
		}
	}
	if before.size > snapshotLimit {
		r.line("[warn]", "adapter drift skipped (snapshot data limit exceeded)")
		return r.err
	}
	if err := before.write(ctx, scratch); err != nil {
		r.line("[warn]", "adapter drift skipped (cannot prepare scratch snapshot: "+err.Error()+")")
		return ctx.Err()
	}
	for _, script := range []string{"sync-claude.sh", "sync-codex.sh"} {
		if !executable(filepath.Join(scratch, "scripts", script)) {
			continue
		}
		result, err := native.ExecuteCommand(ctx, scratch, []string{"./scripts/" + script}, scratchEnvironment(r.environment, scratch), time.Minute)
		if unsafeResult(result) || ctx.Err() != nil {
			return fmt.Errorf("adapter sync %s did not complete safely (%s)", script, result.Outcome)
		}
		if err != nil || !succeeded(result) {
			r.line("[warn]", "adapter drift unverified — "+script+" failed; run 'make sync-harnesses'")
			return r.err
		}
	}
	after, err := capture(ctx, scratch, adapterPaths, true)
	if err != nil {
		r.line("[warn]", "adapter drift unverified: "+err.Error())
		return ctx.Err()
	}
	expected := map[string]entry{}
	for name, item := range before.entries {
		for _, path := range adapterPaths {
			if name == path || strings.HasPrefix(name, path+"/") {
				expected[name] = item
				break
			}
		}
	}
	if maps.Equal(expected, after.entries) {
		r.line("[ ok ]", "harness adapters match the opencode canon (no drift)")
	} else {
		r.line("[warn]", "harness adapters drifted — run 'make sync-harnesses' and commit")
	}
	return r.err
}

func (r *report) proof(ctx context.Context) error {
	r.print("\nProof (break/fix self-test)\n")
	if !executable(r.path("scripts/selftest/run.sh")) {
		r.line("[FAIL]", "scripts/selftest/run.sh is missing — cannot prove the gates fire")
		return r.err
	}
	tty := false
	if file, ok := r.out.(*os.File); ok {
		_, err := unix.IoctlGetWinsize(int(file.Fd()), unix.TIOCGWINSZ)
		tty = err == nil
	}
	if tty {
		r.print("  break/fix self-test still going...\n")
	} else {
		r.print("  breaking each gate on purpose, then fixing it — this takes a few minutes\n")
	}
	if r.err != nil {
		return r.err
	}
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	type completion struct {
		result native.CommandResult
		err    error
	}
	done := make(chan completion, 1)
	go func() {
		result, err := native.ExecuteCommandCombined(child, r.root, []string{"./scripts/selftest/run.sh"}, r.environment, 15*time.Minute)
		done <- completion{result, err}
	}()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	start := time.Now()
	var finished completion
waiting:
	for {
		select {
		case finished = <-done:
			break waiting
		case <-ticker.C:
			if tty {
				r.print("\r  break/fix self-test still going... (%ds)   ", int(time.Since(start).Seconds()))
				if r.err != nil {
					cancel()
				}
			}
		}
	}
	if r.err != nil {
		return r.err
	}
	if tty {
		r.print("\r%*s\r", 72, "")
	}
	if unsafeResult(finished.result) || ctx.Err() != nil {
		return fmt.Errorf("break/fix self-test did not complete safely (%s)", finished.result.Outcome)
	}
	text := string(finished.result.Stdout) + string(finished.result.Stderr)
	var tallies []string
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "selftest:") {
			tallies = append(tallies, line)
		}
	}
	tally := strings.Join(tallies, "\n")
	if finished.err == nil && succeeded(finished.result) {
		if tally == "" {
			tally = "every gate fired on its violation and passed clean"
		}
		r.line("[ ok ]", tally+fmt.Sprintf(" in %ds", int(time.Since(start).Seconds())))
	} else {
		if tally == "" {
			tally = "break/fix self-test failed"
		}
		r.line("[FAIL]", tally)
		for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
			r.print("    %s\n", line)
		}
	}
	return r.err
}
