package installation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/anoop2811/software-factory-template/internal/installationfs"
	"github.com/anoop2811/software-factory-template/internal/staging"
	"github.com/anoop2811/software-factory-template/internal/transition"
	"golang.org/x/sys/unix"
	"sort"
)

type plan struct {
	proposal     Proposal
	image        staging.Inspection
	payload      map[string][]byte
	before       map[string][]byte
	rootIdentity Identity
	request      Request
}

// Propose is read-only; imported archive bytes establish neither origin nor custody.
// docs/adr/0098-whole-installation-upgrade-and-rollback.md:117.
func Propose(ctx context.Context, root string, request Request) (result Proposal, returned error) {
	if request.Rollback != "" || request.Recover != "" {
		return proposeTransition(ctx, root, request)
	}
	lease, err := transition.ReadOnly(ctx, root)
	if err != nil {
		return result, err
	}
	defer func() { returned = errors.Join(returned, lease.Close(context.WithoutCancel(ctx))) }()
	planned, err := buildPlan(ctx, root, request)
	if err != nil {
		return result, err
	}
	if err := lease.Check(ctx); err != nil {
		return result, err
	}
	return planned.proposal, nil
}
func buildPlan(ctx context.Context, path string, request Request) (result plan, returned error) {
	reference, err := reference(ctx, request.Profile)
	if err != nil {
		return result, err
	}
	image, err := staging.Inspect(ctx, request.Source)
	if err != nil {
		return result, err
	}
	tree, err := installationfs.Open(ctx, path)
	if err != nil {
		return result, err
	}
	defer func() { returned = errors.Join(returned, tree.Close()) }()
	file, err := tree.Root.Root.Open(".")
	if err != nil {
		return result, err
	}
	stat, err := statFile(file)
	closeErr := file.Close()
	if err = errors.Join(err, closeErr); err != nil {
		return result, err
	}
	if !safeDirectory(stat) {
		return result, conflict("unsafe installation root")
	}
	id := identity(stat)
	id.Links, id.Size, id.ModifiedSeconds, id.ModifiedNanos, id.ChangedSeconds, id.ChangedNanos = 0, 0, 0, 0, 0, 0
	authentication := "not-verified"
	if request.Source.Local {
		authentication = "local-source"
	}
	proposal := Proposal{SchemaVersion: 1, Mode: "installation-proposal", Profile: request.Profile, Target: request.Source.Target, Authentication: authentication, Actions: []Action{}, Checks: []string{"candidate-help", "installed-assets", "native-gate-proof", "state-compatibility"}, Conflicts: []Issue{}, Blockers: []Issue{}, RollbackBlockers: []Issue{}}
	result = plan{proposal: proposal, image: image, payload: map[string][]byte{}, before: map[string][]byte{}, rootIdentity: id, request: request}
	seen := map[string]bool{}
	for _, role := range roles {
		if err := ctx.Err(); err != nil {
			return plan{}, err
		}
		seen[role.Path] = true
		limit := int64(1 << 20)
		if role.Path == ".factory/bin/factory-runtime" {
			limit = 256 << 20
		}
		before, data, err := observe(ctx, tree, role.Path, limit)
		if err != nil {
			var failure *Failure
			if ErrorStatus(err) == 1 || !errors.As(err, &failure) {
				return plan{}, err
			}
			result.proposal.Conflicts = append(result.proposal.Conflicts, Issue{role.Path, failure.Reason})
			if role.Required {
				result.proposal.Blockers = append(result.proposal.Blockers, Issue{role.Path, failure.Reason})
			}
			result.proposal.Actions = append(result.proposal.Actions, Action{role.Path, role.Source, "retain", "unsafe current selection", role.Required, nil, nil})
			continue
		}
		afterData, err := generate(ctx, role, image, request.Inputs)
		if err != nil {
			if ErrorStatus(err) == 1 {
				return plan{}, err
			}
			issue := Issue{role.Path, err.Error()}
			result.proposal.Conflicts = append(result.proposal.Conflicts, issue)
			result.proposal.Blockers = append(result.proposal.Blockers, issue)
			result.proposal.Actions = append(result.proposal.Actions, Action{role.Path, role.Source, "retain", err.Error(), role.Required, before, before})
			continue
		}
		var after *Image
		if role.Operation != "retire" {
			after = bytesImage(afterData, uint32(modeFor(role)))
			if role.Path == ".factory/bin/factory-runtime" {
				after.SHA256 = image.Runtime.SHA256
				after.Bytes = image.BinaryBytes
			}
		}
		action := "create"
		reason := role.Reason
		if role.Operation == "retire" {
			action = "retain"
			after = before
		}
		if before != nil {
			raw, mode, known, recognitionErr := reference.original(ctx, role.Path, request.Inputs)
			recognized := recognitionErr == nil && known && before.Mode == unix.S_IFREG|mode && bytes.Equal(data, raw)
			if role.Path == ".factory-version" {
				recognized = false
			}
			switch {
			case role.Operation == "retire" && recognized:
				action = "retire"
				after = nil
			case role.Operation != "retire" && recognized:
				action = "replace"
			default:
				action = "retain"
				after = before
				reason = "current entry is preserved; baseline ownership not established"
				if role.Required {
					result.proposal.Conflicts = append(result.proposal.Conflicts, Issue{role.Path, reason})
					result.proposal.Blockers = append(result.proposal.Blockers, Issue{role.Path, reason})
				}
			}
		}
		if before == nil && role.Operation == "retire" {
			reason = "legacy controller is absent"
		}
		result.before[role.Path] = data
		result.payload[role.Path] = afterData
		result.proposal.Actions = append(result.proposal.Actions, Action{role.Path, role.Source, action, reason, role.Required, before, after})
	}
	control, err := descriptorBytes(image, result.proposal.Actions)
	if err != nil {
		return plan{}, err
	}
	result.payload[".factory/installation.current"] = control
	for index := range result.proposal.Actions {
		action := &result.proposal.Actions[index]
		if action.Path == ".factory/installation.current" && action.Action != "retain" {
			action.After = bytesImage(control, 0600)
		}
	}
	preserved := append([]string{}, preservePaths...)
	for _, asset := range image.Manifest.Assets {
		if !seen[asset.Path] {
			preserved = append(preserved, asset.Path)
		}
	}
	sort.Strings(preserved)
	for _, name := range preserved {
		if seen[name] {
			continue
		}
		seen[name] = true
		result.proposal.Actions = append(result.proposal.Actions, Action{Path: name, SourcePath: name, Action: "retain", Reason: "source evidence or project data is outside reviewed installation selection", Required: false})
	}
	sort.Slice(result.proposal.Actions, func(i, j int) bool { return result.proposal.Actions[i].Path < result.proposal.Actions[j].Path })
	state, stateErr := inspectState(ctx, tree)
	if stateErr != nil {
		return plan{}, stateErr
	}
	parents, parentErr := tree.Parents(ctx)
	if parentErr != nil {
		return plan{}, parentErr
	}
	binding := struct {
		Root                         string
		Identity                     Identity
		Profile                      string
		Inputs                       map[string]string
		ArchiveSHA256                string
		Source, Before               staging.Options
		Rollback, Recover, Direction string
		Actions                      []Action
		Parents                      []installationfs.Directory
		State                        []stateObservation
	}{path, id, request.Profile, request.Inputs, image.ArchiveSHA256, request.Source, request.Before, request.Rollback, request.Recover, request.Direction, result.proposal.Actions, parents, state}
	encoded, err := json.Marshal(binding)
	if err != nil {
		return plan{}, err
	}
	digest := sha256.Sum256(encoded)
	result.proposal.ProposalDigest = hex.EncodeToString(digest[:])
	return result, ctx.Err()
}

func proposeTransition(ctx context.Context, root string, request Request) (result Proposal, returned error) {
	var lease *transition.RootLease
	var err error
	if request.Recover != "" {
		lease, err = transition.RootExclusive(ctx, root)
	} else {
		lease, err = transition.ReadOnly(ctx, root)
	}
	if err != nil {
		return result, err
	}
	defer func() { returned = errors.Join(returned, lease.Close(context.WithoutCancel(ctx))) }()
	var guard *transition.Guard
	if request.Recover != "" {
		inspection, err := installationfs.Open(ctx, root)
		if err != nil {
			return result, err
		}
		pending, _, readErr := observe(ctx, inspection, ".factory/runtime-publication.pending", 0)
		if err := errors.Join(readErr, inspection.Close()); err != nil {
			return result, err
		}
		if pending == nil {
			guard, err = transition.ExclusivePreparation(ctx, root)
		} else {
			guard, err = transition.ExclusiveRecovery(ctx, root)
		}
		if err != nil {
			return result, err
		}
		defer func() { returned = errors.Join(returned, guard.Close(context.WithoutCancel(ctx), true)) }()
	}
	tree, err := installationfs.Open(ctx, root)
	if err != nil {
		return result, err
	}
	defer func() { returned = errors.Join(returned, tree.Close()) }()
	record, err := readJournalEvidence(ctx, tree, operationID(request), request.Recover != "")
	if err != nil {
		return result, err
	}
	planned, err := buildTransitionPlan(ctx, root, request, record, request.Rollback != "" || request.Direction == "reverse")
	if err != nil {
		return result, err
	}
	return planned.proposal, lease.Check(ctx)
}
