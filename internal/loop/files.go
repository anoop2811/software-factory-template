package loop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"slices"
	"strings"
	"syscall"

	"github.com/anoop2811/software-factory-template/internal/jsonvalue"
)

const sourceFileLimit int64 = 64 << 20
const sourceTotalLimit int64 = 512 << 20
const nativeFileLimit int64 = 1 << 20
const nativeTotalLimit int64 = 8 << 20

type inspector struct {
	openFile         func(string, int) (*os.File, error)
	betweenSnapshots func(context.Context) error
}

func join(root, name string) string { return root + string(os.PathSeparator) + name }
func mode(info os.FileInfo) (uint32, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, loopError()
	}
	return uint32(stat.Mode) & 07777, nil
}
func (i *inspector) open(path string) (*os.File, error) {
	flags := os.O_RDONLY | syscall.O_NOFOLLOW | syscall.O_NONBLOCK
	if i.openFile != nil {
		return i.openFile(path, flags)
	}
	// Caller-selected roots and external config paths are intentional; NOFOLLOW,
	// NONBLOCK, descriptor validation and bounded reads apply at each call site.
	// docs/adr/0073-go-loop-fingerprint-foundation.md:78.
	return os.OpenFile(path, flags, 0) // #nosec G304 -- caller-authorized path, not confined to the checkout.
}

// File bytes, modes and link text remain distinct; opens refuse replacement
// symlinks/FIFOs and reads enforce actual bounds. docs/adr/0073-go-loop-fingerprint-foundation.md:104.
func (i *inspector) content(ctx context.Context, path string, total *int64) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, loopError()
	}
	bits, err := mode(info)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			return nil, loopError()
		}
		after, err := os.Lstat(path)
		if err != nil || !os.SameFile(info, after) || after.Mode()&os.ModeSymlink == 0 {
			return nil, loopError()
		}
		*total += int64(len(target))
		if *total > sourceTotalLimit {
			return nil, loopError()
		}
		hash := sha256.Sum256([]byte(target))
		return []any{bits, hex.EncodeToString(hash[:])}, nil
	}
	if !info.Mode().IsRegular() || info.Size() > sourceFileLimit {
		return nil, loopError()
	}
	file, err := i.open(path)
	if err != nil {
		return nil, loopError()
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return nil, loopError()
	}
	hash, err := hashFile(ctx, file, sourceFileLimit, sourceTotalLimit, total)
	if err != nil {
		return nil, err
	}
	after, err := file.Stat()
	if err != nil || opened.Size() != after.Size() || !opened.ModTime().Equal(after.ModTime()) || opened.Mode() != after.Mode() {
		return nil, loopError()
	}
	return []any{bits, hash}, nil
}
func hashFile(ctx context.Context, file *os.File, limit, totalLimit int64, total *int64) (string, error) {
	hash := sha256.New()
	buffer := make([]byte, 32<<10)
	var count int64
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, err := file.Read(buffer)
		count += int64(n)
		*total += int64(n)
		if count > limit || *total > totalLimit {
			return "", loopError()
		}
		if n > 0 {
			_, _ = hash.Write(buffer[:n])
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", loopError()
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
func safeAncestors(root, name string) error {
	current := root
	for _, part := range strings.Split(name, "/") {
		current = join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return loopError()
		}
	}
	return nil
}

// Native policy includes ignored known files and trees, preserving missing/null
// inventory and both entry limits. docs/adr/0073-go-loop-fingerprint-foundation.md:74.
func (i *inspector) nativePolicy(ctx context.Context, root string) (map[string]any, error) {
	files := []string{"opencode.json", "opencode.jsonc", ".opencode/opencode.json", ".opencode/opencode.jsonc", ".claude/settings.json", ".claude/settings.local.json", ".claude/CLAUDE.md", ".codex/config.toml", ".codex/AGENTS.md", ".codex/hooks.json"}
	trees := []string{".opencode/agent", ".opencode/agents", ".opencode/plugin", ".opencode/plugins", ".claude/agents", ".claude/rules", ".claude/hooks", ".codex/agents", ".codex/rules", ".codex/hooks"}
	values := map[string]any{}
	entries := 0
	var walk func(string) error
	walk = func(name string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		directory, err := os.OpenFile(join(root, name), os.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if err != nil {
			return loopError()
		}
		children, readErr := directory.ReadDir(513)
		closeErr := directory.Close()
		if (readErr != nil && !errors.Is(readErr, io.EOF)) || closeErr != nil {
			return loopError()
		}
		entries += len(children)
		if entries > 512 {
			return loopError()
		}
		dirs := []string{}
		for _, child := range children {
			childName := name + "/" + child.Name()
			info, err := os.Lstat(join(root, childName))
			if err != nil {
				return loopError()
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return loopError()
			}
			if info.IsDir() {
				if err := safeAncestors(root, childName); err != nil {
					return err
				}
				dirs = append(dirs, childName)
			} else {
				files = append(files, childName)
			}
		}
		if len(files)+len(dirs) > 512 {
			return loopError()
		}
		for _, directory := range dirs {
			if err := walk(directory); err != nil {
				return err
			}
		}
		return nil
	}
	for _, name := range trees {
		if err := safeAncestors(root, name); err != nil {
			return nil, err
		}
		info, err := os.Stat(join(root, name))
		exists := err == nil
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, loopError()
		}
		values[name+"/"] = exists
		if !exists {
			continue
		}
		if !info.IsDir() {
			return nil, loopError()
		}
		if err := walk(name); err != nil {
			return nil, err
		}
	}
	slices.Sort(files)
	files = slices.Compact(files)
	var total, statTotal int64
	for _, name := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := safeAncestors(root, name); err != nil {
			return nil, err
		}
		path := join(root, name)
		info, err := os.Lstat(path)
		key := jsonvalue.RawString(name)
		if errors.Is(err, os.ErrNotExist) {
			values[key] = nil
			continue
		}
		if err != nil || !info.Mode().IsRegular() || info.Size() > nativeFileLimit {
			return nil, loopError()
		}
		statTotal += info.Size()
		if statTotal > nativeTotalLimit {
			return nil, loopError()
		}
		file, err := i.open(path)
		if err != nil {
			return nil, loopError()
		}
		opened, err := file.Stat()
		if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
			_ = file.Close()
			return nil, loopError()
		}
		hash, readErr := hashFile(ctx, file, nativeFileLimit, nativeTotalLimit, &total)
		closeErr := file.Close()
		if readErr != nil || closeErr != nil {
			return nil, loopError()
		}
		bits, err := mode(opened)
		if err != nil {
			return nil, err
		}
		values[key] = []any{bits, hash}
	}
	return values, nil
}
