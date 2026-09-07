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
	"time"

	"github.com/anoop2811/software-factory-template/internal/artifact"
)

const archiveName = "factory-runtime.tar.gz"

type sourceMetadata struct {
	Revision  string `json:"revision"`
	Version   string `json:"version"`
	Target    string `json:"target"`
	GoVersion string `json:"go_version"`
	Kind      string `json:"kind"`
}

func createBundle(root *os.Root, options Options) (string, []string, error) {
	slot := options.Version + "/" + options.Target
	binaryName := "store/" + slot + "/bin/factory-runtime"
	if err := root.MkdirAll(path.Dir(binaryName), 0700); err != nil {
		return "", nil, err
	}
	if err := root.Rename("factory-runtime", binaryName); err != nil {
		return "", nil, err
	}
	binaryDigest, err := hashFile(root, binaryName)
	if err != nil {
		return "", nil, err
	}
	manifest := fmt.Sprintf("FACTORY_RUNTIME_ARTIFACT_V1\nversion\t%s\ntarget\t%s\nbinary\tbin/factory-runtime\nsha256\t%s\nEND\n", options.Version, options.Target, binaryDigest)
	manifestName := "store/" + slot + "/runtime.manifest"
	if err = writeFile(root, manifestName, []byte(manifest), 0644); err != nil {
		return "", nil, err
	}
	metadata, err := json.Marshal(sourceMetadata{options.Revision, options.Version, options.Target, artifact.SourceGoVersion, "source-build"})
	if err != nil {
		return "", nil, err
	}
	metadataName := "store/" + slot + "/source.json"
	if err = writeFile(root, metadataName, append(metadata, '\n'), 0644); err != nil {
		return "", nil, err
	}
	names := []string{binaryName, manifestName, metadataName}
	if err = writeArchive(root, names); err != nil {
		return "", nil, err
	}
	digest, err := hashFile(root, archiveName)
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

func hashFile(root *os.Root, name string) (string, error) {
	file, err := root.Open(name)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func writeArchive(root *os.Root, names []string) error {
	file, err := root.OpenFile(archiveName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	compressed := gzip.NewWriter(file)
	archive := tar.NewWriter(compressed)
	for _, name := range names {
		if err = archiveFile(root, archive, name); err != nil {
			break
		}
	}
	return errors.Join(err, archive.Close(), compressed.Close(), file.Close())
}

func archiveFile(root *os.Root, archive *tar.Writer, name string) error {
	file, err := root.Open(name)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	header := &tar.Header{Name: name[len("store/"):], Typeflag: tar.TypeReg, Mode: int64(bundleMode(name)), Size: info.Size(), ModTime: time.Unix(0, 0), Format: tar.FormatPAX}
	if err = archive.WriteHeader(header); err != nil {
		return err
	}
	_, err = io.Copy(archive, file)
	return err
}

func bundleMode(name string) os.FileMode {
	if path.Base(name) == "factory-runtime" {
		return 0755
	}
	return 0644
}

func publish(ctx context.Context, source *os.Root, output string, names []string) error {
	parent, err := os.OpenRoot(filepath.Dir(output))
	if err != nil {
		return err
	}
	defer parent.Close()
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
	defer destination.Close()
	for _, name := range names {
		if err = ctx.Err(); err != nil {
			return err
		}
		if err = copyBundleFile(source, destination, name); err != nil {
			return err
		}
	}
	return nil
}

func copyBundleFile(source, destination *os.Root, name string) error {
	if err := destination.MkdirAll(path.Dir(name), 0750); err != nil {
		return err
	}
	input, err := source.Open(name)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := destination.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, bundleMode(name))
	if err != nil {
		return err
	}
	modeErr := output.Chmod(bundleMode(name))
	_, copyErr := io.Copy(output, input)
	return errors.Join(modeErr, copyErr, output.Close())
}
