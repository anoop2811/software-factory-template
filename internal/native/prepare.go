package native

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"syscall"
	"unicode"
	"unicode/utf8"
)

const fileLimit = 1 << 20
const outputLimit = 16 << 20
const codexMatcher = "^(apply_patch|Edit|Write)$"
const codexCommand = `FACTORY_AGENT_ROLE=implementer "$(git rev-parse --show-toplevel)/scripts/hooks/test-edit-denial.sh"`

// Prepare constructs literal native arguments without invoking a process.
// docs/adr/0068-go-native-harness-execution.md:60.
func Prepare(request Request, environment map[string]string) (Plan, error) {
	plan := Plan{Harness: request.Harness, Role: request.Role, Root: request.Root, Stdin: request.Prompt}
	if !validRole(request.Role) || !validHarness(request.Harness) || request.Root == "" {
		return Plan{}, errors.New("invalid native request")
	}
	for _, value := range []string{request.Root, request.Model, request.Prompt} {
		if !scalar(value) || len(value) > outputLimit {
			return Plan{}, errors.New("invalid native request")
		}
	}
	instructions, err := roleText(request.Root, request.Role, ".opencode/agent")
	if err != nil {
		return Plan{}, err
	}
	plan.Environment, err = environmentOverrides(request.Harness, request.Role, environment)
	if err != nil {
		return Plan{}, err
	}
	switch request.Harness {
	case "codex":
		sandbox, flags, err := codexSettings(request.Root, request.Role)
		if err != nil {
			return Plan{}, err
		}
		plan.Argv = append([]string{"codex", "exec", "--json", "--sandbox", sandbox, "--cd", request.Root}, flags...)
		plan.Stdin = "Factory role: " + request.Role + "\n\n" + instructions + "\n\nTask:\n" + request.Prompt
	case "claude":
		if _, err := roleText(request.Root, request.Role, ".claude/agents"); err != nil {
			return Plan{}, err
		}
		denied, err := editDenied(request.Root, request.Role)
		if err != nil {
			return Plan{}, err
		}
		mode := "acceptEdits"
		if denied {
			mode = "plan"
		}
		plan.Argv = []string{"claude", "-p", "--output-format", "json", "--agent", request.Role, "--permission-mode", mode}
	case "opencode":
		plan.Argv = []string{"opencode", "run", "--format", "json", "--agent", request.Role}
	}
	if request.Model != "" {
		plan.Argv = append(plan.Argv, "--model", request.Model)
	}
	if request.Harness == "codex" {
		plan.Argv = append(plan.Argv, "-")
	}
	if len(plan.Stdin) > outputLimit {
		return Plan{}, errors.New("native prompt exceeds its limit")
	}
	return plan, nil
}

func validHarness(h string) bool { return h == "codex" || h == "claude" || h == "opencode" }
func validRole(role string) bool {
	switch role {
	case "spec-writer", "implementer", "refactorer", "reviewer", "wiki-maintainer":
		return true
	default:
		return false
	}
}
func scalar(value string) bool { return utf8.ValidString(value) && !strings.ContainsRune(value, 0) }
func stripSpace(r rune) bool   { return unicode.IsSpace(r) || (r >= 0x1c && r <= 0x1f) }

func roleText(root, role, directory string) (string, error) {
	data, err := readLocal(rootPath(root, directory+"/"+role+".md"))
	if err != nil {
		return "", err
	}
	text := strings.ReplaceAll(strings.ReplaceAll(string(data), "\r\n", "\n"), "\r", "\n")
	if strings.HasPrefix(text, "---\n") {
		_, body, found := strings.Cut(text[3:], "\n---")
		if !found {
			return "", errors.New("invalid native role frontmatter")
		}
		text = strings.TrimFunc(body, stripSpace)
	}
	if strings.TrimFunc(text, stripSpace) == "" {
		return "", errors.New("empty native role instructions")
	}
	return text, nil
}

// Open nonblocking before fstat so a special file cannot stall preparation.
// docs/adr/0068-go-native-harness-execution.md:83.
func readLocal(path string) ([]byte, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, errors.New("missing or unreadable native configuration")
	}
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > fileLimit {
		return nil, errors.New("invalid native configuration file")
	}
	data, err := io.ReadAll(io.LimitReader(file, fileLimit+1))
	if err != nil || len(data) > fileLimit || !scalar(string(data)) {
		return nil, errors.New("invalid native configuration file")
	}
	return data, nil
}

func editDenied(root, role string) (bool, error) {
	data, err := readLocal(rootPath(root, "opencode.json"))
	if err != nil {
		return false, err
	}
	config, err := configuration(data)
	if err != nil {
		return false, err
	}
	agents, ok := config["agent"].(map[string]any)
	if !ok {
		return false, errors.New("invalid canonical role permissions")
	}
	agent, ok := agents[role].(map[string]any)
	if !ok {
		return false, errors.New("invalid canonical role permissions")
	}
	permission, ok := agent["permission"].(map[string]any)
	if !ok {
		return false, errors.New("invalid canonical role permissions")
	}
	return permission["edit"] == "deny", nil
}

func codexSettings(root, role string) (string, []string, error) {
	denied, err := editDenied(root, role)
	if err != nil {
		return "", nil, err
	}
	sandbox := "workspace-write"
	if denied {
		sandbox = "read-only"
	}
	if role != "implementer" {
		return sandbox, nil, nil
	}
	hook := rootPath(root, "scripts/hooks/test-edit-denial.sh")
	info, err := os.Stat(hook)
	if err != nil || !info.Mode().IsRegular() || syscall.Access(hook, 1) != nil {
		return "", nil, errors.New("missing executable implementer test-edit-denial hook")
	}
	matcher, _ := json.Marshal(codexMatcher)
	command, _ := json.Marshal(codexCommand)
	config := `hooks.PreToolUse=[{matcher=` + string(matcher) + `,hooks=[{type="command",command=` + string(command) + `,timeout=30,statusMessage="Enforcing implementer test-file boundary"}]}]`
	return sandbox, []string{"-c", config}, nil
}

func environmentOverrides(harness, role string, environment map[string]string) (map[string]string, error) {
	result := map[string]string{"FACTORY_AGENT_ROLE": role}
	if harness != "opencode" {
		return result, nil
	}
	raw := environment["OPENCODE_CONFIG_CONTENT"]
	if raw == "" {
		raw = "{}"
	}
	overlay, err := configuration([]byte(raw))
	if err != nil {
		return nil, err
	}
	agents, err := objectField(overlay, "agent")
	if err != nil {
		return nil, err
	}
	selected, err := objectField(agents, role)
	if err != nil {
		return nil, err
	}
	if selected["disable"] == true {
		return nil, errors.New("selected OpenCode role is disabled")
	}
	selected["mode"] = "all"
	encoded, err := json.Marshal(overlay)
	if err != nil {
		return nil, errors.New("invalid native configuration")
	}
	result["OPENCODE_CONFIG_CONTENT"] = string(encoded)
	return result, nil
}
func objectField(parent map[string]any, key string) (map[string]any, error) {
	value, exists := parent[key]
	if !exists {
		result := map[string]any{}
		parent[key] = result
		return result, nil
	}
	result, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("invalid native configuration object")
	}
	return result, nil
}

// Validate escaped surrogates before encoding/json can replace them with U+FFFD.
// docs/adr/0068-go-native-harness-execution.md:83.
func configuration(data []byte) (map[string]any, error) {
	invalid := errors.New("invalid native configuration")
	if len(data) > fileLimit || !scalar(string(data)) || !json.Valid(data) || !scalarEscapes(data) {
		return nil, invalid
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value map[string]any
	if decoder.Decode(&value) != nil || value == nil || !finiteValues(value) {
		return nil, invalid
	}
	return value, nil
}
func scalarEscapes(data []byte) bool {
	for i := 0; i < len(data); i++ {
		if data[i] != '\\' {
			continue
		}
		i++
		if data[i] != 'u' {
			continue
		}
		code, _ := strconv.ParseUint(string(data[i+1:i+5]), 16, 16)
		i += 4
		if code >= 0xdc00 && code <= 0xdfff {
			return false
		}
		if code < 0xd800 || code > 0xdbff {
			continue
		}
		if i+6 >= len(data) || data[i+1] != '\\' || data[i+2] != 'u' {
			return false
		}
		low, _ := strconv.ParseUint(string(data[i+3:i+7]), 16, 16)
		if low < 0xdc00 || low > 0xdfff {
			return false
		}
		i += 6
	}
	return true
}
func finiteValues(value any) bool {
	switch v := value.(type) {
	case string:
		return scalar(v)
	case json.Number:
		if !strings.ContainsAny(string(v), ".eE") {
			return len(strings.TrimPrefix(string(v), "-")) <= 4300
		}
		f, err := v.Float64()
		return err == nil && !math.IsInf(f, 0) && !math.IsNaN(f)
	case []any:
		for _, child := range v {
			if !finiteValues(child) {
				return false
			}
		}
	case map[string]any:
		for key, child := range v {
			if !scalar(key) || !finiteValues(child) {
				return false
			}
		}
	}
	return true
}

// Preserve symlink/.. traversal so policy reads address the actual execution cwd.
// docs/adr/0068-go-native-harness-execution.md:60.
func rootPath(root, relative string) string { return root + string(os.PathSeparator) + relative }
