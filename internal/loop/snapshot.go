package loop

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/anoop2811/software-factory-template/internal/jsonvalue"
)

// Snapshot observes inherited Git state and binary worktree contents, read-only.
// docs/adr/0073-go-loop-fingerprint-foundation.md:64.
func Snapshot(ctx context.Context, root string, c Config, environment map[string]string) (Fingerprint, error) {
	return (&inspector{}).snapshot(ctx, root, c, environment)
}
func StableSnapshot(ctx context.Context, root string, c Config, environment map[string]string) (Fingerprint, error) {
	return (&inspector{}).stableSnapshot(ctx, root, c, environment)
}
func (i *inspector) stableSnapshot(ctx context.Context, root string, c Config, environment map[string]string) (Fingerprint, error) {
	first, err := i.snapshot(ctx, root, c, environment)
	if err != nil {
		return Fingerprint{}, err
	}
	if i.betweenSnapshots != nil {
		if err := i.betweenSnapshots(ctx); err != nil {
			return Fingerprint{}, err
		}
	}
	second, err := i.snapshot(ctx, root, c, environment)
	if err != nil {
		return Fingerprint{}, err
	}
	if first != second {
		return Fingerprint{}, loopError()
	}
	return first, nil
}
func (i *inspector) snapshot(ctx context.Context, root string, c Config, environment map[string]string) (Fingerprint, error) {
	git := func(args ...string) ([]byte, error) {
		out, code, err := probe(ctx, environment, 10*time.Second, append([]string{"git", "-C", root}, args...), nil)
		if err != nil || code != 0 {
			return nil, loopError()
		}
		return out, nil
	}
	unmerged, err := git("ls-files", "-u")
	if err != nil || len(unmerged) > 0 {
		return Fingerprint{}, loopError()
	}
	rawHead, err := git("rev-parse", "HEAD")
	if err != nil {
		return Fingerprint{}, err
	}
	head := strings.TrimSpace(string(rawHead))
	if len(head) != 40 && len(head) != 64 {
		return Fingerprint{}, loopError()
	}
	for _, character := range head {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return Fingerprint{}, loopError()
		}
	}
	indexed, err := git("ls-files", "--stage", "-z")
	if err != nil {
		return Fingerprint{}, err
	}
	for _, entry := range bytes.Split(indexed, []byte{0}) {
		if bytes.HasPrefix(entry, []byte("160000 ")) {
			return Fingerprint{}, loopError()
		}
	}
	rawNames, err := git("ls-files", "--cached", "--others", "--exclude-standard", "-z")
	if err != nil {
		return Fingerprint{}, err
	}
	names := map[string]struct{}{}
	for _, raw := range bytes.Split(rawNames, []byte{0}) {
		if len(raw) > 0 {
			names[string(raw)] = struct{}{}
		}
	}
	if len(names) > 100000 {
		return Fingerprint{}, loopError()
	}
	entries := map[string]any{}
	rawKeys := []string{}
	var total int64
	orderedNames := make([]string, 0, len(names))
	for name := range names {
		orderedNames = append(orderedNames, name)
	}
	slices.Sort(orderedNames)
	for _, name := range orderedNames {
		if strings.ContainsAny(name, "\n\r") {
			return Fingerprint{}, loopError()
		}
		if name == ".factory" || strings.HasPrefix(name, ".factory/") {
			continue
		}
		for _, part := range strings.Split(name, "/") {
			if part == ".." || part == "." || part == "" {
				return Fingerprint{}, loopError()
			}
		}
		value, err := i.content(ctx, join(root, name), &total)
		if err != nil {
			return Fingerprint{}, err
		}
		entries[jsonvalue.RawString(name)] = value
		rawKeys = append(rawKeys, name)
	}
	tests, err := testNames(ctx, rawKeys, c.TestPatterns, environment)
	if err != nil {
		return Fingerprint{}, err
	}
	criticalFiles := map[string]any{}
	for _, name := range rawKeys {
		governed, err := governing(ctx, name, c.ProtectedPaths, tests)
		if err != nil {
			return Fingerprint{}, err
		}
		if governed {
			criticalFiles[jsonvalue.RawString(name)] = entries[jsonvalue.RawString(name)]
		}
	}
	native, err := i.nativePolicy(ctx, root)
	if err != nil {
		return Fingerprint{}, err
	}
	configPath, exists := environment["FACTORY_LOOP_CONFIG_PATH"]
	if !exists {
		configPath = join(root, "factory.yaml")
	}
	if configPath == "" {
		configPath = "."
	}
	effectiveTotal := int64(0)
	effective, err := i.content(ctx, configPath, &effectiveTotal)
	if err != nil {
		return Fingerprint{}, err
	}
	source, err := digest(ctx, entries)
	if err != nil {
		return Fingerprint{}, err
	}
	safety, err := digest(ctx, map[string]any{"files": criticalFiles, "native_policy": native, "effective_config": effective})
	if err != nil {
		return Fingerprint{}, err
	}
	return Fingerprint{Head: head, Source: source, Safety: safety}, nil
}
func testNames(ctx context.Context, names, patterns []string, environment map[string]string) (map[string]struct{}, error) {
	result := map[string]struct{}{}
	if len(patterns) == 0 || len(names) == 0 {
		return result, nil
	}
	args := []string{"grep", "-E"}
	for _, pattern := range patterns {
		args = append(args, "-e", pattern)
	}
	out, code, err := probe(ctx, environment, 5*time.Second, args, []byte(strings.Join(names, "\n")+"\n"))
	if err != nil || (code != 0 && code != 1) {
		return nil, loopError()
	}
	for _, name := range strings.FieldsFunc(string(out), func(r rune) bool {
		return r == '\n' || r == '\r' || r == '\v' || r == '\f' || r == 0x1c || r == 0x1d || r == 0x1e || r == 0x85 || r == 0x2028 || r == 0x2029
	}) {
		result[name] = struct{}{}
	}
	return result, nil
}
func governing(ctx context.Context, name string, protected []string, tests map[string]struct{}) (bool, error) {
	if _, ok := tests[name]; ok {
		return true, nil
	}
	fixed := []string{"AGENTS.md", "CLAUDE.md", "factory.yaml", "factory.config", "opencode.json", ".opencode", ".claude", ".codex", ".github", ".gitignore"}
	for _, pattern := range append(fixed, protected...) {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		prefix := strings.TrimRight(pattern, "/")
		if name == prefix || strings.HasPrefix(name, prefix+"/") {
			return true, nil
		}
		matched, err := glob(ctx, name, pattern)
		if err != nil || matched {
			return matched, err
		}
	}
	return false, nil
}
func rawRunes(value string) []rune {
	result := make([]rune, 0, len(value))
	for len(value) > 0 {
		r, size := utf8.DecodeRuneInString(value)
		if r == utf8.RuneError && size == 1 {
			r = 0xdc00 + rune(value[0])
		}
		result = append(result, r)
		value = value[size:]
	}
	return result
}

// Unlike filepath.Match, Python fnmatchcase treats slash and backslash literally
// and allows stars across directory boundaries. docs/adr/0073-go-loop-fingerprint-foundation.md:38.
func glob(ctx context.Context, name, pattern string) (bool, error) {
	text, mask := rawRunes(name), rawRunes(pattern)
	x, y, star, restart := 0, 0, -1, 0
	for x < len(text) {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if y < len(mask) && mask[y] == '*' {
			star = y
			y++
			restart = x
			continue
		}
		matched, next := false, y+1
		if y < len(mask) {
			switch mask[y] {
			case '?':
				matched = true
			case '[':
				end := y + 1
				if end < len(mask) && mask[end] == '!' {
					end++
				}
				if end < len(mask) && mask[end] == ']' {
					end++
				}
				for end < len(mask) && mask[end] != ']' {
					if err := ctx.Err(); err != nil {
						return false, err
					}
					end++
				}
				if end == len(mask) {
					matched = text[x] == '['
				} else {
					var err error
					matched, err = characterClass(ctx, text[x], mask[y+1:end])
					if err != nil {
						return false, err
					}
					next = end + 1
				}
			default:
				matched = text[x] == mask[y]
			}
		}
		if matched {
			x++
			y = next
			continue
		}
		if star < 0 {
			return false, nil
		}
		restart++
		x = restart
		y = star + 1
	}
	for y < len(mask) && mask[y] == '*' {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		y++
	}
	return y == len(mask), nil
}

// Range separators are distinct from literal hyphens. Remove reversed ranges
// before interpreting a surviving leading negation, matching fnmatchcase.
// docs/adr/0073-go-loop-fingerprint-foundation.md:38.
func characterClass(ctx context.Context, value rune, pattern []rune) (bool, error) {
	type member struct {
		value     rune
		separator bool
	}
	members := make([]member, len(pattern))
	for index, value := range pattern {
		members[index].value = value
	}
	first := 1
	if len(pattern) > 0 && pattern[0] == '!' {
		first++
	}
	for index := first; index+1 < len(members); index++ {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if members[index].value == '-' {
			members[index].separator = true
			index += 2
		}
	}
	for index := len(members) - 2; index > 0; index-- {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if members[index].separator && members[index-1].value > members[index+1].value {
			members = append(members[:index-1], members[index+2:]...)
			index--
		}
	}
	negate := len(members) > 0 && members[0].value == '!'
	if negate {
		members = members[1:]
	}
	matched := false
	for index := 0; index < len(members); index++ {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if index+2 < len(members) && members[index+1].separator {
			matched = matched || (value >= members[index].value && value <= members[index+2].value)
			index += 2
		} else {
			matched = matched || value == members[index].value
		}
	}
	return matched != negate, nil
}
