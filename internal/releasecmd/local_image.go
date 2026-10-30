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
	if len(args) < 7 || args[0] != localInitProtocol || args[1] != "1" || args[6] != "--" || !revisionPattern.MatchString(args[4]) || len(args[5]) != 64 || strings.Trim(args[5], "0123456789abcdef") != "" {
		fmt.Fprintln(stderr, "factory local-source: invalid private local init protocol")
		return 2
	}
	cache, source, revision, archiveDigest := args[2], args[3], args[4], args[5]
	tree, err := installationfs.Open(ctx, cache)
	if err != nil {
		fmt.Fprintln(stderr, "factory local-source: local archive custody:", err)
		return 1
	}
	defer func() {
		if err := tree.Close(); err != nil {
			fmt.Fprintln(stderr, "factory local-source: local archive custody closure:", err)
			status = 1
		}
	}()
	archive := "package/factory-runtime.tar.gz"
	file, err := tree.OpenFile(ctx, archive, 128<<20)
	if err != nil {
		fmt.Fprintln(stderr, "factory local-source: local archive custody:", err)
		return 1
	}
	digest, _, err := file.Digest(ctx, 128<<20)
	err = errors.Join(err, file.Close())
	if err != nil || digest != archiveDigest {
		fmt.Fprintln(stderr, "factory local-source: local archive digest mismatch")
		return 1
	}
	physical, head, err := inspectLocalSource(ctx, source, environment)
	if err != nil || physical != source || head != revision {
		fmt.Fprintln(stderr, "factory local-source: local source identity mismatch:", err)
		return 1
	}
	option := staging.Options{Archive: filepath.Join(cache, filepath.FromSlash(archive)), Version: "local-" + revision, Revision: revision, Target: runtime.GOOS + "/" + runtime.GOARCH, Local: true, Installation: true}
	image, err := staging.Inspect(ctx, option)
	if err != nil || image.ArchiveSHA256 != archiveDigest {
		fmt.Fprintln(stderr, "factory local-source: local archive identity mismatch:", err)
		return 1
	}
	self, err := currentDigest(ctx)
	if err != nil || self != image.Runtime.SHA256 {
		fmt.Fprintln(stderr, "factory local-source: selected initializer digest mismatch")
		return 1
	}
	// The selected process snapshots regular confined input before staging. It
	// cannot delete the borrowed root, including on malformed/private invocations.
	// docs/adr/0116-explicit-go-local-initialization.md:77.
	owned, err := newAcquired()
	if err != nil {
		fmt.Fprintln(stderr, "factory local-source:", err)
		return 1
	}
	defer func() { status = owned.finish(stderr, status) }()
	if err := importCache(ctx, filepath.Join(cache, "package"), owned.directory, []releaseAsset{{name: "factory-runtime.tar.gz", maxBytes: 128 << 20}}); err != nil {
		fmt.Fprintln(stderr, "factory local-source: local archive snapshot:", err)
		return 1
	}
	option.Archive, option.Output = filepath.Join(owned.directory, "factory-runtime.tar.gz"), filepath.Join(owned.directory, "stage")
	staged, err := staging.Stage(ctx, option)
	if err != nil || staged.SHA256 != archiveDigest {
		fmt.Fprintln(stderr, "factory local-source: local archive staging mismatch:", err)
		return 1
	}
	if err := tree.Check(ctx); err != nil {
		fmt.Fprintln(stderr, "factory local-source: local archive custody changed:", err)
		return 1
	}
	root := filepath.Join(staged.Root, option.Version, option.Target)
	return initcmd.RunQualified(ctx, localInitializerArgs(args[7:]), filepath.Join(root, "installation/assets"), filepath.Join(root, "bin/factory-runtime"), image, localEnvironment(environment), stdin, stdout, stderr)
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
