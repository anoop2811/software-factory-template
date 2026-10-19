package installation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anoop2811/software-factory-template/internal/native"
	"github.com/anoop2811/software-factory-template/internal/staging"
)

type retentionScratch struct {
	path     string
	identity os.FileInfo
}

func newRetentionContext(request Request, resolver RetentionResolver) *retentionContext {
	result := &retentionContext{resolver: resolver, options: map[ReleaseIdentity]staging.Options{}, inspected: map[ReleaseIdentity]staging.Inspection{}, authenticated: map[ReleaseIdentity]bool{}}
	for _, option := range []staging.Options{request.Source, request.Before} {
		if option.Archive != "" {
			result.options[releaseIdentity(option)] = option
		}
	}
	return result
}
func (r *retentionContext) close() error {
	var errs []error
	for _, scratch := range r.scratch {
		current, err := os.Lstat(scratch.path)
		if err != nil || !os.SameFile(scratch.identity, current) {
			errs = append(errs, errors.Join(errors.New("retention staging custody changed; preserve private inputs"), err))
			continue
		}
		errs = append(errs, os.RemoveAll(scratch.path))
	}
	r.scratch = nil
	return errors.Join(errs...)
}
func validRetentionIdentity(hint ReleaseIdentity) bool {
	if !revisionPatternRetention.MatchString(hint.Revision) || !releaseTagPatternRetention.MatchString(hint.Version) {
		return false
	}
	switch strings.ToLower(hint.Version) {
	case "main", "head", "latest", "current":
		return false
	}
	switch hint.Target {
	case "linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64":
		return true
	}
	return false
}
func (r *retentionContext) image(ctx context.Context, hint ReleaseIdentity, authenticate bool) (staging.Inspection, error) {
	if !validRetentionIdentity(hint) {
		return staging.Inspection{}, conflict("historical release identity is unrecognized")
	}
	option, ok := r.options[hint]
	if !ok {
		if r.resolver == nil {
			return staging.Inspection{}, conflict("fresh historical release proof is unavailable")
		}
		var err error
		option, err = r.resolver(ctx, hint)
		if err != nil {
			return staging.Inspection{}, conflict("fresh historical release proof is unavailable")
		}
		if releaseIdentity(option) != hint || option.Local {
			return staging.Inspection{}, conflict("historical release identity changed")
		}
		r.options[hint] = option
	}
	image, ok := r.inspected[hint]
	if !ok {
		var err error
		image, err = staging.Inspect(ctx, option)
		if err != nil {
			return staging.Inspection{}, conflict("historical release image cannot be inspected")
		}
		r.inspected[hint] = image
	}
	if authenticate && !r.authenticated[hint] {
		if option.Local {
			return staging.Inspection{}, conflict("local images do not authorize release retention")
		}
		path, err := os.MkdirTemp("", "factory-retention-image-")
		if err != nil {
			return staging.Inspection{}, err
		}
		path, err = filepath.EvalSymlinks(path)
		if err != nil {
			return staging.Inspection{}, err
		}
		stat, err := os.Lstat(path)
		if err != nil {
			return staging.Inspection{}, err
		}
		r.scratch = append(r.scratch, retentionScratch{path, stat})
		option.Output, option.Installation = filepath.Join(path, "image"), true
		qualified, err := staging.Stage(ctx, option)
		if err != nil {
			var ownership *native.OwnershipError
			if errors.As(err, &ownership) {
				r.scratch = r.scratch[:len(r.scratch)-1]
				return staging.Inspection{}, fmt.Errorf("preserve unconfirmed retention input custody at %s: %w", path, err)
			}
			return staging.Inspection{}, err
		}
		if qualified.SHA256 != image.ArchiveSHA256 {
			return staging.Inspection{}, conflict("historical release image changed during authentication")
		}
		r.authenticated[hint] = true
	}
	return image, nil
}
