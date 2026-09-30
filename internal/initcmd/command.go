// Package initcmd installs template assets through native Go orchestration.
package initcmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/anoop2811/software-factory-template/internal/native"
	"github.com/anoop2811/software-factory-template/internal/output"
	"github.com/anoop2811/software-factory-template/internal/reviewlanecmd"
	"golang.org/x/sys/unix"
)

// Run initializes a target from the explicit source layout, retaining installed
// shell assets and the named external boundaries. docs/adr/0085-go-native-init.md:17.
func Run(parent context.Context, args []string, templateRoot string, environment map[string]string, stdin io.Reader, stdout, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	broken := make(chan os.Signal, 1)
	signal.Notify(broken, syscall.SIGPIPE)
	defer signal.Stop(broken)
	err := run(ctx, args, templateRoot, environment, stdin, stdout, stderr)
	if err == nil {
		return 0
	}
	if errors.Is(err, errDeclined) {
		return 1
	}
	message := err.Error()
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		message = "cannot access initialization files"
	}
	if ctx.Err() != nil {
		message = "initialization canceled"
	}
	_, _ = fmt.Fprintln(stderr, "factory-init: "+message)
	var usage *usageFailure
	if errors.As(err, &usage) {
		return 2
	}
	return 1
}

var errDeclined = errors.New("initialization declined")

func run(ctx context.Context, args []string, templateRoot string, environment map[string]string, stdin io.Reader, stdout, stderr io.Writer) (returned error) {
	o, err := parseArguments(args)
	if err != nil {
		return err
	}
	sourcePath, err := physicalPath(templateRoot)
	if err != nil {
		return err
	}
	targetPath, err := physicalPath(o.target)
	if err != nil {
		return err
	}
	if strings.ContainsAny(sourcePath+targetPath, "\r\n") {
		return errors.New("initialization roots cannot contain newline characters")
	}
	if overlaps(sourcePath, targetPath) {
		return errors.New("source and target roots must not overlap")
	}
	source, err := openTree(sourcePath)
	if err != nil {
		return err
	}
	defer source.root.Close()
	target, err := openTree(targetPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if target != nil {
		defer target.root.Close()
	}
	reader, cleanup, interactive, err := promptInput(ctx, stdin, stdout)
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, cleanup()) }()
	menus := false
	if file, ok := stdout.(*os.File); ok {
		menus = interactive && terminal(file)
	}
	q := questions{reader: bufio.NewReader(reader), out: stdout, promptOut: stderr, prompted: interactive, interactive: menus}
	if err := output.WriteEvent(ctx, stdout, []byte(fmt.Sprintf("=== Software Factory Template Setup ===\nTemplate dir: %s\nTarget dir:   %s\n\n", sourcePath, targetPath))); err != nil {
		return err
	}
	if err := detectStack(ctx, target, stdout); err != nil {
		return err
	}
	values, err := collect(ctx, &q, &o, environment)
	if err != nil {
		return err
	}
	if err := selectJava(ctx, target, &o, values); err != nil {
		return err
	}
	if err := choiceMessages(ctx, stdout, values, o); err != nil {
		return err
	}
	plan, err := buildPlan(ctx, source, target, o, values)
	if err != nil {
		return err
	}
	if err := preflightGit(ctx, target, environment); err != nil {
		return err
	}
	if err := preflightExternal(ctx, target, plan); err != nil {
		return err
	}
	if err := summary(ctx, stdout, values, o); err != nil {
		return err
	}
	answer, err := q.ask(ctx, "Proceed? (y/N): ")
	if err != nil {
		return err
	}
	if !strings.HasPrefix(strings.ToLower(answer), "y") {
		if err := output.WriteEvent(ctx, stdout, []byte("Aborted.\n")); err != nil {
			return err
		}
		return errDeclined
	}
	if q.interactive && values["REVIEW_LANE"] == "on" && values["REVIEW_MODEL"] == "" {
		model := values["FRONTIER_MODEL"]
		switch values["MODEL_PROVIDER"] {
		case "anthropic":
			model = values["CLAUDE_FRONTIER_MODEL"]
			if model == "" {
				model = "claude-opus-4-8"
			}
		case "openai":
			model = values["CODEX_FRONTIER_MODEL"]
			if model == "" {
				model = "gpt-5.6-sol"
			}
		default:
			if model == "" {
				model = "openrouter/z-ai/glm-5.2"
			}
		}
		if err := output.WriteEvent(ctx, stdout, []byte("Review model — the reviewer runs at the frontier tier by default.\n  provider: "+values["MODEL_PROVIDER"]+"\n  default:  "+model+"\n")); err != nil {
			return err
		}
		selected, err := q.ask(ctx, "  Model to use (Enter for the default): ")
		if err != nil {
			return err
		}
		if strings.ContainsAny(selected, "\x00\r\n\"") {
			return errors.New("review model cannot be represented safely")
		}
		values["REVIEW_MODEL"] = selected
		for i := range plan {
			if plan[i].path == "factory.yaml" {
				plan[i].data = setKey(plan[i].data, "review_model", selected)
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if target == nil {
		target, err = createTarget(ctx, targetPath)
		if err != nil {
			return err
		}
		defer target.root.Close()
	}
	for _, directory := range directoriesToCreate {
		parent, _, err := target.parent(ctx, directory+"/directory", true)
		if err != nil {
			return err
		}
		_ = parent.Close()
	}
	for _, a := range plan {
		if err := target.publish(ctx, a); err != nil {
			return err
		}
		if err := output.WriteEvent(ctx, stdout, []byte("  installed: "+a.path+"\n")); err != nil {
			return err
		}
	}
	if err := packMessages(ctx, stdout, o, values); err != nil {
		return err
	}
	if err := externalStages(ctx, targetPath, values, o, initializedEnvironment(environment, values, targetPath), stdout, stderr); err != nil {
		return err
	}
	return nil
}
func createTarget(ctx context.Context, path string) (*tree, error) {
	current := path
	var parts []string
	for {
		_, err := os.Lstat(current)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		parts = append(parts, filepath.Base(current))
		current = filepath.Dir(current)
	}
	fd, err := unix.Open(current, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), "init-parent")
	for i := len(parts) - 1; i >= 0; i-- {
		if err := ctx.Err(); err != nil {
			_ = file.Close()
			return nil, err
		}
		if err := unix.Mkdirat(int(file.Fd()), parts[i], 0755); err != nil {
			_ = file.Close()
			return nil, err
		}
		next, err := unix.Openat(int(file.Fd()), parts[i], unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		_ = file.Close()
		if err != nil {
			return nil, err
		}
		file = os.NewFile(uintptr(next), "init-directory")
	}
	stat, err := fileStat(file)
	if err != nil || !safeDirectory(stat) {
		_ = file.Close()
		return nil, errors.New("unsafe installation directory")
	}
	return &tree{root: file, path: path}, nil
}
func externalStages(ctx context.Context, target string, v map[string]string, o options, environment map[string]string, stdout, stderr io.Writer) error {
	stage := func(root string, argv []string, suppressErrors bool) (bool, error) {
		result, err := native.ExecuteCommand(ctx, root, argv, environment, 10*time.Minute)
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		if result.OwnershipUnconfirmed || result.Outcome == "timeout" || result.Outcome == "interrupted" || result.Outcome == "output_limit" {
			return false, errors.Join(err, errors.New("initializer tool did not complete safely"))
		}
		if err := output.WriteEvent(ctx, stdout, result.Stdout); err != nil {
			return false, err
		}
		if !suppressErrors {
			if err := output.WriteEvent(ctx, stderr, result.Stderr); err != nil {
				return false, err
			}
		}
		return err == nil && result.ExitCode != nil && *result.ExitCode == 0, nil
	}
	if err := output.WriteEvent(ctx, stdout, []byte("\nInstalling opencode plugin dependencies...\n")); err != nil {
		return err
	}
	ok, err := stage(filepath.Join(target, ".opencode"), []string{"npm", "install"}, true)
	if err != nil {
		return err
	}
	if !ok {
		if err := output.WriteEvent(ctx, stdout, []byte("  npm install failed — run manually in .opencode/\n")); err != nil {
			return err
		}
	}
	if err := output.WriteEvent(ctx, stdout, []byte("\nApplying models to opencode, Claude, and Codex...\n")); err != nil {
		return err
	}
	for _, name := range []string{"sync-opencode.sh", "sync-claude.sh", "sync-codex.sh"} {
		ok, err := stage(target, []string{"./scripts/" + name}, false)
		if err != nil {
			return err
		}
		if !ok {
			if err := output.WriteEvent(ctx, stdout, []byte("  warning: harness sync did not complete — run 'make sync-harnesses' (jq required)\n")); err != nil {
				return err
			}
			break
		}
	}
	if v["REVIEW_LANE"] == "on" {
		err := reviewlanecmd.ExecuteNonInteractive(ctx, []string{"enable"}, target, target, environment, stdout, stderr)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			if writeErr := output.WriteEvent(ctx, stderr, []byte(err.Error()+"\n")); writeErr != nil {
				return writeErr
			}
			if err := output.WriteEvent(ctx, stdout, []byte("  warning: could not enable the review lane — run './factory review-lane enable'\n")); err != nil {
				return err
			}
		}
	}

	if err := output.WriteEvent(ctx, stdout, []byte("\n=== Post-install attestation: break/fix self-test of installed gates ===\n")); err != nil {
		return err
	}
	ok, err = stage(target, []string{"./scripts/selftest/run.sh"}, false)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("INSTALL NOT VERIFIED — a gate failed its break/fix proof. Do not rely on enforcement until this passes")
	}
	message := "factory-init: gates proven. Install a language pack to arm the\ntest-edit hook and check command: re-run with --pack go,typescript,java.\n"
	if len(o.packs) > 0 {
		message = "factory-init: gates proven and armed for: " + strings.Join(o.packs, " ") + ". Commit when ready.\n"
	}
	if err := output.WriteEvent(ctx, stdout, []byte(message+"\n=== Setup complete ===\n\nNext steps:\n  1. Run prereq-check:    ./scripts/prereq-check.sh\n  2. Sync adapters:       make sync-harnesses\n  3. Start opencode:      opencode\n  4. Review AGENTS.md and edit the Project section for your project\n  5. Add your protected code to "+v["PROTECTED_PATH"]+"/\n  6. Install pre-push:    cp scripts/pre-push-check.sh .git/hooks/pre-push\n  7. Check health anytime: ./factory doctor\n\nfactory.yaml saved — one config file, parsed and never executed.\n")); err != nil {
		return err
	}
	err = reviewlanecmd.ExecuteNonInteractive(ctx, []string{"pending"}, target, target, environment, stdout, stderr)
	return err
}
