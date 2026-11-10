package installation

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/anoop2811/software-factory-template/internal/ciworkflow"
	"github.com/anoop2811/software-factory-template/internal/config"
)

type ciPreviewInputs struct {
	application ciworkflow.Application
	values      map[string]string
	packs       map[string]bool
}

// Validate the selected flat grammar before the legacy reader can discard ambiguity.
// docs/adr/0147-installed-ci-migration-preview.md:81.
func ciReadInputs(ctx context.Context, data []byte, overrides map[string]string) (ciPreviewInputs, error) {
	result := ciPreviewInputs{values: map[string]string{}, packs: map[string]bool{}}
	keys := map[string]string{"project_display_name": "PROJECT_NAME", "project_name": "PROJECT_SLUG", "github_owner": "GITHUB_OWNER", "opencode_username": "OPENCODE_USERNAME", "protected_paths": "PROTECTED_PATH", "docs_root": "DOCS_ROOT", "citation_prefix": "CITATION_PREFIX", "language_packs": "PACKS", "go_version": "GO_VERSION", "node_version": "NODE_VERSION", "java_version": "JAVA_VERSION", "java_build_tool": "JAVA_BUILD_TOOL"}
	if !utf8.Valid(data) || strings.ContainsRune(string(data), 0) {
		return result, conflict("factory.yaml has invalid Unicode or NUL bytes")
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		key, raw, found := strings.Cut(line, ":")
		selected := strings.TrimSpace(key)
		if !found || keys[selected] == "" {
			continue
		}
		if key != selected || seen[key] || !ciFlatScalar(raw) {
			return result, conflict("factory.yaml has ambiguous or unsupported selected values")
		}
		seen[key] = true
	}
	for key, input := range keys {
		if !seen[key] {
			continue
		}
		value, err := config.GetBytes(ctx, data, key, "")
		if err != nil {
			return result, err
		}
		result.values[input] = value
	}
	if !seen["language_packs"] || !seen["protected_paths"] {
		return result, conflict("factory.yaml lacks explicit language-pack or protected-path settings")
	}
	for _, pack := range strings.Fields(result.values["PACKS"]) {
		if pack != "go" && pack != "typescript" && pack != "java" || result.packs[pack] {
			return result, conflict("factory.yaml selects an unsupported or duplicate language pack")
		}
		result.packs[pack] = true
	}
	for _, entry := range [][2]string{{"go", "GO_VERSION"}, {"typescript", "NODE_VERSION"}, {"java", "JAVA_VERSION"}} {
		if result.packs[entry[0]] && (result.values[entry[1]] == "" || !ciASCII(result.values[entry[1]], "._+-")) {
			return result, conflict("factory.yaml lacks a valid selected language version")
		}
	}
	protected := result.values["PROTECTED_PATH"]
	if !ciProtectedPath(protected) {
		return result, conflict("factory.yaml has unsupported protected-path or mutation inputs")
	}
	if result.packs["java"] && result.values["JAVA_BUILD_TOOL"] != "gradle" && result.values["JAVA_BUILD_TOOL"] != "maven" {
		return result, conflict("factory.yaml has an unsupported Java build choice")
	}
	if result.packs["go"] {
		result.application.GoVersion = result.values["GO_VERSION"]
	}
	if result.packs["typescript"] {
		result.application.NodeVersion = result.values["NODE_VERSION"]
	}
	if result.packs["java"] {
		result.application.JavaVersion = result.values["JAVA_VERSION"]
		result.application.JavaBuildTool = result.values["JAVA_BUILD_TOOL"]
	}
	if protected != "" {
		result.application.MutationDirectory = "./" + protected
	}
	// Only the existing generic input contract can override configuration data.
	for _, key := range []string{"PROJECT_NAME", "PROJECT_SLUG", "GITHUB_OWNER", "OPENCODE_USERNAME", "PROTECTED_PATH", "DOCS_ROOT", "CITATION_PREFIX"} {
		if value, ok := overrides[key]; ok {
			if len(value) > 64<<10 || !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
				return result, conflict("invalid explicit project input")
			}
			result.values[key] = value
		}
	}
	return result, ctx.Err()
}

func ciProtectedPath(value string) bool {
	if strings.HasPrefix(value, "/") || !ciASCII(value, "./_-") {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == ".." {
			return false
		}
	}
	return true
}

func ciASCII(value, punctuation string) bool {
	for i := 0; i < len(value); i++ {
		c := value[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune(punctuation, rune(c)) {
			continue
		}
		return false
	}
	return true
}

func ciFlatScalar(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return true
	}
	if raw[0] == '"' {
		end := strings.IndexByte(raw[1:], '"')
		if end < 0 {
			return false
		}
		tail := raw[end+2:]
		// Quoted comments require whitespace, and escapes are outside the flat reader.
		if strings.ContainsRune(raw[1:end+1], '\\') {
			return false
		}
		return tail == "" || len(tail) > 0 && (tail[0] == ' ' || tail[0] == '\t') && (strings.TrimSpace(tail) == "" || strings.HasPrefix(strings.TrimSpace(tail), "#"))
	}
	// Embedded apostrophes are literal in the existing flat reader.
	// docs/adr/0147-installed-ci-migration-preview.md:256.
	return !strings.HasPrefix(raw, "'") && !strings.ContainsAny(raw, "\"[]{}\r\t") && !strings.HasPrefix(raw, "|") && !strings.HasPrefix(raw, ">")
}
