.PHONY: selftest doctor check eval golden-eval sync-opencode sync-claude sync-codex sync-harnesses check-drift lint-commits prereq-check pre-push diff-aware decision-log pending-lessons

# Core factory targets — language-agnostic. Language packs contribute their
# own test/lint/build targets via packs/<language>/Makefile.pack at init time.

selftest:
	./scripts/selftest/run.sh

.PHONY: budget-selftest loop-selftest adversarial-review-selftest
budget-selftest:
	@if [ -x scripts/selftest/budget.sh ]; then ./scripts/selftest/budget.sh; \
	else echo "budget-selftest: template-only acceptance fixtures not installed"; fi

loop-selftest:
	@if [ -x scripts/selftest/loop.sh ]; then ./scripts/selftest/loop.sh; \
	else echo "loop-selftest: template-only acceptance fixtures not installed"; fi

adversarial-review-selftest:
	@if [ -x scripts/selftest/adversarial-review.sh ]; then ./scripts/selftest/adversarial-review.sh; \
	else echo "adversarial-review-selftest: template-only acceptance fixtures not installed"; fi

# The factory implementation has a Go module; installing this Makefile in an
# adopter does not select a Go application pack or require Go tooling there.
# The workflow also identifies this source tree, so deleting go.mod or cmd does
# not turn a broken factory checkout into a successful template-only skip.
# docs/adr/0097-installation-source-image-bundles.md:231 — suite allowance only.
GO_RUNTIME_TEST_TIMEOUT ?= 25m
# docs/adr/0101-go-release-handoff.md:559 — complete test partitions with headroom.
# docs/adr/0115-frozen-rc3-recipe-compatibility.md:77 — isolate historical reconstruction.
# docs/adr/0116-explicit-go-local-initialization.md:99 — isolate local compilation.
# docs/adr/0117-explicit-go-local-upgrade.md:94 — isolate complete local upgrades.
# docs/adr/0118-default-packaged-go-initialization.md:76 — isolate packaged defaults.
GO_RUNTIME_TEST_GROUP ?= all
# docs/adr/0104-adaptive-qualification-workers.md:22 — resource-aware workers.
GO_RUNTIME_TEST_PROCS ?= auto
GO_RUNTIME_TEST_MAX_PROCS ?= 4
# Freeze raw settings before export: reject Make functions as literal data.
override GO_RUNTIME_TEST_PROCS := $(value GO_RUNTIME_TEST_PROCS)
override GO_RUNTIME_TEST_MAX_PROCS := $(value GO_RUNTIME_TEST_MAX_PROCS)
GO_RUNTIME_REPORT_DIR ?=
export GO_RUNTIME_REPORT_DIR GO_RUNTIME_TEST_PROCS GO_RUNTIME_TEST_MAX_PROCS

.PHONY: go-runtime-check go-runtime-source-check
go-runtime-check:
	@if [ -f .github/workflows/go-runtime.yml ] || \
		grep -q '^module github.com/anoop2811/software-factory-template$$' go.mod 2>/dev/null; then \
		$(MAKE) go-runtime-source-check; \
	else echo "go-runtime-check: factory Go sources not installed; adopter checks remain configured separately"; fi

go-runtime-source-check:
	@set -e; \
	case "$(GO_RUNTIME_TEST_GROUP)" in all|base|installation|release-handoff|historical-recipe|local-init|local-upgrade|default-init) ;; \
		*) echo "go-runtime-check: GO_RUNTIME_TEST_GROUP must be all, base, installation, release-handoff, historical-recipe, local-init, local-upgrade or default-init" >&2; exit 2 ;; esac; \
	test -f go.mod && grep -q '^module github.com/anoop2811/software-factory-template$$' go.mod || \
		{ echo "go-runtime-check: expected the factory Go module" >&2; exit 1; }; \
	test -d cmd/factory && test -d acceptance && test -f runtime/shell/readers.sh && test -f runtime/shell/config.sh && test -f runtime/shell/helpers.sh || \
		{ echo "go-runtime-check: required candidate CLI, shim or acceptance sources are missing" >&2; exit 1; }; \
		GO_WORKER_PLAN="$$(go run -mod=readonly ./internal/qualification/cmd/worker-count)" || exit $$?; \
		set -- $$GO_WORKER_PLAN; test "$$#" -eq 2; \
		GO_BASE_TEST_PROCS="$$1"; GO_HANDOFF_TEST_PROCS="$$2"; \
	sh -n runtime/shell/readers.sh; \
	bash -n runtime/shell/config.sh; \
	sh -n runtime/shell/helpers.sh; \
	git cat-file -e 76952eaa63aebd1ecd282f5ab51dd7c3627cb497^{commit} || \
		{ echo "go-runtime-check: fetch full history for the compatibility baseline" >&2; exit 1; }; \
	GO_DIRS="$$(go list -f '{{.Dir}}' ./...)" || exit 1; \
		UNFORMATTED="$$(printf '%s\n' "$$GO_DIRS" | while IFS= read -r GO_DIR; do gofmt -l "$$GO_DIR" || exit 1; done)" || exit 1; \
		test -z "$$UNFORMATTED" || { printf 'gofmt required:\n%s\n' "$$UNFORMATTED" >&2; exit 1; }; \
	bash -c '. "$$1"' "$$PWD/scripts/hooks/ginkgo-only-check.sh" "$$PWD/packs/go/hooks/ginkgo-only-check.sh"; \
	go vet ./...; \
	GO_INSTALLATION_FAMILIES='^Go factory command acceptance (Whole installation upgrade|Installed runtime root separation|Installed hook-existence library and invocation modes)( |$$)'; \
		GO_RELEASE_HANDOFF_FAMILY='^Go factory command acceptance Whole installation upgrade release candidate handoff( |$$)'; \
		GO_HISTORICAL_RECIPE_FAMILY='^Go factory command acceptance Supported predecessor recipe compatibility( |$$)'; \
		GO_LOCAL_INIT_FAMILY='^Go factory command acceptance Go explicit local source initialization( |$$)'; \
		GO_LOCAL_UPGRADE_FAMILY='^Go factory command acceptance Go explicit local source upgrade( |$$)'; \
		GO_DEFAULT_INIT_FAMILY='^Go factory command acceptance Go default packaged initialization( |$$)'; \
		GO_BASE_TEST_STATUS=0; GO_INSTALLATION_TEST_STATUS=0; GO_RELEASE_HANDOFF_TEST_STATUS=0; GO_HISTORICAL_RECIPE_TEST_STATUS=0; GO_LOCAL_INIT_TEST_STATUS=0; GO_LOCAL_UPGRADE_TEST_STATUS=0; GO_DEFAULT_INIT_TEST_STATUS=0; \
		run_acceptance() { \
			GO_REPORT_GROUP="$$1"; shift; \
			GO_ACCEPTANCE_PROCS="$$GO_BASE_TEST_PROCS"; \
			case "$$GO_REPORT_GROUP" in release-handoff|historical-recipe|local-init|local-upgrade|default-init) GO_ACCEPTANCE_PROCS="$$GO_HANDOFF_TEST_PROCS" ;; esac; \
			set -- go run -mod=readonly github.com/onsi/ginkgo/v2/ginkgo \
				--procs="$$GO_ACCEPTANCE_PROCS" --race --timeout="$(GO_RUNTIME_TEST_TIMEOUT)" \
				--keep-going --fail-on-empty --no-color "$$@"; \
			if [ -n "$$GO_RUNTIME_REPORT_DIR" ]; then \
				mkdir -p "$$GO_RUNTIME_REPORT_DIR" || return 1; \
				set -- "$$@" --json-report="$$GO_RUNTIME_REPORT_DIR/$$GO_REPORT_GROUP.json" \
					--junit-report="$$GO_RUNTIME_REPORT_DIR/$$GO_REPORT_GROUP.xml"; \
			fi; \
			"$$@" ./acceptance -- -test.count=1 -test.timeout="$(GO_RUNTIME_TEST_TIMEOUT)"; \
		}; \
		case "$(GO_RUNTIME_TEST_GROUP)" in all|base) \
			GO_ALL_PACKAGES="$$(go list ./...)" || exit 1; \
			GO_OTHER_PACKAGES="$$(printf '%s\n' "$$GO_ALL_PACKAGES" | sed '\|^github.com/anoop2811/software-factory-template/acceptance$$|d')"; \
			if [ -n "$$GO_OTHER_PACKAGES" ]; then \
				go test -race -count=1 -timeout=$(GO_RUNTIME_TEST_TIMEOUT) $$GO_OTHER_PACKAGES -ginkgo.fail-on-empty || GO_BASE_TEST_STATUS=$$?; \
			else echo "go-runtime-check: expected non-acceptance Go packages" >&2; GO_BASE_TEST_STATUS=1; fi; \
			run_acceptance base --skip="$$GO_INSTALLATION_FAMILIES|$$GO_HISTORICAL_RECIPE_FAMILY|$$GO_LOCAL_INIT_FAMILY|$$GO_LOCAL_UPGRADE_FAMILY|$$GO_DEFAULT_INIT_FAMILY" || GO_BASE_TEST_STATUS=$$? ;; esac; \
		case "$(GO_RUNTIME_TEST_GROUP)" in all|installation) \
			go test -race -count=1 -timeout=$(GO_RUNTIME_TEST_TIMEOUT) ./acceptance -ginkgo.focus="$$GO_INSTALLATION_FAMILIES" -ginkgo.skip="$$GO_RELEASE_HANDOFF_FAMILY" -ginkgo.fail-on-empty || GO_INSTALLATION_TEST_STATUS=$$? ;; esac; \
		case "$(GO_RUNTIME_TEST_GROUP)" in all|release-handoff) \
			run_acceptance release-handoff --focus="$$GO_RELEASE_HANDOFF_FAMILY" || GO_RELEASE_HANDOFF_TEST_STATUS=$$? ;; esac; \
		case "$(GO_RUNTIME_TEST_GROUP)" in all|historical-recipe) \
			run_acceptance historical-recipe --focus="$$GO_HISTORICAL_RECIPE_FAMILY" || GO_HISTORICAL_RECIPE_TEST_STATUS=$$? ;; esac; \
		case "$(GO_RUNTIME_TEST_GROUP)" in all|local-init) \
			run_acceptance local-init --focus="$$GO_LOCAL_INIT_FAMILY" || GO_LOCAL_INIT_TEST_STATUS=$$? ;; esac; \
		case "$(GO_RUNTIME_TEST_GROUP)" in all|local-upgrade) \
			run_acceptance local-upgrade --focus="$$GO_LOCAL_UPGRADE_FAMILY" || GO_LOCAL_UPGRADE_TEST_STATUS=$$? ;; esac; \
		case "$(GO_RUNTIME_TEST_GROUP)" in all|default-init) \
			run_acceptance default-init --focus="$$GO_DEFAULT_INIT_FAMILY" || GO_DEFAULT_INIT_TEST_STATUS=$$? ;; esac; \
		if [ "$$GO_BASE_TEST_STATUS" -ne 0 ]; then exit "$$GO_BASE_TEST_STATUS"; fi; \
		if [ "$$GO_INSTALLATION_TEST_STATUS" -ne 0 ]; then exit "$$GO_INSTALLATION_TEST_STATUS"; fi; \
		if [ "$$GO_RELEASE_HANDOFF_TEST_STATUS" -ne 0 ]; then exit "$$GO_RELEASE_HANDOFF_TEST_STATUS"; fi; \
		if [ "$$GO_HISTORICAL_RECIPE_TEST_STATUS" -ne 0 ]; then exit "$$GO_HISTORICAL_RECIPE_TEST_STATUS"; fi; \
		if [ "$$GO_LOCAL_INIT_TEST_STATUS" -ne 0 ]; then exit "$$GO_LOCAL_INIT_TEST_STATUS"; fi; \
		if [ "$$GO_LOCAL_UPGRADE_TEST_STATUS" -ne 0 ]; then exit "$$GO_LOCAL_UPGRADE_TEST_STATUS"; fi; \
		exit "$$GO_DEFAULT_INIT_TEST_STATUS"
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
check: selftest budget-selftest loop-selftest adversarial-review-selftest go-runtime-check
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

sync-harnesses:
	$(MAKE) sync-opencode
	$(MAKE) sync-claude
	$(MAKE) sync-codex

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
