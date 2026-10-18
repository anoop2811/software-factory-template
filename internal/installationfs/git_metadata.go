package installationfs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

// RequireNoGitMetadata distinguishes a normal nonrepository from damaged or
// unreadable metadata before permitting a detached project proof.
// docs/adr/0101-go-release-handoff.md:493.
func RequireNoGitMetadata(ctx context.Context, path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("git metadata inspection requires an absolute project path")
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := os.Lstat(filepath.Join(path, ".git")); !errors.Is(err, os.ErrNotExist) {
			return errors.Join(err, errors.New("git metadata exists or cannot be inspected; detached project proof refused"))
		}
		parent := filepath.Dir(path)
		if parent == path {
			return nil
		}
		path = parent
	}
}
