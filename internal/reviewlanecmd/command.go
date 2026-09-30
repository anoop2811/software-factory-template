// Package reviewlanecmd manages the opt-in advisory workflow as local data.
package reviewlanecmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/anoop2811/software-factory-template/internal/native"
	"github.com/anoop2811/software-factory-template/internal/output"
	"golang.org/x/sys/unix"
)

const workflow = ".github/workflows/adversarial-review.yml"
const managedHeader = "# Managed by: factory review-lane. Remove with: ./factory review-lane disable"

type commandError struct {
	message string
	status  int
}

func (e *commandError) Error() string { return e.message }

// Run owns signal handling and root discovery; the service never changes cwd.
// docs/adr/0089-go-native-review-lane.md:23.
func Run(parent context.Context, args []string, template string, environment map[string]string, stdout, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	broken := make(chan os.Signal, 1)
	signal.Notify(broken, syscall.SIGPIPE)
	defer signal.Stop(broken)
	root, err := discover(ctx, environment)
	if err == nil {
		err = Execute(ctx, args, root, template, environment, stdout, stderr)
	}
	if err == nil {
		return 0
	}
	status := 1
	var failure *commandError
	if errors.As(err, &failure) {
		status = failure.status
	}
	diagnostic, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()
	_ = output.WriteEvent(diagnostic, stderr, []byte(err.Error()+"\n"))
	return status
}
func discover(ctx context.Context, environment map[string]string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	result, err := native.ExecuteCommand(ctx, cwd, []string{"git", "rev-parse", "--show-toplevel"}, environment, 10*time.Second)
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if unsafe(result) {
		return "", errors.New("review lane root discovery did not complete safely")
	}
	if err != nil || result.ExitCode == nil || *result.ExitCode != 0 {
		//nolint:nilerr // Ordinary unavailable Git retains cwd: docs/adr/0089-go-native-review-lane.md:23.
		return cwd, nil
	}
	root := strings.TrimRight(string(result.Stdout), "\n")
	if root == "" || strings.ContainsRune(root, 0) {
		return "", errors.New("invalid Git root")
	}
	return root, nil
}
func isTerminal(writer io.Writer) bool {
	file, ok := writer.(*os.File)
	if !ok {
		return false
	}
	_, err := unix.IoctlGetWinsize(int(file.Fd()), unix.TIOCGWINSZ)
	return err == nil
}

// Execute is shared by CLI and init, using explicit roots and literal settings.
// docs/adr/0089-go-native-review-lane.md:93.
func Execute(ctx context.Context, args []string, root, template string, environment map[string]string, stdout, stderr io.Writer) error {
	return execute(ctx, args, root, template, environment, stdout, stderr, true)
}

// ExecuteNonInteractive preserves init's captured-child presentation without
// concealing cancellable output descriptors. docs/adr/0089-go-native-review-lane.md:128.
func ExecuteNonInteractive(ctx context.Context, args []string, root, template string, environment map[string]string, stdout, stderr io.Writer) error {
	return execute(ctx, args, root, template, environment, stdout, stderr, false)
}

func execute(ctx context.Context, args []string, root, template string, environment map[string]string, stdout, stderr io.Writer, interactive bool) error {

	if err := ctx.Err(); err != nil {
		return err
	}
	settings, path, err := load(ctx, root, environment)
	if err != nil {
		return err
	}
	command := "status"
	if len(args) > 0 && args[0] != "" {
		command = args[0]
	}
	switch command {
	case "status":
		installed := "not installed"
		if info, err := os.Stat(pathAt(root, workflow)); err == nil && info.Mode().IsRegular() {
			installed = "installed (.github/workflows/adversarial-review.yml)"
		}
		return output.WriteEvent(ctx, stdout, []byte(fmt.Sprintf("review lane: %s\n  model:     %s\n  secret:    %s\n  workflow:  %s\n", settings.value("REVIEW_LANE", "off"), settings.value("REVIEW_MODEL", "<frontier tier for "+settings.value("MODEL_PROVIDER", "openrouter")+">"), settings.Secret(), installed)))
	case "secret-name":
		return output.WriteEvent(ctx, stdout, []byte(settings.Secret()+"\n"))
	case "pending":
		return pending(ctx, root, settings, stdout)
	case "enable":
		secret := settings.Secret()
		if len(args) > 1 && args[1] != "" {
			secret = args[1]
		}
		return enable(ctx, root, template, path, secret, settings, stdout, stderr, interactive)
	case "disable":
		return disable(ctx, root, path, stderr, stdout)
	default:
		return &commandError{"usage: factory review-lane [status|enable|disable|pending|secret-name]", 2}
	}
}
func pending(ctx context.Context, root string, s Settings, out io.Writer) error {
	status, err := SecretStatus(ctx, root, s)
	if err != nil {
		return err
	}
	if status != "missing" && status != "unknown" {
		return nil
	}
	return output.WriteEvent(ctx, out, []byte("The adversarial review lane is ON but needs a repository secret:\n  add a secret named "+s.Secret()+"\n  GitHub -> Settings -> Secrets and variables -> Actions -> New repository secret\n  Until then the lane comments to say the secret is missing.\n"))
}
