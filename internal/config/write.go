package config

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const configWriteLimit = 16 << 20

// Set publishes literal configuration bytes in a trusted, quiescent directory.
// This is not a concurrent-writer CAS: docs/adr/0063-go-configuration-writes.md:59.
func Set(ctx context.Context, path, key, value string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("factory config: %w", err)
	}
	// The legacy setter captures its resolved path through command substitution.
	// docs/adr/0063-go-configuration-writes.md:24.
	path = strings.TrimRight(path, "\n")
	// Split preserves symlink/.. traversal in the caller-selected parent path.
	directory, name := filepath.Split(path)
	if directory == "" {
		directory = "."
	}
	basename := filepath.Base(path)
	if path == "" {
		basename = ""
	}
	missing := func() error {
		//nolint:revive,staticcheck // ST1005: preserve the exact legacy diagnostic punctuation: docs/adr/0063-go-configuration-writes.md:32.
		return fmt.Errorf("factory config: no %s here — run factory init first.", basename)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
			return missing()
		}
		return fmt.Errorf("factory config: open parent: %w", err)
	}
	defer root.Close()
	if name == "" || name == ".." {
		return missing()
	}
	info, err := root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) || (err == nil && info.IsDir()) {
		return missing()
	}
	if err != nil {
		return fmt.Errorf("factory config: inspect input: %w", err)
	}
	original, info, err := readWriteInput(ctx, root, name)
	if err != nil {
		return fmt.Errorf("factory config: read input: %w", err)
	}
	replacement, err := rewriteConfig(ctx, original, key, value)
	if err != nil {
		return fmt.Errorf("factory config: %w", err)
	}
	if err := publishConfig(ctx, root, name, original, replacement, info); err != nil {
		return fmt.Errorf("factory config: publish: %w", err)
	}
	return nil
}

func writeMetadata(info os.FileInfo) (*syscall.Stat_t, error) {
	metadata, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode() != info.Mode().Perm() {
		return nil, errors.New("input must be a regular file with ordinary permission bits")
	}
	if metadata.Nlink != 1 || int64(metadata.Uid) != int64(os.Geteuid()) {
		return nil, errors.New("input must be caller-owned with a single hard link")
	}
	if info.Size() > configWriteLimit {
		return nil, errors.New("configuration exceeds 16 MiB")
	}
	return metadata, nil
}

func sameWriteInput(before, after os.FileInfo) bool {
	a, err := writeMetadata(before)
	if err != nil {
		return false
	}
	b, err := writeMetadata(after)
	return err == nil && os.SameFile(before, after) && before.Mode() == after.Mode() &&
		before.Size() == after.Size() && before.ModTime().Equal(after.ModTime()) &&
		a.Uid == b.Uid && a.Gid == b.Gid
}

func readWriteInput(ctx context.Context, root *os.Root, name string) ([]byte, os.FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	before, err := root.Lstat(name)
	if err != nil {
		return nil, nil, err
	}
	if _, err := writeMetadata(before); err != nil {
		return nil, nil, err
	}
	input, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, nil, err
	}
	defer input.Close()
	after, err := input.Stat()
	if err != nil {
		return nil, nil, err
	}
	if !sameWriteInput(before, after) {
		return nil, nil, errors.New("configuration changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(input, configWriteLimit+1))
	if err != nil {
		return nil, nil, err
	}
	if len(data) > configWriteLimit {
		return nil, nil, errors.New("configuration exceeds 16 MiB")
	}
	final, err := input.Stat()
	if err != nil {
		return nil, nil, err
	}
	if !sameWriteInput(before, final) {
		return nil, nil, errors.New("configuration changed while reading")
	}
	return data, final, ctx.Err()
}

// Rewrite physical lines without YAML escaping or newline repair.
// Replacement and append differ intentionally: docs/adr/0063-go-configuration-writes.md:37.
func rewriteConfig(ctx context.Context, data []byte, key, value string) ([]byte, error) {
	var output bytes.Buffer
	appendBytes := func(value []byte) error {
		if len(value) > configWriteLimit-output.Len() {
			return errors.New("resulting configuration exceeds 16 MiB")
		}
		_, err := output.Write(value)
		return err
	}
	prefix := []byte(key + ":")
	replacement := []byte(key + ": \"" + strings.ReplaceAll(value, "\n", " ") + "\"")
	found := false
	for len(data) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		line, rest, newline := bytes.Cut(data, []byte{'\n'})
		if bytes.HasPrefix(line, prefix) {
			line = replacement
			found = true
		}
		if err := appendBytes(line); err != nil {
			return nil, err
		}
		if newline {
			if err := appendBytes([]byte{'\n'}); err != nil {
				return nil, err
			}
		}
		data = rest
	}
	if !found {
		if err := appendBytes([]byte(key + ": \"" + value + "\"\n")); err != nil {
			return nil, err
		}
	}
	return output.Bytes(), ctx.Err()
}

// Fully prepare and close the exclusive sibling before rechecking and renaming.
// Publication and cleanup boundaries: docs/adr/0063-go-configuration-writes.md:48.
func publishConfig(ctx context.Context, root *os.Root, name string, original, replacement []byte, info os.FileInfo) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	temporaryName := ".factory-config-" + rand.Text()
	temporary, err := root.OpenFile(temporaryName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	created, err := temporary.Stat()
	if err != nil {
		return errors.Join(fmt.Errorf("cleanup refused: cannot identify created temporary configuration: %w", err), temporary.Close())
	}
	closed, published := false, false
	defer func() {
		if !closed {
			result = errors.Join(result, temporary.Close())
		}
		if !published {
			// An observed replacement is never this call's cleanup responsibility.
			// docs/adr/0063-go-configuration-writes.md:71.
			occupant, err := root.Lstat(temporaryName)
			switch {
			case errors.Is(err, os.ErrNotExist):
			case err != nil:
				result = errors.Join(result, fmt.Errorf("cleanup refused: inspect temporary configuration: %w", err))
			case !os.SameFile(created, occupant):
				result = errors.Join(result, errors.New("cleanup refused: temporary configuration identity changed"))
			default:
				result = errors.Join(result, root.Remove(temporaryName))
			}
		}
	}()
	if _, err := temporary.Write(replacement); err != nil {
		return err
	}
	metadata, err := writeMetadata(info)
	if err != nil {
		return err
	}
	if err := temporary.Chown(int(metadata.Uid), int(metadata.Gid)); err != nil {
		return err
	}
	if err := temporary.Chmod(info.Mode().Perm()); err != nil {
		return err
	}
	prepared, err := temporary.Stat()
	if err != nil {
		return err
	}
	preparedMetadata, err := writeMetadata(prepared)
	if err != nil || prepared.Mode() != info.Mode() || preparedMetadata.Uid != metadata.Uid || preparedMetadata.Gid != metadata.Gid {
		return errors.New("could not preserve configuration ownership and permissions")
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	err = temporary.Close()
	closed = true
	if err != nil {
		return err
	}
	current, currentInfo, err := readWriteInput(ctx, root, name)
	if err != nil {
		return err
	}
	if !sameWriteInput(info, currentInfo) || !bytes.Equal(original, current) {
		return errors.New("configuration changed before publication")
	}
	currentTemporary, err := root.Lstat(temporaryName)
	if err != nil {
		return err
	}
	if !sameWriteInput(prepared, currentTemporary) {
		return errors.New("temporary configuration changed before publication")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := root.Rename(temporaryName, name); err != nil {
		return err
	}
	published = true
	return nil
}
