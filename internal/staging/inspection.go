package staging

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"syscall"

	"github.com/anoop2811/software-factory-template/internal/artifact"
	"github.com/anoop2811/software-factory-template/internal/installationimage"
)

// Inspection is bounded read-only evidence, never a trust or activation grant.
// docs/adr/0098-whole-installation-upgrade-and-rollback.md:117.
type Inspection struct {
	Manifest                installationimage.Manifest
	Runtime                 artifact.Metadata
	ArchiveSHA256           string
	BinaryBytes             int64
	RuntimeManifest, Source []byte
	Assets                  map[string][]byte
}

func Inspect(ctx context.Context, options Options) (result Inspection, returned error) {
	options.Installation, options.Output = true, "inspection"
	if err := options.validate(); err != nil {
		return result, err
	}
	before, err := os.Lstat(options.Archive)
	if err != nil {
		return result, err
	}
	if !before.Mode().IsRegular() || before.Size() < 0 || before.Size() > 128<<20 {
		return result, errors.New("invalid archive input")
	}
	file, err := os.OpenFile(options.Archive, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return result, err
	}
	defer func() { returned = errors.Join(returned, file.Close()) }()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) || !opened.Mode().IsRegular() {
		return result, errors.New("archive input changed")
	}
	hash := sha256.New()
	limited := &io.LimitedReader{R: &contextReader{ctx, file}, N: (128 << 20) + 1}
	image, err := inspectArchive(ctx, io.TeeReader(limited, hash), nil, options)
	if err != nil {
		return result, err
	}
	if limited.N == 0 {
		return result, errors.New("compressed archive exceeds limit")
	}
	after, err := file.Stat()
	named, nameErr := os.Lstat(options.Archive)
	if err != nil || nameErr != nil || !sameInspectionFile(opened, after) || !sameInspectionFile(after, named) {
		return result, errors.New("archive changed during inspection")
	}
	prefix := options.Version + "/" + options.Target + "/"
	metadata, err := artifact.DecodeManifest(ctx, bytes.NewReader(image.controls[prefix+"runtime.manifest"]), options.Target)
	if err != nil {
		return result, err
	}
	if metadata.Version != options.Version || metadata.Binary != "bin/factory-runtime" || metadata.SHA256 != image.binaryDigest {
		return result, errors.New("runtime identity or digest mismatch")
	}
	expected := map[string]string{"version": options.Version, "revision": options.Revision, "target": options.Target, "go_version": artifact.SourceGoVersion, "kind": "installation-source-build"}
	decoder := json.NewDecoder(bytes.NewReader(image.controls[prefix+"source.json"]))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return result, errors.New("invalid source identity")
	}
	seen := map[string]bool{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return result, err
		}
		key, ok := token.(string)
		if !ok || seen[key] {
			return result, errors.New("duplicate source field")
		}
		required, known := expected[key]
		if !known {
			return result, errors.New("unknown source field")
		}
		var value string
		if err := decoder.Decode(&value); err != nil || value != required {
			return result, errors.New("source identity mismatch")
		}
		seen[key] = true
	}
	if _, err := decoder.Token(); err != nil || len(seen) != len(expected) {
		return result, errors.New("incomplete source identity")
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return result, errors.New("trailing source identity")
	}
	return Inspection{Manifest: image.manifest, Runtime: metadata, ArchiveSHA256: hex.EncodeToString(hash.Sum(nil)), BinaryBytes: image.binaryBytes, RuntimeManifest: image.controls[prefix+"runtime.manifest"], Source: image.controls[prefix+"source.json"], Assets: image.payload}, ctx.Err()
}

func sameInspectionFile(a, b os.FileInfo) bool {
	x, xok := a.Sys().(*syscall.Stat_t)
	y, yok := b.Sys().(*syscall.Stat_t)
	return xok && yok && os.SameFile(a, b) && a.Mode() == b.Mode() && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime()) && x.Nlink == 1 && y.Nlink == 1 && x.Uid == y.Uid && x.Gid == y.Gid
}

func (image *imageArchive) inspectEntry(ctx context.Context, archive *tar.Reader, header *tar.Header, options Options) error {
	index := image.next
	if index >= len(image.names) || header.Name != image.names[index] || header.Typeflag != tar.TypeReg || header.Linkname != "" {
		return errors.New("unexpected installation archive member")
	}
	mode, limit := int64(0644), int64(64<<10)
	if index == 0 {
		mode, limit = 0755, 256<<20
	}
	if index == 3 {
		limit = installationimage.ManifestLimit
	}
	if index >= 4 {
		limit = image.manifest.Assets[index-4].Bytes
	}
	if header.Mode != mode || header.Size < 0 || header.Size > limit || index >= 4 && header.Size != limit {
		return errors.New("invalid installation member mode or size")
	}
	for key, value := range header.PAXRecords {
		if key != "path" || value != header.Name {
			return errors.New("unexpected installation extended metadata")
		}
	}
	if image.controls == nil {
		image.controls = map[string][]byte{}
		image.payload = map[string][]byte{}
	}
	hash := sha256.New()
	if index == 0 {
		n, err := io.Copy(hash, &contextReader{ctx, archive})
		if err != nil {
			return err
		}
		if n != header.Size {
			return io.ErrUnexpectedEOF
		}
		image.binaryDigest = hex.EncodeToString(hash.Sum(nil))
		image.binaryBytes = n
	} else {
		data, err := io.ReadAll(&contextReader{ctx, io.LimitReader(archive, limit+1)})
		if err != nil {
			return err
		}
		if int64(len(data)) != header.Size {
			return errors.New("installation member length changed")
		}
		if index == 3 {
			image.manifest, err = installationimage.Decode(ctx, data, installationimage.Identity{Version: options.Version, Target: options.Target, Revision: options.Revision})
			if err != nil {
				return err
			}
			for _, asset := range image.manifest.Assets {
				image.names = append(image.names, options.Version+"/"+options.Target+"/installation/assets/"+asset.Path)
			}
		}
		if index >= 4 {
			asset := image.manifest.Assets[index-4]
			digest := sha256.Sum256(data)
			if hex.EncodeToString(digest[:]) != asset.SHA256 {
				return errors.New("source asset digest mismatch")
			}
			image.payload[asset.Path] = data
		} else {
			image.controls[header.Name] = data
		}
	}
	image.next++
	return ctx.Err()
}
