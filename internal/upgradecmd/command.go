// Package upgradecmd provides the source-built read-only upgrade preview.
package upgradecmd

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

	"github.com/anoop2811/software-factory-template/internal/assessment"
	"github.com/anoop2811/software-factory-template/internal/output"
)

type options struct {
	source       string
	paths        []string
	confirmation string
	json         bool
}

// Claimed reserves any preview-marker spelling before legacy script discovery.
// docs/adr/0082-go-public-upgrade-preview.md:23.
func Claimed(args []string) bool {
	for _, arg := range args {
		if arg == "--dry-run" || strings.HasPrefix(arg, "--dry-run=") {
			return true
		}
	}
	return false
}

// Run performs only the strict, read-only preview and owns its diagnostics/status.
// docs/adr/0082-go-public-upgrade-preview.md:96.
func Run(parent context.Context, args []string, stdout, stderr io.Writer) int {
	// Keep broken pipes on the checked I/O path without canceling the preview.
	// docs/adr/0082-go-public-upgrade-preview.md:144.
	brokenPipe := make(chan os.Signal, 1)
	signal.Notify(brokenPipe, syscall.SIGPIPE)
	defer signal.Stop(brokenPipe)
	option, ok := parse(args)
	if !ok {
		_, _ = fmt.Fprintln(stderr, "factory upgrade: invalid preview arguments")
		return 2
	}
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	report := previewReport{SchemaVersion: 1, Mode: "dry_run", Coverage: "partial", OwnershipBasis: "unproven", AuthorizedPaths: []string{}, RecoveryAssessment: "not_assessed"}
	var err error
	if len(option.paths) == 0 {
		report.Plan, err = assessment.Plan(ctx, ".", option.source)
	} else {
		var adopted assessment.AdoptionPreview
		adopted, err = assessment.PreviewAdoption(ctx, ".", option.source, option.paths, option.confirmation)
		if err == nil {
			report.Plan = adopted.Plan
			report.AdoptionProposal = &adopted.Proposal
			report.OwnershipBasis = adopted.OwnershipBasis
			report.AuthorizedPaths = adopted.AuthorizedPaths
		}
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "factory upgrade:", assessmentMessage(err))
		return assessment.ErrorStatus(err)
	}
	var data []byte
	if option.json {
		data, err = json.Marshal(report)
		data = append(data, '\n')
	} else {
		data = []byte(renderText(report, option.confirmation != ""))
	}
	if err == nil {
		err = output.WriteEvent(ctx, stdout, data)
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "factory upgrade: cannot write preview")
		return 1
	}
	return report.Plan.Status()
}

func parse(args []string) (options, bool) {
	var result options
	seen := make(map[string]bool)
	for i := 0; i < len(args); i++ {
		name, value, attached := strings.Cut(args[i], "=")
		switch name {
		case "--dry-run", "--json":
			if attached || seen[name] {
				return options{}, false
			}
			seen[name] = true
			if name == "--json" {
				result.json = true
			}
		case "--source", "--adopt-path", "--confirm-adoption":
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
			case "--source":
				result.source = value
			case "--adopt-path":
				result.paths = append(result.paths, value)
			case "--confirm-adoption":
				result.confirmation = value
			}
		default:
			return options{}, false
		}
	}
	if !seen["--dry-run"] || !seen["--source"] || seen["--confirm-adoption"] && len(result.paths) == 0 {
		return options{}, false
	}
	return result, true
}

// Only known fixed domain reasons may cross the public diagnostic boundary.
func assessmentMessage(err error) string {
	if errors.Is(err, context.Canceled) {
		return "preview canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "preview deadline exceeded"
	}
	var failure *assessment.Failure
	if errors.As(err, &failure) {
		return failure.Error()
	}
	return "cannot assess preview"
}
