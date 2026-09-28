package initcmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/anoop2811/software-factory-template/internal/config"
)

var requiredFiles = []string{
	"scripts/lib/config.sh", "scripts/lib/roles.sh", "scripts/lib/events.sh", "scripts/lib/hookspath.sh", "scripts/lib/color.sh", "scripts/lib/timing.sh", "scripts/lib/budget.py", "scripts/lib/budget_adapters.py", "scripts/lib/budget-config.sh", "scripts/lib/loop.py",
	"scripts/selftest/run.sh", "scripts/pre-push-check.sh", "scripts/factory-doctor.sh", "scripts/factory-upgrade.sh", "scripts/factory-report.sh", "scripts/factory-budget.sh", "scripts/factory-loop.sh", "scripts/factory-metrics.sh", "templates/metrics.html", "scripts/factory-review-lane.sh", "scripts/factory-migrate-config.sh", "scripts/adversarial-review.sh", "packs/review-lane/review-pr.yml", ".githooks/pre-push", "scripts/prereq-check.sh",
	".opencode/plugin/factory-hooks.ts", ".opencode/package.json", ".opencode/.gitignore", ".codex/config.toml", "opencode.json", "AGENTS.md", "Makefile", "factory", ".gitignore", ".github/CODEOWNERS", ".github/workflows/ci.yml", "docs/FACTORY_RULES.md", "docs/BUDGETS.md", "docs/LOOPS.md", "memory/lessons/001-verification-contract.md",
}
var optionalFiles = []string{
	"wiki/README.md", "scripts/golden-task-eval.sh", "eval/README.md", "eval/runners/mock.sh", "eval/runners/example-harness.sh", "eval/runners/claude.sh", "eval/runners/codex.sh", "eval/runners/opencode.sh", "eval/golden-tasks/reference-answer/task.md", "eval/golden-tasks/reference-answer/verify.sh",
	"workflows/review-diamond.md", "workflows/eval-fanout.md", "workflows/README.md", "scripts/sync-opencode.sh", "scripts/sync-claude.sh", "scripts/sync-codex.sh", "scripts/harness-structural-eval.sh", "scripts/citation-lint.sh", "specs/TEMPLATE.md",
}
var directoriesToCreate = []string{".opencode/plugin", ".opencode/agent", ".codex/agents", "scripts/hooks", ".github/workflows", "docs/adr", "memory/lessons", "wiki", "specs", "eval/golden-tasks/reference-answer", "eval/results", "eval/runners", "workflows", "scripts/lib", "scripts/selftest", ".githooks", "templates", "packs/review-lane"}
var substitutedFiles = map[string]bool{
	"opencode.json": true, "AGENTS.md": true, "Makefile": true, ".github/CODEOWNERS": true, ".github/workflows/ci.yml": true, ".opencode/plugin/factory-hooks.ts": true,
	".opencode/agent/spec-writer.md": true, ".opencode/agent/implementer.md": true, ".opencode/agent/refactorer.md": true, ".opencode/agent/wiki-maintainer.md": true, ".opencode/agent/reviewer.md": true,
	"scripts/hooks/test-edit-denial.sh": true, "scripts/hooks/loop-close-check.sh": true, "scripts/hooks/hook-existence-check.sh": true, "scripts/hooks/shared-script-enforcement.sh": true, "scripts/hooks/commit-message-lint.sh": true, "scripts/hooks/diff-aware-check.sh": true, "scripts/hooks/decision-log-gate.sh": true, "scripts/hooks/ginkgo-only-check.sh": true, "scripts/hooks/direct-main-push-block.sh": true,
	"scripts/citation-lint.sh": true, "scripts/sync-claude.sh": true, "scripts/sync-codex.sh": true, "scripts/harness-structural-eval.sh": true, "scripts/prereq-check.sh": true, "scripts/pre-push-check.sh": true, ".codex/config.toml": true,
	".codex/agents/implementer.toml": true, ".codex/agents/refactorer.toml": true, ".codex/agents/reviewer.toml": true, ".codex/agents/spec-writer.toml": true, ".codex/agents/wiki-maintainer.toml": true, "docs/FACTORY_RULES.md": true, "memory/lessons/001-verification-contract.md": true, "README.md": true,
}

func buildPlan(ctx context.Context, source, target *tree, o options, v map[string]string) ([]asset, error) {
	assets := map[string]asset{}
	replacer := replacements(v)
	copyAsset := func(src, dst string, required bool) error {
		data, mode, exists, err := source.read(ctx, src)
		if err != nil {
			return err
		}
		if !exists {
			if required {
				return errors.New("required template asset is missing")
			}
			return nil
		}
		if strings.HasSuffix(src, ".json") && !utf8.Valid(data) {
			return errors.New("template JSON must be valid UTF-8")
		}
		if src == "factory" && (!utf8.Valid(data) || strings.ContainsRune(string(data), 0) || !strings.HasPrefix(string(data), "#!/usr/bin/env bash\n") && !strings.HasPrefix(string(data), "#!/bin/bash\n")) {
			return errors.New("template factory must remain the shell entry point")
		}
		if substitutedFiles[dst] {
			if strings.HasSuffix(dst, ".json") {
				data = []byte(jsonReplacements(v).Replace(string(data)))
			} else {
				data = []byte(replacer.Replace(string(data)))
			}
		}
		if dst == "factory" || dst == ".githooks/pre-push" || strings.HasSuffix(dst, ".sh") && (strings.HasPrefix(dst, "scripts/") && !strings.HasPrefix(dst, "scripts/lib/") || strings.HasPrefix(dst, "eval/")) {
			mode |= 0111
		}
		assets[dst] = asset{path: dst, data: data, mode: mode}
		return nil
	}
	for _, path := range requiredFiles {
		if err := copyAsset(path, path, true); err != nil {
			return nil, err
		}
	}
	for _, path := range optionalFiles {
		if err := copyAsset(path, path, false); err != nil {
			return nil, err
		}
	}
	for _, group := range [][2]string{{"scripts/hooks", ".sh"}, {".opencode/agent", ".md"}, {".codex/agents", ".toml"}} {
		entries, err := source.list(ctx, group[0])
		if err != nil {
			return nil, err
		}
		count := 0
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), group[1]) {
				path := group[0] + "/" + entry.Name()
				if err := copyAsset(path, path, true); err != nil {
					return nil, err
				}
				count++
			}
		}
		if count == 0 {
			return nil, errors.New("required template assets are missing")
		}
	}
	readmeExists := false
	if target != nil {
		_, _, readmeExists, _ = target.read(ctx, "README.md")
	}
	if !readmeExists {
		if err := copyAsset("README.md", "README.md", true); err != nil {
			return nil, err
		}
	}
	yaml := configuration(v)
	var patterns, checks []string
	for _, pack := range o.packs {
		base := "packs/" + pack
		metadata, _, found, err := source.read(ctx, base+"/pack.yaml")
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, errors.New("required language pack is missing")
		}
		get := func(key string) string { value, _ := config.GetBytes(ctx, metadata, key, ""); return value }
		v["PACK_MATURITY_"+pack] = get("maturity")
		pattern := get("test_file_patterns")
		check := get("check_command")
		root := base
		if pack == "java" && o.javaBuild == "maven" {
			root += "/maven"
			check = strings.ReplaceAll(get("maven_check_command"), "__MAVEN_COMMAND__", v["MAVEN_COMMAND"])
		}
		if pattern != "" {
			patterns = append(patterns, pattern)
		}
		if check != "" {
			checks = append(checks, check)
		}
		entries, err := source.list(ctx, root)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() {
				continue
			}
			if name == "pack.yaml" || name == ".DS_Store" {
				continue
			}
			dst := name
			if name == "Makefile.pack" {
				dst = "Makefile." + pack + ".pack"
			}
			if err := copyAsset(root+"/"+name, dst, true); err != nil {
				return nil, err
			}
			if name == "Makefile.pack" {
				a := assets[dst]
				a.data = []byte(strings.ReplaceAll(string(a.data), "__MAVEN_COMMAND__", v["MAVEN_COMMAND"]))
				assets[dst] = a
			}
		}
		entries, err = source.list(ctx, base+"/hooks")
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".sh") {
				if err := copyAsset(base+"/hooks/"+entry.Name(), "scripts/hooks/"+entry.Name(), true); err != nil {
					return nil, err
				}
			}
		}
		workflow := ".github/workflows/" + pack + "-pack.yml"
		if err := copyAsset(root+"/workflows/ci.yml", workflow, false); err != nil {
			return nil, err
		}
		if a, ok := assets[workflow]; ok {
			a.data = []byte(replacer.Replace(strings.NewReplacer("__GO_VERSION__", v["GO_VERSION"], "__JAVA_VERSION__", v["JAVA_VERSION"], "__NODE_VERSION__", v["NODE_VERSION"], "__MAVEN_COMMAND__", v["MAVEN_COMMAND"]).Replace(string(a.data))))
			assets[workflow] = a
		}
		for _, version := range [][2]string{{"go", "GO_VERSION"}, {"java", "JAVA_VERSION"}, {"node", "NODE_VERSION"}} {
			key := version[0] + "_min_version"
			if get(key) != "" {
				yaml = append(yaml, []byte(key+": \""+v[version[1]]+"\"\n")...)
			}
		}
		if pack == "java" {
			yaml = append(yaml, []byte("java_build_tool: \""+o.javaBuild+"\"\n")...)
		}
	}
	if len(o.packs) > 0 {
		yaml = setKey(yaml, "test_file_patterns", strings.Join(strings.Fields(strings.Join(patterns, " ")), " "))
		yaml = setKey(yaml, "check_command", strings.Join(checks, " && "))
		yaml = setKey(yaml, "language_packs", strings.Join(o.packs, " "))
	}
	assets["factory.yaml"] = asset{path: "factory.yaml", data: yaml, mode: 0644}
	pristineMakefile := string(assets["Makefile"].data)
	for _, pack := range o.packs {
		if _, ok := assets["Makefile."+pack+".pack"]; ok {
			pristineMakefile += "\n# " + pack + " pack targets (test, lint, sec, mutate)\ninclude Makefile." + pack + ".pack\n"
		}
	}
	for path, a := range assets {
		if target != nil {
			data, mode, exists, identity, err := target.observe(ctx, path)
			if err != nil {
				return nil, err
			}
			a.previous = data
			a.previousMode = mode
			a.previousIdentity = identity
			a.existed = exists
		}
		if a.existed && (path == "Makefile" || path == ".gitignore") {
			if path == "Makefile" && string(a.previous) == pristineMakefile {
				a.data = a.previous
			} else if string(a.previous) != string(a.data) {
				a.data = mergeBlock(a.previous, a.data, path)
			}
		}
		if path == "README.md" && a.existed {
			delete(assets, path)
			continue
		}
		assets[path] = a
	}
	makefile := assets["Makefile"]
	for _, pack := range o.packs {
		if _, ok := assets["Makefile."+pack+".pack"]; ok {
			line := "include Makefile." + pack + ".pack"
			found := false
			for _, existing := range strings.Split(string(makefile.data), "\n") {
				if existing == line {
					found = true
				}
			}
			if !found {
				makefile.data = append(makefile.data, []byte("\n# "+pack+" pack targets (test, lint, sec, mutate)\n"+line+"\n")...)
			}
		}
	}
	assets["Makefile"] = makefile
	if target != nil {
		for _, directory := range directoriesToCreate {
			parent, _, err := target.parent(ctx, directory+"/preflight", false)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}
			if parent != nil {
				_ = parent.Close()
			}
		}
		// A retained README is still checked as a planned preservation boundary.
		if _, _, _, err := target.read(ctx, "README.md"); err != nil {
			return nil, err
		}
	}
	paths := make([]string, 0, len(assets))
	for path := range assets {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	result := make([]asset, 0, len(paths))
	for _, path := range paths {
		result = append(result, assets[path])
	}
	return result, nil
}

func selectJava(ctx context.Context, target *tree, o *options, v map[string]string) error {
	v["MAVEN_COMMAND"] = "mvn"
	if !hasPack(o.packs, "java") {
		return nil
	}
	exists := func(path string) (bool, os.FileMode, error) {
		if target == nil {
			return false, 0, nil
		}
		_, mode, found, err := target.read(ctx, path)
		return found, mode, err
	}
	gradle, _, err := exists("gradlew")
	if err != nil {
		return err
	}
	pom, _, err := exists("pom.xml")
	if err != nil {
		return err
	}
	maven, mode, err := exists("mvnw")
	if err != nil {
		return err
	}
	if o.javaBuild == "auto" {
		o.javaBuild = "gradle"
		if !gradle && (pom || maven) {
			o.javaBuild = "maven"
		}
	}
	if o.javaBuild == "maven" && maven {
		if mode&0100 == 0 && (os.Geteuid() != 0 || mode&0111 == 0) {
			return &usageFailure{"mvnw must be an executable file; run chmod +x mvnw and retry"}
		}
		v["MAVEN_COMMAND"] = "./mvnw"
	}
	return nil
}

func overlaps(a, b string) bool {
	return a == b || strings.HasPrefix(a, strings.TrimSuffix(b, string(filepath.Separator))+string(filepath.Separator)) || strings.HasPrefix(b, strings.TrimSuffix(a, string(filepath.Separator))+string(filepath.Separator))
}
