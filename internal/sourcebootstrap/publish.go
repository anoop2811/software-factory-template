// Package sourcebootstrap publishes the developer launcher's one local output.
// docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:43.
package sourcebootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anoop2811/software-factory-template/internal/filepublish"
	"github.com/anoop2811/software-factory-template/internal/installationfs"
)

const executableLimit = 256 << 20

// Invocation pins the private executable independently of the shared cache.
// docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:189.
type Invocation struct {
	tree   *installationfs.Tree
	binary *installationfs.File
	path   string
	closed bool
}

// OpenInvocation admits only the launcher's actual single-linked private file.
// docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:199.
func OpenInvocation(ctx context.Context) (*Invocation, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, errors.New("cannot locate private developer build")
	}
	leaf := filepath.Base(executable)
	suffix, private := strings.CutPrefix(leaf, ".factory-go.")
	if !private || len(suffix) != 8 || strings.Trim(suffix, "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz") != "" {
		return nil, errors.New("source invocation requires the launcher's private developer build")
	}
	root, err := installationfs.Open(ctx, filepath.Dir(executable))
	if err != nil {
		return nil, fmt.Errorf("cannot open private developer build directory: %w", err)
	}
	binary, err := root.Root.OpenFile(ctx, leaf, executableLimit)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("retain unqualified private developer build %q: %w", executable, err), root.Close())
	}
	if binary.Metadata().Mode&0111 == 0 {
		return nil, errors.Join(errors.New("private developer build is not executable"), binary.Close(), root.Close())
	}
	return &Invocation{tree: root, binary: binary, path: executable}, nil
}

// Publish keeps the invocation file single-linked and publishes a separate copy.
// docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:189.
func Publish(ctx context.Context) (returned error) {
	invocation, err := OpenInvocation(ctx)
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, invocation.closeHandles()) }()
	directory := invocation.tree.Root.Root
	before, err := directory.Lstat("factory-go")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("cannot inspect developer build destination")
	}
	if err == nil && !before.Mode().IsRegular() {
		return errors.New("developer build destination is unsafe; preserve it")
	}
	digest, _, err := invocation.binary.Digest(ctx, executableLimit)
	if err == nil {
		err = invocation.binary.Rewind(ctx)
	}
	if err != nil {
		return err
	}
	stage, err := filepublish.PrepareReader(ctx, directory, ".factory-go.cache-", invocation.binary.File, invocation.binary.Metadata().Size, digest, 0700)
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, stage.Cleanup()) }()
	if err := invocation.binary.Check(ctx); err != nil {
		return err
	}
	if err := stage.Publish(ctx, "factory-go"); err != nil {
		return fmt.Errorf("cannot publish exact developer build destination: %w", err)
	}
	return invocation.binary.Check(ctx)
}

// Close removes only this still-qualified invocation file, then releases handles.
// docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:199.
func (invocation *Invocation) Close(ctx context.Context) (returned error) {
	if invocation == nil || invocation.closed {
		return nil
	}
	defer func() { returned = errors.Join(returned, invocation.closeHandles()) }()
	if err := invocation.binary.Check(ctx); err != nil {
		return fmt.Errorf("private source cleanup refused; retain %q: %w", invocation.path, err)
	}
	if err := invocation.binary.Parent.Root.Remove(invocation.binary.Name); err != nil {
		return fmt.Errorf("private source cleanup failed; retain %q: %w", invocation.path, err)
	}
	return nil
}

func (invocation *Invocation) closeHandles() error {
	if invocation == nil || invocation.closed {
		return nil
	}
	invocation.closed = true
	return errors.Join(invocation.binary.Close(), invocation.tree.Close())
}
