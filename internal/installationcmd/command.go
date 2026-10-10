// Package installationcmd owns the opt-in whole-installation command boundary.
package installationcmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/anoop2811/software-factory-template/internal/installation"
	"github.com/anoop2811/software-factory-template/internal/output"
	"github.com/anoop2811/software-factory-template/internal/staging"
	"github.com/spf13/cobra"
)

type options struct {
	after, before                                                    staging.Options
	profile, migrationID, confirmation, rollback, recover, direction string
	projectInputs                                                    []string
	installation, dryRun, json, quiescent                            bool
}

var (
	versionPattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,127}$`)
	revisionPattern  = regexp.MustCompile(`^[a-f0-9]{40}$`)
	digestPattern    = regexp.MustCompile(`^[a-f0-9]{64}$`)
	migrationPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
)

// Claimed reserves every explicit marker spelling, even a malformed operand.
// docs/adr/0098-whole-installation-upgrade-and-rollback.md:33.
func Claimed(args []string) bool {
	for _, arg := range args {
		if arg == "--installation" || strings.HasPrefix(arg, "--installation=") {
			return true
		}
	}
	return false
}

// Run owns whole-installation operand parsing and dispatch.
// docs/adr/0098-whole-installation-upgrade-and-rollback.md:37.
func Run(ctx context.Context, args []string, _ map[string]string, stdout, stderr io.Writer) int {
	if ctx.Err() != nil {
		return diagnostic(ctx, stderr, "installation operation canceled", 1)
	}
	if repeatedFlag(args) {
		return diagnostic(ctx, stderr, "invalid installation arguments", 2)
	}
	var option options
	var help strings.Builder
	status := 0
	operated := false
	command := &cobra.Command{
		Use:                "factory upgrade --installation",
		Short:              "Preview, upgrade, recover, or roll back a complete installation",
		Args:               cobra.NoArgs,
		SilenceUsage:       true,
		SilenceErrors:      true,
		DisableSuggestions: true,
		CompletionOptions:  cobra.CompletionOptions{DisableDefaultCmd: true},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !validOptions(option, cmd) {
				status = 2
				return nil
			}
			if option.dryRun {
				status = preview(cmd.Context(), option, stdout, stderr)
				operated = true
				return nil
			}
			status = apply(cmd.Context(), option, stderr)
			operated = true
			return nil
		},
	}
	command.SetOut(&help)
	command.SetErr(io.Discard)
	flags := command.Flags()
	flags.BoolVar(&option.installation, "installation", false, "Select the complete installation consumer")
	flags.BoolVar(&option.dryRun, "dry-run", false, "Inspect a fresh whole proposal without writes or external checks")
	flags.BoolVar(&option.json, "json", false, "Report the complete proposal as JSON")
	flags.BoolVar(&option.quiescent, "quiescent", false, "Declare maintained prevention of unbridged work during maintenance")
	flags.StringVar(&option.profile, "profile", "", "Before profile: v0.1.6, bash-baseline, or go-hybrid-v1 with a before image")
	flags.StringVar(&option.migrationID, "migration-id", "", "Fresh forward migration identifier")
	flags.StringVar(&option.confirmation, "confirm", "", "Current complete proposal digest")
	flags.StringVar(&option.rollback, "rollback", "", "Completed installation migration to reverse")
	flags.StringVar(&option.recover, "recover", "", "Interrupted installation migration to recover")
	flags.StringVar(&option.direction, "direction", "", "Recovery direction: forward or reverse")
	flags.StringArrayVar(&option.projectInputs, "project-input", nil, "Explicit unique baseline initializer input KEY=VALUE (repeatable)")
	imageFlags(command, &option.after, "")
	imageFlags(command, &option.before, "before-")
	command.SetArgs(args)
	if err := command.ExecuteContext(ctx); err != nil || status == 2 && !operated {
		return diagnostic(ctx, stderr, "invalid installation arguments", 2)
	}
	if help.Len() != 0 {
		if err := output.WriteEvent(ctx, stdout, []byte(help.String())); err != nil {
			return diagnostic(ctx, stderr, "cannot write installation help", 1)
		}
		return 0
	}
	if operated {
		return status
	}
	return diagnostic(ctx, stderr, "installation operations are unsupported", 1)
}

func imageFlags(command *cobra.Command, image *staging.Options, prefix string) {
	flags := command.Flags()
	flags.StringVar(&image.Archive, prefix+"source", "", "Complete installation archive")
	flags.StringVar(&image.Version, prefix+"version", "", "Expected literal version")
	flags.StringVar(&image.Revision, prefix+"revision", "", "Expected full lowercase commit SHA")
	flags.StringVar(&image.Target, prefix+"target", "", "Expected GOOS/GOARCH")
	flags.BoolVar(&image.Local, prefix+"local", false, "Explicit unauthenticated local source mode")
	flags.StringVar(&image.Attestation, prefix+"attestation", "", "Offline attestation bundle")
	flags.StringVar(&image.TrustedRoot, prefix+"trusted-root", "", "Independently provisioned trusted roots")
	flags.StringVar(&image.Verifier, prefix+"gh", "", "Trusted GitHub CLI executable")
}

// Repeated scalar operands cannot silently replace earlier consent or identity.
// docs/adr/0098-whole-installation-upgrade-and-rollback.md:362.
func repeatedFlag(args []string) bool {
	seen := map[string]bool{}
	stringFlags := map[string]bool{}
	for _, name := range []string{"source", "profile", "version", "revision", "target", "migration-id", "confirm", "rollback", "recover", "direction", "project-input", "attestation", "trusted-root", "gh", "before-source", "before-version", "before-revision", "before-target", "before-attestation", "before-trusted-root", "before-gh"} {
		stringFlags["--"+name] = true
	}
	for index := 0; index < len(args); index++ {
		name, _, attached := strings.Cut(args[index], "=")
		if !strings.HasPrefix(name, "--") {
			continue
		}
		if name != "--project-input" {
			if seen[name] {
				return true
			}
			seen[name] = true
		}
		if stringFlags[name] && !attached {
			index++
		}
	}
	return false
}

func validOptions(option options, command *cobra.Command) bool {
	for name, value := range map[string]string{"rollback": option.rollback, "recover": option.recover, "direction": option.direction, "confirm": option.confirmation, "migration-id": option.migrationID} {
		if command.Flags().Changed(name) && value == "" {
			return false
		}
	}

	if !option.installation || !validImage(option.after, command, "") || !validInputs(option.projectInputs) {
		return false
	}
	switch option.profile {
	case "v0.1.6", "bash-baseline":
		for _, name := range []string{"source", "version", "revision", "target", "local", "attestation", "trusted-root", "gh"} {
			if command.Flags().Changed("before-" + name) {
				return false
			}
		}
	case "go-hybrid-v1":
		if !validImage(option.before, command, "before-") {
			return false
		}
	default:
		return false
	}
	if option.rollback != "" && option.recover != "" {
		return false
	}
	if option.recover != "" {
		if !migrationPattern.MatchString(option.recover) || option.migrationID != "" || option.direction != "forward" && option.direction != "reverse" {
			return false
		}
	} else if option.direction != "" {
		return false
	}
	if option.rollback != "" && (!migrationPattern.MatchString(option.rollback) || option.migrationID != "") {
		return false
	}
	if option.dryRun {
		return option.migrationID == "" && option.confirmation == "" && !option.quiescent
	}
	if !option.quiescent || !digestPattern.MatchString(option.confirmation) {
		return false
	}
	return option.rollback != "" || option.recover != "" || migrationPattern.MatchString(option.migrationID)
}

func validImage(image staging.Options, command *cobra.Command, prefix string) bool {
	if image.Archive == "" || !versionPattern.MatchString(image.Version) || !revisionPattern.MatchString(image.Revision) ||
		strings.ContainsRune(image.Archive+image.Attestation+image.TrustedRoot+image.Verifier, 0) {
		return false
	}
	switch strings.ToLower(image.Version) {
	case "latest", "current", "main", "head":
		return false
	}
	switch image.Target {
	case "linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64":
	default:
		return false
	}
	verification := command.Flags().Changed(prefix+"attestation") || command.Flags().Changed(prefix+"trusted-root") || command.Flags().Changed(prefix+"gh")
	if image.Local {
		return !verification
	}
	return image.Attestation != "" && image.TrustedRoot != ""
}

func validInputs(inputs []string) bool {
	seen := map[string]bool{}
	for _, input := range inputs {
		key, value, found := strings.Cut(input, "=")
		if !found || seen[key] || len(value) > 64<<10 || !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
			return false
		}
		switch key {
		case "PROJECT_NAME", "PROJECT_SLUG", "GITHUB_OWNER", "OPENCODE_USERNAME", "PROTECTED_PATH", "DOCS_ROOT", "CITATION_PREFIX":
		default:
			return false
		}
		seen[key] = true
	}
	return true
}

func diagnostic(ctx context.Context, stderr io.Writer, message string, status int) int {
	if ctx.Err() != nil {
		status = 1
		var stop context.CancelFunc
		ctx, stop = context.WithTimeout(context.Background(), time.Second)
		defer stop()
	}
	if err := output.WriteEvent(ctx, stderr, []byte(fmt.Sprintf("factory upgrade: %s\n", message))); err != nil {
		return 1
	}
	return status
}

func request(option options) installation.Request {
	inputs := map[string]string{}
	for _, input := range option.projectInputs {
		key, value, _ := strings.Cut(input, "=")
		inputs[key] = value
	}
	return installation.Request{Source: option.after, Before: option.before, Profile: option.profile, MigrationID: option.migrationID, Confirmation: option.confirmation, Rollback: option.rollback, Recover: option.recover, Direction: option.direction, Inputs: inputs, Quiescent: option.quiescent}
}
func preview(ctx context.Context, option options, stdout, stderr io.Writer) int {
	root, err := os.Getwd()
	if err == nil {
		root, err = filepath.EvalSymlinks(root)
	}
	var proposal installation.Proposal
	if err == nil {
		proposal, err = installation.Propose(ctx, root, request(option))
	}
	if err != nil {
		message := "cannot inspect installation proposal"
		var failure *installation.Failure
		if errors.As(err, &failure) {
			message = failure.Reason
		}
		return diagnostic(ctx, stderr, message, installation.ErrorStatus(err))
	}
	var data []byte
	if option.json {
		data, err = json.Marshal(proposal)
		data = append(data, '\n')
	} else {
		data = []byte(fmt.Sprintf("installation profile=%s target=%s authentication=%s\nproposal_digest=%s\nactions=%d blockers=%d\n", proposal.Profile, proposal.Target, proposal.Authentication, proposal.ProposalDigest, len(proposal.Actions), len(proposal.Blockers)))
	}
	if err == nil {
		err = output.WriteEvent(ctx, stdout, data)
	}
	if err != nil {
		return diagnostic(ctx, stderr, "cannot write installation proposal", 1)
	}
	if len(proposal.Blockers) != 0 || len(proposal.RollbackBlockers) != 0 {
		return 2
	}
	return 0
}

func apply(ctx context.Context, option options, stderr io.Writer) int {
	root, err := os.Getwd()
	if err == nil {
		root, err = filepath.EvalSymlinks(root)
	}
	if err == nil {
		err = installation.Apply(ctx, root, request(option))
	}
	if err == nil {
		return 0
	}
	message := "installation transaction failed; preserve pending evidence and request fresh recovery preview"
	var failure *installation.Failure
	if errors.As(err, &failure) {
		message = failure.Reason
	}
	return diagnostic(ctx, stderr, message, installation.ErrorStatus(err))
}
