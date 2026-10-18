package installation

import (
	"context"
	"errors"
	"strings"

	"github.com/anoop2811/software-factory-template/internal/installationfs"
)

// RecoveryPreview returns only inspection operands derived from checked state.
// It is advice, never mutation authority. docs/adr/0101-go-release-handoff.md:179.
func RecoveryPreview(ctx context.Context, root, id string) (args []string, returned error) {
	if id == "" || len(id) > 64 || strings.Trim(id, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-") != "" {
		return nil, conflict("invalid recovery identifier")
	}
	tree, err := installationfs.Open(ctx, root)
	if err != nil {
		return nil, err
	}
	defer func() { returned = errors.Join(returned, tree.Close()) }()
	record, err := readJournalEvidence(ctx, tree, id, true)
	if err != nil {
		return nil, err
	}
	pending, _, err := observe(ctx, tree, ".factory/runtime-publication.pending", 0)
	if err != nil {
		return nil, err
	}
	if record.Phase == "committed" && pending == nil {
		return []string{"--rollback", id, "--dry-run"}, nil
	}
	direction := "reverse"
	if record.Phase == "committed" {
		direction = record.Direction
	}
	return []string{"--recover", id, "--direction", direction, "--dry-run"}, nil
}
