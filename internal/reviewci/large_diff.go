package reviewci

import (
	"bytes"
	"context"
	"crypto/sha1" // #nosec G505 -- Existing Git object format identity, not authentication; docs/adr/0109-complete-large-review-inputs.md:60.
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/anoop2811/software-factory-template/internal/reviewbatch"
)

type apiStatusError struct{ code int }

func (e *apiStatusError) Error() string { return fmt.Sprintf("GitHub HTTP %d", e.code) }

type patchFile struct {
	Filename  string  `json:"filename"`
	Previous  string  `json:"previous_filename"`
	Status    string  `json:"status"`
	SHA       string  `json:"sha"`
	Patch     *string `json:"patch"`
	Additions *int    `json:"additions"`
	Deletions *int    `json:"deletions"`
}

type treeEntry struct {
	Path string `json:"path"`
	Mode string `json:"mode"`
	Type string `json:"type"`
	SHA  string `json:"sha"`
	Size *int64 `json:"size"`
}

// Only GitHub's explicit raw-diff refusal can select a second acquisition path.
// docs/adr/0109-complete-large-review-inputs.md:25.
func (c client) completeDiff(ctx context.Context, o options, manifest []byte) ([]byte, error) {
	diff, err := c.api(ctx, http.MethodGet, "/pulls/"+strconv.Itoa(o.pr), "application/vnd.github.v3.diff", nil)
	var status *apiStatusError
	if !errors.As(err, &status) || status.code != http.StatusNotAcceptable {
		return diff, err
	}
	return c.largeDiff(ctx, o, manifest)
}

func (c client) largeDiff(ctx context.Context, o options, manifest []byte) ([]byte, error) {
	var files []patchFile
	if json.Unmarshal(manifest, &files) != nil || len(files) == 0 || len(files) > 3000 || len(manifest) > apiLimit {
		return nil, errors.New("invalid large-diff manifest")
	}
	seen := make(map[string]bool, len(files))
	for _, file := range files {
		if !reviewPath(file.Filename) || seen[file.Filename] || !revisionPattern.MatchString(file.SHA) ||
			file.Additions == nil || file.Deletions == nil || *file.Additions < 0 || *file.Deletions < 0 || *file.Additions > apiLimit || *file.Deletions > apiLimit {
			return nil, errors.New("invalid or duplicate large-diff file")
		}
		seen[file.Filename] = true
	}
	data, err := c.api(ctx, http.MethodGet, "/compare/"+o.base+"..."+o.head+"?per_page=1&page=1", "application/vnd.github+json", nil)
	if err != nil {
		return nil, err
	}
	var comparison struct {
		Base struct {
			SHA string `json:"sha"`
		} `json:"base_commit"`
		Merge struct {
			SHA string `json:"sha"`
		} `json:"merge_base_commit"`
	}
	if json.Unmarshal(data, &comparison) != nil || comparison.Base.SHA != o.base || !revisionPattern.MatchString(comparison.Merge.SHA) {
		return nil, errors.New("invalid review merge-base identity")
	}
	before, err := c.commitTree(ctx, comparison.Merge.SHA)
	if err != nil {
		return nil, err
	}
	after, err := c.commitTree(ctx, o.head)
	if err != nil {
		return nil, err
	}
	if err := reconcileReviewTrees(ctx, files, before, after); err != nil {
		return nil, err
	}
	var diff strings.Builder
	oldBytes := 0
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		old, current, err := file.trees(before, after)
		if err != nil {
			return nil, err
		}
		if err := c.provePatch(ctx, file, old, current, &oldBytes); err != nil {
			return nil, err
		}
		if err := appendFileDiff(&diff, apiLimit-len(manifest), file, old, current); err != nil {
			return nil, err
		}
	}
	return []byte(diff.String()), nil
}

// A copy reads its source; it cannot explain lost old destination/source bytes.
// Reconcile both sides, including paths outside the file manifest.
// docs/adr/0109-complete-large-review-inputs.md:94.
func reconcileReviewTrees(ctx context.Context, files []patchFile, before, after map[string]treeEntry) error {
	oldWrites, newWrites := make(map[string]bool, len(files)), make(map[string]bool, len(files))
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !reviewTreeDelta(before[file.Filename], after[file.Filename]) {
			return errors.New("review manifest destination is unchanged in complete trees")
		}
		switch file.Status {
		case "added", "copied":
			newWrites[file.Filename] = true
		case "removed":
			oldWrites[file.Filename] = true
		case "modified", "changed":
			oldWrites[file.Filename], newWrites[file.Filename] = true, true
		case "renamed":
			oldWrites[file.Previous], newWrites[file.Filename] = true, true
		default:
			return errors.New("unsupported review file lifecycle")
		}
	}
	for _, file := range files {
		if source := after[file.Previous]; file.Status == "renamed" && source.Path != "" && source.Type != "tree" && !newWrites[file.Previous] {
			return errors.New("retained rename source lacks an explicit manifest row")
		}
	}
	for _, tree := range []map[string]treeEntry{before, after} {
		for name := range tree {
			if err := ctx.Err(); err != nil {
				return err
			}
			old, current := before[name], after[name]
			if !reviewTreeDelta(old, current) {
				continue
			}
			if old.Path != "" && old.Type != "tree" && !oldWrites[name] || current.Path != "" && current.Type != "tree" && !newWrites[name] {
				return errors.New("complete review trees contain unexplained old or new file changes")
			}
		}
	}
	return nil
}

func reviewTreeDelta(old, current treeEntry) bool {
	if old.Type == "tree" {
		old = treeEntry{}
	}
	if current.Type == "tree" {
		current = treeEntry{}
	}
	return old.Path != current.Path || old.Mode != current.Mode || old.Type != current.Type || old.SHA != current.SHA ||
		old.Size != nil && current.Size != nil && *old.Size != *current.Size
}

func (c client) commitTree(ctx context.Context, revision string) (map[string]treeEntry, error) {
	data, err := c.api(ctx, http.MethodGet, "/git/commits/"+revision, "application/vnd.github+json", nil)
	if err != nil {
		return nil, err
	}
	var commit struct {
		SHA  string `json:"sha"`
		Tree struct {
			SHA string `json:"sha"`
		} `json:"tree"`
	}
	if json.Unmarshal(data, &commit) != nil || commit.SHA != revision || !revisionPattern.MatchString(commit.Tree.SHA) {
		return nil, errors.New("invalid review commit-tree identity")
	}
	data, err = c.api(ctx, http.MethodGet, "/git/trees/"+commit.Tree.SHA+"?recursive=1", "application/vnd.github+json", nil)
	if err != nil {
		return nil, err
	}
	var tree struct {
		SHA       string      `json:"sha"`
		Truncated *bool       `json:"truncated"`
		Entries   []treeEntry `json:"tree"`
	}
	if json.Unmarshal(data, &tree) != nil || tree.SHA != commit.Tree.SHA || tree.Truncated == nil || *tree.Truncated || tree.Entries == nil || len(tree.Entries) > 100000 {
		return nil, errors.New("missing, truncated or invalid review tree")
	}
	entries := make(map[string]treeEntry, len(tree.Entries))
	for _, entry := range tree.Entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !reviewPath(entry.Path) || entries[entry.Path].Path != "" || !revisionPattern.MatchString(entry.SHA) {
			return nil, errors.New("invalid or duplicate review tree entry")
		}
		valid := false
		switch entry.Mode {
		case "100644", "100755", "120000":
			valid = entry.Type == "blob" && entry.Size != nil && *entry.Size >= 0
		case "040000":
			valid = entry.Type == "tree"
		case "160000":
			valid = entry.Type == "commit"
		}
		if !valid {
			return nil, errors.New("invalid review tree mode or type")
		}
		entries[entry.Path] = entry
	}
	return entries, nil
}

func reviewPath(value string) bool {
	if value == "" || !utf8.ValidString(value) || path.IsAbs(value) || path.Clean(value) != value || value == "." || value == ".." || strings.HasPrefix(value, "../") {
		return false
	}
	for _, r := range value {
		if r < 32 || r == 127 {
			return false
		}
	}
	return true
}

func (file patchFile) trees(before, after map[string]treeEntry) (treeEntry, treeEntry, error) {
	oldName := file.Filename
	switch file.Status {
	case "added", "removed", "modified", "changed":
		if file.Previous != "" {
			return treeEntry{}, treeEntry{}, errors.New("unexpected previous review filename")
		}
	case "renamed", "copied":
		if !reviewPath(file.Previous) || file.Previous == file.Filename {
			return treeEntry{}, treeEntry{}, errors.New("invalid previous review filename")
		}
		oldName = file.Previous
	default:
		return treeEntry{}, treeEntry{}, errors.New("unsupported review file lifecycle")
	}
	old, oldOK := before[oldName]
	current, currentOK := after[file.Filename]
	if (file.Status == "added") != !oldOK || (file.Status == "removed") != !currentOK {
		return old, current, errors.New("review tree and manifest lifecycle disagree")
	}
	for _, entry := range []treeEntry{old, current} {
		if entry.Path != "" && (entry.Type != "blob" || (entry.Mode != "100644" && entry.Mode != "100755")) {
			return old, current, errors.New("unsupported nonregular review change")
		}
	}
	expected := current.SHA
	if !currentOK {
		expected = old.SHA
	}
	if file.SHA != expected || file.Status == "added" && *file.Deletions != 0 || file.Status == "removed" && *file.Additions != 0 {
		return old, current, errors.New("review blob and manifest identity disagree")
	}
	if oldOK && currentOK && old.SHA == current.SHA && *old.Size != *current.Size {
		return old, current, errors.New("equal review blobs have contradictory sizes")
	}
	if file.Previous == "" && oldOK && currentOK && old.SHA == current.SHA && old.Mode == current.Mode {
		return old, current, errors.New("review manifest declares an unchanged file")
	}
	return old, current, nil
}

func (c client) provePatch(ctx context.Context, file patchFile, old, current treeEntry, oldBytes *int) error {
	empty := gitBlobID(nil)
	if file.Patch == nil || *file.Patch == "" {
		unchanged := old.Path != "" && current.Path != "" && old.SHA == current.SHA
		emptyAdded := old.Path == "" && current.SHA == empty && *current.Size == 0
		emptyRemoved := current.Path == "" && old.SHA == empty && *old.Size == 0
		if *file.Additions == 0 && *file.Deletions == 0 && (unchanged || emptyAdded || emptyRemoved) {
			return nil
		}
		return errors.New("missing complete text patch; content may be binary or truncated")
	}
	var before []byte
	if old.Path != "" {
		var err error
		before, err = c.oldBlob(ctx, old, oldBytes)
		if err != nil {
			return err
		}
	}
	result, err := reviewbatch.ApplyFilePatch(ctx, before, []byte(*file.Patch), *file.Additions, *file.Deletions)
	if err != nil {
		return fmt.Errorf("cannot bind review patch to old blob: %w", err)
	}
	expected := current.SHA
	if current.Path == "" {
		expected = empty
	}
	if gitBlobID(result) != expected || current.Path != "" && int64(len(result)) != *current.Size || current.Path == "" && len(result) != 0 {
		return errors.New("review patch result disagrees with exact head blob")
	}
	return nil
}

func (c client) oldBlob(ctx context.Context, entry treeEntry, used *int) ([]byte, error) {
	if *entry.Size > int64(apiLimit-*used) {
		return nil, errors.New("aggregate review source blobs oversized")
	}
	data, err := c.api(ctx, http.MethodGet, "/git/blobs/"+entry.SHA, "application/vnd.github+json", nil)
	if err != nil {
		return nil, err
	}
	var blob struct {
		SHA      string `json:"sha"`
		Size     *int64 `json:"size"`
		Encoding string `json:"encoding"`
		Content  string `json:"content"`
	}
	if json.Unmarshal(data, &blob) != nil || blob.SHA != entry.SHA || blob.Size == nil || *blob.Size != *entry.Size || blob.Encoding != "base64" {
		return nil, errors.New("invalid review source blob identity or encoding")
	}
	content, err := base64.StdEncoding.Strict().DecodeString(blob.Content)
	if err != nil || int64(len(content)) != *blob.Size || len(content) > apiLimit-*used || !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0 || gitBlobID(content) != entry.SHA {
		return nil, errors.New("invalid, nontext or oversized review source blob")
	}
	*used += len(content)
	return content, nil
}

func gitBlobID(content []byte) string {
	hash := sha1.New() // #nosec G401 -- Match Git's existing SHA1 object protocol; repository/ref/TLS remain the trust boundary.
	_, _ = fmt.Fprintf(hash, "blob %d\x00", len(content))
	_, _ = hash.Write(content)
	return hex.EncodeToString(hash.Sum(nil))
}

func appendFileDiff(diff *strings.Builder, limit int, file patchFile, old, current treeEntry) error {
	oldName := file.Filename
	if file.Previous != "" {
		oldName = file.Previous
	}
	lines := []string{"diff --git " + strconv.Quote("a/"+oldName) + " " + strconv.Quote("b/"+file.Filename)}
	switch {
	case old.Path == "":
		lines = append(lines, "new file mode "+current.Mode)
	case current.Path == "":
		lines = append(lines, "deleted file mode "+old.Mode)
	case old.Mode != current.Mode:
		lines = append(lines, "old mode "+old.Mode, "new mode "+current.Mode)
	}
	if file.Previous != "" {
		kind := "rename"
		if file.Status == "copied" {
			kind = "copy"
		}
		lines = append(lines, kind+" from "+strconv.Quote(oldName), kind+" to "+strconv.Quote(file.Filename))
	}
	oldID, currentID, mode := old.SHA, current.SHA, ""
	if old.Path == "" {
		oldID = strings.Repeat("0", 40)
	}
	if current.Path == "" {
		currentID = strings.Repeat("0", 40)
	}
	if old.Path != "" && current.Path != "" && old.Mode == current.Mode {
		mode = " " + old.Mode
	}
	lines = append(lines, "index "+oldID+".."+currentID+mode)
	if file.Patch != nil && *file.Patch != "" {
		a, b := strconv.Quote("a/"+oldName), strconv.Quote("b/"+file.Filename)
		if old.Path == "" {
			a = "/dev/null"
		}
		if current.Path == "" {
			b = "/dev/null"
		}
		lines = append(lines, "--- "+a, "+++ "+b)
	}
	for _, line := range lines {
		if diff.Len()+len(line)+1 > limit {
			return errors.New("assembled review input oversized")
		}
		diff.WriteString(line)
		diff.WriteByte('\n')
	}
	if file.Patch != nil && *file.Patch != "" {
		patch := *file.Patch
		if !strings.HasSuffix(patch, "\n") {
			patch += "\n"
		}
		if diff.Len()+len(patch) > limit {
			return errors.New("assembled review input oversized")
		}
		diff.WriteString(patch)
	}
	return nil
}
