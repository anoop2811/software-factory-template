package synccmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/anoop2811/software-factory-template/internal/installationfs"
	"go.yaml.in/yaml/v3"
)

type canonical struct {
	object map[string]any
	roles  []role
	bash   []bashRule
	files  []*installationfs.File
}
type role struct {
	name, description, edit string
	object                  map[string]any
	front, body             string
	modelStart, modelEnd    int
}
type bashRule struct{ pattern, permission string }
type jsonEntry struct {
	key   string
	value json.RawMessage
}

func readSource(ctx context.Context, tree *installationfs.Tree) (result *canonical, returned error) {
	result = &canonical{}
	defer func() {
		if returned != nil {
			returned = errors.Join(returned, result.close())
		}
	}()
	data, err := result.read(ctx, tree, "opencode.json")
	if err != nil {
		return result, err
	}
	if !utf8.Valid(data) {
		return result, errors.New("invalid opencode.json encoding")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&result.object); err != nil {
		return result, fmt.Errorf("invalid opencode.json: %w", err)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) || result.object == nil {
		return result, errors.New("invalid opencode.json object")
	}
	entries, err := jsonObject(data)
	if err != nil {
		return result, err
	}
	for _, entry := range entries {
		if entry.key != "permission" || string(entry.value) == "null" {
			continue
		}
		permissions, err := jsonObject(entry.value)
		if err != nil {
			return result, err
		}
		for _, permission := range permissions {
			if permission.key != "bash" || string(permission.value) == "null" {
				continue
			}
			rules, err := jsonObject(permission.value)
			if err != nil {
				return result, err
			}
			for _, rule := range rules {
				var value string
				if err := json.Unmarshal(rule.value, &value); err != nil {
					return result, fmt.Errorf("unsupported bash permission: %w", err)
				}
				result.bash = append(result.bash, bashRule{rule.key, value})
			}
		}
	}
	if _, err := editPermission(result.object); err != nil {
		return result, err
	}
	agents, err := objectField(result.object, "agent")
	if err != nil {
		return result, err
	}
	for _, name := range sortedKeys(agents) {
		if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\") || strings.IndexFunc(name, unicode.IsControl) >= 0 {
			return result, errors.New("invalid canonical agent identifier")
		}
		object, ok := agents[name].(map[string]any)
		if !ok {
			return result, fmt.Errorf("unsupported canonical agent %q", name)
		}
		description, err := stringField(object, "description", "")
		if err != nil {
			return result, err
		}
		edit, err := editPermission(object)
		if err != nil {
			return result, err
		}
		data, err := result.read(ctx, tree, ".opencode/agent/"+name+".md")
		if err != nil {
			return result, fmt.Errorf("read canonical role %q: %w", name, err)
		}
		item, err := parseRole(data)
		if err != nil {
			return result, fmt.Errorf("invalid canonical role %q: %w", name, err)
		}
		item.name, item.description, item.edit, item.object = name, description, edit, object
		result.roles = append(result.roles, item)
	}
	return result, nil
}

func (source *canonical) read(ctx context.Context, tree *installationfs.Tree, path string) ([]byte, error) {
	file, err := tree.OpenFile(ctx, path, inputLimit)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		return nil, unsafe(err)
	}
	source.files = append(source.files, file)
	_, data, err := file.Digest(ctx, 2<<20)
	if err != nil {
		return nil, unsafe(err)
	}
	return data, nil
}

func (source *canonical) check(ctx context.Context) error {
	for _, file := range source.files {
		if err := file.Check(ctx); err != nil {
			return err
		}
	}
	return ctx.Err()
}
func (source *canonical) close() error {
	var failures []error
	for _, file := range source.files {
		failures = append(failures, file.Close())
	}
	return unsafe(errors.Join(failures...))
}

func jsonObject(data []byte) ([]jsonEntry, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, errors.New("unsupported JSON object")
	}
	var entries []jsonEntry
	seen := map[string]bool{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if !ok || seen[key] {
			return nil, errors.New("invalid or duplicate JSON object key")
		}
		seen[key] = true
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		entries = append(entries, jsonEntry{key, value})
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, errors.New("invalid JSON object boundary")
	}
	return entries, nil
}

func objectField(object map[string]any, key string) (map[string]any, error) {
	value := object[key]
	if value == nil {
		return map[string]any{}, nil
	}
	result, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unsupported %s object", key)
	}
	return result, nil
}
func stringField(object map[string]any, key, fallback string) (string, error) {
	value := object[key]
	if value == nil {
		return fallback, nil
	}
	result, ok := value.(string)
	if !ok || !utf8.ValidString(result) || strings.ContainsRune(result, 0) {
		return "", fmt.Errorf("unsupported %s string", key)
	}
	return result, nil
}
func editPermission(object map[string]any) (string, error) {
	permissions, err := objectField(object, "permission")
	if err != nil {
		return "", err
	}
	return stringField(permissions, "edit", "ask")
}

// Frontmatter closes at its first delimiter; the rest is instruction data.
// docs/adr/0099-native-factory-orchestration.md:172.
func parseRole(data []byte) (role, error) {
	result := role{modelStart: -1, modelEnd: -1}
	if !utf8.Valid(data) || strings.IndexFunc(string(data), func(value rune) bool {
		return unicode.IsControl(value) && value != '\n' && value != '\t'
	}) >= 0 {
		return result, errors.New("unsupported role text")
	}
	remaining, ok := strings.CutPrefix(string(data), "---\n")
	if !ok {
		return result, errors.New("missing role frontmatter")
	}
	front, body := "", ""
	closed := false
	for offset := 0; offset <= len(remaining); {
		line, rest, newline := strings.Cut(remaining[offset:], "\n")
		if line == "---" {
			front, body = strings.TrimSuffix(remaining[:offset], "\n"), rest
			closed = true
			break
		}
		if !newline {
			break
		}
		offset += len(line) + 1
	}
	if !closed {
		return result, errors.New("missing role frontmatter closing delimiter")
	}
	var parsed yaml.Node
	if err := yaml.Unmarshal([]byte(front), &parsed); err != nil {
		return result, err
	}
	var metadata map[string]any
	if err := yaml.Unmarshal([]byte(front), &metadata); err != nil {
		return result, err
	}
	if len(parsed.Content) != 0 {
		mapping := parsed.Content[0]
		if mapping.Kind != yaml.MappingNode {
			return result, errors.New("role frontmatter must be a mapping")
		}
		for i := 0; i < len(mapping.Content); i += 2 {
			key, value := mapping.Content[i], mapping.Content[i+1]
			if key.Value != "model" {
				continue
			}
			if value.Kind != yaml.ScalarNode || (value.Tag != "!!str" && value.Tag != "!!null") {
				return result, errors.New("unsupported frontmatter model metadata")
			}
			result.modelStart = key.Line - 1
			result.modelEnd = len(strings.Split(front, "\n"))
			if i+2 < len(mapping.Content) {
				result.modelEnd = mapping.Content[i+2].Line - 1
			}
		}
	}
	result.front, result.body = front, body
	return result, nil
}
