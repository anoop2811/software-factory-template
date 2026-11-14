package releasecmd

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

	"github.com/anoop2811/software-factory-template/internal/installationfs"
	"github.com/anoop2811/software-factory-template/internal/output"
)

const reviewClientUsage = "Usage: factory ci review-client --go-candidate TAG --expected-revision SHA --output DIR\n\nExport an authenticated host-native review client into a new private directory outside the checkout.\n"

type reviewClientOptions struct {
	tag, revision, directory string
}

// Parse all supplied operands even with help, without inspecting the filesystem.
// docs/adr/0153-authenticated-review-client-bootstrap.md:38.
func parseReviewClient(args []string) (reviewClientOptions, bool, error) {
	var option reviewClientOptions
	seen := map[string]bool{}
	help := false
	for index := 0; index < len(args); index++ {
		name, value, supplied := strings.Cut(args[index], "=")
		if name == "-h" {
			name = "--help"
		}
		if seen[name] {
			return option, false, errors.New("duplicate review-client argument")
		}
		seen[name] = true
		switch name {
		case "--help":
			if supplied {
				return option, false, errors.New("unexpected review-client help value")
			}
			help = true
		case "--go-candidate", "--expected-revision", "--output":
			if !supplied {
				index++
				if index == len(args) {
					return option, false, errors.New("missing review-client argument value")
				}
				value = args[index]
			}
			switch name {
			case "--go-candidate":
				if !exactRelease(value) {
					return option, false, errors.New("invalid review-client release")
				}
				option.tag = value
			case "--expected-revision":
				if !revisionPattern.MatchString(value) {
					return option, false, errors.New("invalid review-client expected revision")
				}
				option.revision = value
			case "--output":
				if !reviewClientPath(value) {
					return option, false, errors.New("review-client output must be an absolute canonical non-root path")
				}
				option.directory = value
			}
		default:
			return option, false, errors.New("unknown review-client argument")
		}
	}
	if !help && (option.tag == "" || option.revision == "" || option.directory == "") {
		return option, false, errors.New("review-client requires release, expected revision and output")
	}
	return option, help, nil
}

func reviewClientPath(path string) bool {
	return filepath.IsAbs(path) && path != "/" && filepath.Clean(path) == path && !strings.ContainsRune(path, 0)
}

func runReviewClient(parent context.Context, args []string, environment map[string]string, stdin io.Reader, stdout, stderr io.Writer) (status int) {
	option, help, err := parseReviewClient(args)
	if err != nil {
		_ = writeDiagnostic(stderr, "factory ci review-client: %s\n", err)
		return 2
	}
	if help {
		if output.WriteEvent(parent, stdout, []byte(reviewClientUsage)) != nil {
			return 1
		}
		return 0
	}
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	host, err := acquireWithPolicy(ctx, option.tag, environment, false)
	if err != nil {
		_ = writeDiagnostic(stderr, "factory ci review-client: acquisition failed: %s\n", err)
		return 1
	}
	closed := false
	defer func() {
		if !closed {
			status = host.finish(stderr, status)
		}
	}()
	// Compare before any selected-owner execution, including imported acquisitions.
	// docs/adr/0153-authenticated-review-client-bootstrap.md:56.
	if host.options.Revision != option.revision {
		_ = writeDiagnostic(stderr, "factory ci review-client: acquired source revision does not match expected revision\n")
		return 1
	}
	digest, err := currentDigest(ctx)
	if ctx.Err() != nil {
		_ = writeDiagnostic(stderr, "factory ci review-client: acquisition canceled\n")
		return 1
	}
	if err != nil || digest != host.digest {
		return authenticatedHandoff(ctx, host, append([]string{"ci", "review-client"}, args...), environment, stdin, stdout, stderr)
	}
	path, err := exportReviewClient(ctx, host, option.directory)
	if err != nil {
		_ = writeDiagnostic(stderr, "factory ci review-client: %s\n", err)
		return 1
	}
	// Exporting-owner resource cleanup precedes its receipt. Enclosing supervisors
	// still require final status zero: docs/adr/0153-authenticated-review-client-bootstrap.md:85.
	err = host.close()
	closed = true
	if err != nil {
		_ = writeDiagnostic(stderr, "factory ci review-client: retain complete client at %q; acquisition cleanup failed: %s\n", path, err)
		return 1
	}
	receipt := struct {
		SchemaVersion int    `json:"schema_version"`
		Version       string `json:"version"`
		Revision      string `json:"revision"`
		Target        string `json:"target"`
		SHA256        string `json:"sha256"`
		Path          string `json:"path"`
	}{1, host.options.Version, host.options.Revision, host.options.Target, host.digest, path}
	data, err := json.Marshal(receipt)
	if err == nil {
		err = output.WriteEvent(ctx, stdout, append(data, '\n'))
	}
	if err != nil {
		_ = writeDiagnostic(stderr, "factory ci review-client: receipt output failed; complete client retained at %q: %s\n", path, err)
		return 1
	}
	return 0
}

// Source confinement and output custody both remain live during checked copying.
// docs/adr/0153-authenticated-review-client-bootstrap.md:71.
func exportReviewClient(ctx context.Context, host *acquired, directory string) (path string, returned error) {
	owner, err := openReviewClientOutput(ctx, directory)
	if err != nil {
		return "", err
	}
	defer func() { returned = errors.Join(returned, owner.close(context.WithoutCancel(ctx))) }()
	source, err := installationfs.Open(ctx, filepath.Dir(host.raw))
	if err != nil {
		return "", err
	}
	defer func() { returned = errors.Join(returned, source.Close()) }()
	file, err := source.Root.OpenFile(ctx, filepath.Base(host.raw), 256<<20)
	if err != nil {
		return "", err
	}
	defer func() { returned = errors.Join(returned, file.Close()) }()
	path, err = owner.publish(ctx, file.File, file.Metadata().Size, host.digest)
	if err != nil {
		return "", err
	}
	if err := file.Check(ctx); err != nil {
		return "", fmt.Errorf("complete client retained at %q; source custody changed: %w", path, err)
	}
	return path, nil
}
