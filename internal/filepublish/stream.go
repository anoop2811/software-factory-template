package filepublish

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"
)

// PrepareReader shares checked staging and streams a separately bounded image.
// docs/adr/0098-whole-installation-upgrade-and-rollback.md:175.
func PrepareReader(ctx context.Context, directory *os.Root, prefix string, input io.Reader, size int64, digest string, mode os.FileMode) (*Stage, error) {
	if input == nil || size < 0 || size > 256<<20 || len(digest) != 64 || strings.Trim(digest, "0123456789abcdef") != "" {
		return nil, errors.New("invalid streaming publication input")
	}
	write := func(ctx context.Context, file *os.File) error {
		hash := sha256.New()
		n, err := io.CopyBuffer(io.MultiWriter(file, hash), &streamReader{ctx, io.LimitReader(input, size+1)}, make([]byte, 32<<10))
		if err != nil {
			return err
		}
		if n != size || hex.EncodeToString(hash.Sum(nil)) != digest {
			return errors.New("streamed publication input disagrees with qualified image")
		}
		return ctx.Err()
	}
	verify := func(ctx context.Context, file *os.File) error {
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return err
		}
		hash := sha256.New()
		n, err := io.CopyBuffer(hash, &streamReader{ctx, io.LimitReader(file, size+1)}, make([]byte, 32<<10))
		if err != nil {
			return err
		}
		if n != size || hex.EncodeToString(hash.Sum(nil)) != digest {
			return errors.New("prepared streamed publication bytes changed")
		}
		return ctx.Err()
	}
	return prepareOperation(ctx, directory, prefix, mode, ops{}, write, verify)
}

type streamReader struct {
	ctx   context.Context
	input io.Reader
}

func (reader *streamReader) Read(data []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.input.Read(data)
}

// PublishNoReplace preserves a late foreign addition at the destination.
// docs/adr/0098-whole-installation-upgrade-and-rollback.md:184.
func (stage *Stage) PublishNoReplace(ctx context.Context, name string) (returned error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if name == "" || strings.ContainsAny(name, "/\\\x00") || name == "." || name == ".." {
		return errors.New("invalid no-replace publication name")
	}
	if stage == nil || stage.published {
		return errors.New("staging ownership unavailable")
	}
	current, err := stage.directory.Lstat(stage.name)
	if err != nil {
		return err
	}
	if !samePrepared(stage.identity, current) {
		return errors.New("temporary changed before publication")
	}
	directory, err := stage.directory.Open(".")
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, directory.Close()) }()
	if err := renameNoReplace(int(directory.Fd()), stage.name, name); err != nil {
		return err
	}
	stage.published = true
	return nil
}
