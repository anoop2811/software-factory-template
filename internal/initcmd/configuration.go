package initcmd

import (
	"encoding/json"
	"strings"
)

// Configuration bytes retain the initializer's inert flat format and defaults.
// docs/adr/0085-go-native-init.md:19.
func configuration(v map[string]string) []byte {
	docs := v["DOCS_ROOT"]
	if docs == "" {
		docs = "docs"
	}
	text := `# Software Factory configuration. Flat key: value only — one value per line.
# Lists are space-separated. Parsed by scripts/lib/config.sh. See Decision 2
# in the template's docs/DECISION_LOG.md.
project_name: ` + v["PROJECT_SLUG"] + `
decision_log: docs/DECISION_LOG.md
docs_root: ` + docs + `
citation_prefix: "` + v["CITATION_PREFIX"] + `"
protected_paths: "` + v["PROTECTED_PATH"] + `"
test_file_patterns: ""
language_packs: ""
check_command: ""
# Your own hooks go here, not in hook-existence-check.sh (a framework file
# that upgrade overwrites). Space-separated paths.
local_hooks: ""
wiki_root: wiki
wiki_staleness: false
# Decision 46: explicitly enable before factory budget run may invoke an agent.
budget_enabled: false
budget_max_attempts: 1
budget_max_session_runs: 5
budget_timeout_seconds: 300
budget_session_seconds: 900
budget_max_concurrent: 1
budget_estimated_usd: ""
budget_action: stop
# Decision 47: manual checks by default; bounded repair requires explicit opt-in.
loop_enabled: false
loop_max_attempts: 2
loop_timeout_seconds: 900
loop_check_timeout_seconds: 120
loop_check_command: ""
loop_no_progress_limit: 1

# ── Identity ─────────────────────────────────────────────────────────
project_display_name: "` + v["PROJECT_NAME"] + `"
github_owner: "` + v["GITHUB_OWNER"] + `"
opencode_username: "` + v["OPENCODE_USERNAME"] + `"

# ── Models ───────────────────────────────────────────────────────────
# Which provider seeded the tiers below. Blank tiers mean "inherit": sync writes
# nothing and each harness keeps its own model. See docs/MODELS.md.
cost_profile: "` + v["COST_PROFILE"] + `"
model_provider: "` + v["MODEL_PROVIDER"] + `"
opencode_frontier_model: "` + v["FRONTIER_MODEL"] + `"
opencode_default_model: "` + v["DEFAULT_MODEL"] + `"
opencode_economy_model: "` + v["ECONOMY_MODEL"] + `"
claude_frontier_model: "` + v["CLAUDE_FRONTIER_MODEL"] + `"
claude_default_model: "` + v["CLAUDE_DEFAULT_MODEL"] + `"
claude_economy_model: "` + v["CLAUDE_ECONOMY_MODEL"] + `"
codex_frontier_model: "` + v["CODEX_FRONTIER_MODEL"] + `"
codex_default_model: "` + v["CODEX_DEFAULT_MODEL"] + `"
codex_economy_model: "` + v["CODEX_ECONOMY_MODEL"] + `"

# ── Advisory adversarial PR review ───────────────────────────────────
# Opt-in; costs tokens per PR and needs the repository secret named below.
# Toggle with: ./factory review-lane enable|disable
review_lane: "` + v["REVIEW_LANE"] + `"
review_model: "` + v["REVIEW_MODEL"] + `"
review_api_key_secret: "` + v["REVIEW_API_KEY_SECRET"] + `"
review_max_tokens: "` + v["REVIEW_MAX_TOKENS"] + `"

# ── Toolchain versions ───────────────────────────────────────────────
go_version: "` + v["GO_VERSION"] + `"
java_version: "` + v["JAVA_VERSION"] + `"
node_version: "` + v["NODE_VERSION"] + `"
`
	return []byte(text)
}
func setKey(data []byte, key, value string) []byte {
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, key+":") {
			lines[i] = key + ": \"" + value + "\""
		}
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}
func replacements(v map[string]string) *strings.Replacer {
	docs := v["DOCS_ROOT"]
	if docs == "" {
		docs = "docs"
	}
	protected := v["PROTECTED_PATH"]
	mutation := "./..."
	if protected != "" {
		mutation = "./" + protected + "/..."
	} else {
		protected = "."
	}
	return strings.NewReplacer("__PROJECT_NAME__", v["PROJECT_NAME"], "__DOCS_ROOT__", docs, "__PROJECT_SLUG__", v["PROJECT_SLUG"], "__GITHUB_OWNER__", v["GITHUB_OWNER"], "__OPENCODE_USERNAME__", v["OPENCODE_USERNAME"], "__MUTATION_TARGET__", mutation, "__PROTECTED_PATH__", protected)
}
func mergeBlock(previous, template []byte, kind string) []byte {
	begin := "# BEGIN factory"
	end := "# END factory"
	intro := ""
	if kind == "Makefile" {
		begin += " targets"
		end += " targets"
	} else {
		intro = "# Added by factory-init. Edit above or below this block, not inside it:\n# a re-run replaces the block and leaves the rest of your file alone.\n"
	}
	old := string(previous)
	if strings.Contains(old, begin) {
		var kept strings.Builder
		skip := false
		for _, line := range strings.Split(strings.TrimSuffix(old, "\n"), "\n") {
			if line == begin {
				skip = true
			}
			if !skip {
				kept.WriteString(line)
				kept.WriteByte('\n')
			}
			if line == end {
				skip = false
			}
		}
		old = strings.TrimSuffix(kept.String(), "\n")
	}
	return []byte(old + "\n" + begin + "\n" + intro + string(template) + end + "\n")
}

func jsonReplacements(v map[string]string) *strings.Replacer {
	escaped := make(map[string]string, len(v))
	for key, value := range v {
		data, _ := json.Marshal(value)
		escaped[key] = string(data[1 : len(data)-1])
	}
	return replacements(escaped)
}
