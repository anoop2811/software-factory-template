package initcmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"github.com/anoop2811/software-factory-template/internal/config"
	"github.com/anoop2811/software-factory-template/internal/output"
	"io"
	"strings"
	"unicode/utf8"
)

type usageFailure struct{ message string }

func (e *usageFailure) Error() string { return e.message }

type options struct {
	target        string
	packs         []string
	javaBuild     string
	packsSupplied bool
}

func parseArguments(args []string) (options, error) {
	o := options{target: ".", javaBuild: "auto"}
	for len(args) > 0 {
		arg := args[0]
		args = args[1:]
		switch {
		case arg == "--java-build-tool":
			if len(args) == 0 {
				return o, &usageFailure{"--java-build-tool requires maven or gradle"}
			}
			o.javaBuild = args[0]
			args = args[1:]
		case strings.HasPrefix(arg, "--java-build-tool="):
			o.javaBuild = strings.TrimPrefix(arg, "--java-build-tool=")
		case arg == "--pack":
			o.packsSupplied = true
			if len(args) > 0 {
				o.packs = append(o.packs, strings.Fields(strings.ReplaceAll(args[0], ",", " "))...)
				args = args[1:]
			}
		case strings.HasPrefix(arg, "--pack="):
			o.packsSupplied = true
			o.packs = append(o.packs, strings.Fields(strings.ReplaceAll(strings.TrimPrefix(arg, "--pack="), ",", " "))...)
		default:
			o.target = arg
		}
	}
	if o.javaBuild != "auto" && o.javaBuild != "maven" && o.javaBuild != "gradle" {
		return o, &usageFailure{"--java-build-tool must be maven or gradle"}
	}
	var packs []string
	for _, pack := range o.packs {
		if pack == "none" {
			continue
		}
		if pack != "go" && pack != "java" && pack != "typescript" {
			return o, &usageFailure{"unknown language pack"}
		}
		seen := false
		for _, prior := range packs {
			if prior == pack {
				seen = true
			}
		}
		if !seen {
			packs = append(packs, pack)
		}
	}
	o.packs = packs
	return o, nil
}

type questions struct {
	reader      *bufio.Reader
	out         io.Writer
	promptOut   io.Writer
	prompted    bool
	interactive bool
}

func (q *questions) ask(ctx context.Context, prompt string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if q.prompted {
		if err := output.WriteEvent(ctx, q.promptOut, []byte(prompt)); err != nil {
			return "", err
		}
	}
	var line string
	for {
		part, err := q.reader.ReadSlice('\n')
		if len(line)+len(part) > 64<<10 {
			return "", errors.New("initialization answer is too large")
		}
		line += string(part)
		if err == nil || errors.Is(err, io.EOF) {
			break
		}
		if !errors.Is(err, bufio.ErrBufferFull) {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			return "", err
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	if !utf8.ValidString(line) {
		return "", errors.New("initialization answers must be valid UTF-8")
	}
	return strings.Trim(strings.TrimSuffix(line, "\n"), " \t"), nil
}
func collect(ctx context.Context, q *questions, o *options, environment map[string]string) (map[string]string, error) {
	values := map[string]string{}
	for _, item := range [][2]string{
		{"PROJECT_NAME", "Project name (e.g., MyProject): "}, {"PROJECT_SLUG", "Project slug — lowercase, for paths (e.g., myproject): "},
		{"GITHUB_OWNER", "GitHub owner for CODEOWNERS (e.g., @yourname): "}, {"OPENCODE_USERNAME", "opencode username: "},
		{"PROTECTED_PATH", "Protected path — permanently human-reviewed dir (e.g., internal/billing): "}, {"DOCS_ROOT", "Spec/docs source dir (or leave empty if none): "},
		{"CITATION_PREFIX", "Citation prefix for spec docs (e.g., MYPROJECT_ or leave empty): "}, {"MODEL_PROVIDER", "Provider? [inherit/openrouter/anthropic/openai/other]: "},
		{"COST_PROFILE", "Cost profile — 'standard' or 'economy': "}, {"REVIEW_LANE_ANSWER", "Enable the adversarial review lane? [y/N]: "},
	} {
		if q.interactive {
			message := ""
			switch item[0] {
			case "MODEL_PROVIDER":
				message = "\nModel provider — seeds the default model tiers (override any of them in factory.yaml):\n  inherit     keep whatever each harness already uses — nothing written, no assumptions\n  openrouter  one key, many models\n  anthropic   Claude models directly\n  openai      GPT models directly\n  other       e.g. ollama, bedrock, azure — you supply the model strings\n"
			case "REVIEW_LANE_ANSWER":
				message = "\nAdversarial PR review (optional) — a model reviews the diff of every pull\nrequest and posts an advisory comment. Advisory only, never a required check.\n  costs:  tokens on every PR, at the frontier tier (the reviewer is never cheap)\n  needs:  a repository secret you add in GitHub Settings\n  later:  ./factory review-lane enable   (or disable) at any time\n"
			}
			if err := output.WriteEvent(ctx, q.out, []byte(message)); err != nil {
				return nil, err
			}
		}
		value, err := q.ask(ctx, item[1])
		if err != nil {
			return nil, err
		}
		values[item[0]] = value
	}
	if len(o.packs) == 0 && !o.packsSupplied && q.interactive {
		answer, err := q.ask(ctx, "Pack(s)? [go typescript java / none]: ")
		if err != nil {
			return nil, err
		}
		parsed, err := parseArguments([]string{"--pack", answer})
		if err != nil {
			return nil, err
		}
		o.packs = parsed.packs
	}
	for _, version := range [][3]string{{"go", "GO_VERSION", "Go version for CI (e.g., 1.26): "}, {"java", "JAVA_VERSION", "Java (JDK) version for CI (e.g., 25): "}, {"typescript", "NODE_VERSION", "Node.js version for CI (e.g., 24): "}} {
		if hasPack(o.packs, version[0]) {
			value, err := q.ask(ctx, version[2])
			if err != nil {
				return nil, err
			}
			values[version[1]] = value
		} else {
			values[version[1]] = environment[version[1]]
		}
	}
	defaults(values, environment)
	for _, value := range values {
		if !utf8.ValidString(value) {
			return nil, errors.New("initialization values must be valid UTF-8")
		}
		if strings.ContainsAny(value, "\x00\r\n\"") {
			return nil, errors.New("initialization value cannot be represented safely")
		}
	}
	keyName := values["REVIEW_API_KEY_SECRET"]
	for i := 0; i < len(keyName); i++ {
		b := keyName[i]
		if b != '_' && (!asciiLetterDigit(b) || i == 0 && b >= '0' && b <= '9') {
			return nil, errors.New("review secret name must be an ASCII identifier")
		}
	}
	for _, owner := range strings.Fields(values["GITHUB_OWNER"]) {
		if !strings.ContainsRune(owner, '@') {
			return nil, errors.New("unsafe CODEOWNERS value")
		}
		for i := 0; i < len(owner); i++ {
			b := owner[i]
			if !asciiLetterDigit(b) && !strings.ContainsRune("@/._+-=%", rune(b)) {
				return nil, errors.New("unsafe CODEOWNERS value")
			}
		}
	}
	protected := values["PROTECTED_PATH"]
	if strings.HasPrefix(protected, "/") {
		return nil, errors.New("unsafe protected path")
	}
	for _, part := range strings.Split(protected, "/") {
		if part == ".." {
			return nil, errors.New("unsafe protected path")
		}
	}
	for i := 0; i < len(protected); i++ {
		b := protected[i]
		if !asciiLetterDigit(b) && !strings.ContainsRune("./_-", rune(b)) {
			return nil, errors.New("unsafe protected path")
		}
	}
	for _, key := range []string{"GO_VERSION", "JAVA_VERSION", "NODE_VERSION"} {
		for i := 0; i < len(values[key]); i++ {
			b := values[key][i]
			if !asciiLetterDigit(b) && !strings.ContainsRune("._+-", rune(b)) {
				return nil, errors.New("unsafe toolchain version")
			}
		}
	}
	encoded := configuration(values)
	docs := values["DOCS_ROOT"]
	if docs == "" {
		docs = "docs"
	}
	for _, field := range [][2]string{{"project_name", values["PROJECT_SLUG"]}, {"docs_root", docs}} {
		decoded, err := config.GetBytes(ctx, encoded, field[0], "")
		if err != nil {
			return nil, err
		}
		if decoded != field[1] {
			return nil, errors.New("initialization value cannot be represented safely")
		}
	}
	return values, nil
}
func hasPack(packs []string, name string) bool {
	for _, p := range packs {
		if p == name {
			return true
		}
	}
	return false
}
func defaults(v, environment map[string]string) {
	set := func(key, value string) {
		if v[key] == "" {
			v[key] = value
		}
	}
	set("MODEL_PROVIDER", "inherit")
	v["MODEL_PROVIDER"] = strings.ToLower(v["MODEL_PROVIDER"])
	v["COST_PROFILE"] = strings.ToLower(v["COST_PROFILE"])
	if v["COST_PROFILE"] != "economy" {
		v["COST_PROFILE"] = "standard"
	}
	for _, key := range []string{"DEFAULT_MODEL", "FRONTIER_MODEL", "ECONOMY_MODEL", "CLAUDE_FRONTIER_MODEL", "CLAUDE_DEFAULT_MODEL", "CLAUDE_ECONOMY_MODEL", "CODEX_FRONTIER_MODEL", "CODEX_DEFAULT_MODEL", "CODEX_ECONOMY_MODEL", "REVIEW_MODEL", "REVIEW_API_KEY_SECRET", "REVIEW_MAX_TOKENS"} {
		v[key] = environment[key]
	}
	switch v["MODEL_PROVIDER"] {
	case "openrouter":
		set("DEFAULT_MODEL", "openrouter/z-ai/glm-5.2")
		set("FRONTIER_MODEL", "openrouter/z-ai/glm-5.2")
		set("ECONOMY_MODEL", "openrouter/qwen/qwen3-coder")
	case "anthropic":
		set("DEFAULT_MODEL", "anthropic/claude-sonnet-4-6")
		set("FRONTIER_MODEL", "anthropic/claude-opus-4-8")
		set("ECONOMY_MODEL", "anthropic/claude-haiku-4-5")
	case "openai":
		set("DEFAULT_MODEL", "openai/gpt-5.6-terra")
		set("FRONTIER_MODEL", "openai/gpt-5.6-sol")
		set("ECONOMY_MODEL", "openai/gpt-5.6-luna")
	}
	if v["MODEL_PROVIDER"] != "inherit" {
		set("CLAUDE_FRONTIER_MODEL", "claude-opus-4-8")
		set("CLAUDE_DEFAULT_MODEL", "claude-sonnet-4-6")
		set("CLAUDE_ECONOMY_MODEL", "claude-haiku-4-5")
		set("CODEX_FRONTIER_MODEL", "gpt-5.6-sol")
		set("CODEX_DEFAULT_MODEL", "gpt-5.6-terra")
		set("CODEX_ECONOMY_MODEL", "gpt-5.6-luna")
	}
	keyName := "OPENROUTER_API_KEY"
	if v["MODEL_PROVIDER"] == "anthropic" {
		keyName = "ANTHROPIC_API_KEY"
	}
	if v["MODEL_PROVIDER"] == "openai" {
		keyName = "OPENAI_API_KEY"
	}
	set("REVIEW_API_KEY_SECRET", keyName)
	set("REVIEW_MAX_TOKENS", "8192")
	v["REVIEW_LANE"] = "off"
	answer := strings.ToLower(v["REVIEW_LANE_ANSWER"])
	if answer == "y" || answer == "yes" {
		v["REVIEW_LANE"] = "on"
	}
	set("GO_VERSION", "1.26")
	set("JAVA_VERSION", "25")
	set("NODE_VERSION", "24")
}
func summary(ctx context.Context, destination io.Writer, v map[string]string, o options) error {
	var builder strings.Builder
	out := &builder
	_, err := fmt.Fprintf(out, "\n=== Summary ===\n  Project name:     %s\n  Project slug:     %s\n  GitHub owner:     %s\n  Protected path:   %s\n  Docs source:      %s\n  Citation prefix:  %s\n  Model provider:   %s\n  Cost profile:     %s\n  Review lane:      %s\n  Language pack(s): %s\n", v["PROJECT_NAME"], v["PROJECT_SLUG"], v["GITHUB_OWNER"], v["PROTECTED_PATH"], v["DOCS_ROOT"], v["CITATION_PREFIX"], v["MODEL_PROVIDER"], v["COST_PROFILE"], v["REVIEW_LANE"], strings.Join(o.packs, " "))
	if err != nil {
		return err
	}
	for _, tier := range [][2]string{{"Default model", v["DEFAULT_MODEL"]}, {"Frontier model", v["FRONTIER_MODEL"]}} {
		if tier[1] != "" {
			if _, err := fmt.Fprintf(out, "  %s: %s\n", tier[0], tier[1]); err != nil {
				return err
			}
		}
	}
	if v["REVIEW_LANE"] == "on" {
		if _, err := fmt.Fprintf(out, "  Review lane needs repo secret: %s\n", v["REVIEW_API_KEY_SECRET"]); err != nil {
			return err
		}
	}
	for _, version := range [][3]string{{"go", "Go", v["GO_VERSION"]}, {"java", "Java", v["JAVA_VERSION"]}, {"typescript", "Node", v["NODE_VERSION"]}} {
		if hasPack(o.packs, version[0]) {
			if _, err := fmt.Fprintf(out, "  %s version: %s\n", version[1], version[2]); err != nil {
				return err
			}
		}
	}
	return output.WriteEvent(ctx, destination, []byte(builder.String()))
}

func asciiLetterDigit(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
}
