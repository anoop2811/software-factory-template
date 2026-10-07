package packaging

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/anoop2811/software-factory-template/internal/artifact"
	"github.com/anoop2811/software-factory-template/internal/installationimage"
)

const archiveName = "factory-runtime.tar.gz"

type sourceMetadata struct {
	Revision  string `json:"revision"`
	Version   string `json:"version"`
	Target    string `json:"target"`
	GoVersion string `json:"go_version"`
	Kind      string `json:"kind"`
}

func createBundle(ctx context.Context, root *os.Root, options Options, image installationimage.Manifest) (string, []string, error) {
	slot := options.Version + "/" + options.Target
	binaryName := "store/" + slot + "/bin/factory-runtime"
	if err := root.MkdirAll(path.Dir(binaryName), 0700); err != nil {
		return "", nil, err
	}
	if err := root.Rename("factory-runtime", binaryName); err != nil {
		return "", nil, err
	}
	binaryDigest, err := hashFile(ctx, root, binaryName)
	if err != nil {
		return "", nil, err
	}
	if options.Installation {
		info, statErr := root.Stat(binaryName)
		if statErr != nil {
			return "", nil, statErr
		}
		if info.Size() > 256<<20 {
			return "", nil, errors.New("runtime binary exceeds limit")
		}
	}
	manifest := fmt.Sprintf("FACTORY_RUNTIME_ARTIFACT_V1\nversion\t%s\ntarget\t%s\nbinary\tbin/factory-runtime\nsha256\t%s\nEND\n", options.Version, options.Target, binaryDigest)
	manifestName := "store/" + slot + "/runtime.manifest"
	if err = writeFile(root, manifestName, []byte(manifest), 0644); err != nil {
		return "", nil, err
	}
	kind := "source-build"
	if options.Installation {
		kind = "installation-source-build"
	}
	metadata, err := json.Marshal(sourceMetadata{options.Revision, options.Version, options.Target, artifact.SourceGoVersion, kind})
	if err != nil {
		return "", nil, err
	}
	metadataName := "store/" + slot + "/source.json"
	if err = writeFile(root, metadataName, append(metadata, '\n'), 0644); err != nil {
		return "", nil, err
	}
	names := []string{binaryName, manifestName, metadataName}
	if options.Installation {
		data, encodeErr := installationimage.Encode(ctx, image, installationimage.Identity{Version: options.Version, Target: options.Target, Revision: options.Revision})
		if encodeErr != nil {
			return "", nil, encodeErr
		}
		name := "store/" + slot + "/installation.manifest"
		if err = writeFile(root, name, data, 0644); err != nil {
			return "", nil, err
		}
		names = append(names, name)
		for _, asset := range image.Assets {
			names = append(names, "store/"+slot+"/installation/assets/"+asset.Path)
		}
	}
	if err = writeArchive(ctx, root, names, options.Installation); err != nil {
		return "", nil, err
	}
	digest, err := hashFile(ctx, root, archiveName)
	if err != nil {
		return "", nil, err
	}
	if err = writeFile(root, "SHA256SUMS", []byte(digest+"  "+archiveName+"\n"), 0644); err != nil {
		return "", nil, err
	}
	return digest, append(names, archiveName, "SHA256SUMS"), nil
}

func writeFile(root *os.Root, name string, data []byte, mode os.FileMode) error {
	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	return errors.Join(writeErr, file.Close())
}

func hashFile(ctx context.Context, root *os.Root, name string) (digest string, err error) {
	file, err := root.Open(name)
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	hash := sha256.New()
	if _, err = io.Copy(hash, &contextReader{ctx, file}); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func writeArchive(ctx context.Context, root *os.Root, names []string, installation bool) error {
	file, err := root.OpenFile(archiveName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	compressed := gzip.NewWriter(file)
	var writer io.Writer = compressed
	if installation {
		writer = &wireWriter{writer: compressed, remaining: installationimage.WireLimit}
	}
	archive := tar.NewWriter(writer)
	for _, name := range names {
		if err = ctx.Err(); err != nil {
			break
		}
		if err = archiveFile(ctx, root, archive, name); err != nil {
			break
		}
	}
	err = errors.Join(err, archive.Close(), compressed.Close(), file.Close())
	if installation && err == nil {
		info, statErr := root.Stat(archiveName)
		if statErr != nil {
			return statErr
		}
		if info.Size() > 128<<20 {
			return errors.New("compressed installation archive exceeds limit")
		}
	}
	return err
}

func archiveFile(ctx context.Context, root *os.Root, archive *tar.Writer, name string) (err error) {
	file, err := root.Open(name)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	header := &tar.Header{Name: name[len("store/"):], Typeflag: tar.TypeReg, Mode: int64(bundleMode(name)), Size: info.Size(), ModTime: time.Unix(0, 0), Format: tar.FormatPAX}
	if err = archive.WriteHeader(header); err != nil {
		return err
	}
	_, err = io.Copy(archive, &contextReader{ctx, file})
	return err
}

func bundleMode(name string) os.FileMode {
	parts := strings.Split(name, "/")
	if len(parts) == 6 && parts[0] == "store" && parts[4] == "bin" && parts[5] == "factory-runtime" {
		return 0755
	}
	return 0644
}

type wireWriter struct {
	writer    io.Writer
	remaining int64
}

func (w *wireWriter) Write(data []byte) (int, error) {
	if int64(len(data)) > w.remaining {
		return 0, errors.New("installation archive exceeds aggregate wire limit")
	}
	n, err := w.writer.Write(data)
	w.remaining -= int64(n)
	return n, err
}

func publish(ctx context.Context, source *os.Root, output string, names []string) (err error) {
	parent, err := os.OpenRoot(filepath.Dir(output))
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, parent.Close()) }()
	name := filepath.Base(output)
	// Mkdir is the exclusive reservation. Never remove a publication directory:
	// even a failed publication may contain files created by another process.
	if err = parent.Mkdir(name, 0750); err != nil {
		return err
	}
	destination, err := parent.OpenRoot(name)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, destination.Close()) }()
	for _, name := range names {
		if err = ctx.Err(); err != nil {
			return err
		}
		if err = copyBundleFile(ctx, source, destination, name); err != nil {
			return err
		}
	}
	return nil
}

func copyBundleFile(ctx context.Context, source, destination *os.Root, name string) (err error) {
	if err := destination.MkdirAll(path.Dir(name), 0750); err != nil {
		return err
	}
	input, err := source.Open(name)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, input.Close()) }()
	output, err := destination.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, bundleMode(name))
	if err != nil {
		return err
	}
	modeErr := output.Chmod(bundleMode(name))
	_, copyErr := io.Copy(output, &contextReader{ctx, input})
	return errors.Join(modeErr, copyErr, output.Close())
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(data []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(data)
}
