package installation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anoop2811/software-factory-template/internal/filepublish"
	"github.com/anoop2811/software-factory-template/internal/installationfs"
	"github.com/anoop2811/software-factory-template/internal/installedlayout"
	"github.com/anoop2811/software-factory-template/internal/native"
)

func operationEnvironment() map[string]string {
	environment := map[string]string{}
	for _, entry := range os.Environ() {
		key, value, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(key, "GIT_") {
			environment[key] = value
		}
	}
	environment["FACTORY_BRIDGE_PROTOCOL"] = ""
	return environment
}
func (owner *transaction) materializeCandidate(ctx context.Context) (returned error) {
	if err := os.Mkdir(owner.candidate, 0700); err != nil {
		return err
	}
	tree, err := installationfs.Open(ctx, owner.candidate)
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, tree.Close()) }()
	// Candidate always proves the freshly qualified target, including reverse requests.
	// docs/adr/0098-whole-installation-upgrade-and-rollback.md:200.
	actions := []Action{}
	for _, row := range roles {
		if row.Operation == "retire" {
			continue
		}
		data, err := generate(ctx, row, owner.planned.image, owner.request.Inputs)
		if err != nil {
			return err
		}
		action := Action{Path: row.Path, After: bytesImage(data, uint32(modeFor(row)))}
		if row.Path == ".factory/bin/factory-runtime" {
			action.After.SHA256 = owner.planned.image.Runtime.SHA256
			action.After.Bytes = owner.planned.image.BinaryBytes
		}
		actions = append(actions, action)
		if row.Path == ".factory/installation.current" {
			continue
		}
		if row.Path == ".factory/bin/factory-runtime" {
			err = owner.publishBinary(ctx, tree, row.Path, nil)
		} else {
			err = publishBytes(ctx, tree, row.Path, data, modeFor(row), nil)
		}
		if err != nil {
			return err
		}
	}
	control, err := descriptorBytes(owner.planned.image, actions)
	if err != nil {
		return err
	}
	return publishBytes(ctx, tree, ".factory/installation.current", control, 0600, nil)
}
func (owner *transaction) preflightCandidate(ctx context.Context, reverse bool) error {
	if owner.record.Entries == nil {
		return conflict("installation plan has no journal selections")
	}
	owner.record.Checks = []checkRecord{}
	binary := filepath.Join(owner.candidate, ".factory/bin/factory-runtime")
	for _, step := range []struct {
		name      string
		arguments []string
	}{{"candidate-help", []string{"--help"}}, {"native-gate-proof", []string{"selftest"}}} {
		owner.record.Phase = "checking"
		owner.record.Checks = append(owner.record.Checks, checkRecord{Name: step.name, Phase: "checking", Outcome: "pending", OwnershipUnconfirmed: true})
		index := len(owner.record.Checks) - 1
		if err := owner.save(ctx); err != nil {
			return err
		}
		result, runErr := owner.executeObserved(ctx, owner.candidate, append([]string{binary}, step.arguments...), operationEnvironment(), 2*time.Minute, func(ctx context.Context, pid int) error {
			owner.record.Checks[index].PID = &pid
			return owner.save(ctx)
		})
		owner.record.Checks[index].OwnershipUnconfirmed = result.OwnershipUnconfirmed || !result.ExitConfirmed
		owner.record.Checks[index].Outcome = result.Outcome
		owner.record.Checks[index].Phase = "complete"
		saveErr := owner.save(context.WithoutCancel(ctx))
		if err := errors.Join(runErr, saveErr); err != nil {
			return err
		}
		if !result.ExitConfirmed || result.ExitCode == nil || *result.ExitCode != 0 || result.Outcome != "completed" || len(result.Stdout) == 0 {
			return &Failure{Code: 1, Reason: "candidate installation check failed: " + step.name + " (" + strings.TrimSpace(string(result.Stderr)) + ")", cause: errors.New(string(result.Stderr))}
		}
	}
	tree, err := installationfs.Open(ctx, owner.candidate)
	if err != nil {
		return err
	}
	proved, validationErr := installedlayout.Validate(ctx, tree, nil)
	closeErr := tree.Close()
	if err := errors.Join(validationErr, closeErr); err != nil {
		return err
	}
	owner.proof = &candidateProof{descriptor: proved}
	owner.record.Checks = append(owner.record.Checks, checkRecord{Name: "installed-assets", Phase: "complete", Outcome: "completed"}, checkRecord{Name: "state-compatibility", Phase: "complete", Outcome: "completed"})
	if reverse {
		if err := owner.validateLegacyState(ctx); err != nil {
			return err
		}
		if err := owner.legacyHealth(ctx); err != nil {
			return err
		}
	}
	owner.record.Phase = "prepared"
	return owner.save(ctx)
}
func (owner *transaction) ensurePrivateIgnore(ctx context.Context) error {
	image, data, err := observe(ctx, owner.tree, ".gitignore", 1<<20)
	if err != nil {
		return err
	}
	rules := []string{"/.factory/bin/", "/.factory/assets/", "/.factory/installation.current", "/.factory/installation-transactions/", "/.factory/backups/"}
	updated := append([]byte{}, data...)
	for _, rule := range rules {
		if !strings.Contains("\n"+string(updated)+"\n", "\n"+rule+"\n") {
			if len(updated) > 0 && updated[len(updated)-1] != '\n' {
				updated = append(updated, '\n')
			}
			updated = append(updated, []byte(rule+"\n")...)
		}
	}
	if string(updated) != string(data) {
		mode := os.FileMode(0644)
		if image != nil {
			mode = os.FileMode(image.Mode & 0777)
		}
		if err := publishBytes(ctx, owner.tree, ".gitignore", updated, mode, image); err != nil {
			return err
		}
	}
	for _, path := range []string{".factory/bin/factory-runtime", ".factory/assets/scaffold/factory", ".factory/installation.current", journalPath(operationID(owner.request)), ".factory/backups/" + operationID(owner.request) + "/installation.manifest"} {
		tracked, err := native.ExecuteCommand(ctx, owner.root, []string{"git", "ls-files", "--", path}, operationEnvironment(), 5*time.Second)
		if err != nil || tracked.ExitCode == nil || *tracked.ExitCode != 0 || len(tracked.Stdout) != 0 {
			return errors.Join(conflict("private installation data is tracked or cannot be inspected"), err)
		}
		ignored, err := native.ExecuteCommand(ctx, owner.root, []string{"git", "check-ignore", "--no-index", "-q", "--", path}, operationEnvironment(), 5*time.Second)
		if err != nil || ignored.ExitCode == nil || *ignored.ExitCode != 0 {
			return errors.Join(conflict("private installation data is not effectively ignored"), err)
		}
	}
	return owner.check(ctx)
}
func (owner *transaction) backup(ctx context.Context) error {
	base := ".factory/backups/" + operationID(owner.request)
	existing, err := owner.tree.Root.Root.Lstat(base)
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		_ = existing
		return errors.Join(conflict("installation before-image set is occupied"), err)
	}
	parent, _, _, err := owner.tree.Parent(ctx, base+"/installation.manifest", true)
	if err != nil {
		return err
	}
	// Private namespaces and sets have one separate, finite whole-operation quota.
	// docs/adr/0098-whole-installation-upgrade-and-rollback.md:337.
	if err := operationQuota(ctx, owner.tree, ".factory/backups"); err != nil {
		return err
	}
	if err := operationQuota(ctx, owner.tree, ".factory/installation-transactions"); err != nil {
		return err
	}
	var total int64
	for _, action := range owner.planned.proposal.Actions {
		if action.Action != "replace" && action.Action != "retire" {
			continue
		}
		current, data, err := observe(ctx, owner.tree, action.Path, leafLimit(action.Path))
		if err != nil {
			return err
		}
		if !imageEqual(current, action.Before, true) {
			return conflict("before-image changed during backup")
		}
		total += current.Bytes
		if total > 320<<20 {
			return conflict("installation before-image set exceeds limit")
		}
		saved := base + "/assets/" + action.Path
		if action.Path == ".factory/bin/factory-runtime" {
			file, err := owner.tree.OpenFile(ctx, action.Path, 256<<20)
			if err != nil {
				return err
			}
			destination, name, _, parentErr := owner.tree.Parent(ctx, saved, true)
			if parentErr != nil {
				return errors.Join(parentErr, file.Close())
			}
			stage, err := filepublish.PrepareReader(ctx, destination.Root, ".before-image-", file.File, current.Bytes, current.SHA256, 0600)
			if err == nil {
				err = errors.Join(file.Check(ctx), stage.PublishNoReplace(ctx, name), destination.Sync(ctx), stage.Cleanup())
			}
			if err = errors.Join(err, file.Close()); err != nil {
				return err
			}
		} else {
			if err := publishBytes(ctx, owner.tree, saved, data, 0600, nil); err != nil {
				return err
			}
		}
		savedImage, _, err := observe(ctx, owner.tree, saved, leafLimit(action.Path))
		if err != nil {
			return err
		}
		if savedImage == nil || savedImage.SHA256 != current.SHA256 || savedImage.Bytes != current.Bytes || savedImage.Mode != 0100600 {
			return conflict("installation before-image was not saved exactly")
		}
	}
	manifest := backupManifest{1, "installation-before-images", operationID(owner.request), owner.request.Profile, owner.record.Entries}
	if err := writeRecord(ctx, owner.tree, base+"/installation.manifest", manifest, nil); err != nil {
		return err
	}
	return parent.Sync(ctx)
}
func operationQuota(ctx context.Context, tree *installationfs.Tree, path string) (returned error) {
	parent, name, _, err := tree.Parent(ctx, path, false)
	if err != nil {
		return err
	}
	directory, err := parent.Root.Open(name)
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, directory.Close()) }()
	entries, err := directory.ReadDir(17)
	if err != nil && len(entries) == 0 {
		return err
	}
	if len(entries) > 16 {
		return conflict("installation operation quota exceeded")
	}
	return ctx.Err()
}
