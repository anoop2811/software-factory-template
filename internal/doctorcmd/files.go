package doctorcmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const fileLimit = 16 << 20
const treeLimit = 4096
const snapshotLimit = 64 << 20

func readFile(ctx context.Context, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	defer func() { _ = f.Close() }()
	return readRegular(ctx, f)
}
func readRegular(ctx context.Context, f *os.File) ([]byte, error) {
	before, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Size() > fileLimit {
		return nil, errors.New("unsafe file or input limit exceeded")
	}
	data, err := io.ReadAll(io.LimitReader(f, fileLimit+1))
	if err != nil {
		return nil, err
	}
	if len(data) > fileLimit {
		return nil, errors.New("input limit exceeded")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	after, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if before.Size() != after.Size() || before.ModTime() != after.ModTime() || before.Mode() != after.Mode() {
		return nil, errors.New("input changed during inspection")
	}
	return data, nil
}

type entry struct {
	mode os.FileMode
	data string
}
type snapshot struct {
	entries map[string]entry
	size    int
}

// Only the selected active trees are read; links are never followed by traversal.
// docs/adr/0086-go-native-doctor.md:50.
func capture(ctx context.Context, root string, paths []string, generated bool) (snapshot, error) {
	result := snapshot{entries: map[string]entry{}}
	tree, err := os.OpenRoot(root)
	if err != nil {
		return result, err
	}
	defer func() { _ = tree.Close() }()
	for _, path := range paths {
		for parent := filepath.Dir(path); parent != "."; parent = filepath.Dir(parent) {
			info, err := tree.Lstat(parent)
			if errors.Is(err, os.ErrNotExist) {
				break
			}
			if err != nil {
				return result, err
			}
			if !info.IsDir() {
				return result, errors.New("unsafe snapshot ancestor")
			}
		}
		count := 0
		if err := result.walk(ctx, tree, path, &count, generated); err != nil {
			return result, fmt.Errorf("%s: %w", path, err)
		}
	}
	return result, nil
}
func (s *snapshot) walk(ctx context.Context, tree *os.Root, path string, count *int, generated bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := tree.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	*count++
	if *count > treeLimit {
		return errors.New("tree entry limit exceeded")
	}
	item := entry{mode: info.Mode()}
	if info.Mode()&os.ModeSymlink != 0 {
		link, err := tree.Readlink(path)
		if err != nil {
			return err
		}
		if !generated && (path != "CLAUDE.md" || link != "AGENTS.md") {
			return errors.New("unsafe symbolic link")
		}
		item.data = link
		s.entries[path] = item
		return nil
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return errors.New("unsafe special file")
	}
	// O_NOFOLLOW closes the lstat/open race, and Root confines ancestor resolution.
	f, err := tree.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	opened, err := f.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(info, opened) {
		return errors.New("input changed during inspection")
	}
	if info.IsDir() {
		s.entries[path] = item
		children, err := f.ReadDir(treeLimit + 1)
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		if len(children) > treeLimit {
			return errors.New("tree entry limit exceeded")
		}
		for _, child := range children {
			if err := s.walk(ctx, tree, path+"/"+child.Name(), count, generated); err != nil {
				return err
			}
		}
	} else {
		data, err := readRegular(ctx, f)
		if err != nil {
			return err
		}
		s.size += len(data)
		if s.size > snapshotLimit {
			return errors.New("snapshot data limit exceeded")
		}
		item.data = string(data)
		s.entries[path] = item
	}
	return nil
}
func (s snapshot) write(ctx context.Context, root string) error {
	for path, item := range s.entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		destination := filepath.Join(root, path)
		if item.mode.IsDir() {
			if err := os.MkdirAll(destination, 0700); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
			return err
		}
		if item.mode&os.ModeSymlink != 0 {
			if err := os.Symlink(item.data, destination); err != nil {
				return err
			}
		} else {
			if err := os.WriteFile(destination, []byte(item.data), 0600); err != nil {
				return err
			}
			if err := os.Chmod(destination, item.mode.Perm()); err != nil {
				return err
			}
		}
	}
	// Apply directory modes after materialization, including read-only directories.
	for path, item := range s.entries {
		if item.mode.IsDir() {
			if err := os.Chmod(filepath.Join(root, path), item.mode.Perm()); err != nil {
				return err
			}
		}
	}
	return ctx.Err()
}
func wikiContent(ctx context.Context, path string) (bool, error) {
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return false, err
	}
	defer func() { _ = root.Close() }()
	count := 0
	found := false
	var walk func(context.Context, string) error
	walk = func(ctx context.Context, p string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		count++
		if count > treeLimit {
			return errors.New("wiki tree entry limit exceeded")
		}
		info, err := root.Lstat(p)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("unsafe wiki symbolic link")
		}
		if info.Mode().IsRegular() {
			name := filepath.Base(p)
			if strings.HasSuffix(name, ".md") && name != "README.md" && name != "INDEX.md" {
				found = true
			}
			return nil
		}
		if !info.IsDir() {
			return errors.New("unsafe wiki special file")
		}
		f, err := root.OpenFile(p, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		children, err := f.ReadDir(treeLimit + 1)
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		if len(children) > treeLimit {
			return errors.New("wiki tree entry limit exceeded")
		}
		for _, child := range children {
			if err := walk(ctx, p+"/"+child.Name()); err != nil {
				return err
			}
		}
		return nil
	}
	err = walk(ctx, filepath.Base(path))
	return found, err
}
