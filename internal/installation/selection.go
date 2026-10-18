package installation

import (
	"context"
	"os"

	"github.com/anoop2811/software-factory-template/internal/staging"
)

// SelectedAsset is an inert projection, not authentication or mutation authority.
// docs/adr/0101-go-release-handoff.md:148.
type SelectedAsset struct {
	Path    string
	Data    []byte
	Mode    os.FileMode
	Runtime bool
}

// PrivateIgnoreRules is shared by fresh initialization and journaled upgrades.
func PrivateIgnoreRules() []string {
	return []string{"/.factory/bin/", "/.factory/assets/", "/.factory/installation.current", "/.factory/installation-transactions/", "/.factory/backups/"}
}

// Selection shares the reviewed role generators between fresh init and upgrades.
func Selection(ctx context.Context, image staging.Inspection, inputs map[string]string) ([]SelectedAsset, error) {
	var result []SelectedAsset
	var actions []Action
	for _, row := range roles {
		if row.Operation == "retire" {
			continue
		}
		data, err := generate(ctx, row, image, inputs)
		if err != nil {
			return nil, err
		}
		mode := modeFor(row)
		after := bytesImage(data, uint32(mode))
		binary := row.Path == ".factory/bin/factory-runtime"
		if binary {
			after.SHA256, after.Bytes = image.Runtime.SHA256, image.BinaryBytes
		}
		actions = append(actions, Action{Path: row.Path, After: after})
		result = append(result, SelectedAsset{Path: row.Path, Data: data, Mode: mode, Runtime: binary})
	}
	data, err := descriptorBytes(image, actions)
	if err != nil {
		return nil, err
	}
	for index := range result {
		if result[index].Path == ".factory/installation.current" {
			result[index].Data = data
		}
	}
	return result, ctx.Err()
}
