package doctorcmd

import (
	"context"
	"path/filepath"
	"strings"
)

func (r *report) gates(ctx context.Context, v map[string]string) {
	r.print("\nGates\n")
	if v["test_file_patterns"] != "" {
		r.line("[ARMED]", "test-edit-denial       implementer cannot edit: "+v["test_file_patterns"])
	} else {
		r.line("[inert]", "test-edit-denial       no test_file_patterns set — the implementer can edit tests")
	}
	if v["citation_prefix"] == "" {
		r.line("[inert]", "citation-lint          no citation_prefix set (opt-in)")
	} else if v["docs_root"] != "" && directory(r.path(v["docs_root"])) {
		r.line("[ARMED]", "citation-lint          resolves "+v["citation_prefix"]+"*.md citations against "+v["docs_root"]+"/")
	} else {
		r.line("[warn]", "citation-lint          citation_prefix set but docs_root '"+v["docs_root"]+"' is missing")
	}
	if v["check_command"] == "" {
		r.line("[inert]", "diff-aware-check       no check_command set — nothing re-verified on change")
	} else if regular(r.path("memory/.parity-stale")) {
		r.line("[STALE]", "diff-aware-check       an OBSERVED parity claim is stale (memory/.parity-stale)")
	} else {
		r.line("[ARMED]", "diff-aware-check       re-verifies via: "+v["check_command"])
	}
	if v["decision_log"] == "" {
		r.line("[warn]", "decision-log-gate      no decision_log configured")
	} else if v["protected_paths"] != "" {
		r.line("[ARMED]", "decision-log-gate      governance surfaces + protected_paths ("+v["protected_paths"]+") need a Decision")
	} else {
		r.line("[ARMED]", "decision-log-gate      factory surfaces need a Decision (no protected_paths set)")
	}
	r.line("[ARMED]", "commit-message-lint    verification-claim + conventional-commit lint")
	r.line("[ARMED]", "direct-main-push-block rejects pushes to main (local gate; pair with branch protection)")
	if regular(r.path("memory/PENDING-LESSONS.md")) {
		r.line("[STALE]", "pending-lessons        memory/PENDING-LESSONS.md is unaddressed — push is blocked")
	} else {
		r.line("[ARMED]", "pending-lessons        clears once session lessons are written")
	}
	if directory(r.path(".opencode/plugin")) {
		r.line("[ARMED]", "shared-script-enforce  adapters must call scripts/hooks, not reimplement them")
	} else {
		r.line("[inert]", "shared-script-enforce  no .opencode/plugin present")
	}
	found, err := wikiContent(ctx, r.path(v["wiki_root"]))
	if err != nil {
		r.line("[warn]", "wiki-lint              cannot inspect wiki content: "+err.Error())
	} else if found {
		mode := "cited, reachable (staleness opt-in)"
		if v["wiki_staleness"] == "true" {
			mode = "cited, reachable, fresh"
		}
		r.line("[ARMED]", "wiki-lint              every wiki/ content page: "+mode+", links resolve")
	} else {
		r.line("[inert]", "wiki-lint              no wiki content pages yet (wiki_root: "+v["wiki_root"]+")")
	}
	for _, lang := range shellFields(v["language_packs"]) {
		path, desc := "", ""
		switch lang {
		case "go":
			path = "scripts/hooks/ginkgo-only-check.sh"
			desc = "Go tests use Ginkgo/Gomega"
		case "java":
			path = "scripts/hooks/junit5-only-check.sh"
			desc = "Java tests use JUnit 5"
		case "typescript":
			path = "scripts/hooks/vitest-only-check.sh"
			desc = "TS tests use Vitest"
		default:
			continue
		}
		wired := strings.Contains(v["check_command"], filepath.Base(path))
		present := executable(r.path(path))
		prefix := "pack:" + lang + " dialect gate  "
		switch {
		case wired && present:
			r.line("[ARMED]", prefix+desc)
		case present:
			r.line("[inert]", prefix+"present but not in check_command (not enforced)")
		case wired:
			r.line("[FAIL]", prefix+path+" is in check_command but missing")
		default:
			r.line("[inert]", prefix+"not installed for pack '"+lang+"' (a choice)")
		}
	}
}
func (r *report) codeowners(ctx context.Context, protected string) {
	if protected == "" {
		return
	}
	path := r.path(".github/CODEOWNERS")
	if !regular(path) {
		r.line("[warn]", "protected_paths set but .github/CODEOWNERS is missing")
		return
	}
	data, err := readFile(ctx, path)
	if err != nil {
		r.line("[warn]", "cannot inspect CODEOWNERS: "+err.Error())
		return
	}
	if strings.Contains(string(data), "__PROTECTED_PATH__") {
		r.line("[skip]", "CODEOWNERS still holds the template placeholder (not a substituted adoption)")
		return
	}
	missing := ""
	for _, p := range shellFields(protected) {
		if !strings.Contains(string(data), p) {
			missing += " " + p
		}
	}
	if missing != "" {
		r.line("[warn]", "protected path(s) not in CODEOWNERS:"+missing)
	} else {
		r.line("[ ok ]", "CODEOWNERS references every protected path")
	}
}

func shellFields(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' })
}
