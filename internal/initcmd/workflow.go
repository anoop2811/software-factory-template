package initcmd

import (
	"context"
	"errors"

	"github.com/anoop2811/software-factory-template/internal/ciworkflow"
	"golang.org/x/sys/unix"
)

const initializedWorkflowPath = ".github/workflows/factory.yml"

func (q *qualified) workflowEligible() bool {
	return q.release != nil && ciworkflow.EligibleRelease(q.release.Tag)
}

// Destination absence is checked without opening or following its occupant.
// The generated file then uses the ordinary asset identity/publication checks.
// docs/adr/0146-initialized-installed-ci.md:37.
func (q *qualified) workflow(ctx context.Context, target *tree, o options, values map[string]string) (asset, error) {
	if err := requireWorkflowDestination(ctx, target); err != nil {
		return asset{}, err
	}
	application := ciworkflow.Application{JavaBuildTool: o.javaBuild, MutationDirectory: values["PROTECTED_PATH"]}
	if application.MutationDirectory != "" {
		application.MutationDirectory = "./" + application.MutationDirectory
	}
	if hasPack(o.packs, "go") {
		application.GoVersion = values["GO_VERSION"]
	}
	if hasPack(o.packs, "typescript") {
		application.NodeVersion = values["NODE_VERSION"]
	}
	if hasPack(o.packs, "java") {
		application.JavaVersion = values["JAVA_VERSION"]
	}
	return asset{path: initializedWorkflowPath, data: []byte(ciworkflow.Initialized(*q.release, application)), mode: 0644}, ctx.Err()
}

func requireWorkflowDestination(ctx context.Context, target *tree) error {
	if target != nil {
		parent, name, err := target.parent(ctx, initializedWorkflowPath, false)
		if err != nil && !errors.Is(err, unix.ENOENT) {
			return err
		}
		if parent != nil {
			var metadata unix.Stat_t
			err := unix.Fstatat(int(parent.Fd()), name, &metadata, unix.AT_SYMLINK_NOFOLLOW)
			closeErr := parent.Close()
			if !errors.Is(err, unix.ENOENT) || closeErr != nil {
				return errors.Join(closeErr, errors.New("ordinary factory workflow destination is occupied or unsafe"))
			}
		}
	}
	return ctx.Err()
}
