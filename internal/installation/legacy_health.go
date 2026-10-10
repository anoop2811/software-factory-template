package installation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/anoop2811/software-factory-template/internal/installationfs"
)

// Reverse health executes freshly projected reference entrypoints in an isolated root.
// Saved before-images remain inert; none are used as execution fallback.
// docs/adr/0098-whole-installation-upgrade-and-rollback.md:297.
func (owner *transaction) legacyHealth(ctx context.Context) (returned error) {
	reference, err := reference(ctx, owner.request.Profile)
	if err != nil {
		return err
	}
	root := filepath.Join(owner.scratch, "legacy-candidate")
	if err := os.Mkdir(root, 0700); err != nil {
		return err
	}
	tree, err := installationfs.Open(ctx, root)
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, tree.Close()) }()
	for _, row := range roles {
		data, mode, known, err := reference.original(ctx, row.Path, owner.request.Inputs)
		if err != nil {
			return err
		}
		if !known {
			continue
		}
		if err := publishBytes(ctx, tree, row.Path, data, os.FileMode(mode), nil); err != nil {
			return err
		}
	}
	for _, path := range []string{"factory.yaml", "factory.config", ".factory/budget.json", ".factory/loops.json"} {
		image, data, err := observe(ctx, owner.tree, path, 1<<20)
		if err != nil {
			return err
		}
		if image == nil {
			continue
		}
		if err := publishBytes(ctx, tree, path, data, 0600, nil); err != nil {
			return err
		}
	}
	commands := [][]string{{"./factory", "--help"}, {"./factory", "metrics", "--json"}}
	if owner.request.Profile == "bash-baseline" {
		commands = append(commands, []string{"./factory", "budget", "report", "--json"})
	}
	for _, argv := range commands {
		owner.record.Checks = append(owner.record.Checks, checkRecord{Name: "state-compatibility", Phase: "checking", Outcome: "pending", OwnershipUnconfirmed: true})
		index := len(owner.record.Checks) - 1
		if err := owner.save(ctx); err != nil {
			return err
		}
		environment := operationEnvironment()
		environment["FACTORY_CONFIG"] = filepath.Join(root, "factory.yaml")
		environment["REPO_ROOT"] = root
		result, runErr := owner.executeObserved(ctx, root, argv, environment, 30*time.Second, func(ctx context.Context, pid int) error {
			owner.record.Checks[index].PID = &pid
			return owner.save(ctx)
		})
		owner.record.Checks[index].Phase = "complete"
		owner.record.Checks[index].Outcome = result.Outcome
		owner.record.Checks[index].OwnershipUnconfirmed = result.OwnershipUnconfirmed || !result.ExitConfirmed
		saveErr := owner.save(context.WithoutCancel(ctx))
		if err := errors.Join(runErr, saveErr); err != nil {
			return err
		}
		if result.ExitCode == nil || *result.ExitCode != 0 || !result.ExitConfirmed || result.OwnershipUnconfirmed || len(result.Stdout) == 0 {
			return conflict("fresh legacy runtime health requires available baseline tools and readable current state")
		}
	}
	return owner.check(ctx)
}
