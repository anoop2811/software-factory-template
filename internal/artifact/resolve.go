package artifact

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var versionComponent = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,127}$`)

// Resolve inspects exactly one local slot; it never searches for a substitute.
// docs/adr/0059-deterministic-runtime-selection.md:14.
func Resolve(ctx context.Context, store, version, target string) (Metadata, error) {
	if err := ctx.Err(); err != nil {
		return Metadata{}, err
	}
	if store == "" || !versionComponent.MatchString(version) || !validTarget(target) {
		return Metadata{}, fmt.Errorf("%w: expected store, literal version and GOOS/GOARCH", ErrInvalidRequest)
	}
	switch strings.ToLower(version) {
	case "latest", "current", "main", "head":
		return Metadata{}, fmt.Errorf("%w: moving version alias %q is not permitted", ErrInvalidRequest, version)
	}
	// Check each slot directory, including STORE itself, before reading a
	// manifest. No directory listing or environment-derived fallback is used.
	slot := trimDirectorySuffix(store)
	components := append([]string{"", version}, strings.Split(target, "/")...)
	for _, component := range components {
		if component != "" {
			slot = filepath.Join(slot, component)
		}
		info, err := os.Lstat(slot)
		if err != nil {
			return Metadata{}, fmt.Errorf("selected runtime directory %q: %w", slot, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return Metadata{}, fmt.Errorf("selected runtime directory %q is not a real directory", slot)
		}
		if component == "" {
			slot, err = filepath.EvalSymlinks(slot)
			if err != nil {
				return Metadata{}, fmt.Errorf("resolve selected runtime store: %w", err)
			}
		}
	}
	metadata, err := Verify(ctx, filepath.Join(slot, "runtime.manifest"), slot, target)
	if err != nil {
		return Metadata{}, err
	}
	if metadata.Version != version {
		return Metadata{}, fmt.Errorf("artifact version %q does not match requested %q", metadata.Version, version)
	}
	metadata.Binary = version + "/" + target + "/" + filepath.ToSlash(metadata.Binary)
	return metadata, nil
}

// Lstat follows a final symlink when the spelling ends in '/' or '/.'. Strip
// only those suffixes; cleaning interior '..' can select a different directory
// when an earlier component is a symlink.
func trimDirectorySuffix(path string) string {
	separator := string(filepath.Separator)
	for {
		path = strings.TrimRight(path, separator)
		if path == "" {
			return separator
		}
		if !strings.HasSuffix(path, separator+".") {
			return path
		}
		path = strings.TrimSuffix(path, separator+".")
	}
}
