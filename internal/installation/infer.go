package installation

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/anoop2811/software-factory-template/internal/config"
	"github.com/anoop2811/software-factory-template/internal/installationfs"
	"github.com/anoop2811/software-factory-template/internal/installedlayout"
	"github.com/anoop2811/software-factory-template/internal/staging"
)

// InferCandidate uses configuration hints and independently projected asset bytes.
// Metadata alone cannot establish ownership. docs/adr/0101-go-release-handoff.md:304.
func InferCandidate(ctx context.Context, root string, source staging.Options, overrides []string) (result Request, returned error) {
	tree, err := installationfs.Open(ctx, root)
	if err != nil {
		return result, err
	}
	defer func() { returned = errors.Join(returned, tree.Close()) }()
	_, data, err := observe(ctx, tree, "factory.yaml", 1<<20)
	if err != nil {
		return result, err
	}
	inputs := map[string]string{}
	keys := map[string]string{"PROJECT_NAME": "project_display_name", "PROJECT_SLUG": "project_name", "GITHUB_OWNER": "github_owner", "OPENCODE_USERNAME": "opencode_username", "PROTECTED_PATH": "protected_paths", "DOCS_ROOT": "docs_root", "CITATION_PREFIX": "citation_prefix"}
	for key, yaml := range keys {
		count := 0
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, yaml+":") {
				count++
			}
		}
		if count > 1 {
			return result, conflict("ambiguous project metadata; provide an unambiguous configuration")
		}
		if count == 1 {
			inputs[key], err = config.GetBytes(ctx, data, yaml, "")
			if err != nil {
				return result, err
			}
		}
	}
	seen := map[string]bool{}
	for _, input := range overrides {
		key, value, found := strings.Cut(input, "=")
		if !found || keys[key] == "" || seen[key] || len(value) > 64<<10 || !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
			return result, conflict("invalid explicit project input")
		}
		seen[key], inputs[key] = true, value
	}
	if len(strings.Fields(inputs["PROTECTED_PATH"])) > 1 {
		return result, conflict("multiple protected paths cannot identify a legacy initializer input")
	}
	result = Request{Source: source, Inputs: inputs, Candidate: true}
	current, control, err := observe(ctx, tree, ".factory/installation.current", 2<<20)
	if err != nil {
		return result, err
	}
	if current != nil {
		descriptor, err := installedlayout.Decode(ctx, control)
		if err != nil {
			return result, err
		}
		result.Profile = "go-hybrid-v1"
		result.Before = staging.Options{Version: descriptor.Version, Revision: descriptor.Revision, Target: descriptor.Target, Installation: true}
		return result, nil
	}
	bestScore, tied := 0, false
	for _, profile := range []string{"v0.1.6", "bash-baseline"} {
		request := result
		request.Profile = profile
		proposal, err := Propose(ctx, root, request)
		if err != nil {
			return result, err
		}
		score := 0
		for _, action := range proposal.Actions {
			if action.Path != ".factory-version" && (action.Action == "replace" || action.Action == "retire") {
				score++
			}
		}
		if score > bestScore {
			bestScore, result.Profile, tied = score, profile, false
		} else if score == bestScore {
			tied = true
		}
	}
	if bestScore == 0 || tied {
		return result, conflict("installation does not uniquely match a supported predecessor; preserve files and supply missing project inputs")
	}
	return result, ctx.Err()
}
