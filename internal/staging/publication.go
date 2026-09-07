package staging

import (
	"context"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
)

func publish(ctx context.Context, source *os.Root, output string, o Options) error {
	parent, err := os.OpenRoot(filepath.Dir(output))
	if err != nil {
		return err
	}
	defer parent.Close()
	name := filepath.Base(output)
	// Exclusive reservation: never remove or reuse a final output, even after
	// an interrupted copy. docs/adr/0061-runtime-bundle-staging.md:65.
	if err = parent.Mkdir(name, 0700); err != nil {
		return err
	}
	destination, err := parent.OpenRoot(name)
	if err != nil {
		return err
	}
	defer destination.Close()
	for i, entry := range bundleNames(o) {
		if err = destination.MkdirAll(path.Dir(entry), 0700); err != nil {
			return err
		}
		if err = publishFile(ctx, source, destination, entry, privateMode(i)); err != nil {
			return err
		}
	}
	return nil
}

func publishFile(ctx context.Context, source, destination *os.Root, name string, mode os.FileMode) error {
	input, err := source.Open(name)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := destination.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	modeErr := output.Chmod(mode)
	_, copyErr := io.Copy(output, &contextReader{ctx, input})
	return errors.Join(modeErr, copyErr, output.Close())
}
