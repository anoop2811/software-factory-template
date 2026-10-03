// Package recoverycmd provides the source-built durable local recovery command.
package recoverycmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/anoop2811/software-factory-template/internal/assessment"
	"github.com/anoop2811/software-factory-template/internal/output"
)

type options struct {
	request assessment.RecoveryRequest
	json    bool
}

// Claimed reserves bare and attached creation markers before legacy dispatch.
// docs/adr/0091-durable-local-recovery-creation.md:25.
// Legacy source and revision options retain their detached operands.
// docs/adr/0091-durable-local-recovery-creation.md:192.
func Claimed(args []string) bool {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--source" || arg == "--ref" {
			i++
			continue
		}
		if arg == "--create-backup" || strings.HasPrefix(arg, "--create-backup=") {
			return true
		}
	}
	return false
}

// Run owns the strict creation grammar, bounded reporting and safe diagnostics.
// docs/adr/0091-durable-local-recovery-creation.md:50.
func Run(parent context.Context, args []string, environment map[string]string, stdout, stderr io.Writer) int {
	brokenPipe := make(chan os.Signal, 1)
	signal.Notify(brokenPipe, syscall.SIGPIPE)
	defer signal.Stop(brokenPipe)
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	option, ok := parse(args)
	if !ok {
		return diagnostic(ctx, stderr, "invalid recovery creation arguments", 2)
	}
	root, err := os.Getwd()
	if err == nil {
		root, err = filepath.EvalSymlinks(root)
	}
	if err != nil {
		return diagnostic(ctx, stderr, "cannot resolve current installation", 1)
	}
	report, err := assessment.CreateRecovery(ctx, root, option.request, environment)
	if err != nil {
		return diagnostic(ctx, stderr, creationMessage(err), assessment.ErrorStatus(err))
	}
	var data []byte
	if option.json {
		data, err = json.Marshal(report)
		data = append(data, '\n')
	} else {
		data = []byte(fmt.Sprintf("schema_version=%d\nmode=%s\ncoverage=%s\nscope=%s\nmigration_id=%s\npath=%s\nresult=%s\nfile_count=%d\nbytes=%d\nsource_revision=%s\ntarget_revision=%s\ntarget_authentication=%s\nrestorable=%t\nactivation_ready=%t\nprune_authorized=%t\n", report.SchemaVersion, report.Mode, report.Coverage, report.Scope, report.MigrationID, report.Path, report.Result, report.FileCount, report.Bytes, report.SourceRevision, report.TargetRevision, report.TargetAuthentication, report.Restorable, report.ActivationReady, report.PruneAuthorized))
	}
	if err == nil {
		err = output.WriteEvent(ctx, stdout, data)
	}
	if err != nil {
		return diagnostic(ctx, stderr, "cannot write recovery report; local recovery state may remain and needs inspection", 1)
	}
	return 0
}

// Use the shared checked output path for diagnostics too. An ended operation
// receives only a bounded final reporting allowance, never renewed write authority.
// docs/adr/0091-durable-local-recovery-creation.md:57.
func diagnostic(ctx context.Context, stderr io.Writer, message string, status int) int {
	report := ctx
	stop := func() {}
	if ctx.Err() != nil {
		status = 1
		report, stop = context.WithTimeout(context.Background(), time.Second)
	}
	defer stop()
	if err := output.WriteEvent(report, stderr, []byte("factory upgrade: "+message+"\n")); err != nil {
		return 1
	}
	return status
}

func parse(args []string) (options, bool) {
	var result options
	seen := make(map[string]bool)
	for i := 0; i < len(args); i++ {
		name, value, attached := strings.Cut(args[i], "=")
		switch name {
		case "--create-backup", "--json":
			if attached || seen[name] {
				return options{}, false
			}
			seen[name] = true
			result.json = result.json || name == "--json"
		case "--migration-id", "--target-revision", "--adopt-path", "--confirm-adoption":
			if name != "--adopt-path" && seen[name] {
				return options{}, false
			}
			seen[name] = true
			if !attached {
				i++
				if i == len(args) || strings.HasPrefix(args[i], "-") {
					return options{}, false
				}
				value = args[i]
			}
			if value == "" {
				return options{}, false
			}
			switch name {
			case "--migration-id":
				result.request.MigrationID = value
			case "--target-revision":
				result.request.TargetRevision = value
			case "--adopt-path":
				result.request.Paths = append(result.request.Paths, value)
			case "--confirm-adoption":
				result.request.Confirmation = value
			}
		default:
			return options{}, false
		}
	}
	if !seen["--create-backup"] || !seen["--migration-id"] || !seen["--target-revision"] || !seen["--adopt-path"] || !seen["--confirm-adoption"] {
		return options{}, false
	}
	return result, true
}

func creationMessage(err error) string {
	message := "cannot create local recovery"
	var failure *assessment.Failure
	switch {
	case errors.Is(err, context.Canceled):
		message = "recovery creation canceled"
	case errors.Is(err, context.DeadlineExceeded):
		message = "recovery creation deadline exceeded"
	case errors.As(err, &failure):
		message = failure.Error()
	}
	if assessment.RecoveryStateMayRemain(err) {
		message += "; local recovery state may remain and needs inspection"
	}
	return message
}
