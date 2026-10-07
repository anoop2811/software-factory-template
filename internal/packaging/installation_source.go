package packaging

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/anoop2811/software-factory-template/internal/installationimage"
	"github.com/anoop2811/software-factory-template/internal/native"
)

type committedBlob struct{ name, mode, object string }

func boundedGitOutput(ctx context.Context, repository string, args []string) (data []byte, err error) {
	err = readGit(ctx, repository, args, 64<<10, func(input io.Reader) error {
		var readErr error
		data, readErr = io.ReadAll(io.LimitReader(input, (64<<10)+1))
		if readErr != nil {
			return readErr
		}
		if len(data) > 64<<10 {
			return errors.New("git response exceeds capture limit")
		}
		return nil
	})
	return data, err
}

// Consume only a bounded completed snapshot after confirmed process ownership.
// docs/adr/0097-installation-source-image-bundles.md:205.
func readGit(ctx context.Context, repository string, args []string, limit int, consume func(io.Reader) error) error {
	command := gitCommand(ctx, repository, args...)
	environment := make(map[string]string, len(command.Env))
	for _, entry := range command.Env {
		key, value, _ := strings.Cut(entry, "=")
		environment[key] = value
	}
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	result, err := native.ExecuteCommandBounded(ctx, root, command.Args, environment, limit, 64<<10)
	if err != nil || result.Outcome != "completed" || !result.ExitConfirmed || result.OwnershipUnconfirmed || result.ExitCode == nil || *result.ExitCode != 0 {
		if err == nil {
			err = errors.New("git command did not establish completed process ownership")
		}
		return fmt.Errorf("git %s: %w: %s", args[0], errors.Join(ctx.Err(), err), result.Stderr)
	}
	return errors.Join(ctx.Err(), consume(bytes.NewReader(result.Stdout)))
}

func splitTree(data []byte, atEOF bool) (int, []byte, error) {
	if index := bytes.IndexByte(data, 0); index >= 0 {
		return index + 1, data[:index], nil
	}
	if atEOF && len(data) != 0 {
		return 0, nil, errors.New("git tree record is not NUL-terminated")
	}
	return 0, nil, nil
}

func committedTree(ctx context.Context, repository, revision string, limit int) ([]committedBlob, error) {
	var blobs []committedBlob
	err := readGit(ctx, repository, []string{"ls-tree", "-r", "-z", "--full-tree", revision}, (installationimage.PathLimit+128)*(limit+1), func(input io.Reader) error {
		scanner := bufio.NewScanner(input)
		scanner.Buffer(make([]byte, installationimage.PathLimit+128), installationimage.PathLimit+128)
		scanner.Split(splitTree)
		for scanner.Scan() {
			if err := ctx.Err(); err != nil {
				return err
			}
			if len(blobs) == limit {
				return errors.New("committed source tree exceeds asset count limit")
			}
			metadata, name, present := strings.Cut(scanner.Text(), "\t")
			fields := strings.Fields(metadata)
			if !present || len(fields) != 3 || fields[1] != "blob" ||
				(fields[0] != "100644" && fields[0] != "100755" && fields[0] != "120000") ||
				len(fields[2]) != 40 || strings.Trim(fields[2], "0123456789abcdef") != "" {
				return errors.New("unsupported committed Git tree entry")
			}
			if err := installationimage.ValidatePath(name); err != nil {
				return err
			}
			blobs = append(blobs, committedBlob{name, fields[0], fields[2]})
		}
		return scanner.Err()
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(blobs, func(a, b committedBlob) int { return strings.Compare(a.name, b.name) })
	return blobs, nil
}

func blobSize(ctx context.Context, repository, object string) (int64, error) {
	var size int64
	err := readGit(ctx, repository, []string{"cat-file", "-s", object}, 32, func(input io.Reader) error {
		data, err := io.ReadAll(io.LimitReader(input, 32))
		if err != nil {
			return err
		}
		if len(data) == 32 {
			return errors.New("git blob size response exceeds limit")
		}
		text := strings.TrimSuffix(string(data), "\n")
		size, err = strconv.ParseInt(text, 10, 64)
		if err != nil || size < 0 || strconv.FormatInt(size, 10) != text || size > installationimage.AssetLimit {
			return errors.New("committed source blob exceeds byte limit or has invalid size")
		}
		return nil
	})
	return size, err
}

func collectCatalog(ctx context.Context, repository, revision string, countLimit int, root *os.Root, payloadPrefix string) ([]installationimage.Asset, error) {
	blobs, err := committedTree(ctx, repository, revision, countLimit)
	if err != nil {
		return nil, err
	}
	assets := make([]installationimage.Asset, 0, len(blobs))
	var total int64
	for _, blob := range blobs {
		size, err := blobSize(ctx, repository, blob.object)
		if err != nil {
			return nil, err
		}
		total += size
		if total > installationimage.PayloadLimit {
			return nil, errors.New("committed source payload exceeds total byte limit")
		}
		digest, err := collectBlob(ctx, repository, blob, size, root, payloadPrefix)
		if err != nil {
			return nil, err
		}
		assets = append(assets, installationimage.Asset{Path: blob.name, Mode: blob.mode, SHA256: digest, Bytes: size})
	}
	if err := installationimage.ValidateCatalog(ctx, assets); err != nil {
		return nil, err
	}
	return assets, nil
}

func collectBlob(ctx context.Context, repository string, blob committedBlob, size int64, root *os.Root, prefix string) (digest string, err error) {
	hash := sha256.New()
	var output io.Writer = hash
	if prefix != "" {
		name := prefix + blob.name
		if err = root.MkdirAll(path.Dir(name), 0700); err != nil {
			return "", err
		}
		file, openErr := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if openErr != nil {
			return "", openErr
		}
		defer func() { err = errors.Join(err, file.Close()) }()
		if err = file.Chmod(0644); err != nil {
			return "", err
		}
		output = io.MultiWriter(file, hash)
	}
	err = readGit(ctx, repository, []string{"cat-file", "blob", blob.object}, int(size)+1, func(input io.Reader) error {
		n, copyErr := io.Copy(output, io.LimitReader(input, size+1))
		if copyErr != nil {
			return copyErr
		}
		if n != size {
			return errors.New("committed blob stream disagrees with its preflight byte count")
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func collectInstallation(ctx context.Context, root *os.Root, options Options) (installationimage.Manifest, error) {
	identity := installationimage.Identity{Version: options.Version, Target: options.Target, Revision: options.Revision}
	manifest := installationimage.Manifest{SchemaVersion: 1, Mode: "installation_source_image", Scope: "committed_source_and_baseline_references", Version: identity.Version, Target: identity.Target, SourceRevision: identity.Revision}
	repository := filepath.Join(root.Name(), "repository")
	if err := readGit(ctx, repository, []string{"cat-file", "-t", options.Revision}, 64<<10, func(input io.Reader) error {
		data, err := io.ReadAll(io.LimitReader(input, 32))
		if err != nil {
			return err
		}
		if string(data) != "commit\n" {
			return errors.New("installation source revision must identify a commit")
		}
		return nil
	}); err != nil {
		return installationimage.Manifest{}, err
	}
	prefix := "store/" + options.Version + "/" + options.Target + "/installation/assets/"
	assets, err := collectCatalog(ctx, repository, options.Revision, installationimage.AssetCountLimit, root, prefix)
	if err != nil {
		return installationimage.Manifest{}, err
	}
	manifest.Assets = assets
	for _, reference := range installationimage.References() {
		assets, err = collectCatalog(ctx, repository, reference.Revision, reference.Count, root, "")
		if err != nil {
			return installationimage.Manifest{}, err
		}
		manifest.Baselines = append(manifest.Baselines, installationimage.Baseline{Label: reference.Label, Revision: reference.Revision, Assets: assets})
	}
	if err = installationimage.Validate(ctx, manifest, identity); err != nil {
		return installationimage.Manifest{}, err
	}
	return manifest, nil
}

// Compile from the already collected, inert committed bytes; Git source links
// never become compiler filesystem links. Preserve the V1 runtime source subset.
// docs/adr/0097-installation-source-image-bundles.md:84.
func prepareInstallationSource(ctx context.Context, root *os.Root, options Options, image installationimage.Manifest) error {
	prefix := "store/" + options.Version + "/" + options.Target + "/installation/assets/"
	for _, asset := range image.Assets {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := asset.Path
		if name != "go.mod" && name != "go.sum" && !strings.HasPrefix(name, "cmd/factory/") && !strings.HasPrefix(name, "internal/") {
			continue
		}
		if asset.Mode == "120000" {
			return fmt.Errorf("source symlink cannot be compiled: %s", name)
		}
		input, err := root.Open(prefix + name)
		if err != nil {
			return err
		}
		copyErr := extractFile(root, "source/"+name, input)
		if err = errors.Join(copyErr, input.Close()); err != nil {
			return err
		}
	}
	return nil
}
