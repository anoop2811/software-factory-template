package installedproof

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"

	"github.com/anoop2811/software-factory-template/internal/initinventory"
)

// Seed only this private copy-gate fixture, before its baseline Git commit.
// Shared executable controls and the captured factory wrapper are preserved.
// docs/adr/0125-native-initializer-copy-inventory-gate.md:89.
func (p *proof) prepareCopyInventory(ctx context.Context) error {
	for _, path := range append(initinventory.Required(), initinventory.Readme) {
		_, err := os.Lstat(filepath.Join(p.root, filepath.FromSlash(path)))
		if errors.Is(err, os.ErrNotExist) {
			if err := p.write(ctx, path, "qualified copy input\n", 0600); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}
	for _, group := range initinventory.Groups() {
		if err := p.write(ctx, group.Directory+"/copy-proof"+group.Suffix, "qualified group input\n", 0600); err != nil {
			return err
		}
	}
	return nil
}

func (p *proof) nativeCopyPair(ctx context.Context, name string) error {
	const input = ".opencode/package.json"
	if !slices.Contains(initinventory.Required(), input) {
		return errors.New("copy proof input is no longer a required initializer input")
	}
	if err := p.git(ctx, "--literal-pathspecs", "rm", "--cached", "--force", "--", input); err != nil {
		return err
	}
	if err := p.hook(ctx, name, 1); err != nil {
		return err
	}
	if err := p.git(ctx, "--literal-pathspecs", "add", "--force", "--", input); err != nil {
		return err
	}
	return p.hook(ctx, name, 0)
}
