package reviewbatch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/anoop2811/software-factory-template/internal/fileinput"
)

const inputLimit = 8 << 20

type changedFile struct {
	Filename  string `json:"filename"`
	Previous  string `json:"previous_filename,omitempty"`
	Status    string `json:"status"`
	Additions *int   `json:"additions"`
	Deletions *int   `json:"deletions"`
}

type hunk struct {
	header                       string
	oldStart, newStart, old, new int
	lines                        []string
}

type fileDiff struct {
	file           changedFile
	header         string
	hunks          []hunk
	uncovered      string
	added, removed int
	parts          []fileDiff
}

var hunkPattern = regexp.MustCompile(`^@@ -([0-9]+)(?:,([0-9]+))? \+([0-9]+)(?:,([0-9]+))? @@(?:.*)$`)
var modePattern = regexp.MustCompile(`^[0-7]{6}$`)
var indexPattern = regexp.MustCompile(`^index [a-f0-9]{4,64}\.\.[a-f0-9]{4,64}( [0-7]{6})?$`)
var similarityPattern = regexp.MustCompile(`^(dis)?similarity index ([0-9]|[1-9][0-9]|100)%$`)

// Both independent inputs must agree before any provider receives data.
// docs/adr/0100-budgeted-batched-adversarial-review.md:29.
func readInputs(ctx context.Context, o options) ([]fileDiff, error) {
	raw, err := fileinput.Read(ctx, o.files, inputLimit)
	if err != nil || !utf8.Valid(raw) {
		return nil, errors.New("cannot read bounded files manifest")
	}
	var files []changedFile
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if decoder.Decode(&files) != nil || files == nil || len(files) > 3000 {
		return nil, errors.New("invalid or oversized files manifest")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return nil, errors.New("trailing manifest data")
	}
	manifestBytes := len(raw)
	byPath := make(map[string]changedFile, len(files))
	for _, file := range files {
		if !safePath(file.Filename) || (file.Previous != "" && !safePath(file.Previous)) || byPath[file.Filename].Filename != "" || file.Additions == nil || file.Deletions == nil || *file.Additions < 0 || *file.Deletions < 0 {
			return nil, errors.New("invalid or duplicate manifest file")
		}
		switch file.Status {
		case "added", "removed", "modified", "changed":
			if file.Previous != "" {
				return nil, errors.New("unexpected previous filename")
			}
		case "renamed", "copied":
			if file.Previous == "" {
				return nil, errors.New("missing previous filename")
			}
		default:
			return nil, errors.New("invalid manifest status")
		}
		byPath[file.Filename] = file
	}
	raw, err = fileinput.Read(ctx, o.diff, inputLimit)
	if err != nil || len(raw)+manifestBytes > inputLimit || !utf8.Valid(raw) || bytes.ContainsRune(raw, 0) {
		return nil, errors.New("cannot read bounded UTF-8 diff")
	}
	if len(raw) == 0 && len(files) == 0 {
		return []fileDiff{}, nil
	}
	if !bytes.HasPrefix(raw, []byte("diff --git ")) || !bytes.HasSuffix(raw, []byte("\n")) {
		return nil, errors.New("missing or incomplete Git diff")
	}
	var sections [][]string
	for _, line := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
		if strings.HasPrefix(line, "diff --git ") {
			sections = append(sections, []string{line})
		} else {
			sections[len(sections)-1] = append(sections[len(sections)-1], line)
		}
	}
	result := make([]fileDiff, 0, len(sections))
	grouped := map[string][][]string{}
	for _, section := range sections {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var matches []changedFile
		for _, file := range files {
			old := file.Filename
			if file.Previous != "" {
				old = file.Previous
			}
			if matchGitHeader(section[0], old, file.Filename) {
				matches = append(matches, file)
			}
		}
		if len(matches) != 1 {
			return nil, errors.New("diff and manifest filenames disagree")
		}
		file := matches[0]
		grouped[file.Filename] = append(grouped[file.Filename], section)
	}
	for _, file := range files {
		sections := grouped[file.Filename]
		var parsed fileDiff
		var err error
		switch len(sections) {
		case 1:
			parsed, err = parseSection(sections[0], file, true)
		case 2:
			parsed, err = parseTypeChange(sections, file)
		default:
			return nil, errors.New("missing or repeated diff file section")
		}
		if err != nil {
			return nil, err
		}
		result = append(result, parsed)
	}
	if len(result) != len(files) {
		return nil, errors.New("diff does not cover every manifest file")
	}
	sort.Slice(result, func(i, j int) bool { return result[i].file.Filename < result[j].file.Filename })
	return result, nil
}

func safePath(value string) bool {
	if value == "" || !utf8.ValidString(value) || strings.HasPrefix(value, "/") || path.Clean(value) != value || value == "." || value == ".." || strings.HasPrefix(value, "../") {
		return false
	}
	for _, r := range value {
		if r < 32 || r == 127 {
			return false
		}
	}
	return true
}

func matchGitHeader(line, old, current string) bool {
	rest := strings.TrimPrefix(line, "diff --git ")
	for _, a := range []string{"a/" + old, strconv.Quote("a/" + old)} {
		for _, b := range []string{"b/" + current, strconv.Quote("b/" + current)} {
			if rest == a+" "+b {
				return true
			}
		}
	}
	// Git uses octal byte escapes for non-ASCII paths when core.quotePath is on.
	if strings.HasPrefix(rest, `"`) {
		left, tail, ok := quotedToken(rest)
		if ok {
			right := strings.TrimPrefix(tail, " ")
			if strings.HasPrefix(right, `"`) {
				decoded, remaining, valid := quotedToken(right)
				return valid && remaining == "" && left == "a/"+old && decoded == "b/"+current
			}
			return left == "a/"+old && right == "b/"+current
		}
	}
	if strings.HasPrefix(rest, "a/"+old+` "`) {
		right, remaining, valid := quotedToken(strings.TrimPrefix(rest, "a/"+old+" "))
		return valid && remaining == "" && right == "b/"+current
	}
	return false
}

func quotedToken(value string) (string, string, bool) {
	escaped := false
	for i := 1; i < len(value); i++ {
		if value[i] == '"' && !escaped {
			decoded, err := strconv.Unquote(value[:i+1])
			return decoded, value[i+1:], err == nil
		}
		if value[i] == '\\' {
			escaped = !escaped
		} else {
			escaped = false
		}
	}
	return "", "", false
}

func parseSection(lines []string, file changedFile, checkStatistics bool) (fileDiff, error) {
	result := fileDiff{file: file}
	first := len(lines)
	for i, line := range lines[1:] {
		if strings.HasPrefix(line, "@@ ") {
			first = i + 1
			break
		}
	}
	metadata := lines[:first]
	result.header = strings.Join(metadata, "\n") + "\n"
	oldName, newName, renameFrom, renameTo := "", "", "", ""
	newFile, deleted := false, false
	seen := map[string]bool{}
	for _, line := range metadata[1:] {
		for _, directive := range []string{"--- ", "+++ ", "rename from ", "rename to ", "copy from ", "copy to ", "new file mode ", "deleted file mode ", "old mode ", "new mode ", "index ", "similarity index ", "dissimilarity index "} {
			if strings.HasPrefix(line, directive) {
				if seen[directive] {
					return result, errors.New("duplicate diff metadata directive")
				}
				seen[directive] = true
			}
		}
		switch {
		case strings.HasPrefix(line, "--- "):
			oldName = decodePath(strings.TrimSuffix(strings.TrimPrefix(line, "--- "), "\t"))
		case strings.HasPrefix(line, "+++ "):
			newName = decodePath(strings.TrimSuffix(strings.TrimPrefix(line, "+++ "), "\t"))
		case strings.HasPrefix(line, "rename from "), strings.HasPrefix(line, "copy from "):
			_, name, _ := strings.Cut(line, " from ")
			renameFrom = decodePath(name)
		case strings.HasPrefix(line, "rename to "), strings.HasPrefix(line, "copy to "):
			_, name, _ := strings.Cut(line, " to ")
			renameTo = decodePath(name)
		case strings.HasPrefix(line, "new file mode "):
			newFile = true
			if !modePattern.MatchString(strings.TrimPrefix(line, "new file mode ")) {
				return result, errors.New("invalid new file mode")
			}
		case strings.HasPrefix(line, "deleted file mode "):
			deleted = true
			if !modePattern.MatchString(strings.TrimPrefix(line, "deleted file mode ")) {
				return result, errors.New("invalid deleted file mode")
			}
		case strings.HasPrefix(line, "old mode "), strings.HasPrefix(line, "new mode "):
			if !modePattern.MatchString(line[len("old mode "):]) {
				return result, errors.New("invalid changed file mode")
			}
		case strings.HasPrefix(line, "index "):
			if !indexPattern.MatchString(line) {
				return result, errors.New("invalid diff index")
			}
		case strings.HasPrefix(line, "similarity index "), strings.HasPrefix(line, "dissimilarity index "):
			if !similarityPattern.MatchString(line) {
				return result, errors.New("invalid diff similarity")
			}
		case strings.HasPrefix(line, "Binary files "), line == "GIT binary patch":
			result.uncovered = "nontext change cannot receive text review"
		default:
			if result.uncovered == "" {
				return result, errors.New("invalid diff metadata")
			}
		}
	}
	if seen["old mode "] != seen["new mode "] || ((newFile || deleted) && seen["old mode "]) || seen["--- "] != seen["+++ "] || seen["rename from "] != seen["rename to "] || seen["copy from "] != seen["copy to "] || (seen["rename from "] && seen["copy from "]) {
		return result, errors.New("incomplete or contradictory diff metadata pair")
	}
	if (file.Status == "added") != newFile || (file.Status == "removed") != deleted {
		return result, errors.New("diff and manifest status disagree")
	}
	if file.Previous != "" && (renameFrom != file.Previous || renameTo != file.Filename) {
		return result, errors.New("diff and manifest rename disagree")
	}
	if (file.Status == "renamed" && !seen["rename from "]) || (file.Status == "copied" && !seen["copy from "]) {
		return result, errors.New("diff and manifest rename kind disagree")
	}
	if file.Previous == "" && (renameFrom != "" || renameTo != "") {
		return result, errors.New("undeclared rename metadata")
	}
	if first < len(lines) || seen["--- "] {
		oldExpected, newExpected := "a/"+file.Filename, "b/"+file.Filename
		if file.Previous != "" {
			oldExpected = "a/" + file.Previous
		}
		if newFile {
			oldExpected = "/dev/null"
		}
		if deleted {
			newExpected = "/dev/null"
		}
		if oldName != oldExpected || newName != newExpected || result.uncovered != "" {
			return result, errors.New("invalid unified diff paths")
		}
	}
	added, removed, previousOld, previousNew := 0, 0, 0, 0
	for i := first; i < len(lines); {
		h, err := parseHunkHeader(lines[i])
		if err != nil || h.oldStart < previousOld || h.newStart < previousNew {
			return result, errors.New("invalid or overlapping diff hunk")
		}
		i++
		old, current := 0, 0
		lastWasContent := false
		for i < len(lines) && !strings.HasPrefix(lines[i], "@@ ") {
			line := lines[i]
			if line == `\ No newline at end of file` {
				if !lastWasContent {
					return result, errors.New("misplaced no-newline marker")
				}
				lastWasContent = false
			} else {
				if len(line) == 0 {
					return result, errors.New("missing hunk line prefix")
				}
				switch line[0] {
				case ' ':
					old++
					current++
				case '+':
					current++
					added++
				case '-':
					old++
					removed++
				default:
					return result, errors.New("invalid hunk line")
				}
				lastWasContent = true
			}
			h.lines = append(h.lines, line)
			i++
		}
		if old != h.old || current != h.new {
			return result, errors.New("truncated or contradictory hunk counts")
		}
		if (newFile && old != 0) || (deleted && current != 0) || old+current == 0 {
			return result, errors.New("hunk ranges contradict file lifecycle")
		}
		previousOld, previousNew = h.oldStart+h.old, h.newStart+h.new
		result.hunks = append(result.hunks, h)
	}
	result.added, result.removed = added, removed
	if checkStatistics && (added != *file.Additions || removed != *file.Deletions) {
		return result, errors.New("diff statistics disagree with manifest")
	}
	if len(lines) == 1 || (len(result.hunks) == 0 && result.uncovered == "" && !newFile && !deleted && file.Previous == "" && !strings.Contains(result.header, "old mode ")) {
		return result, errors.New("missing change content")
	}
	return result, nil
}

// Git represents a file-type change as deletion then creation under one path.
// Preserve both parts and compare their combined statistics to the API entry.
func parseTypeChange(sections [][]string, file changedFile) (fileDiff, error) {
	if (file.Status != "changed" && file.Status != "modified") || file.Previous != "" {
		return fileDiff{}, errors.New("repeated diff section is not a type change")
	}
	modes := [2]uint64{}
	for i, prefix := range []string{"deleted file mode ", "new file mode "} {
		if len(sections[i]) < 2 || !strings.HasPrefix(sections[i][1], prefix) {
			return fileDiff{}, errors.New("type change requires deletion followed by creation")
		}
		mode, err := strconv.ParseUint(strings.TrimPrefix(sections[i][1], prefix), 8, 32)
		if err != nil {
			return fileDiff{}, errors.New("invalid type change mode")
		}
		modes[i] = mode
	}
	if modes[0]&0170000 == modes[1]&0170000 {
		return fileDiff{}, errors.New("repeated sections do not change file type")
	}
	result := fileDiff{file: file}
	for i, status := range []string{"removed", "added"} {
		partFile := file
		partFile.Status = status
		part, err := parseSection(sections[i], partFile, false)
		if err != nil {
			return fileDiff{}, err
		}
		result.parts = append(result.parts, part)
		result.hunks = append(result.hunks, part.hunks...)
		result.added += part.added
		result.removed += part.removed
		if part.uncovered != "" {
			result.uncovered = part.uncovered
		}
	}
	if result.added != *file.Additions || result.removed != *file.Deletions {
		return fileDiff{}, errors.New("type change statistics disagree with manifest")
	}
	return result, nil
}

func decodePath(value string) string {
	if strings.HasPrefix(value, `"`) {
		decoded, err := strconv.Unquote(value)
		if err != nil {
			return ""
		}
		return decoded
	}
	return value
}

func parseHunkHeader(line string) (hunk, error) {
	match := hunkPattern.FindStringSubmatch(line)
	if match == nil {
		return hunk{}, errors.New("invalid hunk header")
	}
	values := [4]int{}
	for i := range values {
		text := match[i+1]
		if text == "" {
			text = "1"
		}
		value, err := strconv.Atoi(text)
		if err != nil || value < 0 || value > 1<<40 {
			return hunk{}, errors.New("invalid hunk coordinate")
		}
		values[i] = value
	}
	if (values[1] > 0 && values[0] == 0) || (values[3] > 0 && values[2] == 0) {
		return hunk{}, fmt.Errorf("invalid positive hunk range")
	}
	return hunk{header: line, oldStart: values[0], old: values[1], newStart: values[2], new: values[3]}, nil
}
