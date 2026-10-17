// Package adversarialreviewcmd prepares private provider review requests.
package adversarialreviewcmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/anoop2811/software-factory-template/internal/commandenv"
	"github.com/anoop2811/software-factory-template/internal/fileinput"
	"github.com/anoop2811/software-factory-template/internal/native"
)

const fileLimit = 16 << 20

// Run does not enable the review lane or select a paid fallback.
// docs/adr/0099-native-factory-orchestration.md:120.
func Run(ctx context.Context, args []string, root string, environment map[string]string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "" {
		return fail(stderr, "usage: adversarial-review.sh <diff-file>")
	}
	if info, err := os.Stat(args[0]); err != nil || !info.Mode().IsRegular() {
		return fail(stderr, "usage: adversarial-review.sh <diff-file>")
	}
	variables, _, err := commandenv.Base(ctx, environment)
	if err != nil {
		return fail(stderr, "cannot prepare review configuration.")
	}
	seconds, ok := decimal(variables["REVIEW_TIMEOUT_SECONDS"], 1200, 1, 1200)
	if !ok {
		return fail(stderr, "invalid REVIEW_TIMEOUT_SECONDS; use a decimal value from 1 through 1200.")
	}
	provider := variables["MODEL_PROVIDER"]
	if provider == "" {
		provider = "openrouter"
	}
	key := variables["REVIEW_API_KEY"]
	if key == "" {
		fmt.Fprintln(stderr, "adversarial-review: no REVIEW_API_KEY in the environment.\n  CI supplies it from the repository secret named in factory.yaml.")
		return 1
	}
	model := variables["REVIEW_MODEL"]
	if model == "" {
		switch provider {
		case "anthropic":
			model = defaultValue(variables["CLAUDE_FRONTIER_MODEL"], "claude-opus-4-8")
		case "openai":
			model = defaultValue(variables["CODEX_FRONTIER_MODEL"], "gpt-5.6-sol")
		default:
			model = defaultValue(variables["OPENCODE_FRONTIER_MODEL"], "openrouter/z-ai/glm-5.2")
		}
		model = strings.TrimPrefix(model, "openrouter/")
	}
	diff, err := fileinput.Read(ctx, args[0], fileLimit)
	if err != nil {
		return fail(stderr, "cannot read review diff.")
	}
	diffText := strings.TrimRight(strings.ReplaceAll(string(diff), "\x00", ""), "\n")
	if strings.TrimSpace(diffText) == "" {
		fmt.Fprintln(stdout, "_No reviewable change in this diff._")
		return 0
	}
	user := "Review this diff.\n\n```diff\n" + diffText + "\n```"
	messages := []map[string]string{{"role": "system", "content": systemPrompt}, {"role": "user", "content": user}}
	request := map[string]any{"model": model, "messages": messages}
	endpoint := "https://openrouter.ai/api/v1/chat/completions"
	switch provider {
	case "anthropic":
		endpoint = "https://api.anthropic.com/v1/messages"
		request["max_tokens"] = 4096
		request["system"] = systemPrompt
		request["messages"] = messages[1:]
	case "openai":
		endpoint = "https://api.openai.com/v1/chat/completions"
	default:
		effort, route := variables["REVIEW_REASONING_EFFORT"], variables["REVIEW_OPENROUTER_PROVIDER"]
		switch effort {
		case "", "none", "minimal", "low", "medium", "high", "xhigh", "max":
		default:
			return fail(stderr, "invalid REVIEW_REASONING_EFFORT; use none, minimal, low, medium, high, xhigh, max, or empty.")
		}
		if route != "" && !regexp.MustCompile(`^[a-z0-9]+([-/][a-z0-9]+)*$`).MatchString(route) {
			return fail(stderr, "invalid REVIEW_OPENROUTER_PROVIDER; use a lowercase provider or endpoint slug, or empty.")
		}
		tokens, ok := decimal(variables["REVIEW_MAX_TOKENS"], 8192, 1024, 32768)
		if !ok {
			return fail(stderr, "invalid REVIEW_MAX_TOKENS; use a decimal value from 1024 through 32768.")
		}
		request["max_tokens"] = tokens
		if effort != "" {
			request["reasoning"] = map[string]string{"effort": effort}
		}
		if route != "" {
			request["provider"] = map[string]any{"order": []string{route}, "allow_fallbacks": false, "require_parameters": true}
		}
	}
	body, err := json.Marshal(request)
	if err != nil || len(body) > fileLimit {
		return fail(stderr, "invalid review request.")
	}
	if provider != "anthropic" && provider != "openai" && variables["REVIEW_GO_CLIENT"] != "" {
		return selectedClient(ctx, root, variables, body, seconds, stdout, stderr)
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(seconds)*time.Second)
	defer cancel()
	text, err := requestReview(ctx, endpoint, provider, key, body, variables)
	if err != nil {
		return fail(stderr, err.Error())
	}
	if _, err := io.WriteString(stdout, strings.TrimRight(text, "\n")+"\n"); err != nil {
		return fail(stderr, "cannot write review.")
	}
	return 0
}

func fail(stderr io.Writer, message string) int {
	fmt.Fprintln(stderr, "adversarial-review: "+message)
	return 1
}
func defaultValue(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
func decimal(value string, fallback, minimum, maximum int) (int, bool) {
	if value == "" {
		return fallback, true
	}
	if strings.Trim(value, "0123456789") != "" {
		return 0, false
	}
	value = strings.TrimLeft(value, "0")
	if value == "" || len(value) > len(strconv.Itoa(maximum)) {
		return 0, false
	}
	n, err := strconv.Atoi(value)
	return n, err == nil && n >= minimum && n <= maximum
}

func selectedClient(ctx context.Context, _ string, environment map[string]string, body []byte, seconds int, stdout, stderr io.Writer) int {
	path := environment["REVIEW_GO_CLIENT"]
	if !filepath.IsAbs(path) {
		return fail(stderr, "REVIEW_GO_CLIENT must name an absolute executable file.")
	}
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return fail(stderr, "REVIEW_GO_CLIENT must name an absolute executable file.")
	}
	variables := maps.Clone(environment)
	variables["FACTORY_BRIDGE_PROTOCOL"] = "1"
	variables["REVIEW_TIMEOUT_SECONDS"] = strconv.Itoa(seconds)
	variables["REVIEW_HTTP_RETRIES"] = defaultValue(variables["REVIEW_HTTP_RETRIES"], "0")
	if variables["REVIEW_HTTP_RETRIES"] != "0" && variables["REVIEW_HTTP_RETRIES"] != "1" {
		return fail(stderr, "invalid REVIEW_HTTP_RETRIES; use 0 or 1.")
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(seconds)*time.Second)
	defer cancel()
	// The source adapter calls the selected client from the caller's directory.
	// docs/adr/0099-native-factory-orchestration.md:123.
	cwd, err := os.Getwd()
	if err != nil {
		return fail(stderr, "cannot select review working directory.")
	}
	result, err := native.ExecuteCommandWithOptions(ctx, cwd, []string{path, "review", "openrouter"}, variables, native.CommandOptions{Input: string(body), StdoutLimit: fileLimit, StderrLimit: fileLimit})
	if ctx.Err() != nil || result.OwnershipUnconfirmed || !result.ExitConfirmed || result.ExitCode == nil || (result.Outcome != "completed" && result.Outcome != "failed") {
		return fail(stderr, "selected review client did not complete.")
	}
	_ = err // Ordinary client exit statuses remain the explicit no-fallback contract.
	if *result.ExitCode < 0 {
		return 1
	}
	if _, err := stderr.Write(result.Stderr); err != nil {
		return 1
	}
	if *result.ExitCode == 0 {
		if _, err := stdout.Write(result.Stdout); err != nil {
			return 1
		}
	}
	return *result.ExitCode
}
