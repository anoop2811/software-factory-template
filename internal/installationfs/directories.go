package installationfs

import (
	"context"
	"errors"
	"fmt"
)

// Stat describes a confined entry without following its final component.
// It grants no ownership. docs/adr/0106-go-backup-retention.md:84.
func (tree *Tree) Stat(ctx context.Context, path string) (Metadata, error) {
	parent, name, _, err := tree.Parent(ctx, path, false)
	if err != nil {
		return Metadata{}, err
	}
	stat, err := named(ctx, parent, name)
	if err != nil {
		return Metadata{}, err
	}
	return metadata(stat), nil
}

// RemoveEmptyDirectory retires checked descriptor custody after an intentional
// empty-directory removal. docs/adr/0106-go-backup-retention.md:84.
func (tree *Tree) RemoveEmptyDirectory(ctx context.Context, path string, expected Directory) error {
	if path == "" || expected.Path != path {
		return fmt.Errorf("%w: invalid directory removal", ErrUnsafe)
	}
	parent, name, _, err := tree.Parent(ctx, path, false)
	if err != nil {
		return err
	}
	var node *Node
	for _, candidate := range tree.nodes {
		if candidate.parent == parent && candidate.name == name {
			node = candidate
			break
		}
	}
	if node == nil {
		if _, _, _, err = tree.Parent(ctx, path+"/.retention-probe", false); err != nil {
			return err
		}
		for _, candidate := range tree.nodes {
			if candidate.parent == parent && candidate.name == name {
				node = candidate
				break
			}
		}
	}
	if node == nil {
		return fmt.Errorf("%w: missing directory custody", ErrUnsafe)
	}
	for _, candidate := range tree.nodes {
		if candidate.parent == node {
			return fmt.Errorf("%w: directory still has retained children", ErrUnsafe)
		}
	}
	current, err := tree.Stat(ctx, path)
	if err != nil {
		return err
	}
	if current.Device != expected.Device || current.Inode != expected.Inode || current.User != expected.User || current.Group != expected.Group || current.Mode != expected.Mode {
		return fmt.Errorf("%w: directory changed before removal", ErrUnsafe)
	}
	if err = tree.Check(ctx); err != nil {
		return err
	}
	// Remove refuses nonempty directories; no recursive deletion is permitted.
	if err = parent.Root.Remove(name); err != nil {
		return err
	}
	kept := tree.nodes[:0]
	for _, candidate := range tree.nodes {
		if candidate != node {
			kept = append(kept, candidate)
		}
	}
	tree.nodes = kept
	closeErr := node.Root.Close()
	syncErr := parent.Sync(ctx)
	if err = errors.Join(closeErr, syncErr); err != nil {
		return err
	}
	return tree.Check(ctx)
}
