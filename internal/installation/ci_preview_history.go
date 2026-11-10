package installation

import (
	"context"
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/anoop2811/software-factory-template/internal/ciworkflow"
	"github.com/anoop2811/software-factory-template/internal/staging"
)

type ciWorkflowHistory struct {
	bytes  map[string][][]byte
	reason string
}

// Closed initializer recipes establish old bytes independently of the target renderer.
// docs/adr/0147-installed-ci-migration-preview.md:56.
// docs/adr/0147-installed-ci-migration-preview.md:63.
func ciPredecessorBytes(ctx context.Context, request Request, release ciworkflow.Release, before *staging.Inspection, inputs ciPreviewInputs) (ciWorkflowHistory, error) {
	result := ciWorkflowHistory{bytes: map[string][][]byte{}, reason: "Unsupported predecessor initializer recipe; preserve existing workflow"}
	var blobs map[string][]byte
	copied := false
	switch request.Profile {
	case "v0.1.6", "bash-baseline":
		ref, err := reference(ctx, request.Profile)
		if err != nil {
			return result, err
		}
		blobs, copied = ref.Blobs, true
		result.reason = "Workflow bytes do not match the qualified copied initializer projection"
	case "go-hybrid-v1":
		if !ciBeforeMatches(request, before) {
			result.reason = "Qualified Go predecessor image is unavailable or its release metadata mismatches the request"
			return result, ctx.Err()
		}
		if !ciHistoricalGoInputs(inputs.values) {
			result.reason = "Historical project inputs are outside the qualified Go initializer domain; preserve existing workflow"
			return result, ctx.Err()
		}
		switch {
		case ciExactRecipe(*before, ciGeneratedRecipe):
			old := ciworkflow.Release{Tag: request.Before.Version, SourceRevision: request.Before.Revision, OriginTarget: request.Before.Target, Repository: release.Repository}
			application := inputs.application
			application.MutationDirectory = ""
			if protected := inputs.values["PROTECTED_PATH"]; protected != "" {
				application.MutationDirectory = "./" + protected
			}
			body, err := ciFrozenWorkflow(ctx, *before, old, application)
			if err != nil {
				return result, err
			}
			result.bytes[".github/workflows/factory.yml"] = [][]byte{body}
			result.reason = "Workflow bytes do not match the qualified generated initializer projection"
		case ciExactRecipe(*before, ciCopiedRecipe):
			blobs, copied = before.Assets, true
			result.reason = "Workflow bytes do not match the qualified copied Go initializer projection"
		}
	}
	if !copied {
		return result, ctx.Err()
	}
	sources := map[string]string{".github/workflows/ci.yml": ".github/workflows/ci.yml"}
	for _, pack := range []string{"go", "typescript", "java"} {
		if inputs.packs[pack] {
			source := "packs/" + pack + "/workflows/ci.yml"
			if pack == "java" && inputs.application.JavaBuildTool == "maven" {
				source = "packs/java/maven/workflows/ci.yml"
			}
			sources[".github/workflows/"+pack+"-pack.yml"] = source
		}
	}
	for path, source := range sources {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		raw, present := blobs[source]
		if !present || before != nil && request.Profile == "go-hybrid-v1" && !ciAssetMatches(*before, source) {
			continue
		}
		for _, maven := range []string{"mvn", "./mvnw"} {
			body, err := ciCopiedWorkflow(ctx, raw, inputs.values, maven, request.Profile == "go-hybrid-v1", path != ".github/workflows/ci.yml")
			if err != nil {
				if ErrorStatus(err) == 1 {
					return result, err
				}
				continue
			}
			result.bytes[path] = append(result.bytes[path], body)
		}
	}
	return result, ctx.Err()
}

// Both frozen Go initializers reject these values before producing any workflow.
// This is historical recognition only; the generic override contract is unchanged.
// docs/adr/0147-installed-ci-migration-preview.md:84.
func ciHistoricalGoInputs(values map[string]string) bool {
	for _, value := range values {
		if !utf8.ValidString(value) || strings.ContainsAny(value, "\x00\r\n\"") {
			return false
		}
	}
	for _, owner := range strings.Fields(values["GITHUB_OWNER"]) {
		if !strings.ContainsRune(owner, '@') || !ciASCII(owner, "@/._+-=%") {
			return false
		}
	}
	if !ciProtectedPath(values["PROTECTED_PATH"]) {
		return false
	}
	for _, key := range []string{"GO_VERSION", "JAVA_VERSION", "NODE_VERSION"} {
		if !ciASCII(values[key], "._+-") {
			return false
		}
	}
	return true
}

func ciBeforeMatches(request Request, before *staging.Inspection) bool {
	return before != nil && !request.Before.Local && ciworkflow.EligibleRelease(request.Before.Version) && ciRevision(request.Before.Revision) && request.Before.Target == request.Source.Target && before.Manifest.Version == request.Before.Version && before.Manifest.Target == request.Before.Target && before.Manifest.SourceRevision == request.Before.Revision && before.Runtime.Version == request.Before.Version && before.Runtime.Target == request.Before.Target
}

func ciRecipePath(path string) bool {
	return (strings.HasPrefix(path, "internal/initcmd/") || strings.HasPrefix(path, "internal/initinventory/") || strings.HasPrefix(path, "internal/ciworkflow/")) && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go")
}

func ciExactRecipe(image staging.Inspection, recipe map[string]string) bool {
	if !matchesRecipe(image, recipe) {
		return false
	}
	for path := range image.Assets {
		if ciRecipePath(path) && recipe[path] == "" {
			return false
		}
	}
	for _, asset := range image.Manifest.Assets {
		if ciRecipePath(asset.Path) && recipe[asset.Path] == "" {
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

func ciAssetMatches(image staging.Inspection, path string) bool {
	data, present := image.Assets[path]
	if !present {
		return false
	}
	found := false
	for _, asset := range image.Manifest.Assets {
		if asset.Path != path {
			continue
		}
		if found || asset.Mode != "100644" || asset.Bytes != int64(len(data)) || asset.SHA256 != fmt.Sprintf("%x", sha256.Sum256(data)) {
			return false
		}
		found = true
	}
	return found
}

func ciCopiedWorkflow(ctx context.Context, raw []byte, values map[string]string, maven string, goRecipe, pack bool) ([]byte, error) {
	value := string(raw)
	keys := []string{"PROJECT_NAME", "DOCS_ROOT", "PROJECT_SLUG", "GITHUB_OWNER", "OPENCODE_USERNAME", "MUTATION_TARGET", "PROTECTED_PATH"}
	if pack {
		if goRecipe {
			value = strings.NewReplacer("__GO_VERSION__", values["GO_VERSION"], "__JAVA_VERSION__", values["JAVA_VERSION"], "__NODE_VERSION__", values["NODE_VERSION"], "__MAVEN_COMMAND__", maven).Replace(value)
		} else {
			keys = []string{"GO_VERSION", "JAVA_VERSION", "NODE_VERSION", "MUTATION_TARGET", "MAVEN_COMMAND", "PROTECTED_PATH"}
		}
	}
	replacements := []string{}
	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		input, found := values[key]
		if key == "MAVEN_COMMAND" {
			input, found = maven, true
		}
		if key == "MUTATION_TARGET" {
			input, found = values["PROTECTED_PATH"]
			if input == "" {
				input = "./..."
			} else {
				input = "./" + input + "/..."
			}
		}
		marker := "__" + key + "__"
		if strings.Contains(value, marker) && !found {
			return nil, conflict("historical workflow requires explicit project inputs")
		}
		if key == "DOCS_ROOT" && input == "" {
			input = "docs"
		}
		if key == "PROTECTED_PATH" && input == "" {
			input = "."
		}
		if !goRecipe {
			// Both frozen Bash initializers apply sed expressions in this exact order.
			// In particular, a nonempty dot path emits ././..., not ./....
			if strings.ContainsAny(input, "&\\|\r\n") {
				return nil, conflict("unsupported historical sed replacement syntax")
			}
			value = strings.ReplaceAll(value, marker, input)
		} else {
			// The former Go initializer performs one simultaneous generic replacement.
			replacements = append(replacements, marker, input)
		}
	}
	if goRecipe {
		value = strings.NewReplacer(replacements...).Replace(value)
	}
	return []byte(value), ctx.Err()
}

// Parse inert string constants only after the complete source recipe hash check.
// Frozen ordering deliberately does not call today's renderer for old bytes.
// docs/adr/0147-installed-ci-migration-preview.md:67.
func ciFrozenWorkflow(ctx context.Context, image staging.Inspection, release ciworkflow.Release, app ciworkflow.Application) ([]byte, error) {
	constants := map[string]string{}
	for _, path := range []string{"internal/ciworkflow/workflow.go", "internal/ciworkflow/application.go"} {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, image.Assets[path], 0)
		if err != nil {
			return nil, fmt.Errorf("parse qualified historical renderer: %w", err)
		}
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.CONST {
				continue
			}
			for _, spec := range general.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok || len(value.Names) != 1 || len(value.Values) != 1 {
					continue
				}
				literal, ok := value.Values[0].(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					continue
				}
				text, err := strconv.Unquote(literal.Value)
				if err != nil {
					return nil, fmt.Errorf("decode qualified historical renderer: %w", err)
				}
				constants[value.Names[0].Name] = text
			}
		}
	}
	for _, key := range []string{"preparationTemplate", "goSetup", "nodeSetup", "javaSetup", "actionlintInstall", "goToolsInstall", "installedChecks", "actionlintCheck", "goSecurity", "osvScan", "gradleChecks", "goMutation", "nodeMutation", "gradleMutation", "mockGolden"} {
		if constants[key] == "" {
			return nil, fmt.Errorf("qualified historical renderer lacks %s", key)
		}
	}
	parts := []string{fmt.Sprintf(constants["preparationTemplate"], release.Tag, release.OriginTarget, release.SourceRevision, release.Repository)}
	goPack, nodePack, javaPack := app.GoVersion != "", app.NodeVersion != "", app.JavaVersion != ""
	if goPack {
		parts = append(parts, fmt.Sprintf(constants["goSetup"], strconv.Quote(app.GoVersion)))
	}
	if nodePack {
		parts = append(parts, fmt.Sprintf(constants["nodeSetup"], strconv.Quote(app.NodeVersion)))
	}
	if javaPack {
		parts = append(parts, fmt.Sprintf(constants["javaSetup"], strconv.Quote(app.JavaVersion)))
	}
	parts = append(parts, constants["actionlintInstall"])
	if goPack {
		parts = append(parts, constants["goToolsInstall"])
	}
	parts = append(parts, constants["installedChecks"], constants["actionlintCheck"])
	if goPack {
		parts = append(parts, constants["goSecurity"])
	}
	if nodePack || javaPack {
		parts = append(parts, constants["osvScan"])
	}
	gradle := javaPack && app.JavaBuildTool == "gradle"
	if gradle {
		parts = append(parts, constants["gradleChecks"])
	}
	if goPack {
		directory := app.MutationDirectory
		if directory == "" {
			directory = "."
		}
		parts = append(parts, fmt.Sprintf(constants["goMutation"], strconv.Quote(directory)))
	}
	if nodePack {
		parts = append(parts, constants["nodeMutation"])
	}
	if gradle {
		parts = append(parts, constants["gradleMutation"])
	}
	parts = append(parts, constants["mockGolden"])
	return []byte(strings.Join(parts, "")), ctx.Err()
}
