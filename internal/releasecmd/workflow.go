package releasecmd

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/anoop2811/software-factory-template/internal/ciworkflow"
	"github.com/anoop2811/software-factory-template/internal/output"
)

const ciWorkflowUsage = "Usage: factory ci workflow --go-candidate TAG --origin-target OS/ARCH --installer-revision FULL_COMMIT_SHA\n\nRender a workflow to stdout for review; no project files are changed.\n"

type ciWorkflowOptions struct {
	tag, origin, installerRevision string
}

// The public renderer consumes explicit data and the existing output boundary.
// docs/adr/0145-installed-ci-workflow-rendering.md:29.
func runCIWorkflow(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	option, help, err := parseCIWorkflow(args)
	if err != nil {
		_ = writeDiagnostic(stderr, "factory ci workflow: %s; require --go-candidate, --origin-target and --installer-revision\n", err)
		return 2
	}
	value := ciWorkflowUsage
	if !help {
		value = ciworkflow.Generic(ciworkflow.Release{Tag: option.tag, OriginTarget: option.origin, SourceRevision: option.installerRevision, Repository: officialRepository})
	}
	if output.WriteEvent(ctx, stdout, []byte(value)) != nil {
		_ = writeDiagnostic(stderr, "factory ci workflow: output failed\n")
		return 1
	}
	return 0
}

func parseCIWorkflow(args []string) (ciWorkflowOptions, bool, error) {
	var option ciWorkflowOptions
	seen := map[string]bool{}
	help := false
	for index := 0; index < len(args); index++ {
		name, value, supplied := strings.Cut(args[index], "=")
		if name == "-h" {
			name = "--help"
		}
		if seen[name] {
			return option, false, errors.New("duplicate workflow argument")
		}
		seen[name] = true
		switch name {
		case "--help":
			if supplied {
				return option, false, errors.New("unexpected help value")
			}
			help = true
		case "--go-candidate", "--origin-target", "--installer-revision":
			if !supplied {
				index++
				if index == len(args) {
					return option, false, errors.New("missing workflow argument")
				}
				value = args[index]
			}
			switch name {
			case "--go-candidate":
				if !exactRelease(value) {
					return option, false, errors.New("invalid workflow release")
				}
				option.tag = value
			case "--origin-target":
				if !supportedTarget(value) {
					return option, false, errors.New("invalid workflow origin target")
				}
				option.origin = value
			case "--installer-revision":
				if !revisionPattern.MatchString(value) {
					return option, false, errors.New("invalid installer revision")
				}
				option.installerRevision = value
			}
		default:
			return option, false, errors.New("unknown workflow argument")
		}
	}
	if !help && (option.tag == "" || option.origin == "" || option.installerRevision == "") {
		return option, false, errors.New("missing workflow selection")
	}
	return option, help, nil
}
