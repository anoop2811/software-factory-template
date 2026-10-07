// Package installationimage validates inert committed-source image catalogs.
package installationimage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/anoop2811/software-factory-template/internal/jsonvalue"
)

// Independent ceilings include the whole decompressed wire, not only payload.
// docs/adr/0097-installation-source-image-bundles.md:114.
const (
	ManifestLimit   = 512 << 10
	AssetLimit      = 1 << 20
	AssetCountLimit = 4096
	PayloadLimit    = 64 << 20
	WireLimit       = 321 << 20
	PathLimit       = 1024
)

type Identity struct{ Version, Target, Revision string }

var (
	versionPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,127}$`)
	revisionPattern = regexp.MustCompile(`^[a-f0-9]{40}$`)
)

func validIdentity(identity Identity) bool {
	if !versionPattern.MatchString(identity.Version) || !revisionPattern.MatchString(identity.Revision) {
		return false
	}
	switch strings.ToLower(identity.Version) {
	case "latest", "current", "main", "head":
		return false
	}
	switch identity.Target {
	case "linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64":
		return true
	default:
		return false
	}
}

type Asset struct {
	Path   string `json:"path"`
	Mode   string `json:"mode"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

type Baseline struct {
	Label    string  `json:"label"`
	Revision string  `json:"revision"`
	Assets   []Asset `json:"assets"`
}

// Manifest carries reference evidence; ActivationReady must always be false.
// docs/adr/0097-installation-source-image-bundles.md:65.
type Manifest struct {
	SchemaVersion   int        `json:"schema_version"`
	Mode            string     `json:"mode"`
	Scope           string     `json:"scope"`
	Version         string     `json:"version"`
	Target          string     `json:"target"`
	SourceRevision  string     `json:"source_revision"`
	ActivationReady bool       `json:"activation_ready"`
	Assets          []Asset    `json:"assets"`
	Baselines       []Baseline `json:"baselines"`
}

type Reference struct {
	Label, Revision, Digest string
	Count                   int
}

// References returns independent pins, never mutable imported catalog authority.
// docs/adr/0097-installation-source-image-bundles.md:101.
func References() []Reference {
	return []Reference{
		{"v0.1.6", "b71ecc32e07ecd87eb330ba8e497c86612f92acd", "595f75dddcda285ba3b43a0936970ef96af3b9eae66f4beb9fd3a37df5d7e16b", 137},
		{"bash-baseline", "76952eaa63aebd1ecd282f5ab51dd7c3627cb497", "8514055a8ee6f79a8fd891a6107e46bd9d0f1a2096e349825db83aafc7b91bbe", 160},
	}
}

// Decode reuses the lossless duplicate-rejecting JSON parser, then checks the
// closed scalar shape before typed conversion. Lone surrogates remain detectable.
// docs/adr/0097-installation-source-image-bundles.md:70.
func Decode(ctx context.Context, data []byte, identity Identity) (Manifest, error) {
	if len(data) > ManifestLimit || !utf8.Valid(data) || !json.Valid(data) {
		return Manifest{}, errors.New("invalid or oversized installation manifest")
	}
	value, err := jsonvalue.DecodeUnique(ctx, data)
	if err != nil {
		return Manifest{}, err
	}
	if err = closedShape(ctx, value, reflect.TypeFor[Manifest]()); err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	if err = json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, err
	}
	if err = Validate(ctx, manifest, identity); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func closedShape(ctx context.Context, value any, shape reflect.Type) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	valid := false
	switch shape.Kind() {
	case reflect.Struct:
		object, ok := value.(map[string]any)
		if !ok || len(object) != shape.NumField() {
			break
		}
		for i := 0; i < shape.NumField(); i++ {
			field := shape.Field(i)
			child, present := object[field.Tag.Get("json")]
			if !present {
				return errors.New("installation manifest is missing a required field")
			}
			if err := closedShape(ctx, child, field.Type); err != nil {
				return err
			}
		}
		valid = true
	case reflect.Slice:
		array, ok := value.([]any)
		if !ok {
			break
		}
		for _, child := range array {
			if err := closedShape(ctx, child, shape.Elem()); err != nil {
				return err
			}
		}
		valid = true
	case reflect.String:
		text, ok := value.(string)
		valid = ok && utf8.ValidString(text)
	case reflect.Int, reflect.Int64:
		_, valid = value.(json.Number)
	case reflect.Bool:
		_, valid = value.(bool)
	}
	if !valid {
		return errors.New("installation manifest has an unknown field, invalid type or Unicode")
	}
	return nil
}

func Encode(ctx context.Context, manifest Manifest, identity Identity) ([]byte, error) {
	if err := Validate(ctx, manifest, identity); err != nil {
		return nil, err
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}
	if len(data)+1 > ManifestLimit {
		return nil, errors.New("installation manifest exceeds limit")
	}
	return append(data, '\n'), ctx.Err()
}

func Validate(ctx context.Context, manifest Manifest, identity Identity) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validIdentity(identity) {
		return errors.New("invalid requested installation image identity")
	}
	if manifest.SchemaVersion != 1 || manifest.Mode != "installation_source_image" ||
		manifest.Scope != "committed_source_and_baseline_references" || manifest.ActivationReady ||
		manifest.Version != identity.Version || manifest.Target != identity.Target || manifest.SourceRevision != identity.Revision {
		return errors.New("installation manifest does not match the requested non-activating identity")
	}
	if err := ValidateCatalog(ctx, manifest.Assets); err != nil {
		return err
	}
	references := References()
	if len(manifest.Baselines) != len(references) {
		return errors.New("installation manifest requires both baseline catalogs")
	}
	for i, reference := range references {
		baseline := manifest.Baselines[i]
		if baseline.Label != reference.Label || baseline.Revision != reference.Revision || len(baseline.Assets) != reference.Count {
			return errors.New("installation baseline identity or count does not match its independent pin")
		}
		if err := ValidateCatalog(ctx, baseline.Assets); err != nil {
			return err
		}
		digest, err := CatalogDigest(ctx, baseline.Assets)
		if err != nil {
			return err
		}
		if digest != reference.Digest {
			return errors.New("installation baseline catalog does not match its independent digest")
		}
	}
	return nil
}

// ValidatePath applies the same source-path policy in collection and staging.
// docs/adr/0097-installation-source-image-bundles.md:78.
func ValidatePath(name string) error {
	if name == "" || len(name) > PathLimit || !utf8.ValidString(name) || strings.ContainsAny(name, "\\\x00\r\n\t") ||
		name == ".factory" || strings.HasPrefix(name, ".factory/") {
		return fmt.Errorf("unsafe installation asset path %q", name)
	}
	for _, component := range strings.Split(name, "/") {
		if component == "" || component == "." || component == ".." {
			return fmt.Errorf("noncanonical installation asset path %q", name)
		}
	}
	return nil
}

func ValidateCatalog(ctx context.Context, assets []Asset) error {
	if len(assets) == 0 || len(assets) > AssetCountLimit {
		return errors.New("installation asset count exceeds limit or is empty")
	}
	seen := make(map[string]bool, len(assets))
	previous, total := "", int64(0)
	for _, asset := range assets {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := ValidatePath(asset.Path); err != nil {
			return err
		}
		if asset.Path <= previous {
			return errors.New("installation assets must be sorted and unique")
		}
		for prefix, rest := "", asset.Path; strings.Contains(rest, "/"); {
			component, suffix, _ := strings.Cut(rest, "/")
			prefix += component
			if seen[prefix] {
				return errors.New("installation asset paths contain an ancestor collision")
			}
			prefix += "/"
			rest = suffix
		}
		if asset.Mode != "100644" && asset.Mode != "100755" && asset.Mode != "120000" {
			return errors.New("unsupported installation Git mode")
		}
		if len(asset.SHA256) != 64 || strings.Trim(asset.SHA256, "0123456789abcdef") != "" || asset.Bytes < 0 || asset.Bytes > AssetLimit {
			return errors.New("invalid installation asset digest or byte limit")
		}
		total += asset.Bytes
		if total > PayloadLimit {
			return errors.New("installation payload exceeds total byte limit")
		}
		seen[asset.Path], previous = true, asset.Path
	}
	return nil
}

func CatalogDigest(ctx context.Context, assets []Asset) (string, error) {
	hash := sha256.New()
	_, _ = io.WriteString(hash, "FACTORY_INSTALLATION_CATALOG_V1\n")
	for _, asset := range assets {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		for _, value := range []string{asset.Path, asset.Mode, asset.SHA256, strconv.FormatInt(asset.Bytes, 10)} {
			_, _ = io.WriteString(hash, value+"\x00")
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
