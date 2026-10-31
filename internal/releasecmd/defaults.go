package releasecmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/mod/semver"
)

// DefaultVersion is prepared for stable activation, not a publication claim.
// docs/adr/0118-default-packaged-go-initialization.md:12.
const DefaultVersion = "v0.2.0"

const minimumGoRelease = "v0.2.0"

// RunDefaultInit selects only an authenticated packaged initializer. The caller
// supplies an installed version only after physical admission and validation.
// docs/adr/0118-default-packaged-go-initialization.md:22.
func RunDefaultInit(parent context.Context, args []string, installedVersion string, environment map[string]string, stdin io.Reader, stdout, stderr io.Writer) int {
	return runDefault(parent, "init", args, installedVersion, environment, stdin, stdout, stderr)
}

// RunDefaultUpgrade selects a stable authenticated release without retaining an
// installed caller's ordinary shared lease. docs/adr/0122-default-packaged-go-upgrade.md:9.
func RunDefaultUpgrade(parent context.Context, args []string, environment map[string]string, stdin io.Reader, stdout, stderr io.Writer) int {
	return runDefault(parent, "upgrade", args, "", environment, stdin, stdout, stderr)
}

func runDefault(parent context.Context, verb string, args []string, installedVersion string, environment map[string]string, stdin io.Reader, stdout, stderr io.Writer) int {
	option, help, err := defaultOptions(verb, args, installedVersion, environment)
	if err != nil {
		fmt.Fprintln(stderr, "factory "+verb+": "+err.Error())
		return 2
	}
	if help {
		fmt.Fprint(stdout, defaultUsage(verb))
		return 0
	}
	return runSelected(parent, verb, option, environment, stdin, stdout, stderr)
}

func defaultUsage(verb string) string {
	if verb == "init" {
		return "Usage: factory init [TARGET] [--ref TAG] [--pack PACKS]\n\nSelect an authenticated packaged Go initializer.\nDefault: " + DefaultVersion + "; admitted installed callers reuse their selected release.\nOverride: --ref TAG, then FACTORY_REF (exact stable Go releases).\nExplicit prerelease: --go-candidate TAG. Local build: --local [--source DIR].\n"
	}
	return "Usage: factory upgrade [--ref TAG] [options]\n\nSelect an authenticated packaged Go upgrader.\nDefault: " + DefaultVersion + ". Override: --ref TAG, then FACTORY_REF (exact stable Go releases).\nExplicit prerelease: --go-candidate TAG. Local build: --local [--source DIR] [--before-source DIR].\nOptions: --dry-run --json --confirm DIGEST --quiescent --project-input KEY=VALUE\nJSON is preview-only; omit --json when confirming.\n"
}

// Initialization and upgrade share exact stable selection and argument framing.
// Only initialization may reuse an admitted version. docs/adr/0122-default-packaged-go-upgrade.md:23.
func defaultOptions(verb string, args []string, installedVersion string, environment map[string]string) (options, bool, error) {
	var tag string
	var forwarded []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			forwarded = append(forwarded, args[i:]...)
			break
		}
		if arg != "--ref" && !strings.HasPrefix(arg, "--ref=") {
			forwarded = append(forwarded, arg)
			continue
		}
		if tag != "" {
			return options{}, false, errors.New("select only one --ref")
		}
		if arg == "--ref" {
			i++
			if i == len(args) {
				return options{}, false, errors.New("--ref requires an exact stable Go release tag")
			}
			tag = args[i]
		} else {
			tag = strings.TrimPrefix(arg, "--ref=")
		}
		if !stableGoRelease(tag) {
			return options{}, false, errors.New("--ref requires an exact stable Go release tag at or after " + minimumGoRelease)
		}
	}
	// Share candidate grammar and its terminator-aware help/conflict validation.
	// The synthetic selector precedes every user token, including a literal --.
	// docs/adr/0118-default-packaged-go-initialization.md:44.
	option, help, err := parse(verb, append([]string{"--go-candidate", DefaultVersion}, forwarded...))
	if err != nil || help {
		return option, help, err
	}
	option.requireStable = true
	switch {
	case tag != "":
		option.tag = tag
	case environment["FACTORY_REF"] != "":
		option.tag = environment["FACTORY_REF"]
		if !stableGoRelease(option.tag) {
			return options{}, false, errors.New("FACTORY_REF requires an exact stable Go release tag at or after " + minimumGoRelease)
		}
	case verb == "init" && installedVersion != "":
		if !supportedGoRelease(installedVersion) {
			return options{}, false, errors.New("selected installation has no supported official release; choose --ref, --go-candidate or --local --source DIR")
		}
		option.tag = installedVersion
		option.requireStable = semver.Prerelease(installedVersion) == ""
	}
	return option, false, nil
}

func stableGoRelease(tag string) bool {
	return supportedGoRelease(tag) && semver.Prerelease(tag) == ""
}

// Authenticated predecessors may be official prereleases in the Go family.
// Ordinary target selection remains stable. docs/adr/0122-default-packaged-go-upgrade.md:52.
func supportedGoRelease(tag string) bool {
	return exactRelease(tag) && semver.Compare(tag, minimumGoRelease+"-0") >= 0
}

// Module semver accepts vMAJOR and vMAJOR.MINOR shorthands; release selection
// requires the complete original tag and retains its build metadata.
func exactRelease(tag string) bool {
	return validTag(tag) && semver.IsValid(tag) && semver.Canonical(tag)+semver.Build(tag) == tag
}

func (o options) selector() string {
	if o.requireStable {
		return "--ref"
	}
	return "--go-candidate"
}
