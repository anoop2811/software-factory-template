package installation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"sort"

	"github.com/anoop2811/software-factory-template/internal/config"
	"github.com/anoop2811/software-factory-template/internal/installationfs"
	"github.com/anoop2811/software-factory-template/internal/staging"
	"golang.org/x/sys/unix"
)

// Recovery observes fresh images and actual entries; serialized records are never grants.
// docs/adr/0098-whole-installation-upgrade-and-rollback.md:223.
func buildTransitionPlan(ctx context.Context, root string, request Request, record journal, reverse bool) (result plan, returned error) {
	image, err := staging.Inspect(ctx, request.Source)
	if err != nil {
		return result, err
	}
	reference, err := predecessor(ctx, request, image)
	if err != nil {
		return result, err
	}
	if record.Profile != request.Profile || record.Target != request.Source.Target {
		return result, conflict("fresh installation identities disagree with transaction")
	}
	if request.Rollback != "" && (record.Phase != "committed" || record.Direction != "forward") {
		return result, conflict("completed rollback requires a committed forward installation")
	}
	terminalCleanup := request.Recover != "" && record.Phase == "committed"
	if terminalCleanup && request.Direction != record.Direction {
		return result, conflict("terminal cleanup direction disagrees with completed installation")
	}
	tree, err := installationfs.Open(ctx, root)
	if err != nil {
		return result, err
	}
	defer func() { returned = errors.Join(returned, tree.Close()) }()
	pending, _, pendingErr := observe(ctx, tree, ".factory/runtime-publication.pending", 0)
	if pendingErr != nil {
		return result, pendingErr
	}
	if terminalCleanup && (record.Pending == nil || pending != nil && !imageEqual(pending, record.Pending, true)) {
		return result, conflict("terminal pending observation differs from current marker")
	}
	if request.Recover != "" && pending == nil && !terminalCleanup {
		if preparationOnly(record) && record.Pending == nil {
			return buildPreparationPlan(ctx, root, request, record, tree, image, reference)
		}
		return result, conflict("incomplete installation pending marker is missing")
	}
	for _, check := range record.Checks {
		if check.OwnershipUnconfirmed || check.Phase == "checking" {
			return result, conflict("installation check ownership requires operator reconciliation")
		}
	}
	manifestImage, manifestData, err := observe(ctx, tree, ".factory/backups/"+record.MigrationID+"/installation.manifest", journalLimit)
	if err != nil {
		return result, err
	}
	if manifestImage == nil || manifestImage.Mode != unix.S_IFREG|0600 {
		return result, conflict("whole installation before-image manifest is missing")
	}
	var manifest backupManifest
	if err := decodeRecord(ctx, manifestData, &manifest); err != nil {
		return result, err
	}
	if manifest.SchemaVersion != 1 || manifest.Kind != "installation-before-images" || manifest.MigrationID != record.MigrationID || manifest.Profile != request.Profile || len(manifest.Entries) != len(record.Entries) {
		return result, conflict("whole installation before-image manifest disagrees")
	}
	originals := map[string]journalEntry{}
	for _, entry := range manifest.Entries {
		if originals[entry.Path].Path != "" || !selectedPath(entry.Path) {
			return result, conflict("invalid before-image manifest selection")
		}
		originals[entry.Path] = entry
	}
	result = plan{image: image, request: request, payload: map[string][]byte{}, before: map[string][]byte{}, proposal: Proposal{SchemaVersion: 1, Mode: "installation-proposal", Profile: request.Profile, Target: request.Source.Target, Authentication: "not-verified", Actions: []Action{}, Checks: []string{"candidate-help", "installed-assets", "native-gate-proof", "state-compatibility"}, Conflicts: []Issue{}, Blockers: []Issue{}, RollbackBlockers: []Issue{}}}
	result.reference = reference
	// Historical pending identity is retained; absence can authorize only a freshly
	// qualified completed no-op, never continuation of partial installation.
	// docs/DECISION_LOG.md:4848.
	result.completedRecovery = terminalCleanup && pending == nil
	if request.Source.Local {
		result.proposal.Authentication = "local-source"
	}
	targetActions := []Action{}
	targetPayload := map[string][]byte{}
	for _, row := range roles {
		data, err := generate(ctx, row, image, request.Inputs)
		if err != nil {
			return result, err
		}
		var expected *Image
		if row.Operation != "retire" {
			expected = bytesImage(data, uint32(modeFor(row)))
			if row.Path == ".factory/bin/factory-runtime" {
				expected.SHA256 = image.Runtime.SHA256
				expected.Bytes = image.BinaryBytes
			}
		}
		targetActions = append(targetActions, Action{Path: row.Path, After: expected})
		targetPayload[row.Path] = data
	}
	control, err := descriptorBytes(image, targetActions)
	if err != nil {
		return result, err
	}
	targetPayload[".factory/installation.current"] = control
	targets := map[string]*Image{}
	for _, action := range targetActions {
		if action.Path == ".factory/installation.current" {
			action.After = bytesImage(control, 0600)
		}
		targets[action.Path] = action.After
	}
	if len(record.Entries) != len(roles) {
		return result, conflict("installation journal omits reviewed rows")
	}
	for _, row := range roles {
		original, found := originals[row.Path]
		if !found {
			return result, conflict("before-image manifest omits reviewed row")
		}
		var progress *journalEntry
		for index := range record.Entries {
			if record.Entries[index].Path == row.Path {
				progress = &record.Entries[index]
			}
		}
		if progress == nil || progress.Action != original.Action || !imageEqual(progress.Before, original.Before, true) || !imageEqual(progress.ExpectedAfter, original.ExpectedAfter, false) {
			return result, conflict("installation journal differs from complete before-image manifest")
		}
		current, currentData, err := observe(ctx, tree, row.Path, leafLimit(row.Path))
		if err != nil {
			return result, err
		}
		action := Action{Path: row.Path, SourcePath: row.Source, Action: "retain", Reason: "freshly qualified installation entry", Required: row.Required, Before: current, After: current}
		if original.Action == "retain" {
			result.proposal.Actions = append(result.proposal.Actions, action)
			continue
		}
		expected := targets[row.Path]
		if !imageEqual(expected, original.ExpectedAfter, false) {
			return result, conflict("fresh target projection disagrees with installation record")
		}
		var beforeData []byte
		if original.Before != nil {
			projected, matched, err := reference.matching(ctx, row.Path, original.Before, request.Inputs)
			if err != nil {
				return result, err
			}
			if !matched {
				return result, conflict("recorded before-image is not a qualified baseline selection")
			}
			saved, binary, err := observe(ctx, tree, ".factory/backups/"+record.MigrationID+"/assets/"+row.Path, leafLimit(row.Path))
			if err != nil {
				return result, err
			}
			if saved == nil || saved.Mode != unix.S_IFREG|0600 || saved.SHA256 != projected.SHA256 || saved.Bytes != projected.Bytes {
				return result, conflict("saved before-image disagrees with freshly qualified baseline")
			}
			beforeData = binary
		}
		atBefore := imageEqual(current, original.Before, false)
		atAfter := imageEqual(current, expected, false)
		observedPhase := progress.Phase == "applied" || progress.Phase == "reversed"
		knownObserved := progress.ObservedAfter != nil && imageEqual(current, progress.ObservedAfter, true)
		// Recovery can retain an original leaf already at the requested image.
		// Its full original identity, not equal bytes, proves that it was untouched.
		// docs/DECISION_LOG.md:4892.
		knownOriginal := !observedPhase && imageEqual(current, original.Before, true)
		knownPrepared := preparedEqual(current, progress.PreparedAfter) && imageEqual(current, expected, false)
		if record.Direction == "reverse" && progress.Phase == "reverse_prepared" {
			knownPrepared = preparedEqual(current, progress.PreparedAfter) && imageEqual(current, original.Before, false)
		}
		unconfirmedIntent := (progress.Phase == "prepared" || progress.Phase == "reverse_prepared") && !imageEqual(current, original.Before, true) && !knownObserved && !knownPrepared
		changedObserved := observedPhase && !imageEqual(current, progress.ObservedAfter, true)
		switch {
		case changedObserved || unconfirmedIntent:
			action.Reason = "selected entry has changed or lacks durable observed ownership; preserved"
			issue := Issue{row.Path, action.Reason}
			result.proposal.Conflicts = append(result.proposal.Conflicts, issue)
			result.proposal.Blockers = append(result.proposal.Blockers, issue)
		case terminalCleanup && record.Direction == "reverse" && (!atBefore || current != nil && !knownObserved && !knownOriginal):
			action.Reason = "completed reverse entry changed; preserved"
			issue := Issue{row.Path, action.Reason}
			result.proposal.Conflicts = append(result.proposal.Conflicts, issue)
			result.proposal.Blockers = append(result.proposal.Blockers, issue)
		case record.Phase == "committed" && record.Direction == "forward" && (!atAfter || expected != nil && !knownObserved && !knownOriginal):
			action.Reason = "selected entry changed after installation; preserved"
			issue := Issue{row.Path, action.Reason}
			result.proposal.Conflicts = append(result.proposal.Conflicts, issue)
			result.proposal.Blockers = append(result.proposal.Blockers, issue)
		case !atBefore && !atAfter:
			action.Reason = "current entry is outside both freshly qualified images; preserved"
			issue := Issue{row.Path, action.Reason}
			result.proposal.Conflicts = append(result.proposal.Conflicts, issue)
			result.proposal.Blockers = append(result.proposal.Blockers, issue)
		case terminalCleanup:
			action.After = current
		case reverse:
			action.After = original.Before
			if !atBefore {
				switch {
				case original.Before == nil:
					action.Action = "retire"
				case current == nil:
					action.Action = "create"
				default:
					action.Action = "replace"
				}
			}
			result.payload[row.Path] = beforeData
		default:
			action.After = expected
			if !atAfter {
				switch {
				case expected == nil:
					action.Action = "retire"
				case current == nil:
					action.Action = "create"
				default:
					action.Action = "replace"
				}
			}
			result.payload[row.Path] = targetPayload[row.Path]
		}
		result.before[row.Path] = currentData
		result.proposal.Actions = append(result.proposal.Actions, action)
	}
	if reverse {
		issues, err := legacyStateIssues(ctx, tree, request.Profile)
		if err != nil {
			return result, err
		}
		result.proposal.RollbackBlockers = issues
	}
	sort.Slice(result.proposal.Actions, func(i, j int) bool { return result.proposal.Actions[i].Path < result.proposal.Actions[j].Path })
	state, stateErr := inspectState(ctx, tree)
	if stateErr != nil {
		return result, stateErr
	}
	parents, parentErr := tree.Parents(ctx)
	if parentErr != nil {
		return result, parentErr
	}
	encoded, err := json.Marshal(struct {
		Root          string
		Request       Request
		Archive       string
		BeforeArchive string
		JournalPlan   string
		Manifest      *Image
		Actions       []Action
		State         []Issue
		Parents       []installationfs.Directory
		ObservedState []stateObservation
		Pending       *Image
	}{root, request, image.ArchiveSHA256, reference.ArchiveSHA256, record.PlanDigest, manifestImage, result.proposal.Actions, result.proposal.RollbackBlockers, parents, state, pending})
	if err != nil {
		return result, err
	}
	// Consent itself is an operand, never an input to the proposal it confirms.
	var binding map[string]any
	if err := json.Unmarshal(encoded, &binding); err != nil {
		return result, err
	}
	if inputs, ok := binding["Request"].(map[string]any); ok {
		inputs["Confirmation"] = ""
		inputs["Quiescent"] = false
	}
	encoded, err = json.Marshal(binding)
	if err != nil {
		return result, err
	}
	sum := sha256.Sum256(encoded)
	result.proposal.ProposalDigest = hex.EncodeToString(sum[:])
	if request.Recover != "" {
		result.proposal.RecoveryCommand = "factory upgrade --installation --recover " + request.Recover + " --direction " + request.Direction + " --dry-run"
		result.proposal.RecoveryArguments = recoveryArguments(request, request.Recover, request.Direction)
	}
	return result, tree.Check(ctx)
}

// Recovery guidance retains fresh trust and identity without inventing consent.
// docs/adr/0098-whole-installation-upgrade-and-rollback.md:340.
func recoveryArguments(request Request, id, direction string) []string {
	args := []string{"factory", "upgrade", "--installation", "--recover", id, "--direction", direction, "--dry-run"}
	return append(args, RecoveryInputs(request)...)
}

// RecoveryInputs preserves literal identities and trust operands, never consent.
// It is replay advice, not authority. docs/adr/0117-explicit-go-local-upgrade.md:78.
func RecoveryInputs(request Request) []string {
	args := []string{"--profile", request.Profile}
	imageArgs := func(prefix string, image staging.Options) {
		args = append(args, "--"+prefix+"source", image.Archive, "--"+prefix+"version", image.Version, "--"+prefix+"revision", image.Revision, "--"+prefix+"target", image.Target)
		if image.Local {
			args = append(args, "--"+prefix+"local")
		} else {
			args = append(args, "--"+prefix+"attestation", image.Attestation, "--"+prefix+"trusted-root", image.TrustedRoot)
			if image.Verifier != "" {
				args = append(args, "--"+prefix+"gh", image.Verifier)
			}
		}
	}
	imageArgs("", request.Source)
	if request.Profile == "go-hybrid-v1" {
		imageArgs("before-", request.Before)
	}
	keys := make([]string, 0, len(request.Inputs))
	for key := range request.Inputs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		args = append(args, "--project-input", key+"="+request.Inputs[key])
	}
	return args
}

func legacyStateIssues(ctx context.Context, tree *installationfs.Tree, profile string) ([]Issue, error) {
	issues := []Issue{}
	if profile != "v0.1.6" {
		return issues, ctx.Err()
	}
	for _, path := range []string{"factory.yaml", "factory.config"} {
		_, data, err := observe(ctx, tree, path, 1<<20)
		if err != nil {
			return nil, err
		}
		if data == nil {
			continue
		}
		for _, key := range []string{"budget_enabled", "loop_enabled"} {
			value, err := config.GetBytes(ctx, data, key, "")
			if err != nil {
				return nil, err
			}
			if value == "true" || value == "1" {
				issues = append(issues, Issue{path, "legacy profile has no required " + key + " controller"})
			}
		}
	}
	for _, path := range []string{".factory/budget.json", ".factory/loops.json"} {
		image, _, err := observe(ctx, tree, path, 1<<20)
		if err != nil {
			return nil, err
		}
		if image != nil {
			issues = append(issues, Issue{path, "legacy profile cannot safely interpret modern controller state"})
		}
	}
	return issues, nil
}
func (owner *transaction) validateLegacyState(ctx context.Context) error {
	issues, err := legacyStateIssues(ctx, owner.tree, owner.request.Profile)
	if err != nil {
		return err
	}
	if len(issues) != 0 {
		return conflict("legacy installation cannot safely interpret current state")
	}
	return nil
}
func (owner *transaction) validateLegacy(ctx context.Context) error {
	if err := owner.validateLegacyState(ctx); err != nil {
		return err
	}
	reference, err := predecessor(ctx, owner.request, owner.planned.image)
	if err != nil {
		return err
	}
	for _, action := range owner.planned.proposal.Actions {
		if action.Action == "retain" || action.Path == ".factory-version" {
			continue
		}
		actual, _, err := observe(ctx, owner.tree, action.Path, leafLimit(action.Path))
		if err != nil {
			return err
		}
		if action.After == nil {
			if actual != nil {
				return conflict("legacy restore left target selection")
			}
			continue
		}
		expected, _, known, err := reference.expected(ctx, action.Path, owner.request.Inputs)
		if err != nil {
			return err
		}
		if !known || !imageEqual(actual, expected, false) {
			return conflict("restored legacy installation is incomplete")
		}
	}
	return nil
}

var _ = os.ErrNotExist

func preparedEqual(actual, prepared *Image) bool {
	if actual == nil || prepared == nil || actual.Identity == nil || prepared.Identity == nil || !imageEqual(actual, prepared, false) {
		return false
	}
	current, bound := *actual.Identity, *prepared.Identity
	// The named publication itself may change ctime; all stable prepared identity,
	// ownership, mode, links, size, mtime and freshly reread bytes remain mandatory.
	current.ChangedSeconds, current.ChangedNanos = bound.ChangedSeconds, bound.ChangedNanos
	return current == bound
}
