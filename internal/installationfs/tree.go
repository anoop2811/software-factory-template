// Package installationfs retains confined physical parents for installation reads and writes.
package installationfs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

// A Tree owns only descriptors; creation ownership is returned to the caller.
// docs/adr/0098-whole-installation-upgrade-and-rollback.md:382.
var ErrUnsafe = errors.New("unsafe confined installation path")

type Tree struct {
	Path   string
	Root   *Node
	nodes  []*Node
	closed bool
}
type Node struct {
	Root     *os.Root
	parent   *Node
	name     string
	identity os.FileInfo
	tree     *Tree
}

func Open(ctx context.Context, path string) (result *Tree, returned error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(path) || strings.ContainsRune(path, 0) {
		return nil, fmt.Errorf("%w: invalid confined installation root", ErrUnsafe)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !safeDirectory(info) {
		return nil, fmt.Errorf("%w: unsafe confined installation root", ErrUnsafe)
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	opened, err := root.Stat(".")
	if err != nil || !sameDirectory(info, opened) {
		return nil, errors.Join(fmt.Errorf("%w: confined installation root changed", ErrUnsafe), err, root.Close())
	}
	tree := &Tree{Path: path}
	node := &Node{Root: root, identity: opened, tree: tree}
	tree.Root = node
	tree.nodes = []*Node{node}
	if err := tree.Check(ctx); err != nil {
		return nil, errors.Join(err, tree.Close())
	}
	return tree, nil
}
func safeDirectory(info os.FileInfo) bool {
	if info == nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0022 != 0 || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int64(stat.Uid) == int64(os.Geteuid())
}
func sameDirectory(before, after os.FileInfo) bool {
	if !safeDirectory(after) || !os.SameFile(before, after) || before.Mode() != after.Mode() {
		return false
	}
	a, aok := before.Sys().(*syscall.Stat_t)
	b, bok := after.Sys().(*syscall.Stat_t)
	return aok && bok && a.Uid == b.Uid && a.Gid == b.Gid
}
func (tree *Tree) Check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if tree == nil || tree.closed {
		return errors.New("confined installation tree is closed")
	}
	for _, node := range tree.nodes {
		opened, err := node.Root.Stat(".")
		if err != nil || !sameDirectory(node.identity, opened) {
			return errors.Join(fmt.Errorf("%w: confined installation parent changed", ErrUnsafe), err)
		}
		var named os.FileInfo
		if node.parent == nil {
			named, err = os.Lstat(tree.Path)
		} else {
			named, err = node.parent.Root.Lstat(node.name)
		}
		if err != nil || !sameDirectory(node.identity, named) {
			return errors.Join(fmt.Errorf("%w: confined installation parent location changed", ErrUnsafe), err)
		}
	}
	return ctx.Err()
}
func (tree *Tree) Parent(ctx context.Context, path string, create bool) (*Node, string, []*Node, error) {
	if path == "" || filepath.IsAbs(path) || strings.ContainsAny(path, "\\\x00") {
		return nil, "", nil, fmt.Errorf("%w: invalid confined installation path", ErrUnsafe)
	}
	parts := strings.Split(path, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return nil, "", nil, fmt.Errorf("%w: invalid confined installation path", ErrUnsafe)
		}
	}
	if err := tree.Check(ctx); err != nil {
		return nil, "", nil, err
	}
	current := tree.Root
	created := []*Node{}
	for _, part := range parts[:len(parts)-1] {
		var next *Node
		for _, node := range tree.nodes {
			if node.parent == current && node.name == part {
				next = node
				break
			}
		}
		if next != nil {
			current = next
			continue
		}
		before, err := current.Root.Lstat(part)
		owned := false
		if errors.Is(err, os.ErrNotExist) && create {
			if err := current.Root.Mkdir(part, 0700); err != nil {
				return nil, "", created, err
			}
			owned = true
			before, err = current.Root.Lstat(part)
		}
		if err != nil {
			return nil, "", created, err
		}
		if !safeDirectory(before) {
			return nil, "", created, fmt.Errorf("%w: unsafe confined installation parent", ErrUnsafe)
		}
		root, err := current.Root.OpenRoot(part)
		if err != nil {
			return nil, "", created, err
		}
		opened, err := root.Stat(".")
		named, nameErr := current.Root.Lstat(part)
		if err != nil || nameErr != nil || !sameDirectory(before, opened) || !sameDirectory(opened, named) {
			return nil, "", created, errors.Join(fmt.Errorf("%w: confined installation parent changed while opening", ErrUnsafe), err, nameErr, root.Close())
		}
		next = &Node{Root: root, parent: current, name: part, identity: opened, tree: tree}
		tree.nodes = append(tree.nodes, next)
		if owned {
			created = append(created, next)
			if err := current.Sync(ctx); err != nil {
				return nil, "", created, err
			}
		}
		current = next
	}
	if err := tree.Check(ctx); err != nil {
		return nil, "", created, err
	}
	return current, parts[len(parts)-1], created, nil
}
func (tree *Tree) Close() error {
	if tree == nil || tree.closed {
		return errors.New("confined installation tree is closed")
	}
	tree.closed = true
	var errs []error
	for i := len(tree.nodes) - 1; i >= 0; i-- {
		errs = append(errs, tree.nodes[i].Root.Close())
	}
	return errors.Join(errs...)
}
func (node *Node) Sync(ctx context.Context) (returned error) {
	if err := node.tree.Check(ctx); err != nil {
		return err
	}
	file, err := node.Root.Open(".")
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, file.Close()) }()
	return file.Sync()
}
func (node *Node) Relative() string {
	if node.parent == nil {
		return ""
	}
	base := node.parent.Relative()
	if base == "" {
		return node.name
	}
	return base + "/" + node.name
}

// Directory is stable physical parent evidence; mutable directory timestamps are excluded.
// docs/adr/0098-whole-installation-upgrade-and-rollback.md:133.
type Directory struct {
	Path              string
	Device, Inode     uint64
	User, Group, Mode uint32
}

func (tree *Tree) Parents(ctx context.Context) ([]Directory, error) {
	if err := tree.Check(ctx); err != nil {
		return nil, err
	}
	result := []Directory{}
	for _, node := range tree.nodes {
		path := node.Relative()
		// The transition guard owns the explicit .factory infrastructure; its creation
		// between a read-only proposal and admission cannot change asset consent.
		if path == ".factory" {
			continue
		}
		stat, ok := node.identity.Sys().(*syscall.Stat_t)
		if !ok {
			return nil, errors.New("installation parent metadata unavailable")
		}
		device := uint64(stat.Dev) // #nosec G115 -- Opaque native device identity; retain its existing signed bit representation.
		result = append(result, Directory{path, device, uint64(stat.Ino), stat.Uid, stat.Gid, uint32(stat.Mode)})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result, nil
}
