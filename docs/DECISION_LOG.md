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

## Decision 21 (2026-07-17): commit-message-lint matches claim words at word boundaries

What: `commit-message-lint.sh` matched `verified`/`fixed`/`works` as substrings,
so it false-flagged ordinary words — "frameworks" tripped the "works" claim
rule, "prefixed" the "fixed" rule, "workspace" the "works" rule. The match is
now word-bounded: `(^|[^[:alnum:]_])(verified|fixed|works)([^[:alnum:]_]|$)`.
BSD grep (macOS) lacks `\b`, so the boundary is expressed with non-word
neighbours and string anchors, which is portable. A break/fix fixture proves a
message containing "frameworks" passes while a bare "the retry logic works"
still fails.

Why: a gate that fires on innocent words is a false positive that erodes trust
in the whole system — contributors start reaching for awkward synonyms to dodge
the lint (which this project did, once). The claim rule should catch the claim,
not the letters. Found while a commit describing framework awareness was
rejected for the word "frameworks".

Provenance: observed 2026-07-17 — a `feat:` commit body containing "frameworks"
was rejected by commit-message-lint as an uncited "works" claim.

## Decision 22 (2026-07-17): `install.sh upgrade` upgrades the repo you're in, curl-able

What: `curl … | sh -s -- upgrade` refreshes the machine-wide template cache
(`$FACTORY_HOME`) and then applies the framework update to the current directory
— symmetric with `install.sh init`, which also acts on the current directory. It
runs `factory-upgrade.sh --source "$FACTORY_HOME"` against the repo you invoked
it from, landing a reviewable diff. `./factory upgrade` remains the equivalent
local command for a repo that is already set up.

Why: the first design made upgrading a two-step dance — curl to refresh a hidden
cache, then `cd` and run `./factory upgrade` — which felt strange, because `init`
already operates on the current directory. `upgrade` should be symmetric. The
one thing that genuinely cannot be a single machine-wide command is upgrading
*every* repo at once: each repo owns committed, governance-gated framework files,
so they are upgraded where you stand — but "the repo I'm in" is exactly one
curl away, as it should be.

Provenance: founder question — why can't `install --upgrade` upgrade the
folder I'm already in? — 2026-07-17.

## Decision 23 (2026-07-19): opt-in `economy` cost profile — a third model tier across all three harnesses

What: `factory-init` offers a `COST_PROFILE` (`standard` default, or `economy`),
recorded with a new `ECONOMY_MODEL` in `factory.config`. Under `economy`, the
low-stakes roles — `refactorer`, `wiki-maintainer`, and the opencode
`small_model` — route to a cheaper third tier; `spec-writer` and `reviewer` stay
on the frontier model and `implementer` on the default. Under `standard` the
economy-eligible roles collapse to the default model, so behaviour is unchanged.
Routing reaches all three harnesses: opencode carries the per-role model
natively; `sync-claude` already maps it onto Claude subagents; `sync-codex` now
emits a per-agent `model` for a native Codex id (a cross-provider slug or unset
placeholder is omitted, so the agent inherits — keeping the committed `.codex`
clean). A self-test fixture proves the Codex emission and its inherit fallback.
The intent and phased plan live in `docs/COST_AND_TOKENS.md`.

Why: cost is the first question adopters ask, and the two-tier model routing was
already in place — the economy tier is a third tier plus a profile switch, not a
new subsystem. Keeping it opt-in preserves the simple default; keeping the
review path (`spec-writer`, `reviewer`) on the frontier model is what makes
running a cheaper implementer safe later (Phase 4, eval-gated). Codex per-agent
`model` is supported in agent TOML files, verified against
learn.chatgpt.com/docs/agent-configuration/subagents (2026-07-19), so parity
across the three harnesses is real, not aspirational.

Provenance: founder direction — build Phase 1 of the cost plan and make it work
for opencode, Claude, and Codex — 2026-07-19. Verified this session: end-to-end
`factory-init` runs (economy + standard) routed each role as expected across
opencode.json, `.claude/agents`, and `.codex/agents`; `bash scripts/selftest/run.sh`
reported "37 passed, 0 failed"; `make check-drift` exited 0.

## Decision 24 (2026-07-19): per-harness intelligent model defaults, keyed by role tier

What: each harness carries its own per-tier models instead of one shared string
translated per harness. `factory.config` gains `CLAUDE_{FRONTIER,DEFAULT,ECONOMY}_MODEL`
and `CODEX_{FRONTIER,DEFAULT,ECONOMY}_MODEL` alongside the opencode
`{FRONTIER,DEFAULT,ECONOMY}_MODEL`; `factory-init` ships them as verified defaults
(opencode GLM 5.2 / GLM 5.2 / Qwen3-Coder; Codex gpt-5.6-sol / -terra / -luna;
Claude opus-4-8 / sonnet-4-6 / haiku-4-5), overridable. A new `scripts/lib/roles.sh`
maps role → tier (spec-writer/reviewer → frontier, refactorer/wiki-maintainer →
economy, else default); `sync-claude`/`sync-codex` read that tier's model for their
harness from `factory.config`, falling back to `inherit` when unset (so the
template repo, which has no `factory.config`, keeps clean committed adapters).
Under `standard` each harness's economy tier collapses to its default.

Why: opencode's frontier and default are the same model (GLM 5.2), so a generated
adapter cannot recover a role's tier from the substituted model string — tier has
to come from the role. And the three harnesses have distinct native namespaces
(OpenRouter, OpenAI, Anthropic), so one shared model string cannot give all three
sensible per-tier routing; per-harness defaults give Claude and Codex real
frontier/default/economy ladders out of the box, not just opencode. Every default
model was verified current against its source (OpenRouter, Codex models doc,
Anthropic) rather than assumed.

Provenance: founder direction — set intelligent per-harness model defaults
(opencode GLM 5.2 + Qwen3-Coder economy; Codex sol/terra/luna; Claude
opus/sonnet/haiku) — 2026-07-19. Models verified: OpenRouter `qwen/qwen3-coder`
($0.22/$1.80) and GLM 5.2 pricing; Codex gpt-5.6-sol/terra/luna via
learn.chatgpt.com/docs/models; Anthropic ids from the model docs. Verified this
session: end-to-end `factory-init` (economy + standard) routed every role across
all three harness configs; `bash scripts/selftest/run.sh` reported "40 passed,
0 failed"; `make check-drift` exited 0.

## Decision 25 (2026-07-20): factory.config is the single source of truth for models; reconfigure via one command

What: `factory.config` now holds the raw (uncollapsed) per-tier models for all
three harnesses (`OPENCODE_*`, `CLAUDE_*`, `CODEX_*`) plus `COST_PROFILE`, and
`make sync-harnesses` applies them to every harness — including opencode, via a
new `scripts/sync-opencode.sh` that writes `opencode.json` and the
`.opencode/agent/*.md` models. The standard/economy collapse moved from init
time to sync time: `resolve_tier` (in `scripts/lib/roles.sh`) reads `COST_PROFILE`
and collapses the economy tier to default unless the profile is `economy`. So
reconfiguring later is one edit to `factory.config` (a model, or flipping the
profile) and one `make sync-harnesses`; `factory-init` runs the same sync at the
end so a fresh repo is wired out of the box.

Why: before this, the reconfiguration story was asymmetric and had a footgun —
opencode models lived only in `opencode.json` (editing `factory.config` did
nothing for them), and `COST_PROFILE` was baked at init, so flipping it after
install had no effect. Both surprise adopters. Making sync the single apply-point
for all harnesses, with the collapse at sync time, means one config file and one
command reconfigure everything, matching the factory's usual shape.

Provenance: founder direction — make "configure later" clean rather than just
documented — 2026-07-20. Verified this session: a break/fix self-test drives an
economy config, a profile flip to standard, and a single-model change, asserting
re-routing across `opencode.json`, `.claude/agents`, and `.codex/agents`;
`bash scripts/selftest/run.sh` reported "47 passed, 0 failed"; an end-to-end
`factory-init` applied models to all three harnesses and a later `factory.config`
edit + sync re-routed them; `make check-drift` exited 0.

## Decision 26 (2026-07-20): installer pins to the release tag by default; --ref overrides it

What: `install.sh` defaults `FACTORY_REF` to the pinned release tag (`v0.1.0`)
instead of `main`, so `curl … | sh` is reproducible (Decision 5). A `--ref
<branch-or-tag>` flag overrides it — e.g. `init --ref main` for the latest — and
is extracted from the args before or after the verb, so it works with `init`,
`upgrade`, or a bare fetch, and passes nothing extra to `factory-init`.
Precedence: `--ref` beats the `FACTORY_REF` env var beats the pinned default.

Why: pinning gives adopters a known-good, reproducible install rather than
whatever is on `main` at that moment; future work reaches users when the next
tag is cut and this default is bumped. The `FACTORY_REF` env var was already an
override, but over a curl pipe it must sit on the `sh` invocation, not before
`curl` — a silent footgun. The flag is pipe-safe and discoverable, and mirrors
the `--ref` already on `./factory upgrade`.

Provenance: founder direction — pin the release and add a pipe-safe override to
install from main — 2026-07-20. Verified this session: `shellcheck -S warning
install.sh` passed; an isolated parse test covered flag-before-verb,
flag-after-verb, `--ref=` form, bare fetch, missing-value error (exit 2), env-var
fallback, and flag-beats-env precedence — each resolved the ref and passthrough
args as expected.

## Decision 27 (2026-07-20): `factory report` — an honest cost report, no vanity number

What: a `factory report` subcommand (`scripts/factory-report.sh`) prints three
separated registers — facts the factory computes itself (deterministic gates
installed at 0 model tokens, cost profile and model tiers, and gate *blocks*
recorded), one clearly-labeled review-spend estimate, and a pointer to the
harness for measured token spend. It never prints a "tokens saved" headline. The
blocks come from a new best-effort logger, `scripts/lib/events.sh`
(`factory_log_event`), which the five interactive blocking hooks (test-edit-denial,
commit-message-lint, decision-log-gate, direct-main-push-block,
pending-lessons-push-block) call right before they exit non-zero. It writes to
`$FACTORY_EVENT_LOG` or `.factory/events.log` (gitignored) at the repo root, and
never fails a hook. `factory report --clear` resets the window.

Why: adopters ask "how much does this save?" and the honest answer is not a
single per-session number — that is a counterfactual comparing the run to one
that never happened, the exact vanity metric this project refuses. The report
separates what is *measured* (blocks caught, 0-token enforcement) from what is
*estimated* (review spend avoided, with its R constant visible) from what the
factory cannot know (harness token spend). The only real "saved" figure is an
A/B eval, and the report says so. Logging must never break a hook, so the logger
swallows every error and returns 0.

Provenance: founder direction — build the honest post-session cost report (full
MVP with session-catch logging), skip the dangerous-command guard for now —
2026-07-20. Verified this session: a break/fix self-test fires a gate, asserts an
event is logged, asserts `factory report` shows the block and refuses a
tokens-saved headline, and asserts `--clear` resets the log; `bash
scripts/selftest/run.sh` reported "52 passed, 0 failed"; `make check-drift`
exited 0; a manual `factory report` showed the clean-state and populated output.

## Decision 28 (2026-07-20): factory upgrade adds missing framework files, not just refreshes existing

What: `factory-upgrade.sh` now *adds* a framework file the repo is missing (when
its parent directory exists), rather than skipping any file the repo does not
already have. The framework list gains the files introduced since the earlier
releases — `scripts/lib/roles.sh`, `scripts/lib/events.sh`, `scripts/sync-opencode.sh`,
`scripts/factory-report.sh` — and the copy helper reports each as "added" vs
"updated".

Why: a repo installed before a framework file existed did not receive it on
upgrade, yet the refreshed shipped scripts source it — e.g. `sync-codex.sh` and
the hooks now source `scripts/lib/roles.sh` / `events.sh`, so an upgraded repo
that never had those libs failed with "No such file or directory". Framework
files are byte-identical and non-optional (Decision 2), so adding a missing one
heals the repo; identity/customizable files are still handled separately and
never overwritten. Only the parent directory must pre-exist, which `init`
guarantees.

Provenance: founder report — after `factory upgrade`, `sync-codex.sh` failed on a
missing `scripts/lib/roles.sh` — 2026-07-20. Verified this session: an end-to-end
upgrade of a repo missing the new libs added `roles.sh`/`events.sh`/`sync-opencode.sh`/
`factory-report.sh` and `role_tier` then resolved; a break/fix self-test asserts
upgrade adds a missing lib; `bash scripts/selftest/run.sh` reported "53 passed,
0 failed"; `make check-drift` exited 0.

## Decision 29 (2026-07-23): golden-task eval scores real agent runs via a pluggable runner

What: `golden-task-eval.sh` replaces its scoring stub with real scoring. Each task
is a directory `eval/golden-tasks/<name>/` with `task.md` (a red acceptance spec)
and `verify.sh` (the oracle, exit 0 = solved). A **runner** — contract
`runner <workdir>`, which writes an implementation into the task working copy —
produces the code; the score is the pass rate over N runs, where a run counts only
if `verify.sh` passes *and* its checksum is unchanged (the runner cannot cheat the
oracle). Scores diff against a saved baseline; a drop in any task's pass rate exits
non-zero. A deterministic mock runner (`eval/runners/mock.sh`, no model) and a
`reference-answer` task ship so the harness self-tests in CI without credentials;
`example-harness.sh` is the template for a real runner. A break/fix fixture proves
solved→pass, unsolved→fail, oracle-tamper→fail, and regression→exit 1.

Why: every *gate* was break/fix-proven, but nothing measured whether the *agents*
produce good code under the factory — the evidence layer the whole "cheaper models
are safe because the gates catch them" argument leans on. Splitting the expensive,
non-deterministic part (a live agent) into a pluggable runner keeps the scorer
deterministic and credential-free (so the factory stays self-provable) while the
real agent-quality run happens where the keys and project-specific tasks live. It
is also the foundation for eval-gated model choices (COST_AND_TOKENS Phase 4): a
role's tier drops only when the eval shows the cheaper model still passes.

Provenance: founder direction — build the eval harness (prove the agents, not just
the gates) as the next big bet — 2026-07-23. Verified this session: the eval scored
the reference task pass (1.00) and fail (0.00), caught a runner tampering the oracle
(0.00), and flagged a regression (exit 1); a break/fix self-test asserts all four;
`bash scripts/selftest/run.sh` reported "58 passed, 0 failed"; an end-to-end
`factory-init` copied the eval files, exited 0, and the adopter's `golden-task-eval`
scored the reference task 1/1; `make check-drift` exited 0.

## Decision 30 (2026-07-23): workflow recipes + workflow-lint — the cross-harness graph substrate

What: workflow "recipes" (`workflows/<name>.md`) are a plain-text graph — each
`## <node>` block declares `- role:` (a factory role or `code`) and `- kind:`
(agent | fanout | verify | edge). `scripts/hooks/workflow-lint.sh` (a new gate, in
`make check`, CI, and hook-existence-check) enforces graph hygiene on them: real
roles, plumbing as `code` edges not agents, an `edge` is `role: code`, a `fanout`
declares `over:`, and every recipe has a `verify` node. It fires only if
`workflows/` has recipes — opt-in. Two reference recipes ship (`review-diamond`,
`eval-fanout`); `AGENTS.md` points every harness's agent at `workflows/` so each
runs the same recipe with its native orchestration.

Why: verification showed only Claude Code has a committable workflow *file*;
opencode and Codex orchestrate at runtime (subagent dispatch, `spawn_agent`). So a
per-harness generated workflow file is impossible — but the factory's canonical
model still makes graph engineering cross-harness: define the graph once (a
recipe), lint the shared definition once (harness-agnostic), and let each harness
execute it natively. Most of the graph is already the factory (roles are nodes,
gates are edges, the reviewer is the verifier, models tier by role); the recipe +
lint are the only new substrate needed, and they avoid fabricating opencode/Codex
workflow formats that do not exist. Generating a Claude `.claude/workflows/*.js`
from a recipe is left as an optional Claude-only optimization.

Provenance: founder direction — build the least-common graph-engineering substrate
so it works for Claude, opencode, and Codex — 2026-07-23; grounded on verification
that only Claude has a committable workflow artifact
(learn/adurrr opencode + codex.danielvaughan orchestration, fetched 2026-07-23).
Verified this session: `workflow-lint` passed the two reference recipes (exit 0)
and flagged an unknown role, a plumbing node run as an agent, a fanout without
`over:`, and a missing verifier (exit 1); a break/fix self-test asserts a clean
recipe passes and a plumbing-agent recipe fails; `bash scripts/selftest/run.sh`
reported "60 passed, 0 failed"; `make check-drift` exited 0.
