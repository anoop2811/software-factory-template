package synccmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/anoop2811/software-factory-template/internal/roles"
	"go.yaml.in/yaml/v3"
)

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func render(ctx context.Context, plan *publication, source *canonical, harness string, environment map[string]string, active bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	switch harness {
	case "opencode":
		return renderOpenCode(plan, source, environment, active)
	case "claude":
		return renderClaude(plan, source, environment)
	case "codex":
		return renderCodex(plan, source, environment)
	}
	return errUsage
}

func model(environment map[string]string, harness, tier string) string {
	tier = roles.Resolve(environment["COST_PROFILE"], tier)
	value := environment[strings.ToUpper(harness+"_"+tier)+"_MODEL"]
	if harness == "opencode" && value == "" {
		value = environment["OPENCODE_DEFAULT_MODEL"]
	}
	if harness != "opencode" && (strings.Contains(value, "/") || strings.Contains(value, "__")) {
		value = ""
	}
	if harness == "claude" && value == "" {
		value = "inherit"
	}
	return value
}

func renderOpenCode(plan *publication, source *canonical, environment map[string]string, active bool) error {
	if !active {
		plan.messages = append(plan.messages, "sync-opencode: no harness settings configured — leaving opencode.json as-is")
		return nil
	}
	main := environment["OPENCODE_DEFAULT_MODEL"]
	if main == "" {
		delete(source.object, "model")
		delete(source.object, "small_model")
	} else {
		source.object["model"] = main
		source.object["small_model"] = model(environment, "opencode", "economy")
	}
	for _, role := range source.roles {
		value := ""
		if main == "" {
			delete(role.object, "model")
		} else {
			value = model(environment, "opencode", roles.Tier(role.name))
			role.object["model"] = value
		}
		front := strings.Split(role.front, "\n")
		if role.modelStart >= 0 {
			replacement := []string{}
			if value != "" {
				replacement = append(replacement, "model: "+yamlScalar(value))
			}
			front = append(append(front[:role.modelStart:role.modelStart], replacement...), front[role.modelEnd:]...)
		}
		data := "---\n" + strings.Join(front, "\n") + "\n---\n" + role.body
		plan.add(".opencode/agent/"+role.name+".md", []byte(data))
		if main != "" {
			plan.messages = append(plan.messages, "sync-opencode: "+role.name+" -> "+value)
		}
	}
	data, err := jsonBytes(source.object)
	if err != nil {
		return err
	}
	plan.add("opencode.json", data)
	if main == "" {
		plan.messages = append(plan.messages, "sync-opencode: tiers blank (inherit) — model pins removed; opencode uses its own")
	} else {
		plan.messages = append(plan.messages, "sync-opencode: model="+main+" small_model="+model(environment, "opencode", "economy"), "sync-opencode: done")
	}
	return nil
}

func renderClaude(plan *publication, source *canonical, environment map[string]string) error {
	allow, deny := []string{}, []string{}
	for _, rule := range source.bash {
		switch rule.permission {
		case "allow":
			allow = append(allow, "Bash("+rule.pattern+")")
		case "deny":
			deny = append(deny, "Bash("+rule.pattern+")")
		}
	}
	edit, err := editPermission(source.object)
	if err != nil {
		return err
	}
	if edit == "deny" {
		deny = append(deny, "Edit")
	}
	settings := map[string]any{
		"$schema":     "https://json.schemastore.org/claude-code-settings.json",
		"permissions": map[string]any{"allow": allow, "deny": deny},
		"hooks": map[string]any{"PreToolUse": []any{map[string]any{
			"matcher": "Edit|Write", "hooks": []any{map[string]any{
				"type": "command", "command": "${CLAUDE_PROJECT_DIR}/scripts/hooks/test-edit-denial.sh", "args": []string{},
			}},
		}}},
	}
	data, err := jsonBytes(settings)
	if err != nil {
		return err
	}
	plan.add(".claude/settings.json", data)
	plan.messages = append(plan.messages, "sync-claude: wrote .claude/settings.json")
	servers, err := mcpServers(source.object)
	if err != nil {
		return err
	}
	data, err = jsonBytes(map[string]any{"mcpServers": servers})
	if err != nil {
		return err
	}
	plan.add(".mcp.json", data)
	plan.messages = append(plan.messages, "sync-claude: wrote .mcp.json")
	for _, role := range source.roles {
		permission := "default"
		if role.edit == "deny" {
			permission = "plan"
		}
		var out strings.Builder
		fmt.Fprintf(&out, "---\nname: %s\ndescription: %s\nmodel: %s\npermissionMode: %s\n", yamlScalar(role.name), yamlScalar(role.description), yamlScalar(model(environment, "claude", roles.Tier(role.name))), permission)
		if role.name == "implementer" {
			out.WriteString(claudeImplementerHook)
		}
		// Shell command substitution used to remove the body's final newlines.
		// Preserve its ordinary bytes, including the leading blank paragraph.
		fmt.Fprintf(&out, "---\n\n%s\n", strings.TrimRight(role.body, "\n"))
		path := ".claude/agents/" + role.name + ".md"
		plan.add(path, []byte(out.String()))
		plan.messages = append(plan.messages, "sync-claude: wrote "+path)
	}
	plan.claudeLink = true
	plan.messages = append(plan.messages, "sync-claude: done")
	return nil
}

func mcpServers(object map[string]any) (map[string]any, error) {
	mcp, err := objectField(object, "mcp")
	if err != nil {
		return nil, err
	}
	result := map[string]any{}
	for _, name := range sortedKeys(mcp) {
		server, ok := mcp[name].(map[string]any)
		if !ok {
			return nil, errors.New("unsupported MCP server")
		}
		if enabled, exists := server["enabled"]; exists {
			value, ok := enabled.(bool)
			if !ok {
				return nil, errors.New("unsupported MCP enabled setting")
			}
			if !value {
				continue
			}
		}
		command, args := "", []string{}
		switch value := server["command"].(type) {
		case string:
			command = value
		case []any:
			if len(value) == 0 {
				return nil, errors.New("empty MCP command")
			}
			for i, part := range value {
				text, ok := part.(string)
				if !ok {
					return nil, errors.New("unsupported MCP command argument")
				}
				if i == 0 {
					command = text
				} else {
					args = append(args, text)
				}
			}
		default:
			return nil, errors.New("unsupported MCP command")
		}
		if command == "" || strings.ContainsRune(command, 0) {
			return nil, errors.New("invalid MCP command")
		}
		environment, err := objectField(server, "env")
		if err != nil {
			return nil, err
		}
		for _, value := range environment {
			if _, ok := value.(string); !ok {
				return nil, errors.New("unsupported MCP environment value")
			}
		}
		result[name] = map[string]any{"command": command, "args": args, "env": environment}
	}
	return result, nil
}

func renderCodex(plan *publication, source *canonical, environment map[string]string) error {
	plan.add(".codex/config.toml", []byte(codexConfig))
	for _, role := range source.roles {
		if strings.Contains(role.front+role.body, `"""`) {
			return fmt.Errorf("role body contains TOML multiline delimiter: %s", role.name)
		}
		var out strings.Builder
		fmt.Fprintf(&out, "# Generated by scripts/sync-codex.sh. Do not hand-edit.\nname = %s\ndescription = %s\n", tomlString(role.name), tomlString(strings.ReplaceAll(role.description, "\n", " ")))
		if value := model(environment, "codex", roles.Tier(role.name)); value != "" {
			fmt.Fprintf(&out, "model = %s\n", tomlString(value))
		}
		sandbox := "workspace-write"
		if role.edit == "deny" {
			sandbox = "read-only"
		}
		body := strings.TrimRight(role.body, "\n")
		body = strings.ReplaceAll(body, `\`, `\\`)
		fmt.Fprintf(&out, "sandbox_mode = %s\ndeveloper_instructions = \"\"\"\n%s\n\"\"\"\n", tomlString(sandbox), body)
		if role.name == "implementer" {
			out.WriteString(codexImplementerHook)
		}
		path := ".codex/agents/" + role.name + ".toml"
		plan.add(path, []byte(out.String()))
		plan.messages = append(plan.messages, "sync-codex: wrote "+path)
	}
	plan.messages = append(plan.messages, "sync-codex: wrote .codex/config.toml", "sync-codex: done")
	return nil
}

func jsonBytes(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	return append(data, '\n'), err
}
func yamlScalar(value string) string {
	data, err := yaml.Marshal(value)
	if err == nil && strings.Count(string(data), "\n") == 1 {
		return strings.TrimSuffix(string(data), "\n")
	}
	data, _ = json.Marshal(value)
	return string(data)
}
func tomlString(value string) string {
	// JSON's string escapes are also TOML basic-string escapes; disable HTML
	// escaping to retain the legacy generator's ordinary bytes.
	var out strings.Builder
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
	return strings.TrimSuffix(out.String(), "\n")
}

const claudeImplementerHook = `hooks:
  PreToolUse:
    - matcher: "Edit|Write"
      hooks:
        - type: command
          command: "FACTORY_AGENT_ROLE=implementer ${CLAUDE_PROJECT_DIR}/scripts/hooks/test-edit-denial.sh"
`

const codexConfig = `# Generated by scripts/sync-codex.sh from opencode.json.
# Codex loads project config only after the repository is trusted.

[agents]
max_threads = 4
max_depth = 1

[mcp_servers.playwright]
command = "npx"
args = ["-y", "@playwright/mcp"]
`

const codexImplementerHook = `
[[hooks.PreToolUse]]
matcher = "^(apply_patch|Edit|Write)$"

[[hooks.PreToolUse.hooks]]
type = "command"
command = 'FACTORY_AGENT_ROLE=implementer "$(git rev-parse --show-toplevel)/scripts/hooks/test-edit-denial.sh"'
timeout = 30
statusMessage = "Enforcing implementer test-file boundary"
`
