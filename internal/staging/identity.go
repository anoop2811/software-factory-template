package staging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/anoop2811/software-factory-template/internal/artifact"
)

func validateIdentity(ctx context.Context, content *os.Root, directory string, o Options) error {
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
	defer file.Close()
	expected := map[string]string{"revision": o.Revision, "version": o.Version, "target": o.Target, "go_version": "go1.27.1", "kind": "source-build"}
	decoder := json.NewDecoder(file)
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if token != json.Delim('{') {
		return errors.New("source metadata must be a JSON object")
	}
	seen := make(map[string]bool)
	for decoder.More() {
		token, err = decoder.Token()
		if err != nil {
			return err
		}
		key, ok := token.(string)
		value, known := expected[key]
		if !ok || !known || seen[key] {
			return fmt.Errorf("unknown or duplicate source field %q", token)
		}
		var actual string
		if err = decoder.Decode(&actual); err != nil {
			return err
		}
		if actual != value {
			return fmt.Errorf("source field %q does not match requested identity", key)
		}
		seen[key] = true
	}
	if _, err = decoder.Token(); err != nil {
		return err
	}
	if len(seen) != len(expected) {
		return errors.New("source metadata missing required fields")
	}
	if _, err = decoder.Token(); !errors.Is(err, io.EOF) {
		return errors.New("source metadata has trailing data")
	}
	return nil
}
