package installation

import (
	"bytes"
	"context"
	"maps"
	"strings"
	"unicode/utf8"

	"github.com/anoop2811/software-factory-template/internal/ciworkflow"
	"github.com/anoop2811/software-factory-template/internal/installationfs"
	"github.com/anoop2811/software-factory-template/internal/staging"
)

// Recorded inputs describe a closed projection; fresh images still establish it.
// No configuration secrets or caller-supplied paths enter this finite component.
// docs/adr/0148-transactional-installed-ci-migration.md:73.
type ciWorkflowContext struct {
	SchemaVersion int                `json:"schema_version"`
	Release       ciworkflow.Release `json:"release"`
	Before        ReleaseIdentity    `json:"before"`
	Current       map[string]string  `json:"current"`
	Historical    map[string]string  `json:"historical"`
}

func ciWorkflowPath(path string) bool {
	for _, name := range ciWorkflowNames {
		if path == ".github/workflows/"+name {
			return true
		}
	}
	return false
}

// These rows belong only to a transaction, never Selection or runtime admission.
func transactionRecipe(image staging.Inspection, ci *ciWorkflowContext) []role {
	recipe := rolesForImage(image)
	if ci == nil {
		return recipe
	}
	result := append([]role{}, recipe...)
	for _, name := range ciWorkflowNames {
		path := ".github/workflows/" + name
		operation := "retire"
		if name == "factory.yml" {
			operation = "generated-ci"
		}
		result = append(result, role{Path: path, Operation: operation, Mode: "0644", Required: true, Reason: "qualified CI transaction component"})
	}
	return result
}

func transactionRuntimeEntries(entries []journalEntry, ci *ciWorkflowContext) ([]journalEntry, bool) {
	if ci == nil {
		return entries, true
	}
	runtimeEntries := make([]journalEntry, 0, len(entries))
	seen := map[string]bool{}
	for _, entry := range entries {
		if ciWorkflowPath(entry.Path) {
			if seen[entry.Path] {
				return nil, false
			}
			seen[entry.Path] = true
		} else {
			runtimeEntries = append(runtimeEntries, entry)
		}
	}
	return runtimeEntries, len(seen) == len(ciWorkflowNames)
}

func matchesTransactionEntries(entries []journalEntry, recipe []role, ci *ciWorkflowContext) bool {
	runtimeEntries, complete := transactionRuntimeEntries(entries, ci)
	return complete && matchesRoleEntries(runtimeEntries, recipe)
}

func knownTransactionEntries(entries []journalEntry, ci *ciWorkflowContext) bool {
	runtimeEntries, complete := transactionRuntimeEntries(entries, ci)
	return complete && knownRoleEntries(runtimeEntries)
}

func selectedTransactionPath(path string, ci *ciWorkflowContext) bool {
	return selectedPath(path) || ci != nil && ciWorkflowPath(path)
}

func ciContextEqual(a, b *ciWorkflowContext) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.SchemaVersion == b.SchemaVersion && a.Release == b.Release && a.Before == b.Before && maps.Equal(a.Current, b.Current) && maps.Equal(a.Historical, b.Historical)
}

func ciGenericInput(key string) bool {
	switch key {
	case "PROJECT_NAME", "PROJECT_SLUG", "GITHUB_OWNER", "OPENCODE_USERNAME", "PROTECTED_PATH", "DOCS_ROOT", "CITATION_PREFIX":
		return true
	default:
		return false
	}
}

func validateCIContext(ctx context.Context, ci *ciWorkflowContext) error {
	if ci == nil {
		return nil
	}
	if ci.SchemaVersion != 1 {
		return conflict("unsupported CI transaction context")
	}
	if _, err := ciInputsFromValues(ctx, ci.Current); err != nil {
		return err
	}
	allowed := map[string]bool{}
	for _, key := range ciInputKeys {
		allowed[key] = true
	}
	for key, value := range ci.Historical {
		if !allowed[key] || len(value) > 64<<10 || !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
			return conflict("invalid historical CI input")
		}
		if !ciGenericInput(key) {
			current, found := ci.Current[key]
			if !found || current != value {
				return conflict("CI history changes current pack settings")
			}
		}
	}
	for key, value := range ci.Current {
		if !ciGenericInput(key) {
			historical, found := ci.Historical[key]
			if !found || historical != value {
				return conflict("CI history omits current pack settings")
			}
		}
	}
	return ctx.Err()
}

// Replay is from the original qualified image, not today's renderer constants.
// docs/adr/0148-transactional-installed-ci-migration.md:75.
func qualifyCIProjection(ctx context.Context, ci *ciWorkflowContext, request Request, image staging.Inspection, before *staging.Inspection) (ciWorkflowHistory, []byte, error) {
	if ci == nil {
		return ciWorkflowHistory{}, nil, nil
	}
	if err := validateCIContext(ctx, ci); err != nil {
		return ciWorkflowHistory{}, nil, err
	}
	if problem := ciReleaseUnavailable(request, &ci.Release); problem != "" {
		return ciWorkflowHistory{}, nil, conflict(problem)
	}
	expectedBefore := ReleaseIdentity{}
	if request.Profile == "go-hybrid-v1" {
		expectedBefore = releaseIdentity(request.Before)
	}
	if ci.Before != expectedBefore {
		return ciWorkflowHistory{}, nil, conflict("CI predecessor identity disagrees with fresh inputs")
	}
	if image.Manifest.Version != ci.Release.Tag || image.Manifest.SourceRevision != ci.Release.SourceRevision || image.Manifest.Target != ci.Release.OriginTarget || !ciExactRecipe(image, ciGeneratedRecipe) {
		return ciWorkflowHistory{}, nil, conflict("CI target renderer recipe is not qualified")
	}
	inputs, err := ciInputsFromValues(ctx, ci.Current)
	if err != nil {
		return ciWorkflowHistory{}, nil, err
	}
	target, err := ciFrozenWorkflow(ctx, image, ci.Release, inputs.application)
	if err != nil {
		return ciWorkflowHistory{}, nil, err
	}
	inputs.values = maps.Clone(ci.Historical)
	history, err := ciPredecessorBytes(ctx, request, ci.Release, before, inputs)
	return history, target, err
}

// Initial CI intent is supplied only by the authenticated release owner.
// docs/adr/0148-transactional-installed-ci-migration.md:31.
func (planned *plan) planCI(ctx context.Context, tree *installationfs.Tree) error {
	request := planned.request
	if ciReleaseUnavailable(request, request.CIRelease) != "" {
		return nil
	}
	observed := ciWorkflowObservation{before: map[string]*Image{}, data: map[string][]byte{}}
	report, err := inspectCIWorkflow(ctx, tree, request, request.CIRelease, planned.reference.Runtime, &observed)
	if err != nil {
		return err
	}
	report.ApplySupported = true
	report.Reason = "Confirmation includes these CI workflow changes in the installation transaction, backup and recovery."
	planned.ciConfig = observed.config
	if observed.current != nil {
		ci := &ciWorkflowContext{SchemaVersion: 1, Release: *request.CIRelease, Current: maps.Clone(observed.current), Historical: maps.Clone(observed.inputs.values)}
		if request.Profile == "go-hybrid-v1" {
			ci.Before = releaseIdentity(request.Before)
		}
		_, target, qualifyErr := qualifyCIProjection(ctx, ci, request, planned.image, planned.reference.Runtime)
		switch {
		case qualifyErr != nil:
			if ErrorStatus(qualifyErr) == 1 {
				return qualifyErr
			}
			report.Conflicts = append(report.Conflicts, Issue{Path: ".github/workflows/factory.yml", Reason: qualifyErr.Error()})
		case !bytes.Equal(target, observed.target):
			return conflict("current CI renderer differs from the qualified target projection")
		default:
			planned.ci = ci
		}
	}
	for index := range report.Changes {
		change := &report.Changes[index]
		action := Action{Path: change.Path, Action: "retain", Required: true, Reason: change.Reason, Before: observed.before[change.Path], After: observed.before[change.Path]}
		switch change.Disposition {
		case "create", "replace":
			action.Action = change.Disposition
			action.After = bytesImage(observed.target, 0644)
			action.Reason = "qualified target CI workflow in installation transaction"
			planned.payload[action.Path] = observed.target
		case "retire":
			action.Action, action.After = "retire", nil
			action.Reason = "qualified copied CI workflow retirement in installation transaction"
		}
		change.Reason = action.Reason
		planned.before[action.Path] = observed.data[action.Path]
		planned.proposal.Actions = append(planned.proposal.Actions, action)
	}
	if len(report.Conflicts) > 0 {
		report.Reason += " Preserved CI conflicts block the combined automatic upgrade."
		planned.proposal.Conflicts = append(planned.proposal.Conflicts, report.Conflicts...)
		planned.proposal.Blockers = append(planned.proposal.Blockers, report.Conflicts...)
	}
	planned.proposal.CIWorkflow = &report
	return ctx.Err()
}

func ciExpectedBefore(history ciWorkflowHistory, path string, before *Image) (*Image, bool) {
	if before == nil {
		return nil, true
	}
	for _, candidate := range history.bytes[path] {
		expected := bytesImage(candidate.data, candidate.mode)
		if imageEqual(expected, before, false) {
			return expected, true
		}
	}
	return nil, false
}

// Require complete, freshly reproduced delta labels before shared recovery logic.
func qualifyCIRows(ctx context.Context, entries []journalEntry, history ciWorkflowHistory, target []byte) error {
	for _, name := range ciWorkflowNames {
		path := ".github/workflows/" + name
		var original *journalEntry
		for index := range entries {
			if entries[index].Path == path {
				original = &entries[index]
				break
			}
		}
		if original == nil {
			return conflict("CI transaction omits a selected workflow")
		}
		if _, matched := ciExpectedBefore(history, path, original.Before); !matched {
			return conflict("CI before-image is not a qualified historical workflow")
		}
		var after *Image
		if name == "factory.yml" {
			after = bytesImage(target, 0644)
		}
		action := "retain"
		switch {
		case original.Before != nil && after == nil:
			action = "retire"
		case original.Before != nil && after != nil:
			action = "replace"
		case original.Before == nil && after != nil:
			action = "create"
		}
		if original.Action != action || !imageEqual(original.ExpectedAfter, after, false) {
			return conflict("CI recorded delta differs from fresh qualified projection")
		}
	}
	return ctx.Err()
}

func (owner *transaction) checkCIConfig(ctx context.Context) error {
	if owner.planned.ciConfig == nil {
		return nil
	}
	current, _, err := observe(ctx, owner.tree, "factory.yaml", 1<<20)
	if err != nil {
		return err
	}
	if !imageEqual(current, owner.planned.ciConfig, true) {
		return conflict("CI configuration changed after consent")
	}
	return nil
}

// Runtime health and project CI publication have distinct authority boundaries.
// docs/adr/0148-transactional-installed-ci-migration.md:97.
func (owner *transaction) validateCIResult(ctx context.Context) error {
	if owner.planned.ci == nil {
		return nil
	}
	for _, action := range owner.planned.proposal.Actions {
		if !ciWorkflowPath(action.Path) {
			continue
		}
		current, _, err := observe(ctx, owner.tree, action.Path, leafLimit(action.Path))
		if err != nil {
			return err
		}
		expected, found := owner.published[action.Path]
		if !found && action.Action == "retain" {
			expected, found = action.Before, true
		}
		if !found || !imageEqual(current, expected, true) || !imageEqual(current, action.After, false) {
			return conflict("CI workflow changed before transaction completion")
		}
	}
	return ctx.Err()
}
