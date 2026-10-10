package installation

import (
	"context"
	"errors"
	"io"

	"github.com/anoop2811/software-factory-template/internal/budget"
	"github.com/anoop2811/software-factory-template/internal/installationfs"
	"github.com/anoop2811/software-factory-template/internal/loop"
)

type stateObservation struct {
	Path  string
	Image *Image
}

// State semantics reuse the native readers independently qualified against legacy readers.
// History is observed and preserved, never reset by installation publication.
// docs/adr/0098-whole-installation-upgrade-and-rollback.md:215.
func inspectState(ctx context.Context, tree *installationfs.Tree) (result []stateObservation, returned error) {
	result = []stateObservation{}
	for _, path := range []string{"factory.yaml", "factory.config", ".factory/budget.json", ".factory/loops.json"} {
		limit := int64(1 << 20)
		if path == ".factory/budget.json" || path == ".factory/loops.json" {
			limit = 32 << 20
		}
		image, _, err := observe(ctx, tree, path, limit)
		if err != nil {
			return nil, err
		}
		result = append(result, stateObservation{path, image})
		if image == nil || limit == 1<<20 {
			continue
		}
		file, err := tree.OpenFile(ctx, path, limit)
		if err != nil {
			return nil, err
		}
		var stateErr error
		if path == ".factory/budget.json" {
			history, err := budget.ParseHistory(ctx, io.LimitReader(file.File, limit+1))
			stateErr = err
			if err == nil {
				active, err := history.HasActive(ctx)
				stateErr = err
				if active {
					stateErr = conflict("installation requires reconciliation of active budget history")
				}
			}
		} else {
			history, err := loop.ParseHistory(ctx, io.LimitReader(file.File, limit+1))
			stateErr = err
			if err == nil {
				unresolved, err := history.HasUnresolved(ctx)
				stateErr = err
				if unresolved {
					stateErr = conflict("installation requires reconciliation of unresolved loop checkpoints")
				}
			}
		}
		checkErr := file.Check(ctx)
		closeErr := file.Close()
		if err := errors.Join(stateErr, checkErr, closeErr); err != nil {
			return nil, err
		}
	}
	return result, tree.Check(ctx)
}
