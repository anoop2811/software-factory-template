package native

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// Preflight inspects local CLI capabilities, never invoking an agent or model.
// docs/adr/0068-go-native-harness-execution.md:92.
func Preflight(ctx context.Context, plan Plan) error {
	if err := validatePlan(plan); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return errors.New("native preflight context ended")
	}
	environment := map[string]string{"OPENCODE_CONFIG_CONTENT": os.Getenv("OPENCODE_CONFIG_CONTENT")}
	if _, err := Prepare(Request{Harness: plan.Harness, Role: plan.Role, Root: plan.Root}, environment); err != nil {
		return err
	}
	// shutil.which refuses an explicitly empty PATH, while Popen still permits
	// current-directory execution. Preserve that preflight/execution distinction.
	if path, exists := os.LookupEnv("PATH"); exists && path == "" {
		return errors.New("native CLI is missing")
	}
	binary, err := findExecutable(plan.Harness, ".")
	if err != nil {
		return errors.New("native CLI is missing")
	}
	var args, flags []string
	switch plan.Harness {
	case "codex":
		args = []string{"exec", "--help"}
		flags = []string{"--json", "--sandbox", "--cd", "--model", "--config"}
	case "claude":
		args = []string{"--help"}
		flags = []string{"--print", "--output-format", "--agent", "--permission-mode", "--model"}
	case "opencode":
		args = []string{"run", "--help"}
		flags = []string{"--format", "--agent", "--model"}
	}
	probe := Plan{Harness: plan.Harness, Role: plan.Role, Root: plan.Root, Argv: append([]string{binary}, args...)}
	result, err := supervise(ctx, probe, 10*time.Second, probeSpawn, defaultProcessOps(), runOptions{limit: 2 << 20, mergeStderr: true})
	if err != nil {
		return err
	}
	if result.Outcome != "completed" {
		return errors.New("native CLI help failed")
	}
	for _, flag := range flags {
		if !helpFlag(string(result.Stdout), flag) {
			return errors.New("native CLI lacks required flags")
		}
	}
	if plan.Harness == "codex" && plan.Role == "implementer" {
		if ctx.Err() != nil {
			return errors.New("native preflight context ended")
		}
		_, hookFlags, err := codexSettings(plan.Root, plan.Role)
		if err != nil {
			return err
		}
		root, err := filepath.EvalSymlinks(plan.Root)
		if err != nil {
			return errors.New("invalid native project root")
		}
		root, err = filepath.Abs(root)
		if err != nil {
			return errors.New("invalid native project root")
		}
		return preflightHooks(ctx, binary, root, hookFlags)
	}
	return nil
}
func probeSpawn(context.Context, int) error { return nil }

type hookResponses struct {
	mutex   sync.Mutex
	pending []byte
	rows    map[int]map[string]any
}

func (r *hookResponses) consume(ctx context.Context, data []byte) (bool, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.pending = append(r.pending, data...)
	for {
		if ctx.Err() != nil {
			return false, errors.New("native hook context ended")
		}
		line, rest, found := bytes.Cut(r.pending, []byte{'\n'})
		if !found {
			break
		}
		r.pending = rest
		// Each line is bounded by the shared probe capture limit. Strict scalar JSON
		// makes malformed trust data incapable of authorizing execution.
		row, err := probeObject(line)
		if err != nil {
			return false, err
		}
		id, ok := row["id"].(json.Number)
		if !ok {
			continue
		}
		numeric, err := strconv.ParseFloat(string(id), 64)
		if err != nil {
			continue
		}
		if numeric == 2 || numeric == 3 || numeric == 4 {
			r.rows[int(numeric)] = row
		}
	}
	return len(r.rows) == 3, nil
}
func probeObject(data []byte) (map[string]any, error) {
	if !json.Valid(data) || !scalar(string(data)) || !scalarEscapes(data) {
		return nil, errors.New("invalid native hook response")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if decoder.Decode(&value) != nil || !finiteValues(value) {
		return nil, errors.New("invalid native hook response")
	}
	row, _ := value.(map[string]any)
	return row, nil
}

func preflightHooks(ctx context.Context, binary, root string, flags []string) error {
	requests := []any{
		map[string]any{"id": 1, "method": "initialize", "params": map[string]any{"clientInfo": map[string]any{"name": "factory-budget-preflight", "version": "1"}, "capabilities": map[string]any{"experimentalApi": true}}},
		map[string]any{"method": "initialized"},
		map[string]any{"id": 2, "method": "config/read", "params": map[string]any{"cwd": root, "includeLayers": false}},
		map[string]any{"id": 3, "method": "hooks/list", "params": map[string]any{"cwds": []string{root}}},
		map[string]any{"id": 4, "method": "configRequirements/read", "params": map[string]any{}},
	}
	var input bytes.Buffer
	encoder := json.NewEncoder(&input)
	for _, request := range requests {
		if encoder.Encode(request) != nil {
			return errors.New("cannot prepare native hook probe")
		}
	}
	responses := &hookResponses{rows: map[int]map[string]any{}}
	probe := Plan{Harness: "codex", Role: "implementer", Root: root, Argv: append([]string{binary, "app-server"}, flags...), Stdin: input.String()}
	result, err := supervise(ctx, probe, 10*time.Second, probeSpawn, defaultProcessOps(), runOptions{limit: 2 << 20, consume: responses.consume})
	if err != nil {
		return err
	}
	if result.Outcome == "timeout" || result.Outcome == "interrupted" || result.Outcome == "output_limit" {
		return errors.New("native hook capability probe failed")
	}
	if !trustedHooks(responses.snapshot(), root) {
		return errors.New("native CLI cannot establish required hook trust")
	}
	return nil
}
func mapping(value any) (map[string]any, bool) {
	result, ok := value.(map[string]any)
	return result, ok
}
func resultObject(rows map[int]map[string]any, id int) (map[string]any, bool) {
	return mapping(rows[id]["result"])
}
func optionalObject(value any) (map[string]any, bool) {
	if !truthy(value) {
		return map[string]any{}, true
	}
	return mapping(value)
}
func hooksEnabled(features map[string]any) bool {
	value, exists := features["hooks"]
	if !exists {
		value = features["codex_hooks"]
	}
	return value != false
}
func trustedHooks(rows map[int]map[string]any, root string) bool {
	effective, ok := resultObject(rows, 2)
	if !ok {
		return false
	}
	config, ok := mapping(effective["config"])
	if !ok {
		return false
	}
	features, ok := optionalObject(config["features"])
	if !ok || !hooksEnabled(features) {
		return false
	}
	policy, ok := resultObject(rows, 4)
	if !ok {
		return false
	}
	raw, exists := policy["requirements"]
	if !exists {
		return false
	}
	requirements, ok := optionalObject(raw)
	if !ok {
		return false
	}
	required, ok := optionalObject(requirements["featureRequirements"])
	if !ok || !hooksEnabled(required) {
		return false
	}
	hooks, ok := resultObject(rows, 3)
	if !ok {
		return false
	}
	entries, ok := hooks["data"].([]any)
	if !ok {
		return false
	}
	allowed := false
	for _, rawEntry := range entries {
		entry, ok := mapping(rawEntry)
		if !ok {
			return false
		}
		if entry["cwd"] != root || truthy(entry["errors"]) {
			continue
		}
		candidates, ok := entry["hooks"].([]any)
		if !ok {
			return false
		}
		for _, rawHook := range candidates {
			hook, ok := mapping(rawHook)
			if !ok {
				return false
			}
			if hook["eventName"] == "preToolUse" && hook["handlerType"] == "command" && hook["command"] == codexCommand && hook["matcher"] == codexMatcher && hook["enabled"] == true && (hook["trustStatus"] == "trusted" || hook["trustStatus"] == "managed") && !truthy(hook["async"]) {
				allowed = true
			}
		}
	}
	return allowed
}
func truthy(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case bool:
		return v
	case string:
		return v != ""
	case json.Number:
		f, err := v.Float64()
		return err != nil || f != 0
	case []any:
		return len(v) != 0
	case map[string]any:
		return len(v) != 0
	default:
		return true
	}
}

// Python's help regex uses a Unicode word boundary, not Go regexp's ASCII one.
// docs/adr/0068-go-native-harness-execution.md:92.
func helpFlag(help, flag string) bool {
	for {
		_, tail, found := strings.Cut(help, flag)
		if !found {
			return false
		}
		next, _ := utf8.DecodeRuneInString(tail)
		if tail == "" || next != '_' && !unicode.IsLetter(next) && !unicode.IsNumber(next) {
			return true
		}
		help = tail
	}
}

func (r *hookResponses) snapshot() map[int]map[string]any {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	rows := make(map[int]map[string]any, len(r.rows))
	for id, row := range r.rows {
		rows[id] = row
	}
	return rows
}
