package installation

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/anoop2811/software-factory-template/internal/installationimage"
	"io"
	"strings"
)

// Raw immutable blobs are recognition evidence only, never execution fallback.
// docs/adr/0098-whole-installation-upgrade-and-rollback.md:294.
//
//go:embed reference-*.json.gz
var referenceData embed.FS

type baseline struct {
	Profile     string                    `json:"profile"`
	Revision    string                    `json:"revision"`
	Catalog     []installationimage.Asset `json:"catalog"`
	Blobs       map[string][]byte         `json:"blobs"`
	Substituted []string                  `json:"substituted"`
}

func reference(ctx context.Context, profile string) (baseline, error) {
	if profile != "v0.1.6" && profile != "bash-baseline" {
		return baseline{}, conflict("installation profile requires a qualified predecessor")
	}
	data, err := referenceData.ReadFile("reference-" + profile + ".json.gz")
	if err != nil {
		return baseline{}, err
	}
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return baseline{}, err
	}
	decoded, readErr := io.ReadAll(io.LimitReader(reader, (4<<20)+1))
	closeErr := reader.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return baseline{}, err
	}
	if len(decoded) > 4<<20 {
		return baseline{}, errors.New("embedded reference exceeds bound")
	}
	data = decoded
	var result baseline
	if err := json.Unmarshal(data, &result); err != nil {
		return baseline{}, err
	}
	pins := installationimage.References()
	var pin installationimage.Reference
	for _, value := range pins {
		if value.Label == profile {
			pin = value
		}
	}
	digest, err := installationimage.CatalogDigest(ctx, result.Catalog)
	if err != nil {
		return baseline{}, err
	}
	if digest != pin.Digest || result.Revision != pin.Revision || len(result.Catalog) != pin.Count || len(result.Blobs) != pin.Count {
		return baseline{}, errors.New("embedded installation reference disagrees with independent catalog")
	}
	for _, asset := range result.Catalog {
		if err := ctx.Err(); err != nil {
			return baseline{}, err
		}
		data, found := result.Blobs[asset.Path]
		sum := sha256.Sum256(data)
		if !found || int64(len(data)) != asset.Bytes || hex.EncodeToString(sum[:]) != asset.SHA256 {
			return baseline{}, errors.New("embedded installation reference bytes disagree with catalog")
		}
	}
	return result, nil
}
func (reference baseline) original(ctx context.Context, path string, inputs map[string]string) ([]byte, uint32, bool, error) {
	raw, found := reference.Blobs[path]
	if !found {
		return nil, 0, false, nil
	}
	transformed := false
	for _, name := range reference.Substituted {
		transformed = transformed || name == path
	}
	if transformed {
		var err error
		raw, err = project(ctx, raw, inputs)
		if err != nil {
			return nil, 0, true, err
		}
	}
	mode := uint32(0644)
	for _, asset := range reference.Catalog {
		if asset.Path == path && asset.Mode == "100755" {
			mode = 0755
		}
	}
	if path == "factory" || path == ".githooks/pre-push" || strings.HasPrefix(path, "scripts/hooks/") || strings.HasPrefix(path, "scripts/") && strings.HasSuffix(path, ".sh") && !strings.HasPrefix(path, "scripts/lib/") {
		mode |= 0111
	}
	return raw, mode, true, nil
}
func project(ctx context.Context, raw []byte, inputs map[string]string) ([]byte, error) {
	value := string(raw)
	for _, key := range []string{"PROJECT_NAME", "DOCS_ROOT", "PROJECT_SLUG", "GITHUB_OWNER", "OPENCODE_USERNAME", "MUTATION_TARGET", "PROTECTED_PATH"} {
		marker := "__" + key + "__"
		if !strings.Contains(value, marker) {
			continue
		}
		input, found := inputs[key]
		if key == "MUTATION_TARGET" {
			input, found = inputs["PROTECTED_PATH"]
			if input == "" || input == "." {
				input = "./..."
			} else {
				input = "./" + input + "/..."
			}
		}
		if !found {
			return nil, conflict("installation baseline transformation requires explicit project inputs")
		}
		if key == "DOCS_ROOT" && input == "" {
			input = "docs"
		}
		if key == "PROTECTED_PATH" && input == "" {
			input = "."
		}
		// The baseline sed substitution interprets ampersands and backslashes. Refuse
		// unqualified replacement syntax rather than recognize a guessed projection.
		if strings.ContainsAny(input, "&\\|\r\n") {
			return nil, conflict("installation project input needs an unsupported baseline transformation")
		}
		value = strings.ReplaceAll(value, marker, input)
	}
	return []byte(value), ctx.Err()
}
