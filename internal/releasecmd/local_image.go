package releasecmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/anoop2811/software-factory-template/internal/initcmd"
	"github.com/anoop2811/software-factory-template/internal/installationfs"
	"github.com/anoop2811/software-factory-template/internal/staging"
)

const localInitProtocol = "--factory-local-init"

// LocalImageClaimed identifies a private transport, never an ambient cache hint.
// docs/adr/0116-explicit-go-local-initialization.md:54.
func LocalImageClaimed(args []string) bool {
	return len(args) > 0 && args[0] == localInitProtocol
}

// RunLocalImage runs only after installed admission has excluded this transport.
// It borrows the parent archive; only its own fresh snapshot may be removed.
// docs/adr/0116-explicit-go-local-initialization.md:64.
func RunLocalImage(parent context.Context, args []string, environment map[string]string, stdin io.Reader, stdout, stderr io.Writer) (status int) {
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	if !validLocalImage(args, localInitProtocol) {
		fmt.Fprintln(stderr, "factory local-source: invalid private local init protocol")
		return 2
	}
	owned, err := snapshotLocalImage(ctx, args, environment, "initializer")
	if err != nil {
		fmt.Fprintln(stderr, "factory local-source:", err)
		return 1
	}
	defer func() { status = owned.finish(stderr, status) }()
	return initcmd.RunQualified(ctx, localInitializerArgs(args[7:]), filepath.Join(owned.staged, "installation/assets"), owned.raw, owned.image, localEnvironment(environment), stdin, stdout, stderr)
}

func validLocalImage(args []string, protocol string) bool {
	return len(args) >= 7 && args[0] == protocol && args[1] == "1" && args[6] == "--" && revisionPattern.MatchString(args[4]) && len(args[5]) == 64 && strings.Trim(args[5], "0123456789abcdef") == ""
}

// Both local receivers snapshot borrowed qualified bytes into their own custody.
// Their private grammars and operation owners remain separate.
// docs/adr/0117-explicit-go-local-upgrade.md:52.
func snapshotLocalImage(ctx context.Context, args []string, environment map[string]string, purpose string) (result *acquired, returned error) {
	var owned *acquired
	defer func() {
		if returned != nil && owned != nil {
			returned = owned.failed(returned)
			result = nil
		}
	}()
	cache, source, revision, archiveDigest := args[2], args[3], args[4], args[5]
	tree, err := installationfs.Open(ctx, cache)
	if err != nil {
		return nil, fmt.Errorf("local archive custody: %w", err)
	}
	defer func() {
		returned = errors.Join(returned, tree.Close())
	}()
	archive := "package/factory-runtime.tar.gz"
	file, err := tree.OpenFile(ctx, archive, 128<<20)
	if err != nil {
		return nil, fmt.Errorf("local archive custody: %w", err)
	}
	digest, _, err := file.Digest(ctx, 128<<20)
	err = errors.Join(err, file.Close())
	if err != nil || digest != archiveDigest {
		return nil, errors.Join(errors.New("local archive digest mismatch"), err)
	}
	physical, head, err := inspectLocalSource(ctx, source, environment)
	if err != nil || physical != source || head != revision {
		return nil, errors.Join(errors.New("local source identity mismatch"), err)
	}
	option := staging.Options{Archive: filepath.Join(cache, filepath.FromSlash(archive)), Version: "local-" + revision, Revision: revision, Target: runtime.GOOS + "/" + runtime.GOARCH, Local: true, Installation: true}
	image, err := staging.Inspect(ctx, option)
	if err != nil || image.ArchiveSHA256 != archiveDigest {
		return nil, errors.Join(errors.New("local archive identity mismatch"), err)
	}
	self, err := currentDigest(ctx)
	if err != nil || self != image.Runtime.SHA256 {
		return nil, errors.Join(fmt.Errorf("selected %s digest mismatch", purpose), err)
	}
	// The selected process snapshots regular confined input before staging. It
	// cannot delete the borrowed root, including on malformed/private invocations.
	// docs/adr/0116-explicit-go-local-initialization.md:77.
	owned, err = newAcquired()
	if err != nil {
		return nil, err
	}
	if err := importCache(ctx, filepath.Join(cache, "package"), owned.directory, []releaseAsset{{name: "factory-runtime.tar.gz", maxBytes: 128 << 20}}); err != nil {
		return nil, fmt.Errorf("local archive snapshot: %w", err)
	}
	option.Archive, option.Output = filepath.Join(owned.directory, "factory-runtime.tar.gz"), filepath.Join(owned.directory, "stage")
	staged, err := staging.Stage(ctx, option)
	if err != nil || staged.SHA256 != archiveDigest {
		return nil, errors.Join(errors.New("local archive staging mismatch"), err)
	}
	if err := tree.Check(ctx); err != nil {
		return nil, fmt.Errorf("local archive custody changed: %w", err)
	}
	owned.options, owned.image = option, image
	owned.staged = filepath.Join(staged.Root, option.Version, option.Target)
	owned.raw, owned.digest = filepath.Join(owned.staged, "bin/factory-runtime"), image.Runtime.SHA256
	return owned, nil
}

// Preserve literal target operands without changing the existing initializer's
// non-terminated option grammar or other callers. A trailing delimiter retains
// its default target. docs/adr/0116-explicit-go-local-initialization.md:26.
func localInitializerArgs(args []string) []string {
	for index, arg := range args {
		if arg != "--" {
			continue
		}
		forwarded := append([]string{}, args[:index]...)
		for _, target := range args[index+1:] {
			if target != "" && !filepath.IsAbs(target) {
				target = "." + string(filepath.Separator) + target
			}
			forwarded = append(forwarded, target)
		}
		return forwarded
	}
	return args
}
