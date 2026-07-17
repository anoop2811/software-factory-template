.PHONY: selftest doctor check eval sync-claude sync-codex sync-harnesses check-drift lint-commits prereq-check pre-push diff-aware decision-log pending-lessons

# Core factory targets — language-agnostic. Language packs contribute their
# own test/lint/build targets via packs/<language>/Makefile.pack at init time.

selftest:
	./scripts/selftest/run.sh

doctor:
	./scripts/factory-doctor.sh

check: selftest
	./scripts/citation-lint.sh
	./scripts/hooks/shared-script-enforcement.sh
	./scripts/hooks/hook-existence-check.sh
	./scripts/hooks/wiki-lint.sh

prereq-check:
	./scripts/prereq-check.sh

eval:
	./scripts/harness-structural-eval.sh --harness=opencode
	./scripts/harness-structural-eval.sh --harness=claude
	./scripts/harness-structural-eval.sh --harness=codex

sync-claude:
	./scripts/sync-claude.sh

sync-codex:
	./scripts/sync-codex.sh

sync-harnesses: sync-claude sync-codex

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

pre-push: check check-drift
	./scripts/hooks/commit-message-lint.sh HEAD || true
	./scripts/hooks/diff-aware-check.sh origin/main HEAD || true
	./scripts/hooks/decision-log-gate.sh origin/main HEAD || true
	./scripts/hooks/pending-lessons-push-block.sh
	@echo ""
	@echo "pre-push: all checks passed — run ./scripts/pre-push-check.sh for the full gate"
