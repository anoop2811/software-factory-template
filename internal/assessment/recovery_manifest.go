package assessment

import (
	"context"
	"encoding/json"
	"strconv"
	"unicode/utf8"

	"github.com/anoop2811/software-factory-template/internal/jsonvalue"
)

type recoveryManifest struct {
	held   bool
	assets []referenceAsset
}

func recoveryID(name string) bool {
	if len(name) < 1 || len(name) > 64 {
		return false
	}
	for i, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			continue
		}
		if i == 0 || (c != '-' && c != '_') {
			return false
		}
	}
	return true
}
func recoveryRevision(text string) bool {
	if len(text) != 40 {
		return false
	}
	for _, c := range text {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// The format is strict reference data; even valid metadata grants no transaction authority.
// docs/adr/0083-go-recovery-set-inspection.md:32.
func parseRecoveryManifest(ctx context.Context, data []byte, name string) (recoveryManifest, bool) {
	if !utf8.Valid(data) {
		return recoveryManifest{}, false
	}
	value, err := jsonvalue.DecodeUnique(ctx, data)
	if err != nil {
		return recoveryManifest{}, false
	}
	object, ok := value.(map[string]any)
	if !ok || len(object) != 7 {
		return recoveryManifest{}, false
	}
	schema, ok := object["schema_version"].(json.Number)
	if !ok || schema.String() != "1" || object["migration_id"] != name || object["source_revision"] != referenceRevision || object["scope"] != "g2-budget-loop-six" {
		return recoveryManifest{}, false
	}
	target, ok := object["target_revision"].(string)
	if !ok || !recoveryRevision(target) || target == referenceRevision {
		return recoveryManifest{}, false
	}
	held, ok := object["held"].(bool)
	if !ok {
		return recoveryManifest{}, false
	}
	assets, ok := object["assets"].([]any)
	if !ok || len(assets) < 1 || len(assets) > len(catalog) {
		return recoveryManifest{}, false
	}
	result := recoveryManifest{held: held, assets: make([]referenceAsset, 0, len(assets))}
	previous := ""
	for _, value := range assets {
		if ctx.Err() != nil {
			return recoveryManifest{}, false
		}
		asset, ok := value.(map[string]any)
		if !ok || len(asset) != 4 {
			return recoveryManifest{}, false
		}
		path, ok := asset["path"].(string)
		if !ok || path <= previous {
			return recoveryManifest{}, false
		}
		previous = path
		found := false
		for _, reference := range catalog {
			if reference.Path != path {
				continue
			}
			size, ok := asset["bytes"].(json.Number)
			if !ok || size.String() != strconv.FormatInt(reference.Reference.Bytes, 10) || asset["sha256"] != reference.Reference.SHA256 || asset["mode"] != reference.Reference.Mode {
				return recoveryManifest{}, false
			}
			found = true
			result.assets = append(result.assets, reference)
			break
		}
		if !found {
			return recoveryManifest{}, false
		}
	}
	return result, true
}
