package bridge

import "context"

type installedRootKey struct{}

// RunInstalled reuses main's admitted physical root and retained installation
// lease; protocol/environment strings cannot grant this context capability.
// docs/adr/0125-native-initializer-copy-inventory-gate.md:135.
func RunInstalled(ctx context.Context, args []string, root string) int {
	return Run(context.WithValue(ctx, installedRootKey{}, root), args)
}
