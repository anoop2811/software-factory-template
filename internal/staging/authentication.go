package staging

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anoop2811/software-factory-template/internal/native"
)

func authenticate(ctx context.Context, options Options, workspace string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	verifier := options.Verifier
	if verifier == "" {
		verifier = "gh"
	}
	args := []string{verifier, "attestation", "verify", filepath.Join(workspace, "archive"),
		"--repo", "anoop2811/software-factory-template",
		"--signer-workflow", "anoop2811/software-factory-template/.github/workflows/runtime-release.yml",
		"--source-digest", options.Revision, "--source-ref", "refs/tags/" + options.Version,
		"--bundle", filepath.Join(workspace, "attestation"), "--custom-trusted-root", filepath.Join(workspace, "trusted-root"), "--deny-self-hosted-runners"}
	environment := map[string]string{}
	for _, entry := range os.Environ() {
		key, value, _ := strings.Cut(entry, "=")
		environment[key] = value
	}
	environment["GH_HOST"], environment["GH_PROMPT_DISABLED"] = "github.com", "1"
	// Verification shares the same bounded descendant ownership as all native
	// release tools. docs/adr/0101-go-release-handoff.md:140.
	result, err := native.ExecuteCommandBounded(ctx, workspace, args, environment, 64<<10, 64<<10)
	if err != nil || result.ExitCode == nil || *result.ExitCode != 0 || !result.ExitConfirmed || result.OwnershipUnconfirmed {
		if err == nil {
			err = errors.New("trusted verifier did not complete successfully")
		}
		exit := "unavailable"
		if result.ExitCode != nil {
			exit = fmt.Sprint(*result.ExitCode)
		}
		// docs/adr/0101-go-release-handoff.md:534 — preserve observed rejection details.
		return fmt.Errorf("GitHub attestation verification: %w (outcome=%s exit=%s exit_confirmed=%t ownership_unconfirmed=%t): stdout=%q stderr=%q", errors.Join(err, ctx.Err()), result.Outcome, exit, result.ExitConfirmed, result.OwnershipUnconfirmed, result.Stdout, result.Stderr)
	}
	return nil
}
