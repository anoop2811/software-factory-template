package staging

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/anoop2811/software-factory-template/internal/installationimage"
)

type imageArchive struct {
	names    []string
	next     int
	manifest installationimage.Manifest
}

func newImageArchive(o Options) *imageArchive {
	names := bundleNames(o)
	return &imageArchive{names: append(names, o.Version+"/"+o.Target+"/installation.manifest")}
}

// The fixed controls precede the manifest-derived payload map exactly. No member
// or mode is inferred from the basename of untrusted source data.
// docs/adr/0097-installation-source-image-bundles.md:59.
func (image *imageArchive) extractEntry(ctx context.Context, root *os.Root, archive *tar.Reader, header *tar.Header, o Options) error {
	index := image.next
	if index >= len(image.names) || header.Name != image.names[index] {
		return fmt.Errorf("unexpected or reordered installation entry %q", header.Name)
	}
	if header.Typeflag != tar.TypeReg || header.Linkname != "" {
		return errors.New("installation entries must be regular files")
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
	if header.Mode != mode || header.Size < 0 || header.Size > limit || (index >= 4 && header.Size != limit) {
		return errors.New("invalid installation entry mode or byte length")
	}
	for key, value := range header.PAXRecords {
		if key != "path" || value != header.Name {
			return errors.New("unexpected installation extended metadata")
		}
	}
	if index == 3 {
		data, err := io.ReadAll(&contextReader{ctx, io.LimitReader(archive, installationimage.ManifestLimit+1)})
		if err != nil {
			return err
		}
		manifest, err := installationimage.Decode(ctx, data, installationimage.Identity{Version: o.Version, Target: o.Target, Revision: o.Revision})
		if err != nil {
			return err
		}
		image.manifest = manifest
		for _, asset := range manifest.Assets {
			image.names = append(image.names, o.Version+"/"+o.Target+"/installation/assets/"+asset.Path)
		}
		if err = writeImageControl(ctx, root, header.Name, data); err != nil {
			return err
		}
	} else {
		if err := writeImageEntry(ctx, root, archive, header.Name, privateMode(index), index, image.manifest); err != nil {
			return err
		}
	}
	image.next++
	return nil
}

func writeImageControl(ctx context.Context, root *os.Root, name string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := root.MkdirAll(path.Dir(name), 0700); err != nil {
		return err
	}
	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	return errors.Join(writeErr, file.Close())
}

func writeImageEntry(ctx context.Context, root *os.Root, input io.Reader, name string, mode os.FileMode, index int, manifest installationimage.Manifest) error {
	if err := root.MkdirAll(path.Dir(name), 0700); err != nil {
		return err
	}
	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	hash := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(file, hash), &contextReader{ctx, input})
	err = errors.Join(copyErr, file.Close())
	if err != nil {
		return err
	}
	if index >= 4 {
		asset := manifest.Assets[index-4]
		if n != asset.Bytes || hex.EncodeToString(hash.Sum(nil)) != asset.SHA256 {
			return errors.New("installation payload disagrees with manifest digest or bytes")
		}
	}
	return nil
}

func installationNames(ctx context.Context, root *os.Root, o Options) (names []string, err error) {
	image := newImageArchive(o)
	file, err := root.Open(image.names[3])
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	data, err := io.ReadAll(&contextReader{ctx, io.LimitReader(file, installationimage.ManifestLimit+1)})
	if err != nil {
		return nil, err
	}
	manifest, err := installationimage.Decode(ctx, data, installationimage.Identity{Version: o.Version, Target: o.Target, Revision: o.Revision})
	if err != nil {
		return nil, err
	}
	for _, asset := range manifest.Assets {
		image.names = append(image.names, o.Version+"/"+o.Target+"/installation/assets/"+asset.Path)
	}
	return image.names, nil
}
