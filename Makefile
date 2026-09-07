.PHONY: selftest doctor check eval golden-eval sync-opencode sync-claude sync-codex sync-harnesses check-drift lint-commits prereq-check pre-push diff-aware decision-log pending-lessons

# Core factory targets — language-agnostic. Language packs contribute their
# own test/lint/build targets via packs/<language>/Makefile.pack at init time.

selftest:
	./scripts/selftest/run.sh

.PHONY: budget-selftest loop-selftest
budget-selftest:
	@if [ -x scripts/selftest/budget.sh ]; then ./scripts/selftest/budget.sh; \
	else echo "budget-selftest: template-only acceptance fixtures not installed"; fi

loop-selftest:
	@if [ -x scripts/selftest/loop.sh ]; then ./scripts/selftest/loop.sh; \
	else echo "loop-selftest: template-only acceptance fixtures not installed"; fi

# The factory implementation has a Go module; installing this Makefile in an
# adopter does not select a Go application pack or require Go tooling there.
# The workflow also identifies this source tree, so deleting go.mod or cmd does
# not turn a broken factory checkout into a successful template-only skip.
.PHONY: go-runtime-check go-runtime-source-check
go-runtime-check:
	@if [ -f .github/workflows/go-runtime.yml ] || \
		grep -q '^module github.com/anoop2811/software-factory-template$$' go.mod 2>/dev/null; then \
		$(MAKE) go-runtime-source-check; \
	else echo "go-runtime-check: factory Go sources not installed; adopter checks remain configured separately"; fi

go-runtime-source-check:
	@test -f go.mod && grep -q '^module github.com/anoop2811/software-factory-template$$' go.mod || \
		{ echo "go-runtime-check: expected the factory Go module" >&2; exit 1; }
	@test -d cmd/factory && test -d acceptance || \
		{ echo "go-runtime-check: required candidate CLI or acceptance sources are missing" >&2; exit 1; }
	@git cat-file -e 76952eaa63aebd1ecd282f5ab51dd7c3627cb497^{commit} || \
		{ echo "go-runtime-check: fetch full history for the compatibility baseline" >&2; exit 1; }
	@GO_DIRS="$$(go list -f '{{.Dir}}' ./...)" || exit 1; \
		UNFORMATTED="$$(printf '%s\n' "$$GO_DIRS" | while IFS= read -r GO_DIR; do gofmt -l "$$GO_DIR" || exit 1; done)" || exit 1; \
		test -z "$$UNFORMATTED" || { printf 'gofmt required:\n%s\n' "$$UNFORMATTED" >&2; exit 1; }
# The canonical pack gate expects its installed scripts/hooks path in argv[0].
# Source it with that path so it scans this repository, not packs/go; all gate
# behavior remains in the pack rather than a second factory implementation.
	bash -c '. "$$1"' "$$PWD/scripts/hooks/ginkgo-only-check.sh" "$$PWD/packs/go/hooks/ginkgo-only-check.sh"
	go vet ./...
	go test -race -count=1 ./...
	@BUILD_DIR="$$(mktemp -d)" || exit 1; trap 'rm -rf "$$BUILD_DIR"' EXIT HUP INT TERM; \
		go build -o "$$BUILD_DIR/factory" ./cmd/factory
	golangci-lint run --config packs/go/.golangci.yml ./...
	gosec ./...
	govulncheck ./...

doctor:
	./scripts/factory-doctor.sh

# `check` runs the configured product checks as well as the factory's own. It
# used to run only selftest plus the hook scripts, so a green `make check` said
# nothing about whether the adopter's code compiled or its tests passed — while
# reading, to anyone using it, like the repository was checked.
#
# check_command comes from factory.yaml, which the language pack sets. Empty means
# no pack is installed yet, and the factory gates alone are the honest answer.
check: selftest budget-selftest loop-selftest go-runtime-check
	@CMD="$$(FACTORY_CONFIG=factory.yaml bash -c '. scripts/lib/config.sh; factory_config_get check_command')"; \
	if [ -n "$$CMD" ]; then \
		echo "check: running the configured product checks"; \
		echo "  $$CMD"; \
		sh -c "$$CMD"; \
	else \
		echo "check: no check_command configured (install a language pack to arm it)"; \
	fi
	./scripts/citation-lint.sh
	./scripts/hooks/shared-script-enforcement.sh
	./scripts/hooks/hook-existence-check.sh
	./scripts/hooks/copy-manifest-check.sh
	./scripts/hooks/gate-instrumentation-check.sh
	./scripts/hooks/wiki-lint.sh
	./scripts/hooks/workflow-lint.sh

prereq-check:
	./scripts/prereq-check.sh

eval:
	./scripts/harness-structural-eval.sh --harness=opencode
	./scripts/harness-structural-eval.sh --harness=claude
	./scripts/harness-structural-eval.sh --harness=codex

golden-eval:
	./scripts/golden-task-eval.sh

sync-opencode:
	./scripts/sync-opencode.sh

sync-claude:
	./scripts/sync-claude.sh

sync-codex:
	./scripts/sync-codex.sh

sync-harnesses: sync-opencode sync-claude sync-codex

lint-commits:
	./scripts/hooks/commit-message-lint.sh HEAD

check-drift: sync-harnesses
	@if ! git diff --quiet .claude/settings.json .mcp.json .claude/agents/ 2>/dev/null; then \
		echo "DRIFT: Claude config files do not match sync output. Run 'make sync-claude' and commit."; \
		exit 1; \
	fi
	@if ! git diff --quiet .codex/config.toml .codex/agents/ 2>/dev/null; then \
		echo "DRIFT: Codex config files do not match sync output. Run 'make sync-codex' and commit."; \
		exit 1; \
	fi

diff-aware:
	./scripts/hooks/diff-aware-check.sh

decision-log:
	./scripts/hooks/decision-log-gate.sh

pending-lessons:
	./scripts/hooks/pending-lessons-push-block.sh

# No `|| true` on the gates below. With it, a failing commit-message, diff-aware
# or decision-log check was swallowed and the target still printed "all checks
# passed" — a pre-push target that could not block a bad push, which is the one
# thing it exists to do. If a gate cannot run here (no origin/main in a fresh
# clone, say), fetch it rather than ignoring the result.
pre-push: check check-drift
	./scripts/hooks/commit-message-lint.sh HEAD
	./scripts/hooks/diff-aware-check.sh origin/main HEAD
	./scripts/hooks/decision-log-gate.sh origin/main HEAD
	./scripts/hooks/pending-lessons-push-block.sh
	@echo ""
	@echo "pre-push: all checks passed — run ./scripts/pre-push-check.sh for the full gate"
