package installation

import (
	"context"
	"errors"
	"io"
	"os"

	"github.com/anoop2811/software-factory-template/internal/installationfs"
	"golang.org/x/sys/unix"
)

// Resume validates every remaining owned row and absence justified by prior
// durable intent. Unknown additions are preserved, never recursively removed.
// docs/adr/0106-go-backup-retention.md:89.
func validateRemainingRetention(ctx context.Context, tree *installationfs.Tree, progress retentionProgress) error {
	allowed := map[string]bool{}
	for i, row := range progress.Rows {
		allowed[row.Path] = true
		absentAllowed := i < progress.Next || i == progress.Next && progress.Intent
		if row.Directory != nil {
			actual, err := tree.Stat(ctx, row.Path)
			if errors.Is(err, os.ErrNotExist) && absentAllowed {
				continue
			}
			if err != nil {
				return err
			}
			if i < progress.Next {
				return conflict("removed retention directory is occupied again")
			}
			expected := row.Directory
			if actual.Device != expected.Device || actual.Inode != expected.Inode || actual.Mode != expected.Mode || actual.User != expected.User || actual.Group != expected.Group {
				return conflict("retention directory changed after interruption")
			}
		} else {
			current, _, err := observe(ctx, tree, row.Path, retentionLeafLimit(progress.Snapshot.ID, row.Path))
			if err != nil {
				return err
			}
			if current == nil && absentAllowed {
				continue
			}
			if i < progress.Next {
				return conflict("removed retention file is occupied again")
			}
			if !imageEqual(current, row.Image, true) {
				return conflict("retention file changed after interruption")
			}
		}
	}
	base := backupBase(progress.Snapshot.ID)
	_, err := tree.Stat(ctx, base)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var walk func(string) error
	walk = func(path string) (returned error) {
		node, _, _, err := tree.Parent(ctx, path+"/.retention-probe", false)
		if err != nil {
			return err
		}
		directory, err := node.Root.Open(".")
		if err != nil {
			return err
		}
		defer func() { returned = errors.Join(returned, directory.Close()) }()
		entries, err := directory.ReadDir(4097)
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		if len(entries) > 4096 {
			return conflict("interrupted backup exceeds its inventory limit")
		}
		for _, entry := range entries {
			name := path + "/" + entry.Name()
			if !allowed[name] {
				return conflict("interrupted backup contains an unclassified entry")
			}
			metadata, err := tree.Stat(ctx, name)
			if err != nil {
				return err
			}
			if metadata.Mode&unix.S_IFMT == unix.S_IFDIR {
				if err = walk(name); err != nil {
					return err
				}
			}
		}
		return tree.Check(ctx)
	}
	return walk(base)
}
