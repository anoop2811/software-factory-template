package staging

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
)

func bundleNames(o Options) []string {
	prefix := o.Version + "/" + o.Target + "/"
	return []string{prefix + "bin/factory-runtime", prefix + "runtime.manifest", prefix + "source.json"}
}

func extract(ctx context.Context, snapshots, destination *os.Root, o Options) error {
	file, err := snapshots.Open("archive")
	if err != nil {
		return err
	}
	defer file.Close()
	compressed := bufio.NewReader(file)
	reader, err := gzip.NewReader(compressed)
	if err != nil {
		return err
	}
	defer reader.Close()
	reader.Multistream(false)
	const totalLimit = 257 << 20
	limited := &io.LimitedReader{R: &contextReader{ctx, reader}, N: totalLimit + 1}
	counted := &countReader{reader: limited}
	archive := tar.NewReader(counted)
	names := bundleNames(o)
	seen := make(map[string]bool)
	for {
		before := counted.count
		header, nextErr := archive.Next()
		if errors.Is(nextErr, io.EOF) {
			if counted.count-before < 1024 {
				return errors.New("missing complete tar end markers")
			}
			break
		}
		if nextErr != nil {
			return nextErr
		}
		if err = extractEntry(destination, archive, header, names, seen); err != nil {
			return err
		}
	}
	if len(seen) != len(names) {
		return errors.New("archive is missing required entries")
	}
	// Drain through gzip EOF, including its checksum, but allow only zero tar
	// padding. A ByteReader keeps the next compressed stream outside gzip.
	// docs/adr/0061-runtime-bundle-staging.md:51.
	if _, err = io.Copy(zeroWriter{}, counted); err != nil {
		return err
	}
	if limited.N == 0 {
		return errors.New("decompressed archive exceeds limit")
	}
	if _, err = compressed.ReadByte(); !errors.Is(err, io.EOF) {
		if err != nil {
			return err
		}
		return errors.New("archive has concatenated gzip or trailing compressed data")
	}
	return nil
}

func extractEntry(root *os.Root, archive *tar.Reader, header *tar.Header, names []string, seen map[string]bool) error {
	index := -1
	for i, name := range names {
		if header.Name == name {
			index = i
			break
		}
	}
	if index < 0 || seen[header.Name] {
		return fmt.Errorf("unexpected or duplicate archive entry %q", header.Name)
	}
	if header.Typeflag != tar.TypeReg || header.Linkname != "" {
		return errors.New("archive entries must be regular files")
	}
	mode, limit := int64(0644), int64(64<<10)
	if index == 0 {
		mode, limit = 0755, 256<<20
	}
	if header.Mode != mode || header.Size < 0 || header.Size > limit {
		return errors.New("invalid archive entry mode or size")
	}
	for key, value := range header.PAXRecords {
		if key != "path" || value != header.Name {
			return errors.New("unexpected extended archive metadata")
		}
	}
	if err := root.MkdirAll(path.Dir(header.Name), 0700); err != nil {
		return err
	}
	file, err := root.OpenFile(header.Name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, privateMode(index))
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, archive)
	if err = errors.Join(copyErr, file.Close()); err != nil {
		return err
	}
	seen[header.Name] = true
	return nil
}

func privateMode(index int) os.FileMode {
	if index == 0 {
		return 0700
	}
	return 0600
}

type countReader struct {
	reader io.Reader
	count  int64
}

func (r *countReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.count += int64(n)
	return n, err
}

type zeroWriter struct{}

func (zeroWriter) Write(p []byte) (int, error) {
	for _, value := range p {
		if value != 0 {
			return 0, errors.New("nonzero data after tar end")
		}
	}
	return len(p), nil
}
