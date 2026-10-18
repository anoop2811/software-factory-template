package releasecmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/anoop2811/software-factory-template/internal/filepublish"
	"github.com/anoop2811/software-factory-template/internal/installationfs"
	"github.com/anoop2811/software-factory-template/internal/native"
	"github.com/anoop2811/software-factory-template/internal/staging"
)

const officialRepository = "anoop2811/software-factory-template"

var revisionPattern = regexp.MustCompile(`^[a-f0-9]{40}$`)

type acquired struct {
	directory           string
	identity            os.FileInfo
	options             staging.Options
	image               staging.Inspection
	staged, raw, digest string
	retain              bool
}

func (a *acquired) close() error {
	if a.retain {
		return fmt.Errorf("preserve private candidate recovery evidence at %s", a.directory)
	}
	info, err := os.Lstat(a.directory)
	if err != nil || !os.SameFile(info, a.identity) {
		return errors.Join(err, errors.New("candidate cache changed; cleanup refused"))
	}
	return os.RemoveAll(a.directory)
}

// Acquisition authenticates independent inputs before closed-world staging.
// docs/adr/0101-go-release-handoff.md:94, docs/adr/0101-go-release-handoff.md:119.
func acquire(ctx context.Context, tag string, environment map[string]string) (result *acquired, returned error) {
	target := runtime.GOOS + "/" + runtime.GOARCH
	switch target {
	case "linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64":
	default:
		return nil, errors.New("unsupported candidate platform")
	}
	directory, err := os.MkdirTemp("", "factory-release-")
	if err != nil {
		return nil, err
	}
	created := directory
	directory, err = filepath.EvalSymlinks(directory)
	if err != nil {
		return nil, fmt.Errorf("preserve unqualified private candidate cache at %s: %w", created, err)
	}
	identity, err := os.Lstat(directory)
	if err != nil {
		return nil, fmt.Errorf("preserve unqualified private candidate cache at %s: %w", created, err)
	}
	a := &acquired{directory: directory, identity: identity}
	defer func() {
		if result == nil {
			var ownership *native.OwnershipError
			if errors.As(returned, &ownership) {
				a.retain = true
			}
			returned = errors.Join(returned, a.close())
		}
	}()
	env := make(map[string]string, len(environment)+2)
	for key, value := range environment {
		env[key] = value
	}
	env["GH_HOST"], env["GH_PROMPT_DISABLED"] = "github.com", "1"
	gh := func(parent context.Context, limit int, args ...string) ([]byte, error) {
		child, cancel := context.WithTimeout(parent, 2*time.Minute)
		defer cancel()
		value, err := native.ExecuteCommandBounded(child, directory, append([]string{"gh"}, args...), env, limit, 64<<10)
		if value.OwnershipUnconfirmed {
			a.retain = true
		}
		if err != nil || value.ExitCode == nil || *value.ExitCode != 0 || !value.ExitConfirmed || value.OwnershipUnconfirmed {
			return nil, errors.Join(err, fmt.Errorf("trusted gh %s failed", args[0]))
		}
		return value.Stdout, nil
	}
	revision, err := gh(ctx, 256, "api", "repos/"+officialRepository+"/commits/refs/tags/"+tag, "--jq", ".sha")
	if err != nil {
		return nil, err
	}
	sha := strings.TrimSuffix(string(revision), "\n")
	if !revisionPattern.MatchString(sha) {
		return nil, errors.New("invalid candidate source revision")
	}
	release, err := gh(ctx, 512, "release", "view", tag, "--repo", officialRepository, "--json", "tagName,isDraft", "--jq", "[.tagName,.isDraft]|@tsv")
	if err != nil {
		return nil, err
	}
	if string(release) != tag+"\tfalse\n" {
		return nil, errors.New("candidate release is unavailable")
	}
	assetTarget := strings.ReplaceAll(target, "/", "-")
	raw := "factory-bootstrap-" + tag + "-" + assetTarget
	archive := "factory-installation-" + tag + "-" + assetTarget + ".tar.gz"
	names := []string{raw, archive, "runtime-attestations.jsonl"}
	limits := []int64{256 << 20, 128 << 20, 16 << 20}
	if source := environment["FACTORY_RELEASE_DIR"]; source != "" {
		if err := importCache(ctx, source, directory, names, limits); err != nil {
			return nil, err
		}
		// The shell-resolved revision is a hint only; a moved tag cannot replace it.
		pinned, err := readCacheRevision(ctx, source)
		if err != nil || pinned != sha+"\n" {
			return nil, errors.Join(err, errors.New("candidate release identity changed"))
		}
	} else {
		args := []string{"release", "download", tag, "--repo", officialRepository}
		for _, name := range names {
			args = append(args, "--pattern", name)
		}
		args = append(args, "--dir", directory)
		if err := boundedDownload(ctx, directory, names, limits, func(child context.Context) error { _, err := gh(child, 64<<10, args...); return err }); err != nil {
			return nil, err
		}
	}
	// Snapshot downloaded regular files under pinned custody before verification.
	input := filepath.Join(directory, "inputs")
	if err := os.Mkdir(input, 0700); err != nil {
		return nil, err
	}
	if err := importCache(ctx, directory, input, names, limits); err != nil {
		return nil, err
	}
	roots, err := gh(ctx, 16<<20, "attestation", "trusted-root")
	if err != nil {
		return nil, err
	}
	if len(roots) == 0 {
		return nil, errors.New("trusted roots are empty")
	}
	rootPath := filepath.Join(input, "trusted-root.jsonl")
	if err := os.WriteFile(rootPath, roots, 0600); err != nil {
		return nil, err
	}
	// Bind each official subject name as well as its exact bytes. The generic
	// staging verifier retains its existing offline contract.
	// docs/adr/0101-go-release-handoff.md:448.
	authenticate := func(name string, limit int64) (string, error) {
		digest, err := digestFile(ctx, filepath.Join(input, name), limit)
		if err != nil {
			return "", err
		}
		query := fmt.Sprintf(`any(.[]; any(.verificationResult.statement.subject[]; .name == %q and .digest.sha256 == %q))`, name, digest)
		verified, err := gh(ctx, 256, "attestation", "verify", filepath.Join(input, name), "--repo", officialRepository,
			"--signer-workflow", officialRepository+"/.github/workflows/runtime-release.yml", "--source-digest", sha,
			"--source-ref", "refs/tags/"+tag, "--bundle", filepath.Join(input, "runtime-attestations.jsonl"),
			"--custom-trusted-root", rootPath, "--deny-self-hosted-runners", "--format", "json", "--jq", query)
		if err != nil || string(verified) != "true\n" {
			return "", errors.Join(err, fmt.Errorf("candidate subject authentication failed: %s", name))
		}
		return digest, nil
	}
	digest, err := authenticate(raw, limits[0])
	if err != nil {
		return nil, err
	}
	archiveDigest, err := authenticate(archive, limits[1])
	if err != nil {
		return nil, err
	}
	a.options = staging.Options{Archive: filepath.Join(input, archive), Version: tag, Revision: sha, Target: target,
		Output: filepath.Join(directory, "stage"), Attestation: filepath.Join(input, "runtime-attestations.jsonl"), TrustedRoot: rootPath, Installation: true}
	staged, err := staging.Stage(ctx, a.options)
	if err != nil {
		return nil, err
	}
	a.image, err = staging.Inspect(ctx, a.options)
	if err != nil {
		return nil, err
	}
	if a.image.ArchiveSHA256 != staged.SHA256 || staged.SHA256 != archiveDigest || a.image.Runtime.SHA256 != digest {
		return nil, errors.New("candidate image disagrees with authenticated bootstrap")
	}
	a.raw, a.digest = filepath.Join(input, raw), digest
	a.staged = filepath.Join(staged.Root, tag, target)
	if err := os.WriteFile(filepath.Join(input, "revision"), []byte(sha+"\n"), 0600); err != nil {
		return nil, err
	}
	return a, nil
}

func importCache(ctx context.Context, source, destination string, names []string, limits []int64) (returned error) {
	tree, err := installationfs.Open(ctx, source)
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, tree.Close()) }()
	root, err := os.OpenRoot(destination)
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, root.Close()) }()
	for index, name := range names {
		file, err := tree.OpenFile(ctx, name, limits[index])
		if err != nil {
			return err
		}
		digest, _, err := file.Digest(ctx, limits[index])
		if err == nil {
			err = file.Rewind(ctx)
		}
		if err != nil {
			return errors.Join(err, file.Close())
		}
		stage, err := filepublish.PrepareReader(ctx, root, ".release-input-", file.File, file.Metadata().Size, digest, 0600)
		if err == nil {
			err = errors.Join(file.Check(ctx), stage.PublishNoReplace(ctx, name), stage.Cleanup())
		}
		if err = errors.Join(err, file.Close()); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func readCacheRevision(ctx context.Context, path string) (value string, returned error) {
	tree, err := installationfs.Open(ctx, path)
	if err != nil {
		return "", err
	}
	defer func() { returned = errors.Join(returned, tree.Close()) }()
	file, err := tree.OpenFile(ctx, "revision", 64)
	if err != nil {
		return "", err
	}
	defer func() { returned = errors.Join(returned, file.Close()) }()
	_, data, err := file.Digest(ctx, 64)
	return string(data), err
}

func digestFile(ctx context.Context, path string, limit int64) (string, error) {
	tree, err := installationfs.Open(ctx, filepath.Dir(path))
	if err != nil {
		return "", err
	}
	file, err := tree.OpenFile(ctx, filepath.Base(path), limit)
	if err != nil {
		return "", errors.Join(err, tree.Close())
	}
	digest, _, err := file.Digest(ctx, limit)
	return digest, errors.Join(err, file.Close(), tree.Close())
}

func currentDigest(ctx context.Context) (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", err
	}
	return digestFile(ctx, path, 256<<20)
}
