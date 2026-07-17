# Software Factory Template — Decision Log

Decisions about the template itself. One entry per decision, recorded before
the code that implements it. The template's adopters keep their own log; this
one governs only this repository.

## Decision 1 (2026-07-16): MIT license; working name "software-factory-template"

What: The template is released under the MIT license. The repository name
stays `software-factory-template` until a public name is ratified at release.

Why: A template's value is adoption; MIT removes every integration question a
prospective adopter's counsel could raise. The name is deliberately literal —
memorable branding is a release-time decision, not a build-time one.

Provenance: founder decision, 2026-07-16.

## Decision 2 (2026-07-16): Runtime configuration file replaces install-time placeholder substitution

What: Hooks and scripts read project-specific values (protected paths, test
file patterns, decision-log path, citation prefix, docs root, language packs,
check command) from a `factory.yaml` at the repository root, parsed by
`scripts/lib/config.sh`. The `__PLACEHOLDER__` sed-substitution mechanism in
`setup.sh` is removed. `factory.yaml` uses a deliberately constrained format:
flat `key: value` pairs, one per line, space-separated lists, no nesting.

Why: Substituted placeholders fork every adopter from the template at install
time — upgrades require re-substitution and diff archaeology, and the
template's own hooks cannot run (and therefore cannot be tested) in the
template repository itself. With runtime config, hook files stay byte-identical
between the template and every adopter: upgrades are file copies, and the
template can dogfood its own gates. The constrained format keeps the parser
~20 lines of POSIX shell with no yq/jq dependency for configuration.

Boundary: `factory.yaml` is configuration, not policy. A key that changes what
a hook enforces (e.g., weakening `protected_paths`) is a governance change in
the adopter's repository and should be treated as such by their review process.

Provenance: extraction review, 2026-07-16 — the placeholder mechanism was
identified as the reason the extracted template had already drifted from its
originating factory with no upgrade path.

## Decision 3 (2026-07-16): Language-agnostic core plus one blessed stack per language, with maturity labels

What: The template splits into a core (contract, roles, harness canon and
adapters, commit/decision/push gates, docs structure) that never mentions a
language, and `packs/<language>/` directories that carry opinionated stack
choices: Go (Ginkgo+Gomega, golangci-lint, gosec, govulncheck, gremlins),
TypeScript (Vitest, ESLint flat config, Stryker), Java/Spring Boot (JUnit 5 +
AssertJ, Checkstyle + ErrorProne, PIT). One blessed stack per language — no
alternatives matrix. Every pack carries a maturity label: `battle-tested`
(a real project shipped under it), `beta` (adopted by at least one real
repository), `experimental` (fixtures only). Labels change only on evidence.

Why: Opinionation is the product — a template that supports everything
enforces nothing. Maturity labels apply the Verification Contract to the
roadmap itself: claiming a pack works without a real adopter is a claim
without observation, and the label states exactly what has been observed.

Provenance: founder direction on multi-language support, 2026-07-16.

## Decision 4 (2026-07-16): Public landing page at softwareaifactory.sh, served from the repository root

What: A single self-contained `index.html` at the repository root, plus a
`CNAME` file for GitHub Pages custom-domain hosting at `softwareaifactory.sh`.
No external requests (fonts, scripts, analytics — none); the page works
offline and adds zero tracking. factory-init does not copy `index.html` or
`CNAME` into adopter repositories.

Why: The template needs one public page that states the thesis (computational
controls, proven gates, honest claims) and routes to GitHub. Root-level Pages
hosting requires no build step, no branch, and no third-party service beyond
the repository host itself — consistent with the template's zero-dependency
stance.

Provenance: founder purchased the domain and requested the page, 2026-07-16.

## Decision 5 (2026-07-16): Install channel is a transparent, fetch-only bootstrap

What: `install.sh` at the repository root (served at `softwareaifactory.sh/install.sh`
once Pages is live) clones the template at a pinned ref into `$FACTORY_HOME`
and prints the `factory-init` command. It executes nothing it downloads,
touches nothing outside its target directory, and never uses sudo. The
landing page shows the one-liner next to a download-inspect-run alternative.
From the first tagged release, the default ref is that tag, never a moving
branch.

Why: a curl-pipe installer is the friendliest install and also the pattern a
careful engineer distrusts most. The resolution is to make the bootstrap
fetch-only, pinned, and short enough to actually read — and to say so where
the command is offered.

Provenance: founder request for a curl-based install, 2026-07-16.

## Decision 6 (2026-07-16): Existence checks verify git-tracking, not just presence

What: `hook-existence-check.sh` asserts that each required script is tracked
by git (`git ls-files --error-unmatch`), not merely that the file exists on
disk. A file present in a working tree but never committed passes every local
check and then vanishes in CI's clean clone. The tracked pre-push hook
(`.githooks/pre-push`) is enrolled in this list.

Why: an untracked-but-present enforcement script is a silent hole — the gate
it belongs to fails open in every fresh checkout while looking healthy
locally. Checking tracking closes the "works on my machine, missing in a clean
clone" failure class that this repository itself hit.

Provenance: CI failure on the first push to main, 2026-07-16.

## Decision 7 (2026-07-16): The decision-log gate skips merge commits

What: `decision-log-gate.sh` ignores commits with two or more parents. A merge
commit authors no new change; the governance change it carries is attributed
to the real commit, which the gate checks on its own.

Why: CI checks out a synthetic `refs/pull/N/merge` commit whose message is
`Merge ... into ...`. Its diff against the base includes the branch's
governance-path changes, but its message references no Decision, so the gate
failed a merge for changes it only inherited. This passed locally and in `act`
(both check out the branch head, not a merge ref) and failed only on GitHub.

Provenance: PR #1 CI failure that reproduced only under a merge ref, 2026-07-16.

## Decision 8 (2026-07-16): One-shot init, and factory-init works end-to-end

What: `install.sh init` (`curl … | sh -s -- init`) fetches the template and
then runs `factory-init` against the current directory in one step. The bare
command stays fetch-only; the `init` word is explicit consent to modify the
current repo. To make this work, `factory-init`'s prompts read from `/dev/tty`
so they survive a pipe, and the copy manifest was reconciled with the current
file layout.

Why: the one-shot flow forced the first real end-to-end run of `factory-init`,
which surfaced that it had never completed: an interactive-prompt path
incompatible with pipes, a `${VAR^^}` expansion that fails on bash 3.2, a
target-path resolver that embedded a newline on existing directories, and a
copy manifest missing `scripts/lib/config.sh` (sourced by every hook),
`scripts/selftest/run.sh` (the attestation), `scripts/pre-push-check.sh`, and
`.githooks/pre-push`, plus a stale `.golangci.yml` reference left by the
core/packs split. All fixed; a scratch-repo run now completes with the
break/fix attestation passing (selftest 17/17) inside the target.

Provenance: founder request for a one-shot installer, 2026-07-16.

## Decision 9 (2026-07-16): factory-init installs a language pack that arms the gates

What: `factory-init` takes `--pack go|typescript|java` (and prompts for one on
a tty). Selecting a pack merges its `test_file_patterns` and `check_command`
from `packs/<lang>/pack.yaml` into the generated `factory.yaml` and copies the
real files the pack ships (Go: `.golangci.yml`, `ginkgo-only-check.sh`, a CI
workflow). `install.sh init --pack <lang>` passes straight through. Pack
`check_command` values are now self-contained shell commands, not `make check`
— the value is `eval`'d by the gates, so depending on Makefile-target merging
was fragile.

Why: before this, `init` left `test_file_patterns` and `check_command` empty,
so the test-edit hook and the diff-aware check were inert until hand-editing.
A pack makes onboarding actually arm the gates for the language. Building it
surfaced a real defect: pack patterns were double-escaped (`_test\\.go`), so
`grep -E` matched nothing — the hook was silently disarmed. Fixed to single
backslash, with a selftest case per pack that fails if a pattern stops
denying its sample test file.

Honesty: only Go is battle-tested. TypeScript and Java arm their test patterns
and check command but ship no linter/CI configs yet, and say so at install.

Provenance: founder question — should install offer go/typescript/java? —
2026-07-16.

## Decision 10 (2026-07-16): Config references are project-agnostic; a glossary and onboarding depth are added

What: The docs and canon describe configuration by the `factory.yaml` key that
sets it (`protected_paths`, `docs_root`, `test_file_patterns`, `citation_prefix`)
rather than by any single project's paths, and every unsubstituted placeholder
is either a live install-time slot or removed. The spec-source directory slot is
renamed `__DOCS_ROOT__` and added to `factory-init`'s substitution list; a stale
`__CITATION_PREFIX__` example that no substitution filled is replaced with a
concrete path. A `docs/GLOSSARY.md` defines the load-bearing terms, `wiki/README.md`
explains the agent-maintained wiki, and `CONCEPTS.md`, `ADAPTING.md`, `HOOKS.md`,
and `README.md` gain a two-config-layers explanation, a full `factory.yaml`
example, a hook-authoring walkthrough, and per-hook configuration keys.

Why: a placeholder that no code substitutes ships as literal text — `__DOCS_ROOT__`
reached `opencode.json`'s permission paths verbatim, and an empty spec-source
answer would have expanded its glob to `/**`, granting the repository root. Naming
config by its `factory.yaml` key rather than an example path makes the docs read
the same for every adopter and removes the drift where a doc names a directory a
given project happens to use. The glossary and onboarding depth close the gap
between the concepts the docs assume and the ones a first-time adopter has.

Provenance: docs review, 2026-07-16 — flagged undefined terms, thin onboarding,
and config references pinned to specific paths; the dead-placeholder class was
found while resolving them.

## Decision 11 (2026-07-16): Community-health files, with conduct reports routed through GitHub's private channel

What: The repository adds the standard open-source community files —
`CODE_OF_CONDUCT.md` (Contributor Covenant 2.1), a pull-request template, and
bug-report and feature-request issue forms with a template `config.yml` that
disables blank issues and routes security reports to the private advisory flow.
The README gains a Contributing section linking all three of CONTRIBUTING,
CODE_OF_CONDUCT, and SECURITY. Code-of-conduct reports are routed through
GitHub's private "Report a vulnerability" advisory form rather than a published
email address.

Why: an inviting project states its standards and gives contributors a shaped
path in. The issue and PR templates carry the factory's own discipline into the
contribution flow — the bug form asks for a break/fix reproduction, the PR
checklist asks for the Decision reference, the sync step, and the fixture. A
published conduct-report email is a durable identity and spam surface; the
private advisory form gives reporters a confidential channel to the maintainers
with nothing new exposed. An adopter who wants a dedicated address can set one.

Provenance: founder direction, 2026-07-16 — make the repository welcoming to
contributions with the docs a strong open-source project carries.

## Decision 12 (2026-07-17): The Java/Spring Boot pack reaches Go parity on a modernized, verified stack

What: The `java` pack now ships the same class of artifacts the Go pack does —
a CI workflow (`workflows/ci.yml`), a root config the adopter applies
(`quality.gradle`), `Makefile.pack` targets, and a dialect gate
(`hooks/junit5-only-check.sh`, with a break/fix fixture in the selftest). The
blessed stack named in Decision 3 is amended to the current best-of-breed,
all open-source and verified 2026-07-17 against each tool's release page:
Spotless 8.8.0 + palantir-java-format 2.96.0 (replacing Checkstyle — auto-fix
over nagging), Error Prone 2.50.0, SpotBugs 6.5.x + find-sec-bugs 1.14.0,
OSV-Scanner (replacing OWASP Dependency-Check — the `govulncheck` analog, no
NVD API key), PIT 1.19.0 + pitest-junit5-plugin 1.2.2, and Testcontainers 2.x
added for real integration tests. `factory-init` gained a JDK-version prompt,
a generalized pack-file copy, and — fixing a latent bug that affected the Go
pack too — substitution of `__PROTECTED_PATH__` in the installed pack workflow.

Why: a pack that reads as using dated tooling undercuts the template's whole
claim. Checkstyle-for-formatting and OWASP Dependency-Check are still fine but
carry friction (manual style rules; an NVD API key and CPE false positives)
that the modern equivalents remove. Every version was resolved against the
tool's release page rather than from memory, per the project's standing rule.
The stack now lines up category-for-category with Go (format, correctness,
security, deps, mutation), so the two packs are conceptually one design.

Honesty: the `java` pack stays `experimental`. The label tracks adoption, not
completeness — the full stack and CI ship, but no real repository has adopted
it, so it cannot claim more. This refines Decision 3's gloss ("fixtures only")
which no longer fits a complete-but-unadopted pack.

Provenance: founder direction, 2026-07-17 — build the Java pack to Go parity,
and first confirm the tools are current best-of-breed and open-source, not
dated. Versions verified via each tool's release page, 2026-07-17.

## Decision 13 (2026-07-17): The TypeScript pack reaches Go/Java parity on a Biome-centered stack

What: The `typescript` pack now ships the same class of artifacts as the Go and
Java packs — a CI workflow, root config (`biome.json`, `stryker.config.json`),
`Makefile.pack`, and a dialect gate (`hooks/vitest-only-check.sh`, with a
break/fix fixture in the selftest). The blessed stack named in Decision 3 is
amended to the current best-of-breed, all open-source and verified 2026-07-17:
Biome 2.5.4 (format + lint in one fast tool, replacing ESLint + Prettier),
`tsc --noEmit` for type correctness (the adopter's own TypeScript), Vitest
4.1.10, Stryker 9.6.1 with the Vitest runner, and OSV-Scanner for dependency
CVEs (shared with the Java pack). Package manager is npm; Node.js 24 (Active
LTS). `factory-init` gained a Node-version prompt and `__NODE_VERSION__`
substitution.

Why: Biome collapses formatting and linting into one Rust tool that is 25-35x
faster than ESLint + Prettier and needs no separate formatter — the same
"auto-fix over nagging, one tool" move the Java pack made with Spotless. `tsc`
stays the type ground truth. CI pins the tools the pack introduces — Biome and
Stryker — at their `npx` call, while type-checking and tests run the adopter's
own TypeScript and Vitest from `node_modules`, so a missing binary fails fast
instead of silently downloading an unrelated package (notably, an unrelated
`tsc` package exists on npm). The pack lines up category-for-category with Go
and Java (format, types/correctness, tests, mutation, deps), making the three
packs one design.

Honesty: the `typescript` pack stays `experimental` — the full stack ships but
no real repository has adopted it, per the label semantics clarified in
Decision 12.

Provenance: founder direction, 2026-07-17 — build the TypeScript pack to
parity on the absolute-best toolchain, choosing Biome and npm. Versions
verified via each tool's release page, 2026-07-17.

## Decision 14 (2026-07-17): A pure-shell `factory` dispatcher and a `factory doctor` health command; no compiled CLI

What: A single shell entrypoint `factory` dispatches subcommands
(`factory init | doctor | check | selftest`). `factory doctor` reports the
health of an installed factory: it classifies every gate as armed / inert /
stale from `factory.yaml`, verifies each hook exists and is executable, checks
the generated adapters for drift, checks that `protected_paths` are covered by
CODEOWNERS, and runs the break/fix self-test so the adopter watches each gate
fire. `make` targets become thin aliases. There is no compiled binary.

Why: the template's value is adoption, and adoption needs trust — an adopter
has to see that the gates are live in their repo, not just installed. A Go (or
any compiled) CLI was considered and rejected: it would break three properties
that are the product's trust story — the enforcement layer is auditable plain
shell you can read, it has zero install dependency and is language-agnostic,
and the hooks must stay shell because three harnesses invoke them as shell
commands and read `factory.yaml` at runtime (Decision 2). A binary would either
ship as a supply-chain artifact the template itself warns against, or force a
Go toolchain onto Java/TypeScript adopters. A shell dispatcher gives the clean
`factory <verb>` surface without any of that cost. Revisit a binary only if a
real adopter needs Windows support or the orchestration outgrows shell — and
even then the hooks stay shell and the binary stays optional.

Provenance: founder question — do we need a Go CLI instead of Makefile
commands? — 2026-07-17.

## Decision 15 (2026-07-17): wiki-lint operationalizes the LLM-maintained wiki pattern

What: `scripts/hooks/wiki-lint.sh` enforces the "lint" operation of the
LLM-maintained wiki pattern (raw sources -> agent-written wiki -> lint). v1
requires every `wiki/` content page to carry provenance (a `file:line`
citation, a URL with a date, or `observed YYYY-MM-DD`) and every wiki-local
markdown link and `[[wikilink]]` to resolve. It reads `wiki_root` from
`factory.yaml` (default `wiki`), skips when there is no wiki, and runs in CI,
`make check`, and `factory doctor` with a break/fix fixture in the self-test.
Orphan detection and source-drift/staleness are planned v2.

Why: an agent can write a wiki quickly but cannot be trusted to keep every page
cited and every cross-reference real — so an LLM-maintained wiki is only
trustworthy if a deterministic gate makes a dishonest page fail the build. That
gate is the template's whole thesis applied to knowledge: ingest and query are
the model's job, lint is ours. Until this shipped, `wiki/README.md` claimed
pages were "lint-gated at merge" with nothing enforcing it — an overclaim this
decision removes by making it true. The pattern is Karpathy's LLM-wiki; we do
not advertise its benefit on the landing page until the lint that earns the
claim is in place.

Provenance: founder direction, 2026-07-17 — actually use the wiki pattern for
adopter projects, not just ship an empty folder and a role prompt. Pattern:
Andrej Karpathy's LLM-wiki gist (https://gist.github.com/karpathy/442a6bf555914893e9891c11519de94f,
read 2026-07-17).

## Decision 16 (2026-07-17): `factory upgrade` — framework-only, report the rest

What: `factory upgrade [--ref <tag>] [--source <dir>]` re-fetches the template
and refreshes the byte-identical framework files an adopter already has — the
hooks, `scripts/`, the `factory` dispatcher, `factory-doctor`, `.githooks`, and
installed pack dialect hooks. It never introduces new files, never touches
`factory.yaml`, the adopter's content (`wiki/` pages, `memory/lessons/`,
`specs/`, `docs/DECISION_LOG.md`), or their code, and never overwrites
identity/customizable files (`opencode.json`, agent prompts, `AGENTS.md`,
`README.md`, `CODEOWNERS`, `Makefile`) — it *reports* which of those differ from
upstream so the adopter reconciles them. It records `.factory-version`, runs
`factory doctor`, and leaves everything as an uncommitted diff for review.

Why: Decision 2 (runtime config) is what makes this safe — the hooks carry no
placeholders, so refreshing them is a byte-identical copy, and `factory.config`
holds the substitution values if a future version needs them. Framework-only is
the conservative default: it can update where behaviour lives (the gates)
without any chance of clobbering an adopter's customizations. Full
re-substitution of identity files was considered and deferred; framework-only +
report never destroys work. The copy is an atomic rename, so the upgrader can
safely upgrade itself mid-run.

Provenance: founder request — do we need a way to upgrade the template in an
existing repo? — 2026-07-17.

## Decision 17 (2026-07-17): wiki-lint v2 — reachability and opt-in freshness

What: `wiki-lint` gains the two checks deferred from Decision 15, completing
Karpathy's "lint" operation. **Reachability** (always on when an index exists):
every content page must be linked from some other wiki page, or it is an
orphan and fails. It is gated on the presence of a `README.md`/`INDEX.md` so an
index-less wiki has no false positives. **Freshness** (opt-in via
`wiki_staleness: true`, default false): a content page whose cited source file
has a newer last-commit time than the page itself is flagged stale. Both ship
with break/fix fixtures (the staleness one drives git commit timestamps), and
`factory doctor` reports the mode.

Why: an orphaned page is knowledge nothing can reach — the compounding graph
has a hole. And a page whose source moved on is the "contradiction" Karpathy's
lint is meant to catch; making it fail forces a re-review, the same discipline
the Verification Contract applies to claims. Staleness is opt-in because it is
the most aggressive check — it fires on every source change until the page is
re-touched — so a team enables it deliberately. Reachability is gated on an
index so it never punishes a wiki that has not adopted one.

Provenance: founder direction, 2026-07-17 — build the deferred wiki-lint v2
(orphan detection + source-drift/staleness).

## Decision 18 (2026-07-17): install-manifest files must be git-tracked; a hook enforces it

What: `.opencode/package.json` (which declares the opencode plugin's dependency)
and `.opencode/.gitignore` were ignored by `.opencode/.gitignore` itself, so
they lived only in the working tree and were absent from a clean clone.
factory-init copies them unconditionally, so a real `curl … | sh -s -- init`
aborted on `cp: .opencode/package.json: No such file or directory`. Both are now
tracked (the `.opencode/.gitignore` no longer ignores `package.json` or itself),
and `scripts/hooks/copy-manifest-check.sh` fails the build if any file
factory-init copies unconditionally is not tracked by git.

Why: this is Decision 6's failure class again — a file present locally but
untracked passes every local test and then vanishes in the clean clone an
adopter installs from. Decision 6 fixed it for the hooks; nothing generalized
the rule to the whole install manifest, so it recurred against `.opencode/`.
The new hook closes the class: the installer's `cp` list is now verified against
git at CI time, with a break/fix fixture.

Provenance: founder bug report, 2026-07-17 — a live `curl … | sh -s -- init
--pack go` aborted copying `.opencode/package.json`.

## Decision 19 (2026-07-17): factory-init installs multiple packs and only asks for relevant versions

What: `factory-init` accepts more than one language pack — `--pack go,typescript`
(comma-separated) or a repeated `--pack` — because real apps are polyglot (a Go
backend, a React/TypeScript frontend). Packs are selected before the version
prompts, and only the versions the selected packs need are asked (a Go-only
install no longer prompts for a JDK or Node version). Multiple packs merge
cleanly: `test_file_patterns` becomes the union, `check_command` the packs'
checks joined with `&&`, and each pack's root config, dialect hook, per-language
CI workflow, and version key install side by side. `language_packs` records the
space-separated set.

Why: the single-pack model forced a false choice on any multi-language repo and
asked for versions of languages the project doesn't use — a confusing, sloppy
first impression. The data model already allowed it (`language_packs` was always
space-separated); only the installer lagged. Merging by union/`&&` means the
test-edit hook denies test files in every selected language and the diff-aware
check runs every language's suite.

Provenance: founder question — a Go backend with a React/TS frontend still gets
asked for Java and Node versions; how do we handle polyglot? — 2026-07-17.

## Decision 20 (2026-07-17): frameworks ride on language packs — awareness, not new packs

What: Frameworks do not get their own packs. The TypeScript pack's `biome.json`
enables Biome's `react` and `vue` linter domains (Biome auto-applies a domain's
rules when it sees the framework in `package.json`), so a React or Vue app gets
framework-aware linting from the TypeScript pack. Spring Boot uses the Java pack
unchanged — its JUnit 5 + Testcontainers stack is Spring Boot's own blessed
testing approach. `factory-init` detects React, Vue, and Spring Boot (from
`package.json` / `pom.xml` / `build.gradle`) and prints a hint pointing at the
right pack.

Why: a pack arms language-level knobs (`test_file_patterns`, `check_command`)
and ships a language's stack; a framework adds libraries on top but does not
change what a test file is or what "run the checks" means. A React/Vue/Next/
Spring-Boot/Quarkus pack matrix is exactly the alternatives explosion Decision 3
avoids. Biome's domains give real React/Vue rules with no new pack and no false
positives on non-framework code (the rules only match framework patterns). A
framework-specific invariant beyond that is a custom dialect hook, the template's
standard extension point — not a pack.

Provenance: founder direction, 2026-07-17 — add React/Vue-aware rules and
framework detection hints; frameworks like Spring Boot ride on the language pack.
Biome domains verified against biomejs.dev/linter/domains, 2026-07-17.
