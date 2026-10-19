package releasecmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/anoop2811/software-factory-template/internal/initcmd"
	"github.com/anoop2811/software-factory-template/internal/installation"
	"github.com/anoop2811/software-factory-template/internal/native"
	"github.com/anoop2811/software-factory-template/internal/output"
)

func execute(ctx context.Context, verb string, option options, environment map[string]string, stdin io.Reader, stdout, stderr io.Writer) (status int) {
	candidate, err := acquire(ctx, option.tag, environment)
	if err != nil {
		fmt.Fprintln(stderr, "factory candidate: "+err.Error())
		return 1
	}
	defer func() { status = candidate.finish(stderr, status) }()
	digest, err := currentDigest(ctx)
	if ctx.Err() != nil {
		fmt.Fprintln(stderr, "factory candidate: candidate handoff canceled")
		return 1
	}
	// A trusted system/shared-directory caller need not satisfy private image
	// custody. If it cannot prove the same private bytes, run the freshly
	// authenticated candidate instead; never use it as an installation shortcut.
	if err != nil || digest != candidate.digest {
		return handoff(ctx, verb, option, candidate, environment, stdin, stdout, stderr)
	}
	if verb == "init" {
		return initcmd.RunQualified(ctx, option.args, filepath.Join(candidate.staged, "installation/assets"), filepath.Join(candidate.staged, "bin/factory-runtime"), candidate.image, environment, stdin, stdout, stderr)
	}
	root, err := projectRoot(ctx, environment)
	if err != nil {
		fmt.Fprintln(stderr, "factory candidate: target root unavailable")
		return 1
	}
	request, err := installation.InferCandidate(ctx, root, candidate.options, option.projectInputs)
	if err != nil {
		fmt.Fprintln(stderr, "factory candidate: "+err.Error())
		return installation.ErrorStatus(err)
	}
	var previous *acquired
	if request.Profile == "go-hybrid-v1" {
		previous, err = acquire(ctx, request.Before.Version, withoutCache(environment))
		if err != nil {
			fmt.Fprintln(stderr, "factory candidate: predecessor acquisition failed: "+err.Error())
			return 1
		}
		defer func() { status = previous.finish(stderr, status) }()
		if previous.options.Revision != request.Before.Revision || previous.options.Target != request.Source.Target {
			fmt.Fprintln(stderr, "factory candidate: predecessor release identity changed")
			return 2
		}
		request.Before = previous.options
	}
	references := newRetentionAcquisitions(environment, candidate, previous)
	defer func() { status = references.finish(stderr, status) }()
	proposal, err := installation.ProposeRetaining(ctx, root, request, references.resolve)
	if err != nil {
		fmt.Fprintln(stderr, "factory candidate: "+err.Error())
		return installation.ErrorStatus(err)
	}
	if option.dryRun || option.confirmation == "" {
		var data []byte
		if option.json {
			data, err = json.Marshal(proposal)
			data = append(data, '\n')
		} else {
			apply := []string{"upgrade", "--go-candidate", option.tag, "--confirm", proposal.ProposalDigest, "--quiescent"}
			for _, input := range option.projectInputs {
				apply = append(apply, "--project-input", input)
			}
			data = []byte(fmt.Sprintf("candidate %s commit=%s target=%s (network acquisition; project unchanged)\nprofile=%s proposal_digest=%s\nactions=%d blockers=%d\nApply with:\ncurl -fsSL https://softwareaifactory.sh/install.sh | sh -s -- %s\n", option.tag, request.Source.Revision, request.Source.Target, proposal.Profile, proposal.ProposalDigest, len(proposal.Actions), len(proposal.Blockers), quotedArguments(apply)))
		}
		if !option.json {
			data = append(data, installation.RetentionDiagnosticReport(proposal.Retention)...)
		}
		if err == nil {
			err = output.WriteEvent(ctx, stdout, data)
		}
		if err != nil {
			return 1
		}
		if len(proposal.Blockers) != 0 || len(proposal.RollbackBlockers) != 0 {
			return 2
		}
		return 0
	}
	request.Confirmation, request.Quiescent = option.confirmation, option.quiescent
	if request.Confirmation != proposal.ProposalDigest || len(proposal.Blockers) != 0 || len(proposal.RollbackBlockers) != 0 {
		fmt.Fprintln(stderr, "factory candidate: proposal changed or has preserved blockers; request a fresh preview")
		return 2
	}
	request.MigrationID = "candidate-" + proposal.ProposalDigest[:24]
	retention, applyErr := installation.ApplyRetaining(ctx, root, request, references.resolve)
	// Preserve pending recovery custody even if the diagnostic stream fails.
	// docs/adr/0106-go-backup-retention.md:79.
	if applyErr != nil && !installation.CleanupIncomplete(applyErr) {
		reportRecovery(root, request, candidate, previous, stderr)
	}
	if reportErr := output.WriteEvent(context.WithoutCancel(ctx), stderr, []byte(installation.RetentionDiagnosticReport(retention))); reportErr != nil {
		return 1
	}
	if applyErr != nil {
		fmt.Fprintln(stderr, "factory candidate: "+applyErr.Error())
		return installation.ErrorStatus(applyErr)
	}
	fmt.Fprintln(stderr, "factory candidate: installation completed")
	return 0
}

// docs/adr/0101-go-release-handoff.md:579 — report retention without changing
// the operation's classified status; only actual cleanup failure overrides it.
func (a *acquired) finish(stderr io.Writer, status int) int {
	if a.retain {
		fmt.Fprintln(stderr, "factory candidate: "+a.preservationNotice())
	}
	if err := a.close(); err != nil {
		fmt.Fprintln(stderr, "factory candidate: "+err.Error())
		return 1
	}
	return status
}

// A partial transaction keeps its authenticated inputs and an executable path
// outside the interrupted active selection. Saved scripts are never executed.
// docs/adr/0101-go-release-handoff.md:179.
func reportRecovery(root string, request installation.Request, candidate, previous *acquired, stderr io.Writer) {
	retained := false
	for _, path := range []string{".factory/runtime-publication.pending", ".factory/installation-transactions/" + request.MigrationID + ".json"} {
		if _, err := os.Lstat(filepath.Join(root, path)); !os.IsNotExist(err) {
			retained = true
		}
	}
	if !retained {
		return
	}
	candidate.retain = true
	if previous != nil {
		previous.retain = true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	preview, err := installation.RecoveryPreview(ctx, root, request.MigrationID)
	if err != nil {
		fmt.Fprintf(stderr, "factory candidate: preserve migration %s and private inputs; durable recovery state needs manual reconciliation before a fresh recovery preview.\n", request.MigrationID)
		return
	}
	args := append([]string{filepath.Join(candidate.staged, "bin/factory-runtime"), "upgrade", "--installation"}, preview...)
	args = append(args, "--profile", request.Profile)
	imageArgs := func(prefix string, image *acquired) {
		args = append(args, "--"+prefix+"source", image.options.Archive, "--"+prefix+"version", image.options.Version, "--"+prefix+"revision", image.options.Revision, "--"+prefix+"target", image.options.Target, "--"+prefix+"attestation", image.options.Attestation, "--"+prefix+"trusted-root", image.options.TrustedRoot)
	}
	imageArgs("", candidate)
	if previous != nil {
		imageArgs("before-", previous)
	}
	keys := make([]string, 0, len(request.Inputs))
	for key := range request.Inputs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		args = append(args, "--project-input", key+"="+request.Inputs[key])
	}
	fmt.Fprintf(stderr, "factory candidate: interrupted migration %s; preserve the project and recovery evidence.\nRecovery preview (no changes):\n%s\n", request.MigrationID, quotedArguments(args))
}

func quotedArguments(args []string) string {
	quoted := make([]string, len(args))
	for index, value := range args {
		quoted[index] = "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
	}
	return strings.Join(quoted, " ")
}

func withoutCache(environment map[string]string) map[string]string {
	result := make(map[string]string, len(environment))
	for key, value := range environment {
		if key != "FACTORY_RELEASE_DIR" {
			result[key] = value
		}
	}
	return result
}

func handoff(ctx context.Context, verb string, option options, candidate *acquired, environment map[string]string, stdin io.Reader, stdout, stderr io.Writer) int {
	// Only freshly authenticated bytes with the complete image's exact hash run.
	// docs/adr/0101-go-release-handoff.md:102.
	digest, err := digestFile(ctx, candidate.raw, 256<<20)
	if err != nil || digest != candidate.digest {
		fmt.Fprintln(stderr, "factory candidate: authenticated executable changed")
		return 1
	}
	if err := os.Chmod(candidate.raw, 0700); err != nil { // #nosec G302 -- This exact owner-only executable was authenticated against the pinned official release before chmod.
		fmt.Fprintln(stderr, "factory candidate: cannot make authenticated handoff executable: "+err.Error())
		return 1
	}
	env := withoutCache(environment)
	env["FACTORY_RELEASE_DIR"] = filepath.Dir(candidate.raw)
	args := append([]string{candidate.raw, verb, "--go-candidate", option.tag}, option.args...)
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(stderr, "factory candidate: cannot resolve handoff working directory: "+err.Error())
		return 1
	}
	var result native.Execution
	inputFile, inputOK := stdin.(*os.File)
	outFile, outOK := stdout.(*os.File)
	errFile, errOK := stderr.(*os.File)
	if inputOK && outOK && errOK {
		result, err = native.ExecuteInherited(ctx, root, args, env, native.InheritedStreams{Stdin: inputFile, Stdout: outFile, Stderr: errFile}, time.Hour)
	} else {
		// This command's public process boundary uses real inherited descriptors.
		err = errors.New("candidate handoff requires inherited process streams")
	}
	if err != nil || result.ExitCode == nil || !result.ExitConfirmed || result.OwnershipUnconfirmed {
		candidate.retain = result.OwnershipUnconfirmed
		fmt.Fprintln(stderr, "factory candidate: authenticated handoff did not complete safely")
		return 1
	}
	return *result.ExitCode
}
