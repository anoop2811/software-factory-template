package installation

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/anoop2811/software-factory-template/internal/installationfs"
	"github.com/anoop2811/software-factory-template/internal/staging"
)

func markerHint(ctx context.Context, tree *installationfs.Tree, snapshot retentionSnapshot) (ReleaseIdentity, bool) {
	if validRetentionIdentity(snapshot.BeforeHint) {
		return snapshot.BeforeHint, true
	}
	if snapshot.Receipt != nil && snapshot.Receipt.Profile == "go-hybrid-v1" && validRetentionIdentity(snapshot.Receipt.Before) {
		return snapshot.Receipt.Before, true
	}
	_, data, err := observe(ctx, tree, backupBase(snapshot.ID)+"/assets/.factory-version", 4096)
	if err != nil {
		return ReleaseIdentity{}, false
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) != 3 || lines[2] != "" || !strings.HasPrefix(lines[0], "ref=") || !strings.HasPrefix(lines[1], "commit=") {
		return ReleaseIdentity{}, false
	}
	hint := ReleaseIdentity{strings.TrimPrefix(lines[0], "ref="), strings.TrimPrefix(lines[1], "commit="), snapshot.Record.Target}
	return hint, validRetentionIdentity(hint)
}
func afterHint(snapshot retentionSnapshot, hints []ReleaseIdentity) (ReleaseIdentity, bool) {
	entry, err := matchingManifestEntry(snapshot.Manifest, ".factory-version")
	if err != nil {
		return ReleaseIdentity{}, false
	}
	if validRetentionIdentity(snapshot.AfterHint) {
		hint := snapshot.AfterHint
		expected := bytesImage([]byte("ref="+hint.Version+"\ncommit="+hint.Revision+"\n"), 0644)
		return hint, imageEqual(expected, entry.ExpectedAfter, false)
	}
	if snapshot.Receipt != nil {
		hint := snapshot.Receipt.After
		expected := bytesImage([]byte("ref="+hint.Version+"\ncommit="+hint.Revision+"\n"), 0644)
		return hint, validRetentionIdentity(hint) && imageEqual(expected, entry.ExpectedAfter, false)
	}
	for _, hint := range hints {
		if hint.Target != snapshot.Record.Target {
			continue
		}
		expected := bytesImage([]byte("ref="+hint.Version+"\ncommit="+hint.Revision+"\n"), 0644)
		if imageEqual(expected, entry.ExpectedAfter, false) {
			return hint, true
		}
	}
	return ReleaseIdentity{}, false
}
func canonicalSelection(ctx context.Context, image staging.Inspection, inputs map[string]string) (map[string]*Image, error) {
	rows, err := Selection(ctx, image, inputs)
	if err != nil {
		return nil, err
	}
	result := map[string]*Image{}
	for _, row := range rows {
		expected := bytesImage(row.Data, uint32(row.Mode))
		if row.Runtime {
			expected.SHA256, expected.Bytes = image.Runtime.SHA256, image.BinaryBytes
		}
		result[row.Path] = expected
	}
	return result, nil
}

// Fresh projections, not serialized action labels, establish the complete delta.
// docs/adr/0106-go-backup-retention.md:48.
func qualifyRetentionSet(ctx context.Context, tree *installationfs.Tree, snapshot retentionSnapshot, hints []ReleaseIdentity, fallbackInputs map[string]string, images *retentionContext, authenticate, completePredecessor bool) error {
	after, ok := afterHint(snapshot, hints)
	if !ok {
		return conflict("backup target release cannot be independently identified")
	}
	afterImage, err := images.image(ctx, after, authenticate)
	if err != nil {
		return err
	}
	inputs := fallbackInputs
	if snapshot.Receipt != nil {
		inputs = snapshot.Receipt.Inputs
	}
	for key, value := range inputs {
		switch key {
		case "PROJECT_NAME", "PROJECT_SLUG", "DOCS_ROOT", "PROTECTED_PATH", "GITHUB_OWNER", "OPENCODE_USERNAME", "MUTATION_TARGET", "CITATION_PREFIX":
		default:
			return conflict("backup project inputs are unrecognized")
		}
		if len(value) > 64<<10 || !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
			return conflict("backup project inputs are unsafe")
		}
	}
	request := Request{Profile: snapshot.Record.Profile, Source: images.options[after], Inputs: inputs}
	if request.Profile == "go-hybrid-v1" {
		before, found := markerHint(ctx, tree, snapshot)
		if !found {
			return conflict("backup predecessor release cannot be independently identified")
		}
		if _, err = images.image(ctx, before, authenticate); err != nil {
			return err
		}
		request.Before = images.options[before]
	}
	baseline, err := predecessor(ctx, request, afterImage)
	if err != nil {
		return conflict("backup generation recipe is unsupported")
	}
	targets, err := canonicalSelection(ctx, afterImage, inputs)
	if err != nil {
		return err
	}
	for _, row := range roles {
		original, err := matchingManifestEntry(snapshot.Manifest, row.Path)
		if err != nil {
			return err
		}
		expectedBefore, _, known, err := baseline.expected(ctx, row.Path, inputs)
		if err != nil {
			return err
		}
		if original.Before != nil {
			_, matched, err := baseline.matching(ctx, row.Path, original.Before, inputs)
			if err != nil {
				return err
			}
			if !matched {
				return conflict("backup contains an unqualified predecessor selection")
			}
		} else if completePredecessor && known && expectedBefore != nil {
			return conflict("alternate recovery omits a predecessor asset")
		}
		expectedAfter := targets[row.Path]
		if !imageEqual(expectedAfter, original.ExpectedAfter, false) {
			return conflict("backup target delta disagrees with freshly qualified images")
		}
		action := "retain"
		switch {
		case original.Before != nil && expectedAfter == nil:
			action = "retire"
		case original.Before != nil && expectedAfter != nil:
			action = "replace"
		case original.Before == nil && expectedAfter != nil:
			action = "create"
		}
		if original.Action != action {
			return conflict("backup action labels omit a freshly derived change")
		}
		if original.Before != nil {
			saved := snapshot.Files["assets/"+row.Path]
			if saved == nil || saved.SHA256 != original.Before.SHA256 || saved.Bytes != original.Before.Bytes {
				return conflict("backup omits a qualified saved predecessor asset")
			}
		}
	}
	return tree.Check(ctx)
}

func qualifyCurrentRetention(ctx context.Context, tree *installationfs.Tree, current staging.Inspection, inputs map[string]string) error {
	selected, err := canonicalSelection(ctx, current, inputs)
	if err != nil {
		return err
	}
	for path, expected := range selected {
		actual, _, err := observe(ctx, tree, path, leafLimit(path))
		if err != nil {
			return err
		}
		if !imageEqual(actual, expected, false) {
			return conflict("current installation is not the freshly qualified complete selection")
		}
	}
	_, err = inspectState(ctx, tree)
	return errors.Join(err, tree.Check(ctx))
}
