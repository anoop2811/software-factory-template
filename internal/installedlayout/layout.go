// Package installedlayout validates fixed installed runtime custody before admission.
package installedlayout

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/anoop2811/software-factory-template/internal/installationfs"
	"github.com/anoop2811/software-factory-template/internal/jsonvalue"
	"github.com/anoop2811/software-factory-template/internal/transition"
	"golang.org/x/sys/unix"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

type Asset struct {
	Path   string `json:"path"`
	Mode   uint32 `json:"mode"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}
type Descriptor struct {
	SchemaVersion int     `json:"schema_version"`
	Kind          string  `json:"kind"`
	Version       string  `json:"version"`
	Revision      string  `json:"revision"`
	Target        string  `json:"target"`
	RuntimeSHA256 string  `json:"runtime_sha256"`
	Assets        []Asset `json:"assets"`
}
type Context struct {
	Root, Assets string
	Descriptor   Descriptor
	lease        *transition.RootLease
	tree         *installationfs.Tree
	closed       bool
}

// PhysicalRoot identifies only the declared executable location, never an environment locator.
// docs/adr/0098-whole-installation-upgrade-and-rollback.md:379.
func PhysicalRoot(ctx context.Context, executable string) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	if filepath.Base(executable) != "factory-runtime" || filepath.Base(filepath.Dir(executable)) != "bin" || filepath.Base(filepath.Dir(filepath.Dir(executable))) != ".factory" {
		return "", false, nil
	}
	path, err := filepath.Abs(filepath.Dir(filepath.Dir(filepath.Dir(executable))))
	if err != nil {
		return "", true, err
	}
	physical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", true, err
	}
	if physical != path {
		return "", true, errors.New("installed runtime physical path contains links")
	}
	return path, true, nil
}
func Open(ctx context.Context, root string, expected []string) (result *Context, returned error) {
	lease, err := transition.ReadOnly(ctx, root)
	if err != nil {
		return nil, err
	}
	owned := &Context{Root: root, Assets: filepath.Join(root, ".factory/assets/scaffold"), lease: lease}
	defer func() {
		if result == nil {
			returned = errors.Join(returned, owned.Close(context.WithoutCancel(ctx)))
		}
	}()
	owned.tree, err = installationfs.Open(ctx, root)
	if err != nil {
		return nil, err
	}
	owned.Descriptor, err = Validate(ctx, owned.tree, expected)
	if err != nil {
		return nil, err
	}
	if err := lease.Check(ctx); err != nil {
		return nil, err
	}
	return owned, nil
}
func Decode(ctx context.Context, data []byte) (Descriptor, error) {
	if len(data) > 2<<20 || !utf8.Valid(data) {
		return Descriptor{}, errors.New("invalid installed descriptor")
	}
	value, err := jsonvalue.DecodeUnique(ctx, data)
	if err != nil {
		return Descriptor{}, err
	}
	object, ok := value.(map[string]any)
	if !ok || len(object) != 7 {
		return Descriptor{}, errors.New("invalid installed descriptor fields")
	}
	for _, key := range []string{"schema_version", "kind", "version", "revision", "target", "runtime_sha256", "assets"} {
		if _, ok := object[key]; !ok {
			return Descriptor{}, errors.New("incomplete installed descriptor")
		}
	}
	var descriptor Descriptor
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&descriptor); err != nil {
		return descriptor, err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return descriptor, errors.New("trailing installed descriptor")
	}
	if descriptor.SchemaVersion != 1 || descriptor.Kind != "go-hybrid-v1" || len(descriptor.Assets) == 0 || len(descriptor.Assets) > 512 {
		return descriptor, errors.New("unsupported installed descriptor")
	}
	previous := ""
	sort.SliceIsSorted(descriptor.Assets, func(i, j int) bool { return descriptor.Assets[i].Path < descriptor.Assets[j].Path })
	for _, asset := range descriptor.Assets {
		if asset.Path <= previous || asset.Path == ".factory/installation.current" || asset.Path == ".factory-version" || len(asset.SHA256) != 64 || strings.Trim(asset.SHA256, "0123456789abcdef") != "" || asset.Bytes < 0 || asset.Bytes > 256<<20 {
			return descriptor, errors.New("invalid installed selection")
		}
		previous = asset.Path
	}
	return descriptor, nil
}
func Validate(ctx context.Context, tree *installationfs.Tree, expected []string) (Descriptor, error) {
	return validate(ctx, tree, expected, true)
}

// ValidateAssets is the bounded in-process preparation boundary; it grants no admission.
// docs/adr/0098-whole-installation-upgrade-and-rollback.md:203.
func ValidateAssets(ctx context.Context, tree *installationfs.Tree, expected []string) (Descriptor, error) {
	return validate(ctx, tree, expected, false)
}
func validate(ctx context.Context, tree *installationfs.Tree, expected []string, checkVersion bool) (Descriptor, error) {
	file, err := tree.OpenFile(ctx, ".factory/installation.current", 2<<20)
	if err != nil {
		return Descriptor{}, err
	}
	_, data, readErr := file.Digest(ctx, 2<<20)
	mode := file.Metadata().Mode
	closeErr := file.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return Descriptor{}, err
	}
	if mode != unix.S_IFREG|0600 {
		return Descriptor{}, errors.New("unsafe installed descriptor mode")
	}
	descriptor, err := Decode(ctx, data)
	if err != nil {
		return descriptor, err
	}
	if len(expected) != 0 && (len(expected) != 3 || expected[0] != descriptor.Version || expected[1] != descriptor.Revision || expected[2] != descriptor.RuntimeSHA256) {
		return descriptor, errors.New("installed launcher identity mismatch")
	}
	if checkVersion {
		versionFile, err := tree.OpenFile(ctx, ".factory-version", 1<<20)
		if err != nil {
			return descriptor, err
		}
		_, versionData, versionErr := versionFile.Digest(ctx, 1<<20)
		if err := errors.Join(versionErr, versionFile.Close()); err != nil {
			return descriptor, err
		}
		if string(versionData) != "ref="+descriptor.Version+"\ncommit="+descriptor.Revision+"\n" {
			return descriptor, errors.New("installed version metadata mismatch")
		}
	}
	binary := false
	manifest := false
	source := false
	var total int64
	for _, asset := range descriptor.Assets {
		limit := int64(1 << 20)
		if asset.Path == ".factory/bin/factory-runtime" {
			limit = 256 << 20
			if asset.SHA256 != descriptor.RuntimeSHA256 || asset.Mode != unix.S_IFREG|0700 {
				return descriptor, errors.New("installed binary identity mismatch")
			}
			binary = true
		} else {
			total += asset.Bytes
		}
		manifest = manifest || asset.Path == ".factory/bin/runtime.manifest"
		source = source || asset.Path == ".factory/bin/source.json"
		if total > 64<<20 || asset.Bytes > limit {
			return descriptor, errors.New("installed selection exceeds bounds")
		}
		file, err := tree.OpenFile(ctx, asset.Path, limit)
		if err != nil {
			return descriptor, err
		}
		actual, data, readErr := file.Digest(ctx, limit)
		metadata := file.Metadata()
		closeErr := file.Close()
		_ = data
		if err := errors.Join(readErr, closeErr); err != nil {
			return descriptor, err
		}
		if actual != asset.SHA256 || metadata.Mode != asset.Mode || metadata.Size != asset.Bytes {
			return descriptor, fmt.Errorf("installed selection identity changed")
		}
	}
	if !binary || !manifest || !source {
		return descriptor, errors.New("installed controls are incomplete")
	}
	return descriptor, tree.Check(ctx)
}
func (owned *Context) Check(ctx context.Context) error {
	if owned == nil || owned.closed {
		return errors.New("installed context is closed")
	}
	if err := owned.lease.Check(ctx); err != nil {
		return err
	}
	return owned.tree.Check(ctx)
}
func (owned *Context) Close(ctx context.Context) error {
	if owned == nil || owned.closed {
		return errors.New("installed context is closed")
	}
	owned.closed = true
	var errs []error
	if owned.tree != nil {
		errs = append(errs, owned.tree.Close())
	}
	if owned.lease != nil {
		errs = append(errs, owned.lease.Close(ctx))
	}
	return errors.Join(errs...)
}
