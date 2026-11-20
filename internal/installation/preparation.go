package installation

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/anoop2811/software-factory-template/internal/filepublish"
	"github.com/anoop2811/software-factory-template/internal/installationfs"
	"github.com/anoop2811/software-factory-template/internal/installedlayout"
	"github.com/anoop2811/software-factory-template/internal/native"
	"github.com/anoop2811/software-factory-template/internal/staging"
	"github.com/anoop2811/software-factory-template/internal/transition"
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
	return materializeSelection(ctx, owner.candidate, owner.planned.image, owner.request.Inputs, owner.stageTree)
}

func materializeSelection(ctx context.Context, path string, image staging.Inspection, inputs map[string]string, source *installationfs.Tree) (returned error) {
	if err := os.Mkdir(path, 0700); err != nil {
		return err
	}
	tree, err := installationfs.Open(ctx, path)
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, tree.Close()) }()
	// Candidate always proves the freshly qualified target, including reverse requests.
	// docs/adr/0098-whole-installation-upgrade-and-rollback.md:200.
	selection, err := Selection(ctx, image, inputs)
	if err != nil {
		return err
	}
	for _, row := range selection {
		if row.Runtime {
			err = publishRuntime(ctx, source, image, tree, row.Path, nil, nil)
		} else {
			err = publishBytes(ctx, tree, row.Path, row.Data, row.Mode, nil)
		}
		if err != nil {
			return err
		}
	}
	return ctx.Err()
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

// MergePrivateIgnoreRules preserves adopter rules and appends missing root rules for
// installation assets and the permanent transition lock. docs/DECISION_LOG.md:9048.
func MergePrivateIgnoreRules(data []byte) ([]byte, error) {
	return mergePrivateRules(data, append(PrivateIgnoreRules(), "/"+transition.LockPath))
}

func mergePrivateRules(data []byte, rules []string) ([]byte, error) {
	limit := leafLimit(".gitignore")
	if int64(len(data)) > limit {
		return nil, conflict("installation ignore file exceeds limit")
	}
	lines := "\n" + strings.ReplaceAll(string(data), "\r\n", "\n") + "\n"
	updated := bytes.Clone(data)
	for _, rule := range rules {
		if strings.Contains(lines, "\n"+rule+"\n") {
			continue
		}
		if len(updated) > 0 && updated[len(updated)-1] != '\n' {
			updated = append(updated, '\n')
		}
		updated = append(updated, rule...)
		updated = append(updated, '\n')
	}
	if int64(len(updated)) > limit {
		return nil, conflict("installation ignore file exceeds limit")
	}
	return updated, nil
}

func (owner *transaction) ensurePrivateIgnore(ctx context.Context) error {
	image, data, err := observe(ctx, owner.tree, ".gitignore", leafLimit(".gitignore"))
	if err != nil {
		return err
	}
	env := operationEnvironment()
	if err := CheckTransitionLockIndex(ctx, owner.root, env, nil); err != nil {
		return err
	}
	id := operationID(owner.request)
	paths := []string{".factory/bin/factory-runtime", ".factory/assets/scaffold/factory", ".factory/installation.current", journalPath(id), ".factory/backups/" + id + "/installation.manifest"}
	tracked, err := native.ExecuteCommand(ctx, owner.root, append([]string{"git", "--no-optional-locks", "-c", "core.fsmonitor=false", "ls-files", "--"}, paths...), env, 5*time.Second)
	if err != nil || tracked.Outcome != "completed" || len(tracked.Stdout) != 0 {
		return errors.Join(conflict("private installation data is tracked or cannot be inspected"), err, ctx.Err())
	}
	baseline, err := mergePrivateRules(data, PrivateIgnoreRules())
	if err != nil {
		return err
	}
	updated, err := owner.proposedPrivateIgnore(ctx, baseline, paths)
	if err != nil {
		return err
	}
	if !bytes.Equal(updated, data) {
		if bytes.Equal(baseline, data) {
			// A lock-only edit cannot repair a backup or runtime refusal.
			if err := checkPrivatePathsIgnored(ctx, owner.root, env, paths); err != nil {
				return err
			}
		}
		mode := os.FileMode(0644)
		if image != nil {
			mode = os.FileMode(image.Mode & 0777)
		}
		if err := publishBytes(ctx, owner.tree, ".gitignore", updated, mode, image); err != nil {
			return err
		}
	}
	if err := checkPrivatePathsIgnored(ctx, owner.root, env, append(paths, transition.LockPath)); err != nil {
		return err
	}
	return owner.check(ctx)
}

func checkPrivatePathsIgnored(ctx context.Context, root string, env map[string]string, paths []string) error {
	for _, path := range paths {
		result, err := native.ExecuteCommand(ctx, root, []string{"git", "check-ignore", "--no-index", "-q", "--", path}, env, 5*time.Second)
		if err != nil || result.Outcome != "completed" {
			return errors.Join(conflict("private installation data is not effectively ignored"), err, ctx.Err())
		}
	}
	return ctx.Err()
}

// Evaluate the candidate at its actual root precedence before publishing. Only
// the six private pathnames and their ancestors' ignore files participate.
// docs/DECISION_LOG.md:9065.
func (owner *transaction) proposedPrivateIgnore(ctx context.Context, data []byte, paths []string) (_ []byte, returned error) {
	scratch, err := os.MkdirTemp(owner.scratch, "factory-ignore-")
	if err != nil {
		return nil, err
	}
	identity, err := os.Lstat(scratch)
	if err != nil {
		return nil, err
	}
	defer func() {
		current, err := os.Lstat(scratch)
		if err != nil || !os.SameFile(identity, current) {
			returned = errors.Join(returned, errors.New("ignore proof scratch changed"), err)
			return
		}
		returned = errors.Join(returned, os.RemoveAll(scratch))
	}()
	proof, err := os.OpenRoot(scratch)
	if err != nil {
		return nil, err
	}
	defer func() { returned = errors.Join(returned, proof.Close()) }()
	parents := make(map[string]bool)
	for _, path := range append(paths, transition.LockPath) {
		if err := proof.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return nil, err
		}
		if err := proof.WriteFile(path, nil, 0600); err != nil {
			return nil, err
		}
		for dir := filepath.Dir(path); dir != "."; dir = filepath.Dir(dir) {
			parents[dir] = true
		}
	}
	if err := proof.WriteFile(".gitignore", data, 0600); err != nil {
		return nil, err
	}
	env := operationEnvironment()
	env["GIT_INDEX_FILE"] = filepath.Join(scratch, "index")
	// Explicit exclude options omit machine excludes and the repository's
	// info/exclude. Use the real repository's other Git settings.
	matches := func(option, ignoreCase string, names ...string) (bool, error) {
		args := []string{"git", "--no-optional-locks", "-c", "core.fsmonitor=false", "--work-tree=" + scratch}
		if ignoreCase != "" {
			args = append(args, "-c", "core.ignoreCase="+ignoreCase)
		}
		args = append(args, "ls-files", "--others", "--ignored", "-z", option, "--")
		result, err := native.ExecuteCommand(ctx, owner.root, append(args, names...), env, 5*time.Second)
		if err != nil || result.Outcome != "completed" {
			return false, errors.Join(errors.New("cannot inspect proposed private ignore rules"), err, ctx.Err())
		}
		return string(result.Stdout) == strings.Join(names, "\x00")+"\x00", ctx.Err()
	}
	updated := data
	rootRules := "--exclude-from=" + filepath.Join(scratch, ".gitignore")
	for _, folding := range []string{"false", "true"} {
		covered, err := matches(rootRules, folding, transition.LockPath)
		if err != nil {
			return nil, err
		}
		if !covered {
			updated, err = mergePrivateRules(data, []string{"/" + transition.LockPath})
			if err != nil {
				return nil, err
			}
			break
		}
	}
	if err := proof.WriteFile(".gitignore", updated, 0600); err != nil {
		return nil, err
	}
	for dir := range parents {
		path := filepath.Join(dir, ".gitignore")
		image, contents, err := observe(ctx, owner.tree, path, leafLimit(".gitignore"))
		if err != nil {
			return nil, err
		}
		if image != nil {
			if err := proof.WriteFile(path, contents, 0600); err != nil {
				return nil, err
			}
		}
	}
	projectRules := "--exclude-per-directory=.gitignore"
	// ls-files emits pathnames in index order, independently of argv order.
	names := slices.Sorted(slices.Values(paths))
	covered, err := matches(projectRules, "", names...)
	if err != nil || !covered {
		return nil, errors.Join(conflict("private installation data is not effectively ignored"), err)
	}
	for _, folding := range []string{"false", "true"} {
		for _, option := range []string{rootRules, projectRules} {
			covered, err := matches(option, folding, transition.LockPath)
			if err != nil || !covered {
				return nil, errors.Join(conflict("private transition lock is not effectively ignored"), err)
			}
		}
	}
	return updated, nil
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
	manifest := backupManifest{SchemaVersion: 1, Kind: "installation-before-images", MigrationID: operationID(owner.request), Profile: owner.request.Profile, Entries: owner.record.Entries, CIWorkflow: owner.record.CIWorkflow}
	if err := writeRecord(ctx, owner.tree, base+"/installation.manifest", manifest, nil); err != nil {
		return err
	}
	if owner.retention != nil {
		if err := writeRetentionReceipt(ctx, owner); err != nil {
			return err
		}
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
	entries, err := directory.ReadDir(18)
	if err != nil && len(entries) == 0 {
		return err
	}
	count := 0
	for _, entry := range entries {
		if path == ".factory/backups" && entry.Name() == ".retention.json" {
			continue
		}
		count++
	}
	if count > 16 {
		return conflict("installation operation quota exceeded")
	}
	return ctx.Err()
}
