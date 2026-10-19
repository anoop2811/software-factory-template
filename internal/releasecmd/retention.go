package releasecmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/anoop2811/software-factory-template/internal/installation"
	"github.com/anoop2811/software-factory-template/internal/native"
	"github.com/anoop2811/software-factory-template/internal/output"
	"github.com/anoop2811/software-factory-template/internal/staging"
	"github.com/spf13/cobra"
)

// BackupClaimed reserves standalone controls before installed or legacy dispatch.
// docs/adr/0106-go-backup-retention.md:63.
func BackupClaimed(args []string) bool {
	for _, arg := range args {
		if arg == "--keep-backup" || strings.HasPrefix(arg, "--keep-backup=") || arg == "--prune-backup" || strings.HasPrefix(arg, "--prune-backup=") {
			return true
		}
	}
	return false
}

type retentionAcquisitions struct {
	environment map[string]string
	borrowed    map[installation.ReleaseIdentity]*acquired
	opened      []*acquired
}

func newRetentionAcquisitions(environment map[string]string, borrowed ...*acquired) *retentionAcquisitions {
	result := &retentionAcquisitions{environment: environment, borrowed: map[installation.ReleaseIdentity]*acquired{}}
	for _, image := range borrowed {
		if image != nil {
			result.borrowed[installation.ReleaseIdentity{Version: image.options.Version, Revision: image.options.Revision, Target: image.options.Target}] = image
		}
	}
	return result
}
func (r *retentionAcquisitions) resolve(ctx context.Context, hint installation.ReleaseIdentity) (staging.Options, error) {
	if image := r.borrowed[hint]; image != nil {
		return image.options, nil
	}
	image, err := acquire(ctx, hint.Version, withoutCache(r.environment))
	if err != nil {
		return staging.Options{}, err
	}
	r.opened = append(r.opened, image)
	actual := installation.ReleaseIdentity{Version: image.options.Version, Revision: image.options.Revision, Target: image.options.Target}
	if actual != hint {
		return staging.Options{}, errors.New("historical release identity changed")
	}
	r.borrowed[hint] = image
	return image.options, nil
}
func (r *retentionAcquisitions) finish(stderr io.Writer, status int) int {
	for i := len(r.opened) - 1; i >= 0; i-- {
		status = r.opened[i].finish(stderr, status)
	}
	return status
}

// RunBackup owns typed standalone controls; only pruning acquires trusted images.
// docs/adr/0106-go-backup-retention.md:63.
func RunBackup(parent context.Context, args []string, environment map[string]string, stdin io.Reader, stdout, stderr io.Writer) (status int) {
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	keep, prune := "", ""
	help := false
	seen := map[string]bool{}
	for i := 0; i < len(args); i++ {
		name, _, attached := strings.Cut(args[i], "=")
		if name == "--keep-backup" || name == "--prune-backup" {
			if seen[name] {
				fmt.Fprintln(stderr, "factory backup: repeated control operand")
				return 2
			}
			seen[name] = true
			if !attached {
				i++
			}
		}
	}
	command := &cobra.Command{Use: "factory upgrade", Args: cobra.NoArgs, SilenceErrors: true, SilenceUsage: true, DisableSuggestions: true, CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true}}
	command.SetOut(io.Discard)
	command.SetErr(io.Discard)
	command.Flags().StringVar(&keep, "keep-backup", "", "Hold a complete local recovery set")
	command.Flags().StringVar(&prune, "prune-backup", "", "Prune a qualified older recovery with a usable alternate")
	command.Flags().BoolVarP(&help, "help", "h", false, "Show backup controls")
	command.Run = func(*cobra.Command, []string) {}
	command.SetArgs(args)
	if err := command.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(stderr, "factory backup: invalid standalone backup arguments")
		return 2
	}
	if help {
		if err := output.WriteEvent(ctx, stdout, []byte("Usage: factory upgrade --keep-backup MIGRATION_ID\n       factory upgrade --prune-backup MIGRATION_ID\n\nCompleted controls and incomplete cleanup emit one JSON report. Invalid/preflight refusals use stderr without a report. Pruning authenticates historical images and preserves the immediately preceding usable recovery.\n")); err != nil {
			return 1
		}
		return 0
	}
	if (keep == "") == (prune == "") || keep != "" && !installation.ValidBackupID(keep) || prune != "" && !installation.ValidBackupID(prune) {
		fmt.Fprintln(stderr, "factory backup: select exactly one valid backup identifier")
		return 2
	}
	root, err := projectRoot(ctx, environment)
	if err != nil {
		fmt.Fprintln(stderr, "factory backup: project root unavailable")
		return 1
	}
	var report installation.RetentionReport
	if keep != "" {
		report, err = installation.KeepBackup(ctx, root, keep)
	} else {
		hint, hintErr := installation.CurrentReleaseHint(ctx, root)
		if hintErr != nil {
			fmt.Fprintln(stderr, "factory backup: "+hintErr.Error())
			return installation.ErrorStatus(hintErr)
		}
		current, acquireErr := acquire(ctx, hint.Version, withoutCache(environment))
		if acquireErr != nil {
			fmt.Fprintln(stderr, "factory backup: current release authentication failed: "+acquireErr.Error())
			return 1
		}
		defer func() { status = current.finish(stderr, status) }()
		if current.options.Revision != hint.Revision || current.options.Target != hint.Target {
			fmt.Fprintln(stderr, "factory backup: current release identity changed")
			return 2
		}
		digest, digestErr := currentDigest(ctx)
		if digestErr != nil || digest != current.digest {
			return handoffBackup(ctx, args, current, environment, stdin, stdout, stderr)
		}
		references := newRetentionAcquisitions(environment, current)
		defer func() { status = references.finish(stderr, status) }()
		report, err = installation.PruneBackup(ctx, root, prune, references.resolve)
	}
	if err != nil {
		fmt.Fprintln(stderr, "factory backup: "+err.Error())
		status = installation.ErrorStatus(err)
	}
	if report.Status == "planned" && err != nil {
		return status
	}
	data, writeErr := json.Marshal(report)
	if writeErr == nil {
		writeErr = output.WriteEvent(ctx, stdout, append(data, '\n'))
	}
	if writeErr != nil {
		fmt.Fprintln(stderr, "factory backup: cannot write backup report")
		return 1
	}
	return status
}
func handoffBackup(ctx context.Context, args []string, current *acquired, environment map[string]string, stdin io.Reader, stdout, stderr io.Writer) int {
	digest, err := digestFile(ctx, current.raw, 256<<20)
	if err != nil || digest != current.digest {
		fmt.Fprintln(stderr, "factory backup: authenticated executable changed")
		return 1
	}
	if err = os.Chmod(current.raw, 0700); err != nil {
		fmt.Fprintln(stderr, "factory backup: cannot prepare authenticated control executable")
		return 1
	} // #nosec G302 -- exact authenticated owner-only executable.
	input, inOK := stdin.(*os.File)
	out, outOK := stdout.(*os.File)
	diagnostic, errOK := stderr.(*os.File)
	if !inOK || !outOK || !errOK {
		fmt.Fprintln(stderr, "factory backup: authenticated control requires inherited streams")
		return 1
	}
	root, err := os.Getwd()
	if err != nil {
		return 1
	}
	argv := append([]string{current.raw, "upgrade"}, args...)
	result, err := native.ExecuteInherited(ctx, root, argv, withoutCache(environment), native.InheritedStreams{Stdin: input, Stdout: out, Stderr: diagnostic}, time.Hour)
	if err != nil || result.ExitCode == nil || !result.ExitConfirmed || result.OwnershipUnconfirmed {
		current.retain = result.OwnershipUnconfirmed
		fmt.Fprintln(stderr, "factory backup: authenticated control did not complete safely")
		return 1
	}
	return *result.ExitCode
}
