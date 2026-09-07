// Package staging authenticates runtime archives before confined extraction.
package staging

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ErrInvalidRequest distinguishes malformed operands from failed verification.
var ErrInvalidRequest = errors.New("invalid staging request")

var (
	revisionPattern = regexp.MustCompile(`^[a-f0-9]{40}$`)
	versionPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,127}$`)
)

// Options supplies all identities and explicit trust inputs for one operation.
type Options struct {
	Archive, Version, Revision, Target, Output string
	Attestation, TrustedRoot, Verifier         string
	Local, ExplicitVerification                bool
}

// Result reports this operation only; it is not durable activation authority.
type Result struct {
	Version        string `json:"version"`
	Revision       string `json:"revision"`
	Target         string `json:"target"`
	Root           string `json:"root"`
	SHA256         string `json:"sha256"`
	Authentication string `json:"authentication"`
}

// Stage follows docs/adr/0061-runtime-bundle-staging.md:25. Neither trust mode
// executes candidate content, and no release failure falls back to local mode.
func Stage(ctx context.Context, options Options) (Result, error) {
	if err := options.validate(); err != nil {
		return Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	output, err := resolveOutput(options.Output)
	if err != nil {
		return Result{}, fmt.Errorf("output: %w", err)
	}
	workspace, err := os.MkdirTemp("", "factory-runtime-stage-")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(workspace)
	root, err := os.OpenRoot(workspace)
	if err != nil {
		return Result{}, err
	}
	defer root.Close()
	digest, err := snapshot(ctx, root, options.Archive, "archive", 128<<20)
	if err != nil {
		return Result{}, fmt.Errorf("archive snapshot: %w", err)
	}
	authentication := "local-source"
	if !options.Local {
		if _, err = snapshot(ctx, root, options.Attestation, "attestation", 16<<20); err != nil {
			return Result{}, fmt.Errorf("attestation snapshot: %w", err)
		}
		if _, err = snapshot(ctx, root, options.TrustedRoot, "trusted-root", 16<<20); err != nil {
			return Result{}, fmt.Errorf("trusted-root snapshot: %w", err)
		}
		if err = authenticate(ctx, options, workspace); err != nil {
			return Result{}, err
		}
		authentication = "github-attestation"
	}
	if err = root.Mkdir("content", 0700); err != nil {
		return Result{}, err
	}
	content, err := root.OpenRoot("content")
	if err != nil {
		return Result{}, err
	}
	defer content.Close()
	if err = extract(ctx, root, content, options); err != nil {
		return Result{}, fmt.Errorf("archive: %w", err)
	}
	if err = validateIdentity(ctx, content, filepath.Join(workspace, "content"), options); err != nil {
		return Result{}, fmt.Errorf("bundle identity: %w", err)
	}
	if err = publish(ctx, content, output, options); err != nil {
		return Result{}, fmt.Errorf("publish: %w", err)
	}
	return Result{options.Version, options.Revision, options.Target, output, digest, authentication}, nil
}

func (o Options) validate() error {
	if o.Archive == "" || o.Output == "" || !revisionPattern.MatchString(o.Revision) || !versionPattern.MatchString(o.Version) {
		return fmt.Errorf("%w: archive, literal version, full lowercase commit SHA and output are required", ErrInvalidRequest)
	}
	switch strings.ToLower(o.Version) {
	case "latest", "current", "main", "head":
		return fmt.Errorf("%w: moving version alias", ErrInvalidRequest)
	}
	switch o.Target {
	case "linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64":
	default:
		return fmt.Errorf("%w: unsupported target", ErrInvalidRequest)
	}
	if strings.ContainsRune(o.Archive+o.Output+o.Attestation+o.TrustedRoot+o.Verifier, '\x00') {
		return fmt.Errorf("%w: NUL in path", ErrInvalidRequest)
	}
	if o.Local {
		if o.ExplicitVerification || o.Attestation != "" || o.TrustedRoot != "" || o.Verifier != "" {
			return fmt.Errorf("%w: local mode conflicts with verification inputs", ErrInvalidRequest)
		}
	} else if o.Attestation == "" || o.TrustedRoot == "" {
		return fmt.Errorf("%w: attestation and independently provisioned trusted-root are required", ErrInvalidRequest)
	}
	return nil
}

// Preserve filesystem traversal semantics until the physical parent is known.
func resolveOutput(output string) (string, error) {
	separator := string(filepath.Separator)
	for {
		output = strings.TrimRight(output, separator)
		if output == "" {
			output = separator
			break
		}
		if !strings.HasSuffix(output, separator+".") {
			break
		}
		output = strings.TrimSuffix(output, separator+".")
	}
	if _, err := os.Lstat(output); !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			err = os.ErrExist
		}
		return "", fmt.Errorf("output must not exist: %w", err)
	}
	index := strings.LastIndex(output, separator)
	parent, name := ".", output
	if index >= 0 {
		parent, name = output[:index], output[index+1:]
		if parent == "" {
			parent = separator
		}
	}
	parent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return "", err
	}
	parent, err = filepath.Abs(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, name), nil
}
