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

	"github.com/anoop2811/software-factory-template/internal/staging"
)

// Raw immutable blobs are recognition evidence only, never execution fallback.
// docs/adr/0098-whole-installation-upgrade-and-rollback.md:294.
//
//go:embed reference-*.json.gz
var referenceData embed.FS

type baseline struct {
	Profile       string                    `json:"profile"`
	Revision      string                    `json:"revision"`
	Catalog       []installationimage.Asset `json:"catalog"`
	Blobs         map[string][]byte         `json:"blobs"`
	Substituted   []string                  `json:"substituted"`
	Expected      map[string]*Image         `json:"-"`
	ArchiveSHA256 string                    `json:"-"`
	Runtime       *staging.Inspection       `json:"-"`
}

// A predecessor uses a closed supported generation recipe, never saved code.
// docs/adr/0101-go-release-handoff.md:275, docs/adr/0101-go-release-handoff.md:285.
func predecessor(ctx context.Context, request Request, after staging.Inspection) (baseline, error) {
	if request.Profile != "go-hybrid-v1" {
		return reference(ctx, request.Profile)
	}
	if request.Before.Target != request.Source.Target || request.Before.Archive == "" {
		return baseline{}, conflict("Go predecessor requires its exact qualified image")
	}
	before, err := staging.Inspect(ctx, request.Before)
	if err != nil {
		return baseline{}, err
	}
	// Conservative V1 closure: changes to a generator, transformation, mode/hash
	// helper or descriptor schema require an independently supported next recipe.
	for _, path := range []string{"internal/installation/generate.go", "internal/installation/roles.go", "internal/installation/orchestration.go", "internal/installation/reference.go", "internal/installation/model.go", "internal/installation/observe.go", "internal/installation/selection.go", "internal/installedlayout/layout.go"} {
		old, oldOK := before.Assets[path]
		current, currentOK := after.Assets[path]
		if !oldOK || !currentOK || !bytes.Equal(old, current) {
			return baseline{}, conflict("unsupported Go predecessor generation recipe; preserve the existing installation")
		}
	}
	selection, err := Selection(ctx, before, request.Inputs)
	if err != nil {
		return baseline{}, err
	}
	result := baseline{Profile: request.Profile, Revision: before.Manifest.SourceRevision, Blobs: map[string][]byte{}, Expected: map[string]*Image{}, ArchiveSHA256: before.ArchiveSHA256, Runtime: &before}
	for _, entry := range selection {
		result.Blobs[entry.Path] = entry.Data
		expected := bytesImage(entry.Data, uint32(entry.Mode))
		if entry.Runtime {
			expected.SHA256, expected.Bytes = before.Runtime.SHA256, before.BinaryBytes
		}
		result.Expected[entry.Path] = expected
	}
	return result, nil
}

func (reference baseline) expected(ctx context.Context, path string, inputs map[string]string) (*Image, []byte, bool, error) {
	if reference.Expected != nil {
		image, known := reference.Expected[path]
		return image, reference.Blobs[path], known, ctx.Err()
	}
	raw, mode, known, err := reference.original(ctx, path, inputs)
	if err != nil || !known {
		return nil, raw, known, err
	}
	return bytesImage(raw, mode), raw, true, nil
}

// matching qualifies exact before-images, including the two historical legacy
// marker spellings. Marker matching alone never establishes profile ownership.
// docs/adr/0101-go-release-handoff.md:465.
func (reference baseline) matching(ctx context.Context, path string, observed *Image, inputs map[string]string) (*Image, bool, error) {
	expected, _, known, err := reference.expected(ctx, path, inputs)
	if err != nil || !known {
		return expected, false, err
	}
	if imageEqual(observed, expected, false) {
		return expected, true, nil
	}
	if path != ".factory-version" || reference.Expected != nil || len(reference.Revision) != 40 || (reference.Profile != "v0.1.6" && reference.Profile != "bash-baseline") {
		return expected, false, nil
	}
	refs := []string{"main"}
	if reference.Profile == "v0.1.6" {
		refs = append(refs, reference.Profile)
	}
	for _, ref := range refs {
		for _, revision := range []string{reference.Revision, reference.Revision[:7]} {
			marker := bytesImage([]byte("ref="+ref+"\ncommit="+revision+"\n"), 0644)
			if imageEqual(observed, marker, false) {
				return marker, true, nil
			}
		}
	}
	return expected, false, ctx.Err()
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
	if path == ".factory-version" {
		ref := reference.Profile
		if ref == "bash-baseline" {
			ref = "main"
		}
		return []byte("ref=" + ref + "\ncommit=" + reference.Revision + "\n"), 0644, true, ctx.Err()
	}
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
