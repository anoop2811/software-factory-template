package releasecmd

import (
	"context"
	"fmt"
	"io"
	"strings"

	"golang.org/x/mod/semver"
)

// DefaultVersion is prepared separately from the public installer's active pin.
// docs/adr/0108-default-go-cutover.md:26.
const DefaultVersion = "v0.2.0"

// RunDefault selects a literal release and reuses candidate authentication and
// transaction consent. A selected Go release never falls back to source scripts.
// docs/adr/0108-default-go-cutover.md:26.
func RunDefault(ctx context.Context, verb string, args []string, selectedVersion string, environment map[string]string, stdin io.Reader, stdout, stderr io.Writer) int {
	version := DefaultVersion
	if verb == "init" && selectedVersion != "" {
		version = selectedVersion
	}
	callerVersion := environment["FACTORY_REF"]
	if callerVersion != "" {
		version = callerVersion
	}
	forwarded := make([]string, 0, len(args)+2)
	seen := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg != "--ref" && !strings.HasPrefix(arg, "--ref=") {
			forwarded = append(forwarded, arg)
			continue
		}
		if seen {
			fmt.Fprintln(stderr, "factory release: repeated release selector")
			return 2
		}
		seen = true
		if arg == "--ref" {
			i++
			if i == len(args) {
				fmt.Fprintln(stderr, "factory release: --ref requires an exact Go release")
				return 2
			}
			version = args[i]
		} else {
			version = strings.TrimPrefix(arg, "--ref=")
		}
	}
	// An installed prerelease may initialize its own authenticated selection;
	// stable defaults and explicit --ref select stable Go releases only.
	requireStable := seen || callerVersion != "" || selectedVersion == "" || verb != "init"
	if !validTag(version) || requireStable && (!semver.IsValid(version) || semver.Prerelease(version) != "" || semver.Compare(version, DefaultVersion) < 0) {
		fmt.Fprintln(stderr, "factory release: select an exact stable Go release; use --go-candidate for a prerelease or explicit rollback for a predecessor")
		return 2
	}
	forwarded = append(forwarded, "--go-candidate", version)
	return run(ctx, verb, forwarded, environment, stdin, stdout, stderr, requireStable)
}
