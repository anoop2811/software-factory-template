package staging

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func authenticate(ctx context.Context, options Options, workspace string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	verifier := options.Verifier
	if verifier == "" {
		verifier = "gh"
	}
	// #nosec G204 -- Explicit trusted verifier, fixed argument structure, and private snapshots; no shell or candidate execution.
	command := exec.CommandContext(ctx, verifier, "attestation", "verify", filepath.Join(workspace, "archive"),
		"--repo", "anoop2811/software-factory-template",
		"--signer-workflow", "anoop2811/software-factory-template/.github/workflows/runtime-release.yml",
		"--source-digest", options.Revision, "--source-ref", "refs/tags/"+options.Version,
		"--bundle", filepath.Join(workspace, "attestation"), "--custom-trusted-root", filepath.Join(workspace, "trusted-root"), "--deny-self-hosted-runners")
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if key != "GH_HOST" && key != "GH_PROMPT_DISABLED" {
			command.Env = append(command.Env, entry)
		}
	}
	command.Env = append(command.Env, "GH_HOST=github.com", "GH_PROMPT_DISABLED=1")
	var diagnostics boundedDiagnostics
	command.Stdout, command.Stderr = &diagnostics, &diagnostics
	command.WaitDelay = time.Second
	if err := command.Run(); err != nil {
		return fmt.Errorf("GitHub attestation verification: %w: %s", errors.Join(err, ctx.Err()), diagnostics.text.String())
	}
	return nil
}

type boundedDiagnostics struct{ text strings.Builder }

func (b *boundedDiagnostics) Write(p []byte) (int, error) {
	n := len(p)
	remaining := 64*1024 - b.text.Len()
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = b.text.Write(p)
	}
	return n, nil
}
