package installation

import (
	"bytes"
	"context"
	"strings"
	"unicode/utf8"

	"github.com/anoop2811/software-factory-template/internal/ciworkflow"
	"github.com/anoop2811/software-factory-template/internal/config"
	"github.com/anoop2811/software-factory-template/internal/staging"
)

const ciReviewPath = ".github/workflows/adversarial-review.yml"
const ciReviewTemplate = "packs/review-lane/review-pr.yml"
const ciReviewHeader = "# Managed by: factory review-lane. Remove with: ./factory review-lane disable\n"

// Presence is captured before consent; an enabled but absent lane stays absent.
// Only a repository secret name, never its value, participates in this context.
// docs/adr/0154-installed-review-workflow-migration.md:66.
type ciReviewContext struct {
	Secret  string `json:"secret"`
	Present bool   `json:"present"`
}

// This closed grammar also qualified the original Go renderer and the safe
// literal subset of the frozen Bash sed replacement; it is not ambient config.
func ciValidReviewSecret(value string) bool {
	if value == "" || len(value) > 64<<10 || value[0] >= '0' && value[0] <= '9' || strings.HasPrefix(strings.ToUpper(value), "GITHUB_") {
		return false
	}
	return ciASCII(value, "_")
}

func ciReviewValues(ctx context.Context, data []byte, keys ...string) (map[string]string, error) {
	if !utf8.Valid(data) || bytes.ContainsRune(data, 0) {
		return nil, conflict("factory.yaml has invalid review input bytes")
	}
	wanted := make(map[string]bool, len(keys))
	for _, key := range keys {
		wanted[key] = true
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		key, raw, found := strings.Cut(line, ":")
		selected := strings.TrimSpace(key)
		if !found || !wanted[selected] {
			continue
		}
		if key != selected || seen[key] || !ciFlatScalar(raw) {
			return nil, conflict("factory.yaml has ambiguous or unsupported selected review values")
		}
		seen[key] = true
	}
	values := make(map[string]string, len(keys))
	for _, key := range keys {
		value, err := config.GetBytes(ctx, data, key, "")
		if err != nil {
			return nil, err
		}
		if len(value) > 64<<10 {
			return nil, conflict("factory.yaml review input exceeds its bound")
		}
		values[key] = value
	}
	return values, ctx.Err()
}

func ciReadReview(ctx context.Context, data []byte) (*ciReviewContext, error) {
	lane, err := ciReviewValues(ctx, data, "review_lane")
	if err != nil {
		return nil, err
	}
	switch lane["review_lane"] {
	case "", "off":
		return nil, nil
	case "on":
	default:
		return nil, conflict("factory.yaml selects an unsupported review lane state")
	}
	values, err := ciReviewValues(ctx, data, "model_provider", "review_api_key_secret")
	if err != nil {
		return nil, err
	}
	name := "OPENROUTER_API_KEY"
	switch values["model_provider"] {
	case "", "inherit", "other", "openrouter":
	case "anthropic":
		name = "ANTHROPIC_API_KEY"
	case "openai":
		name = "OPENAI_API_KEY"
	default:
		return nil, conflict("factory.yaml selects an unsupported review provider")
	}
	if values["review_api_key_secret"] != "" {
		name = values["review_api_key_secret"]
	}
	if !ciValidReviewSecret(name) {
		return nil, conflict("factory.yaml has an invalid review secret name")
	}
	return &ciReviewContext{Secret: name}, ctx.Err()
}

func ciReviewRecipePath(path string) bool {
	return strings.HasPrefix(path, "internal/reviewlanecmd/") && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go")
}

func ciExactReviewRecipe(image staging.Inspection, recipe map[string]string) bool {
	if !matchesRecipe(image, recipe) {
		return false
	}
	for path := range image.Assets {
		if ciReviewRecipePath(path) && recipe[path] == "" {
			return false
		}
	}
	for _, asset := range image.Manifest.Assets {
		if ciReviewRecipePath(asset.Path) && recipe[asset.Path] == "" {
			return false
		}
	}
	for path := range recipe {
		if !ciAssetMatches(image, path) {
			return false
		}
	}
	return true
}

func ciKnownReviewRecipe(image staging.Inspection) bool {
	return ciExactReviewRecipe(image, ciReviewOriginalRecipe) || ciExactReviewRecipe(image, ciReviewSharedRecipe)
}

// Replay uses the original closed renderer's literal projection, independent
// of today's enable implementation. Both historical families publish 0644.
// docs/adr/0154-installed-review-workflow-migration.md:59.
func ciFrozenReview(ctx context.Context, template []byte, secret string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	marker := []byte("__REVIEW_API_KEY_SECRET__")
	count := bytes.Count(template, marker)
	size := int64(len(ciReviewHeader)+len(template)) + int64(count)*int64(len(secret)-len(marker))
	if !ciValidReviewSecret(secret) || count == 0 || size > 1<<20 {
		return nil, conflict("review workflow is outside the qualified literal projection")
	}
	return append([]byte(ciReviewHeader), bytes.ReplaceAll(template, marker, []byte(secret))...), ctx.Err()
}

func ciReviewPredecessor(ctx context.Context, request Request, before *staging.Inspection, secret string) ([]ciWorkflowCandidate, string, error) {
	var template []byte
	switch request.Profile {
	case "v0.1.6", "bash-baseline":
		historical, err := reference(ctx, request.Profile)
		if err != nil {
			return nil, "", err
		}
		template = historical.Blobs[ciReviewTemplate]
	case "go-hybrid-v1":
		if !ciBeforeMatches(request, before) || !ciKnownReviewRecipe(*before) || !ciAssetMatches(*before, ciReviewTemplate) {
			return nil, "Review predecessor renderer or template is not qualified", ctx.Err()
		}
		template = before.Assets[ciReviewTemplate]
	default:
		return nil, "Review predecessor is unsupported", ctx.Err()
	}
	data, err := ciFrozenReview(ctx, template, secret)
	if err != nil {
		return nil, "Review predecessor inputs are outside the qualified projection", err
	}
	return []ciWorkflowCandidate{{data: data, mode: 0644}}, "Review workflow bytes or mode differ from the qualified predecessor", ctx.Err()
}

func ciReviewTarget(ctx context.Context, image staging.Inspection, release ciworkflow.Release, secret string) ([]byte, error) {
	if image.Manifest.Version != release.Tag || image.Manifest.SourceRevision != release.SourceRevision || image.Manifest.Target != release.OriginTarget || !ciKnownReviewRecipe(image) || !ciAssetMatches(image, ciReviewTemplate) {
		return nil, conflict("review target renderer or template is not qualified")
	}
	return ciFrozenReview(ctx, image.Assets[ciReviewTemplate], secret)
}
