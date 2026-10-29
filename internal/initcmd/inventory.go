package initcmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/anoop2811/software-factory-template/internal/native"
)

// Inventory checks the canonical initializer inputs, not source-code spelling.
// Its source-only Git check grants no initialization or execution authority.
// docs/adr/0108-default-go-cutover.md:95.
func Inventory(ctx context.Context, root string) (count int, returned error) {
	path, err := physicalPath(root)
	if err != nil {
		return 0, err
	}
	source, err := openTree(path)
	if err != nil {
		return 0, err
	}
	defer func() { returned = errors.Join(returned, source.root.Close()) }()
	paths := append([]string{}, requiredFiles...)
	paths = append(paths, freshFiles...)
	for _, group := range requiredGroups {
		entries, err := source.list(ctx, group[0])
		if err != nil {
			return 0, fmt.Errorf("%s: cannot enumerate canonical initializer group", group[0])
		}
		found := 0
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), group[1]) {
				paths = append(paths, group[0]+"/"+entry.Name())
				found++
			}
		}
		if found == 0 {
			return 0, fmt.Errorf("%s: canonical initializer group is empty", group[0])
		}
	}
	environment := map[string]string{"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.DevNull}
	for _, entry := range os.Environ() {
		key, value, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(key, "GIT_") {
			environment[key] = value
		}
	}
	result, err := native.ExecuteCommandBounded(ctx, path, []string{"git", "ls-files", "-z"}, environment, 16<<20, 64<<10)
	if err != nil || result.ExitCode == nil || *result.ExitCode != 0 || !result.ExitConfirmed || result.OwnershipUnconfirmed {
		return 0, errors.Join(err, errors.New("cannot enumerate Git-tracked initializer inputs"))
	}
	tracked := map[string]bool{}
	for _, name := range strings.Split(string(result.Stdout), "\x00") {
		tracked[name] = true
	}
	seen := map[string]bool{}
	for _, name := range paths {
		if seen[name] {
			continue
		}
		seen[name] = true
		if _, _, exists, err := source.read(ctx, name); err != nil || !exists {
			return 0, fmt.Errorf("%s: canonical initializer asset is missing or unsafe", name)
		}
		if !tracked[name] {
			return 0, fmt.Errorf("%s: canonical initializer asset is not Git-tracked", name)
		}
		count++
	}
	return count, ctx.Err()
}

// InventoryCommand bounds source inspection independently of a caller's deadline.
func InventoryCommand(ctx context.Context, root string) (int, error) {
	child, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return Inventory(child, root)
}
