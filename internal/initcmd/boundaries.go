package initcmd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anoop2811/software-factory-template/internal/native"

	"golang.org/x/sys/unix"
)

// Preflight also covers the fixed files written by retained adapter scripts.
// docs/adr/0085-go-native-init.md:75.
func preflightExternal(ctx context.Context, target *tree, plan []asset) error {
	var document struct {
		Agent map[string]json.RawMessage `json:"agent"`
	}
	for _, a := range plan {
		if a.path == "opencode.json" {
			if json.Unmarshal(a.data, &document) != nil {
				return errors.New("invalid template opencode configuration")
			}
		}
	}
	paths := []string{".claude/settings.json", ".mcp.json", ".github/workflows/adversarial-review.yml", "factory.yaml.factory-bak", ".opencode/package-lock.json"}
	for role := range document.Agent {
		if role == "" || strings.ContainsAny(role, "/\\. \t\r\n\x00") {
			return errors.New("unsafe template role name")
		}
		paths = append(paths, ".claude/agents/"+role+".md", ".codex/agents/"+role+".toml", ".opencode/agent/"+role+".md.bak")
	}
	if target == nil {
		return nil
	}
	for _, path := range paths {
		if _, _, _, err := target.read(ctx, path); err != nil {
			return err
		}
	}
	for _, directory := range []string{".claude", ".claude/agents", ".opencode/node_modules"} {
		parent, _, err := target.parent(ctx, directory+"/preflight", false)
		if parent != nil {
			_ = parent.Close()
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	var stat unix.Stat_t
	err := unix.Fstatat(int(target.root.Fd()), "CLAUDE.md", &stat, unix.AT_SYMLINK_NOFOLLOW)
	if err == nil && stat.Mode&unix.S_IFMT == unix.S_IFLNK {
		var buffer [4096]byte
		n, err := unix.Readlinkat(int(target.root.Fd()), "CLAUDE.md", buffer[:])
		if err != nil || string(buffer[:n]) != "AGENTS.md" {
			return errors.New("unsafe CLAUDE.md link")
		}
	} else if !errors.Is(err, unix.ENOENT) {
		if err != nil {
			return err
		}
		if _, _, _, err := target.read(ctx, "CLAUDE.md"); err != nil {
			return err
		}
	}
	entries, err := target.root.ReadDir(8193)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if len(entries) > 8192 {
		return errors.New("too many target entries")
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "CLAUDE.md.replaced-by-symlink") || strings.HasPrefix(entry.Name(), "opencode.json.sync-tmp.") {
			if _, _, _, err := target.read(ctx, entry.Name()); err != nil {
				return err
			}
		}
	}
	return nil
}
func childEnvironment(environment map[string]string, target string) map[string]string {
	result := make(map[string]string, len(environment)+2)
	for key, value := range environment {
		if strings.HasPrefix(key, "GIT_") {
			continue
		}
		result[key] = value
	}
	result["FACTORY_CONFIG"] = filepath.Join(target, "factory.yaml")
	result["GIT_CEILING_DIRECTORIES"] = filepath.Dir(target)
	return result
}

func preflightGit(ctx context.Context, target *tree, environment map[string]string) error {
	if target == nil {
		return nil
	}
	result, err := native.ExecuteCommand(ctx, target.path, []string{"git", "rev-parse", "--show-toplevel"}, childEnvironment(environment, target.path), 5*time.Second)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if result.OwnershipUnconfirmed || result.Outcome == "timeout" || result.Outcome == "interrupted" || result.Outcome == "output_limit" {
		return errors.New("cannot establish target Git boundary")
	}
	// Missing Git and an ordinary non-repository retain the shell's cwd fallback.
	if result.ProcessPID == 0 && err != nil {
		return ctx.Err()
	}
	if err != nil {
		return errors.New("cannot establish target Git boundary")
	}
	if result.ExitCode == nil {
		return errors.New("cannot establish target Git boundary")
	}
	if *result.ExitCode != 0 {
		return nil
	}
	path := strings.TrimRight(string(result.Stdout), "\n")
	if path == "" || strings.ContainsAny(path, "\r\n\x00") {
		return errors.New("invalid target Git boundary")
	}
	resolved, err := physicalPath(path)
	if err != nil || resolved != target.path {
		return errors.New("git worktree must match the initialization target")
	}
	return nil
}

// Bash assignments update a variable's inherited export without exporting every
// newly collected answer. Preserve that distinction for retained child tools.
func initializedEnvironment(environment, values map[string]string, target string) map[string]string {
	result := childEnvironment(environment, target)
	for key := range environment {
		if strings.HasPrefix(key, "PACK_MATURITY_") {
			continue
		}
		if value, assigned := values[key]; assigned {
			result[key] = value
		}
	}
	return result
}
