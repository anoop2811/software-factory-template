package staging

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/anoop2811/software-factory-template/internal/artifact"
)

func validateIdentity(ctx context.Context, content *os.Root, directory string, o Options) (err error) {
	slot := o.Version + "/" + o.Target
	root := filepath.Join(directory, filepath.FromSlash(slot))
	metadata, err := artifact.Verify(ctx, filepath.Join(root, "runtime.manifest"), root, o.Target)
	if err != nil {
		return err
	}
	if metadata.Version != o.Version || metadata.Binary != "bin/factory-runtime" {
		return errors.New("manifest version or binary does not match bundle identity")
	}
	file, err := content.Open(slot + "/source.json")
	if err != nil {
		return err
	}
	defer func() {
		closeErr := file.Close()
		if o.Installation {
			err = errors.Join(err, closeErr)
		}
	}()
	return validateSourceIdentity(ctx, file, o)
}
