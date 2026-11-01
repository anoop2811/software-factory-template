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
	"syscall"

	"github.com/anoop2811/software-factory-template/internal/installationfs"
)

// Publish is private compiler-output plumbing, never installed admission.
// Exact-leaf rename refuses a directory even if it appears after inspection.
// docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:48.
func Publish(ctx context.Context) (returned error) {
	executable, err := os.Executable()
	if err != nil {
		return errors.New("cannot locate private developer build")
	}
	leaf := filepath.Base(executable)
	suffix, private := strings.CutPrefix(leaf, ".factory-go.")
	if !private || len(suffix) != 8 || strings.Trim(suffix, "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz") != "" {
		return errors.New("publication requires the launcher's private developer build")
	}
	root, err := installationfs.Open(ctx, filepath.Dir(executable))
	if err != nil {
		return fmt.Errorf("cannot open private developer build directory: %w", err)
	}
	defer func() { returned = errors.Join(returned, root.Close()) }()
	info, err := root.Root.Root.Lstat(leaf)
	if err != nil || !safeBuild(info) {
		return errors.Join(errors.New("private developer build is unsafe"), err)
	}
	before, err := root.Root.Root.Lstat("factory-go")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("cannot inspect developer build destination")
	}
	if err == nil && (!before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0) {
		return errors.New("developer build destination is unsafe; preserve it")
	}
	if err := root.Check(ctx); err != nil {
		return err
	}
	if err := root.Root.Root.Rename(leaf, "factory-go"); err != nil {
		return fmt.Errorf("cannot publish exact developer build destination: %w", err)
	}
	return root.Check(ctx)
}

func safeBuild(info os.FileInfo) bool {
	if info == nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 || info.Mode().Perm()&0022 != 0 || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int64(stat.Uid) == int64(os.Geteuid()) && stat.Nlink == 1
}
