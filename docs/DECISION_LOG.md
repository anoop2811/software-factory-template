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
offline and adds zero tracking. **Superseded in part by Decision 36**: the page
now loads Google Analytics, consent-gated and off by default. The rest of this
decision — root-level hosting, no build step, and factory-init never copying the
page into adopter repositories — still holds. factory-init does not copy `index.html` or
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

## Decision 31 (2026-07-26): eval baselines carry input fingerprints and go stale

What: `golden-task-eval.sh` records a fingerprint of what each score was measured
against — per task, its `task.md` plus its `verify.sh` oracle; globally, the runner,
`AGENTS.md`, and the model-tier lines from `factory.config`. When a fingerprint
differs from the baseline's, the run reports `BASELINE STALE` with the specific
invalidation reason and exits non-zero, instead of comparing scores that are not
like-for-like. Stale is distinct from both pass and regression: it means this run
cannot know. A baseline written before fingerprinting still compares, with a
warning that staleness is unchecked, so existing baselines keep working.

Why: the previous compare was silently wrong in a way that mattered. Edit an
oracle or the instructions and a task's pass rate can stay identical while
measuring something entirely different — the eval would print "no regression" and
be confidently incorrect, which is exactly the class of claim the Verification
Contract exists to prevent. A passed state must carry what it passed against, or
it decays into an assertion. `factory doctor` already classifies gates as
armed/inert/stale; this extends the same honesty to saved evidence over time.

Provenance: adapted from a public critique of state-machine factories — that each
transition should carry an input/code fingerprint, verifier version, and
invalidation reason, or the system resumes from stale "passed" state after the
code, rubric, or dependency changes (x.com/swordlight_ai reply to mfishbein,
read 2026-07-26); founder direction to adopt it, same date. Verified this session:
with an unchanged setup the eval reported no regression (exit 0); with a lowered
score it reported REGRESSION (exit 1); with the oracle edited but the score
unchanged at 1.00 it reported STALE naming the oracle (exit 1) where the previous
code would have printed "no regression"; with `AGENTS.md` changed it reported
stale for all tasks; a fingerprint-less baseline still compared and warned. Four
break/fix self-test cases cover it; `bash scripts/selftest/run.sh` reported
"63 passed, 0 failed"; `make check-drift` exited 0.

## Decision 32 (2026-07-26): doctor asks Git what it will run; the eval caps a hung runner

What: two gaps closed in already-shipped code.

1. `scripts/lib/hookspath.sh` reports what Git will *actually* execute for
   pre-push — `armed` (this repo's `.githooks`), `hijacked` (`core.hooksPath`
   points elsewhere), or `absent`. `factory doctor` reports a hijacked path as an
   INERT push gate, naming the file Git will really run and the one-line fix.
2. `golden-task-eval.sh` gains a per-run wall-clock cap (`--timeout`, default
   300s, implemented by hand since macOS ships no `timeout(1)`). A run that hits
   the cap is counted and reported as failed rather than allowed to wedge the
   suite. The runner contract and `eval/README.md` now document why: headless
   `ask` permissions do not fail cleanly.

Why: both were silent failures in code that looked correct. A populated
`.githooks/pre-push` is not evidence Git will run it — an inherited global
`core.hooksPath` redirects Git and leaves an installed-looking gate completely
dead, which is exactly the inert-gate class `factory doctor` exists to surface.
And a headless agent does not merely fail: the primary session auto-rejects `ask`
permissions (so a task fails as though the model were incapable) while a subagent
*hangs* on a permission queue nothing services — an unbounded eval would wait
forever rather than score.

Provenance: adapted from the originating factory's memory lessons on
`core.hooksPath` silently disabling repository hooks (observed 2026-07-14) and on
headless permission semantics (2026-07-05); founder direction to port them,
2026-07-26. Verified this session: `hookspath_status` returned armed, hijacked,
and absent across three sandbox repos (hermetic, ignoring global config); a fresh
`factory-init` on this machine reported the push gate INERT and named the
inherited hooks path; a runner sleeping 600s was capped at 3s and scored 0.00
with a cap note, while a normal runner still scored 1.00. Four break/fix cases
cover both; `bash scripts/selftest/run.sh` reported "67 passed, 0 failed";
`make check-drift` exited 0; `copy-manifest-check` confirmed the new lib ships.

## Decision 33 (2026-07-26): no provider is assumed — a provider picker that only seeds, and blank means inherit

What: `factory-init` asks for a `MODEL_PROVIDER` once (`inherit`, `openrouter`,
`anthropic`, `openai`, or anything else) instead of prompting for individual
OpenRouter-shaped model strings. The provider only *seeds* defaults: opencode
tiers follow the chosen provider, while Claude Code and Codex tiers always use
Anthropic and OpenAI ids because those harnesses reach nothing else. `inherit`
writes no model values at all, and `sync-opencode` then *removes* every model pin
from `opencode.json` and the role frontmatter so each harness keeps its own
configuration. A provider we do not seed (ollama, bedrock, azure) leaves the
opencode tiers blank and prints where to set them. `docs/MODELS.md` documents the
shape, example strings per provider, and which credential each one needs.

Why: the mechanism was already provider-agnostic — any string works, and blank
already meant inherit for Claude and Codex — but every default, example, and
prompt was OpenRouter, and nothing in the docs mentioned providers or credentials
at all. An adopter without an OpenRouter key got defaults that silently did not
work and no hint why. Seeding from a declared provider keeps the curated tiers for
people who want them while making "I run my own models" a first-class, one-word
answer. Stripping rather than skipping on inherit matters: an unresolved
`__DEFAULT_MODEL__` placeholder left in `opencode.json` would be read as a model
name, which is worse than no pin at all.

Provenance: founder direction — not everybody has OpenRouter, they could have many
options; the user should also choose the model — 2026-07-26. opencode's
`provider/model` string format and its 75+ providers verified against
opencode.ai/docs/models and /docs/providers, 2026-07-26. Verified this session:
end-to-end `factory-init` runs for `inherit` (all tiers blank, zero model pins and
zero placeholders left in `opencode.json` or the role files), `openrouter`,
`anthropic`, and `openai` (each seeding its own opencode tiers with native
Claude/Codex tiers alongside), and `ollama` (blank opencode tiers plus a pointer);
three break/fix self-test cases cover the inherit strip path;
`bash scripts/selftest/run.sh` reported "70 passed, 0 failed"; `make check-drift`
exited 0.

## Decision 34 (2026-07-26): a hook enforces even when its optional lib is missing

What: every hook sources `scripts/lib/events.sh` defensively — if the file is
absent it defines a no-op `factory_log_event` and carries on. `factory-doctor`
sources `lib/hookspath.sh` the same way and skips only the check that lib powers.
Load-bearing libs (`config.sh`, which supplies the patterns a gate enforces) are
still sourced strictly, because a gate that cannot read its configuration must
not pretend to work.

Why: found by simulating a real upgrade. A repository installed at v0.1.0 runs
its own `factory upgrade`, which predates the add-missing-files fix (Decision 28)
— so the hooks get refreshed to versions that source `lib/events.sh` while the
lib itself is never added. Every gate then aborted on the missing source: it
failed closed rather than open, so enforcement was not silently lost, but the
repository was hard-blocked on every commit and push. Enforcement is the hook's
job; event logging is bookkeeping, and bookkeeping must never be able to break
enforcement — the same principle `events.sh` already applies internally by
swallowing its own errors.

Provenance: founder request to check for upgrade bugs, 2026-07-26. Verified this
session by building adopters from the actual v0.1.0 and v0.1.1 tags and upgrading
them to main: before the fix, a v0.1.0 repo's refreshed `commit-message-lint` and
`direct-main-push-block` both aborted with "lib/events.sh: No such file or
directory"; after it, with `events.sh` still absent, push-block denied `main`
(exit 1) and allowed a branch (exit 0), commit-lint accepted a valid message and
rejected an invalid one, and test-edit-denial denied the implementer on a test
file (exit 2) while allowing a source file and the spec-writer. The v0.1.1 path
was clean throughout: models intact, no placeholders, `hookspath.sh` and
`workflow-lint.sh` added, and its self-test reported "63 passed, 0 failed". Two
break/fix cases now cover a missing `events.sh`; `bash scripts/selftest/run.sh`
reported "72 passed, 0 failed"; `make check-drift` exited 0.

## Decision 35 (2026-07-26): repo-local hooks are registered in factory.yaml, not in a framework file

What: `hook-existence-check.sh` reads a `local_hooks` key from `factory.yaml` — a
space-separated list of hooks the adopter wrote — and checks each one for
existence and the execute bit exactly like the shipped hooks. `factory-init`
generates the key, and ADAPTING's "writing your own hooks" step now points at it.

Why: the previous instruction told adopters to register their hook by editing
`scripts/hooks/hook-existence-check.sh`. That file is a framework file, and
`factory upgrade` refreshes every hook the template ships byte-for-byte — so the
registration was silently deleted on the adopter's next upgrade, taking with it
the CI net that catches a missing or non-executable hook. Documentation asked for
something the upgrade mechanism was guaranteed to undo. Configuration is the only
place a customization survives (Decision 2), so that is where a local hook is
declared.

Provenance: found while planning how the originating factory could consume this
template rather than fork it — it has four repository-local hooks, so it would
have hit this on its first upgrade; founder direction to fix the template bug
first, 2026-07-26. Verified this session: with `local_hooks` naming a missing
hook the check failed and named it; with the hook present and tracked it reported
OK; with the key blank the hook was not checked at all; the template's own run
(no local hooks) still exits 0. Three break/fix cases cover it;
`bash scripts/selftest/run.sh` reported "75 passed, 0 failed"; `make check-drift`
exited 0.

## Decision 36 (2026-07-27, revised 2026-07-28): the landing page measures with analytics on by default; the installed factory still never phones home

What: `index.html` loads Google Analytics 4 through Consent Mode v2 with
`analytics_storage` **granted by default** and every advertising signal —
`ad_storage`, `ad_user_data`, `ad_personalization` — denied permanently. A visitor
turns analytics off from a footer link; the preference lives in `localStorage`,
not a cookie, and never leaves the browser. There is no interstitial gate: the
settings bar is a control that appears when asked for, not a wall on arrival. A
`Content-Security-Policy` meta tag restricts the page to exactly one third-party
host. Decision 4's "zero tracking" claim is annotated as superseded rather than
edited away.

Boundary, and it is the point: this covers the **website**. The installed factory
sends nothing — no telemetry in `install.sh`, the hooks, or any `factory-*`
script — and `install.sh` continues to promise it never phones home. Analytics on
a marketing page and telemetry on someone's machine are different things, and the
footer says so where a visitor can read it.

Why analytics at all, and why say so: the honest options were to keep the page
clean or to measure it and disclose it. Quietly adding a tracker while a
committed decision claimed "zero tracking" was not an option — on a project whose
whole argument is that it claims only what it has observed, that is the one
contradiction a skeptic would rightly seize on.

Revised on evidence, which is the part worth recording. This shipped first with
`analytics_storage` **denied** — cookieless pings, no storage, no ePrivacy
Article 5(3) trigger. It was the better privacy default and it did not work.
Verified on the deployed site: the tag loaded (200) and a `page_view` hit reached
`/g/collect` (204) carrying `gcs=G100`, so collection was genuinely happening —
yet nothing appeared in reports. GA4 only surfaces cookieless pings through
behavioural modelling, and modelling requires roughly 1,000 denied events per day
for seven days **and** roughly 1,000 granted daily users across seven of the
previous 28 days. A site this size meets neither, and the second is unsatisfiable
by construction when nobody grants — so the model can never train and the pings
are collected and never reported. Cookieless GA4 below that scale is the worst of
both: a third-party script that returns nothing. The choice was therefore between
no analytics and analytics that work, and the flip to granted was taken
deliberately.

Trade-off accepted, stated rather than glossed: ePrivacy Article 5(3) expects
prior consent before storage in the EU and UK, and on-by-default does not give
that. The narrow national analytics exemptions (France, Italy, Spain, the UK
statistical exception) require first-party aggregate-only data with no sharing,
which GA4 does not satisfy. Keeping every advertising signal denied and the
opt-out one click from every page view narrows the exposure; it does not remove
it. Revisit if the audience or the regulatory posture changes, or if traffic ever
reaches the modelling thresholds that would make cookieless viable.

No `integrity` hash is set on the gtag.js tag: it is served non-versioned,
updated continuously, and is not CORS-enabled for integrity checking, so a pinned
hash would break analytics rather than secure it. The CSP is the compensating
control, and it omits `frame-ancestors` deliberately — that directive is ignored
in a `<meta>` CSP, and static Pages hosting cannot set response headers. The
Measurement ID is not a secret — it ships in page source by design — so it is
committed, not injected; the Stream ID is not used by the page at all.

Provenance: founder direction — remove the no-tracking claim, be honest, and add
Google Analytics beyond what a cookieless third-party provider gives — 2026-07-27;
revised 2026-07-28 after the deployed cookieless configuration was observed
collecting hits that never reached reports, and the modelling thresholds were
verified against Google's own documentation.

## Decision 37 (2026-07-28): an opt-in adversarial review lane, advisory and never a gate

What: `./factory review-lane enable` installs a `pull_request_target` workflow
that fetches a PR's diff, has a model review it, and posts one advisory comment.
It is off unless asked for, at init and forever after. `factory-init` states both
costs before the question — tokens on every PR at the frontier tier, and a
repository secret only the adopter can add — and `factory upgrade` announces the
capability to repositories that do not have it, without ever enabling it.
`disable` deletes the workflow rather than leaving it inert. The reviewer calls
the provider's HTTP API directly (`scripts/adversarial-review.sh`), so CI needs
only curl and jq — no agent runtime — and the secret name follows `MODEL_PROVIDER`.

Why advisory and never a required check: a model's opinion is not a computational
control. The whole argument of this template is that gates block because they are
deterministic; making a stochastic reviewer a merge gate would borrow the
authority of the gates without their property. It costs a model instead of a
human's attention on the first pass, and that is all it claims.

The privilege boundary is the load-bearing part. Posting a review comment needs
write permission, which `pull_request` does not grant on a fork, so the workflow
runs as `pull_request_target` — a token worth containing. Three constraints do
that: same-repo PRs only, so a fork PR never drives a privileged job; checkout of
the base commit with `persist-credentials: false`, so a pull request cannot edit
the thing that reviews it; and the PR head treated strictly as data — the diff is
fetched through the API and passed to a model, never executed, sourced, or built.

The prompt asks the model to refute rather than assess, requires `file:line`
citations, forbids praise and restatement, and states that "No findings." is a
valid answer — because a reviewer that summarises is a reviewer that approves,
and inventing a finding to look thorough is the failure mode being designed
against.

Provenance: adapted from the originating factory's advisory review lane and its
trusted-base boundary lessons; founder direction to finish the lane with the
prompts, and earlier direction that it be opt-in with a stated cost, a chosen
model, and announcement on upgrade — 2026-07-28. Verified this session:
`factory-init` left the lane off and the workflow absent by default; `enable`
installed the workflow with the provider's secret substituted and no placeholder
left; `disable` removed the file; `factory upgrade` announced the capability to a
repository without it and enabled nothing. Five break/fix cases cover the
lifecycle and the fork boundary; `bash scripts/selftest/run.sh` reported
"81 passed, 0 failed"; `make check-drift` exited 0; `copy-manifest-check` and
`hook-existence-check` both passed.

An opt-in capability is offered exactly once, and the answer is recorded in
`factory.config`. The key's *presence* is the record — "off" is a decision that
was made, not an absence — so a repository that has answered is never asked
again in either direction, and one that has never been offered is still told.
Where there is no terminal (an upgrade piped through `curl … | sh`), or the read
fails, nothing is recorded: it prints how to enable and leaves the question open
for an interactive run, rather than banking an answer the adopter never gave.

Also fixed here: `run_with_timeout` in `golden-task-eval.sh` killed only the
runner process, leaving its children orphaned and holding the inherited pipe —
which stalled the caller for two minutes when a runner spawned a background
child. It now runs the child in its own process group and kills the tree.

## Decision 38 (2026-07-29): the review lane reaches existing repositories, and upgrade cannot recurse into itself

What: three defects that together made the review lane invisible to anyone who
already had the factory.

1. `packs/review-lane/review-pr.yml` was copied by `factory-init` but was not in
   `factory-upgrade`'s framework list, so an upgraded repository never received
   it. The capability offer is guarded on that file existing, so the offer
   silently never fired — and `factory review-lane enable` would have failed too.
   It is now shipped, and `copy_framework` creates a framework file's parent
   directory rather than skipping the file when the directory is absent.
2. `factory-init` recorded `REVIEW_LANE="on"` without installing the workflow — a
   flag governing nothing. Init now routes through `factory review-lane enable`,
   the same path the upgrade offer uses, so there is one install path instead of
   two that drift.
3. Nothing asked which model reviews. `enable` now prompts when there is a
   terminal and no model is already chosen, shows the provider's frontier
   default, and records `REVIEW_MODEL`. Both init and the upgrade offer inherit
   it by routing through `enable`.

Also fixed, and more serious than any of the above: `factory upgrade` could
**fork-bomb**. Upgrade runs `factory doctor`, doctor proves itself with the
self-test, and the self-test exercises upgrade — an unbounded cycle. It was
latent until fix (1) made `copy_framework` create parent directories, which let
the nested sandbox receive `factory-doctor.sh` and the self-test and so complete
the loop. Observed: 53 processes and climbing from a single `factory upgrade`,
which had to be killed. A re-entrancy guard (`FACTORY_UPGRADE_ACTIVE`) now makes
a nested upgrade skip the doctor proof, and the same run finishes in 18 seconds
with no processes left behind. The capability offer also moved ahead of the
doctor proof, so an adopter is asked while the upgrade is fresh rather than after
a full break/fix run.

Why it matters beyond this feature: a guard is only as good as its delivery. Two
of these three defects are the same shape as Decision 28 — a new file that init
ships and upgrade does not — which is now covered by a self-test case asserting
the framework list contains the workflow template, and another asserting the
re-entrancy guard exists.

Provenance: founder report — upgraded a repository to the latest main and was
never prompted about the adversarial review or which model to use — 2026-07-29.
Verified this session: a repository built from the v0.1.1 tag and upgraded to
main received the workflow template, was offered the capability, and when
answered interactively through a pty recorded `REVIEW_LANE="on"`,
`REVIEW_MODEL="openrouter/anthropic/claude-opus-4.8"` and
`REVIEW_API_KEY_SECRET="OPENROUTER_API_KEY"`, installed the workflow with the
secret substituted and no placeholder left, and did not ask again on the next
upgrade; the same upgrade completed in 18s leaving no stray processes. `bash
scripts/selftest/run.sh` reported "86 passed, 0 failed"; `make check-drift` and
`copy-manifest-check` exited 0.

## Decision 39 (2026-08-02): work the adopter must do is reported last, highlighted, and until it is done

What: the review lane's required repository secret is surfaced three ways instead
of one line in the middle of a run.

1. `scripts/lib/color.sh` adds emphasis that degrades correctly — colour only
   when stdout is a terminal and `NO_COLOR` is unset, so a piped log or CI
   transcript gets plain text rather than escape codes.
2. `factory init` and `factory upgrade` end with a highlighted **Action required**
   block, printed after the doctor proof so it cannot scroll away. It is
   recomputed from live state each run (`factory review-lane pending`), so it
   appears only while the work is outstanding and disappears once done.
3. `factory doctor` reports the lane as **armed** only when the secret is
   confirmed present, as a **warning** when the lane is on but the secret is
   missing or unverifiable, and **inert** when the lane is off. Where `gh` is
   available and authenticated the secret is checked for real; otherwise the
   state is reported as unverified rather than guessed.

Why: a one-time message is not a control. The instruction was already printed at
enable time, but a doctor run and an upgrade summary followed it, so the only
thing the adopter had to act on scrolled past. An enabled lane with no secret is
precisely the inert-gate class this project exists to surface — so it belongs in
the same armed/inert vocabulary as every other gate, reported on every run until
it is true.

Also fixed, and it is the same lesson twice more. `color.sh` was sourced by init
and upgrade but not added to the upgrade framework list, so under `set -u` an
upgraded repository aborted on an unbound colour variable — the third instance of
"a new file init ships and upgrade does not" (Decisions 28, 38). It is now
shipped, its variables are referenced with defaults, and `action_box` has a plain
fallback, so a missing optional lib can never be why a command fails
(Decision 34's rule). And the recursion guard from Decision 38 proved
insufficient: it cut `upgrade → doctor → selftest → upgrade`, but not the same
cycle entered from `doctor`, because the guard only helps when the *outer*
process is an upgrade. The self-test now marks the upgrades it spawns as nested,
which cuts the cycle at its source; `factory doctor` completes in 17s leaving no
processes behind.

Provenance: founder report — after upgrading, the secret instruction was present
but not prominent enough and arrived too early to act on — 2026-08-02. Verified
this session: an upgrade of a v0.1.1 repository ended with the highlighted block
as its final output with no unbound-variable error; `factory doctor` reported the
lane as a warning naming the missing secret when on, and inert when off, and
terminated in 17s with no stray processes; `pending` named the secret when the
lane was on and printed nothing when off. Four break/fix cases cover the nested
marker, the shipped colour lib, and both pending states; `bash
scripts/selftest/run.sh` reported "90 passed, 0 failed"; `make check-drift` and
`copy-manifest-check` exited 0.

## Decision 40 (2026-08-08): local metrics, no phone-home, no exporters, no server

What: `factory metrics` reports what the factory is doing to the repository —
enforcement (gates installed, armed vs inert, blocks by gate, repeat-block
friction), loop health from git (commits, merges, reverts, files reworked),
verification discipline (claims carrying evidence), and agent quality (eval pass
rate against baseline, and stale baselines). `--json` emits a versioned document
(`factory.metrics/v1`); `--html` writes a self-contained `.factory/metrics.html`
generated from `templates/metrics.html`. `.factory/events.log` is capped
(`FACTORY_EVENT_MAX_LINES`, default 5000) and trimmed to its most recent half.

Boundary, restated because the word invites confusion: this is **not telemetry**.
Nothing is transmitted. The numbers are computed locally, from the adopter's own
repository, for the adopter. `install.sh`, the hooks, and every `factory-*`
script still send nothing, and the landing page's promise that the installed
factory never phones home remains exactly true. A feature named "telemetry" would
have contradicted a published claim; the feature itself does not.

Two rules govern what appears. Every metric names the decision it informs — a
number nobody acts on is noise. And the uncomfortable numbers are first-class:
inert gates, repeat blocks, reverts and stale baselines are shown as prominently
as the wins, for the same reason `factory report` refuses a "tokens saved"
headline. A report that only shows wins is marketing.

Most metrics derive from git history rather than collected events, so the report
is meaningful the day the factory is installed rather than after months of
instrumentation. Token spend is explicitly not measured — the harness owns it —
and code quality is not claimed, because it is not honestly measurable without
judgment.

No exporters ship. Prometheus, OTel and Datadog integrations would be
dependencies most adopters carry for nothing, so the contract is the versioned
JSON document and the documentation shows the few lines needed to pipe it
anywhere.

No server, and no compiled binary. A Go service was considered and rejected: it
would reverse Decision 14 (plain shell you can read, zero install dependency,
language-agnostic core), require per-platform binaries with signing and
checksums, put a listening process on a developer machine, and — the deciding
argument — make upgrades harder rather than easier, since the upgrade model is
byte-identical file copies (Decision 2) and a binary cannot be one. The
"easier to update" goal is met instead by generating the page from an ordinary
HTML template with the data injected at a marker: the UI is edited like any web
page and ships as a framework file.

Provenance: founder direction — build factory metrics covering the factory's
impact, and consider a Go server with a UI — 2026-08-08. Verified this session:
`factory metrics` reported this repository's own state (12 gates, 2 armed and 2
inert, one commit-message-lint block, 47 commits, 58 of 117 files reworked,
2 of 2 claims cited); `--json` emitted the versioned schema; `--html` produced a
page whose data was injected with no placeholder left and no external requests,
confirmed by rendering it in a browser; all three formats also succeeded in a
bare repository with no hooks and no events. Five break/fix cases cover the
schema, the not-measured disclosure, HTML injection, self-containment, and log
trimming; `bash scripts/selftest/run.sh` reported "95 passed, 0 failed";
`make check-drift` and `copy-manifest-check` exited 0.

Also fixed while building: nine command substitutions in the metrics script died
under `set -o pipefail` when a `find` or `grep -c` legitimately matched nothing —
a bare repository could not produce a report at all. Every such pipeline is now
guarded and every counter normalised, so an empty repository reports zeros rather
than failing.

### Amendment (2026-08-08): a gate that can block must be able to say so

Found in the field, on the first adopter repo to run the report. It said 14 gates
installed and 0 blocks caught. Both numbers were accurate and the pair was
misleading: only 6 of those gates called `factory_log_event`, so 8 could stop a
push and leave nothing behind — including `vitest-only-check`, the dialect gate
most likely to fire in that repository. The report was not calm. It was deaf, and
it read as calm, which is precisely the flattery this decision's second rule was
written to prevent.

Every shipped gate with a non-zero exit path is now instrumented:
`copy-manifest-check`, `diff-aware-check`, `hook-existence-check`,
`shared-script-enforcement`, `wiki-lint`, and the three pack dialect gates
(`vitest-only-check`, `ginkgo-only-check`, `junit5-only-check`). The pack gates
resolve their script directory before their `cd`, which moves them out of their
own tree, and source the lib defensively so bookkeeping can never be the reason
enforcement fails.

`scripts/hooks/gate-instrumentation-check.sh` makes it an invariant rather than a
one-time sweep: CI fails when a gate with a blocking exit does not call
`factory_log_event`, or calls it without sourcing `scripts/lib/events.sh` — the
subtler mute, where a grep for the call succeeds but no event is ever written.

`loop-close-check` is deliberately not instrumented. Its non-zero exit writes a
reminder and stops no work, and counting nudges as blocks would inflate the
enforcement numbers in the other direction. It opts out with a
`# factory: no-block-event` marker in the file, beside the reason. Both the check
and the report read that marker, so the exemption travels with the gate rather
than sitting in a central list that drifts out of date.

The report now discloses the gap instead of assuming it away, since an adopter's
own gates may be mute: `gates_reporting` and `gates_mute` are measured per run,
and when any gate can block without recording it, the terminal, JSON and HTML
outputs all say so beside the block count rather than in a footnote.

Provenance: observed 2026-08-08 on the `tutr` adopter repository, which reported
14 gates and 0 blocks with 8 gates unable to record one. Verified this session:
each instrumented gate was driven into its blocking path in a sandbox with the
pack gates installed where adopters have them (`scripts/hooks/`), and the event
log was read back — `vitest-only-check`, `ginkgo-only-check`,
`junit5-only-check`, `wiki-lint`, `hook-existence-check`,
`shared-script-enforcement` and `copy-manifest-check` each blocked (rc=1) and
each wrote one event, 7 distinct gates in the log. `diff-aware-check` is
instrumented but was not driven, since it is a rollup that dispatches other
checks. The new gate was proven to fail before being trusted: a mute blocking
gate, a mute pack gate, and a gate calling the logger without sourcing it are all
rejected; an advisory `exit 0` script and a marker-exempt gate pass.
`bash scripts/selftest/run.sh` reported "110 passed, 0 failed" (was 103);
`make check-drift`, `copy-manifest-check` and `hook-existence-check` exited 0.

### Amendment (2026-08-08): pack dialect gates were never upgradeable

Found immediately after tagging v0.1.3, while verifying that an adopter on
v0.1.2 could actually reach the new instrumentation. They could not.

`copy_framework` derived its source as `$TEMPLATE/<destination path>`, which is
correct for everything the template stores where the adopter stores it. Pack
dialect gates are the one exception: upstream they live in
`packs/<lang>/hooks/`, and they install into `scripts/hooks/`. So the upgrade
asked for `$TEMPLATE/scripts/hooks/vitest-only-check.sh`, the guard
`[ -f "$src" ] || return 0` found nothing, and the function returned success.
No error, no skip message, nothing in the file list — an adopter kept whichever
dialect gate they first installed with, forever, and had no way to know.

This predates v0.1.3; the instrumentation work only made it visible, because
`vitest-only-check` was the gate that most obviously failed to change. The fix
gives `copy_framework` an optional explicit source and passes the real pack path
for those gates. Two break/fix cases cover it: a stale gate must lose its stale
marker after an upgrade, and the refreshed gate must contain the instrumentation
— content arriving, not merely a file being touched.

A second, milder wrinkle surfaced in the same test and is not a defect: upgrading
across a release that adds framework files takes two runs, because the upgrade
script executing the copy is the adopter's old one, carrying the old file list.
It replaces itself on the first run, so the second delivers the rest. That is
inherent to a self-upgrading shell script, and the atomic-rename comment in
`copy_framework` already anticipates it.

Provenance: observed 2026-08-08 against the published v0.1.3 tag. A repository
seeded from the v0.1.2 tree and upgraded with `--ref v0.1.3` still carried the
stale pack gate after two upgrade runs, while `scripts/factory-metrics.sh` and
`templates/metrics.html` arrived on the second. Verified after the fix by
re-running both paths against a stale gate: the released script left the stale
marker in place with zero `factory_log_event` occurrences; the fixed script
removed the marker and delivered a gate carrying the call.
`bash scripts/selftest/run.sh` reported "112 passed, 0 failed".

## Decision 41 (2026-08-08): one configuration file, not two

What: `factory.yaml` becomes the single configuration file. The keys that lived
in `factory.config` — cost profile, model provider, the nine per-harness model
tiers, and the three review-lane settings — move into it as ordinary flat keys.
`scripts/lib/config.sh` gains `factory_config_export`, which reads them into the
shell variable names the scripts already use, so consumers stop sourcing a second
file without changing how they refer to a value.

Why two files existed: `factory.yaml` is *parsed* by `factory_config_get`, and is
read by hooks that must not execute anything they read. `factory.config` was
*sourced*, which was convenient for scripts that wanted shell variables and
wanted them cheaply. That convenience is what accreted: each new setting went
wherever it was easiest to reach from, and the split stopped tracking any
principle.

The cost was not tidiness. The same fact came to live in both files under
different names — `citation_prefix` and `CITATION_PREFIX`, `protected_paths` and
`PROTECTED_PATH`, `docs_root` and `DOCS_ROOT` — and nothing kept them equal. Two
spellings of one fact is a drift bug waiting for someone to edit the one the
hooks do not read, and then wonder why the gate did not change.

That was not hypothetical, and the migration found it. `citation_prefix` in the
YAML held the answer the adopter gave at init; `CITATION_PREFIX` in the shell
file held a different value derived from the project slug. One setting, two
spellings, two values — and the second was read by nothing, so the discrepancy
had no symptom to notice. It is deleted rather than carried across; the parsed
key is the one the citation gate has always enforced.

The second cost is sharper: a sourced file is executed. `factory.config` is
ordinary project configuration that an adopter edits, a merge can conflict in,
and a pull request can touch — and every script that sourced it ran whatever it
contained. Nothing has gone wrong, and nothing needs to for this to be worth
closing: configuration should be read, not run. After this change one file is
parsed, never executed.

Compatibility, because adopters have repositories in the field: `factory.yaml`
wins, and `factory.config` is still read for any key the YAML does not define.
An existing repository keeps working with no action at all. `factory upgrade`
offers a one-time migration through the ask-once mechanism, so it is proposed
exactly once and never nags; the migration writes the missing keys into
`factory.yaml`, renames the old file to `factory.config.migrated`, and leaves it
in the working tree for review rather than deleting it. `factory doctor` reports
a repository still carrying the legacy file, since a fallback nobody notices is
how a deprecation lives forever.

Not done here: `factory.config` also recorded values used only during `init`
(project name, GitHub owner, tool versions). Those are written once and read by
nothing afterwards, so they move as plain keys without ceremony. Anything genuinely
needing more structure than `key: value` belongs in a hook, which is the rule
Decision 2 already set for this format.

Provenance: founder direction, "why is there a factory.config and factory.yaml
files? shouldnt we just have a single yaml file for configuration?" — 2026-07-26,
deferred until after the v0.1.2 release and taken up 2026-08-08.

### Amendment (2026-08-08): an upgrade asks the arriving version's questions

Raised by the founder while reviewing the migration prompt: adopters skip
versions, so someone may go from v0.1.4 straight to v0.1.6. That is the case
that decides whether an opt-in feature is ever discovered, because nobody reads
the release notes of a version they jumped over. It also decided the prompt
itself — a documented command alone would reach only the people already looking
for it.

But it exposed a hole in the prompt. The script performing an upgrade is the one
the repository already has, and everything after the file copy — which
capabilities to offer, which migrations to propose — is logic that ships *with* a
release. So an adopter was asked the questions of the version they were leaving
and never heard about the one they were arriving at. Running the upgrade twice
cured it, which is a fine explanation and a poor default: the second run is
precisely the one nobody does.

The upgrade now hands off. If the copy replaced `factory-upgrade.sh`, it execs
the new one, which asks. A single hand-off is enforced by `FACTORY_UPGRADE_REEXEC`,
deliberately a different flag from `FACTORY_UPGRADE_ACTIVE`: the latter means "an
upgrade is running", true of both halves, while the former means "the hand-off
already happened" and gates the only path that could recurse. This repository has
produced a fork bomb before (doctor → self-test → upgrade → doctor), so that
bound is load-bearing rather than defensive. Nestedness is carried across
explicitly, because a child re-deriving it from `FACTORY_UPGRADE_ACTIVE` — which
the parent always exports — would judge every hand-off nested and silently skip
the proof that the gates still fire.

One limit stands and is worth stating plainly: this cannot help the upgrade *into*
the first release that contains it, since the old script is the one running. For
that jump the discovery channel is `factory doctor`, which the upgrade runs at the
end and which by then is the new version — it reports the legacy config file and
names `./factory migrate-config`. Verified: after a single upgrade driven by the
old script, doctor emits "factory.config is still present — two config files, one
job (Decision 41)" with the command beneath it.

Provenance: founder direction, "people may go directly from 0.1.4 to 0.1.6 so I
dont think there is any extra safetly in not providing the prompt" — 2026-08-08.
Verified this session: a repository whose upgrade script differed from the
template handed off exactly once and the stale copy was replaced; a second
invocation of the now-current script did not hand off again, and no upgrade
processes were left behind. With a non-nested caller the child did not claim
nesting and did run the doctor proof; with a nested caller it reported nested
exactly once. `bash scripts/selftest/run.sh` reported "124 passed, 0 failed".

### Amendment (2026-08-08): nestedness has one source, and the handshake carries none of it

The hand-off shipped with a fork bomb, and it ran on an adopter's machine before
the first release could carry it. Reported as an upgrade that hung; it was a
`doctor -> self-test -> upgrade -> doctor` cycle forking without bound.

The mechanism was the hand-off's own handshake. The parent passed its nestedness
across as an exported variable so the child would not mistake the hand-off for
nesting. Exported variables are inherited by every descendant, not just the one
being exec'd — and the doctor proof spawns a self-test that runs upgrade fixtures
of its own with an explicit `FACTORY_UPGRADE_ACTIVE=1`. Those fixtures read the
stale handshake, concluded they were top-level, ran the doctor, and recursed. The
explicit guard was overridden by an inherited description of a different process.

The repair is to stop describing nestedness at all. It has exactly one source —
`FACTORY_UPGRADE_ACTIVE` — and before exec'ing, the hand-off restores that
variable to whatever the caller had. The child then derives its own nestedness
the same way every other invocation does. `FACTORY_UPGRADE_REEXEC` survives only
to bound the hand-off to one, is read into a plain shell variable, and is unset
immediately so nothing downstream can inherit it. A fact with one representation
cannot go stale against itself.

Two lessons worth keeping, both general. A guard that can be overridden by an
inherited environment variable is not a guard. And the second half of a fix
belongs to the environment it leaves behind, not only to the process it starts:
the earlier version was correct for the child and wrong for the grandchild.

Also fixed, found by the same run: the pack-gate fixture assumed the upgrade
source contains a `packs/` tree. This suite ships to adopters and runs inside
`factory doctor`, and an adopter has the dialect gate installed in
`scripts/hooks/` but no `packs/` directory at all — so the fixture failed in
every adopter repository and turned a healthy doctor red for a condition that was
never theirs. It is now skipped where there is no pack tree to upgrade from.

Provenance: observed 2026-08-08 on the `tutr` adopter repository, upgrading from
main; multiple concurrent `factory-doctor.sh` and `selftest/run.sh` processes
were confirmed with `pgrep` before being killed. Verified after the repair: the
fork-bomb precondition (an inherited handshake alongside an explicit nested
marker) now reports nested and starts no doctor; a nested caller with a stale
script hands off once and starts no doctor; a non-nested caller hands off once,
starts the doctor exactly once, and its self-test reported "117 passed, 0 failed"
with a healthy verdict and zero processes left behind.

### Amendment (2026-08-08): silence is not a status

The doctor's break/fix proof printed nothing between "Proof (break/fix
self-test)" and its verdict. On a repository with language packs armed that is
minutes of nothing, and an adopter reasonably read a working upgrade as hung —
having just been given a real reason to suspect one. The run completed; the
output gave no way to know that while it was happening.

A terminal now gets a live count of proven cases, read from the file the
self-test is already writing, so the progress source is the artifact rather than
a second mechanism to keep in sync. A log gets one honest line instead of a
redrawing counter, because escape sequences in a CI transcript are noise
pretending to be feedback — the same rule the colour lib already follows.

Durations are reported for the stages that take one: the template fetch, the
file copy, the proof, and the total. `scripts/lib/timing.sh` formats them the
same way everywhere, in seconds resolution — these are minute-scale operations
and a millisecond stopwatch would imply a measurement it is not. The total is
carried across the hand-off and handed over by `install.sh`, so it is wall clock
from the adopter's command rather than from whichever half of the run happens to
print it.

Also fixed, and visible in the reported run: the file count restarted at the
hand-off, so the summary said "0 file(s) updated" immediately after listing two
dozen updates. The running total now crosses the boundary while the per-stage
line reports only what that pass did.

Provenance: founder direction, "we need to show some indication when it gets to
proof state to show that things are working... put the time taken for the install
and upgrade scripts as well towards the end as a metric and for each stage" —
2026-08-08. Verified this session through a pty and through a pipe: the terminal
run showed 29 counter updates climbing to 115 cases and reported "37 file(s)
updated in 31s / of which the break/fix proof took 30s"; the piped run contained
zero carriage-return characters, one heads-up line, and the same timings.
`bash scripts/selftest/run.sh` reported "133 passed, 0 failed".

### Amendment (2026-08-08): the eval scaffold reaches existing adopters, and its advice works

Two problems, reported together by an adopter whose `factory metrics` kept saying
"no eval baselines yet — run golden-task-eval --save-baseline", and whose eval
answered "no tasks in eval/golden-tasks/" every time they did.

The advice was a dead end because the repository had no eval scaffold at all —
`eval/golden-tasks/` and `eval/runners/` did not exist. `factory init` creates
them, but `eval/` was never in the upgrade's framework list, so a repository
installed before the eval feature could not receive it. Exactly the class of bug
that hid the pack dialect gates: ships at init, unreachable by upgrade. The
scaffold is framework by every test that matters — identical for every adopter,
carrying no per-repo values — so it is in the list now, with the execute bits
restored alongside the hooks. A runner or an oracle that arrives without its
execute bit is a task that cannot be scored, which reads as a failing agent
rather than a broken install.

The message was wrong independently. Three states need three different next
steps: no scaffold, a scaffold with no tasks, and tasks with no baseline. Naming
the last one in all three cases sent an adopter to a command that could only
write an empty file, and then repeated itself after they ran it. The report now
says which state the repository is in and what actually advances it.

`golden-task-eval` also stopped writing a results file when it scored nothing.
An empty result is still a result: it makes an unconfigured repository look
instrumented, and invites a later comparison against a baseline of nothing.

Provenance: adopter report, 2026-08-08 — "I keep getting ... No eval baselines
yet ... but there is no change in the metrics report anyway". Confirmed in that
repository: `eval/results/` present with a zero-task `mock-current.json`, and no
`eval/golden-tasks/` or `eval/runners/` directory. Verified after the fix on a
repository seeded to the same shape: the upgrade added five eval files with the
oracle executable, and the three states each produced their own next step —
"factory upgrade", "no eval tasks yet", and "1 task(s), no baseline yet" — with
the baseline actually written once that last advice was followed.
`bash scripts/selftest/run.sh` reported "137 passed, 0 failed".

### Amendment (2026-08-08): a mock score is not an agent score

An adopter saved their first baseline and asked why it said "mock". The right
question. The mock runner calls no model — it writes a fixed answer so the
scorer is provable in CI without credentials — and it scores 1.00 by
construction. The report showed that as:

    Agents  getting better, or worse?
        reference-answer               baseline 1.0  current 1.0

A perfect score, under that heading, from a runner that cannot fail, with the
word "mock" nowhere on the line. That is precisely the vanity number Decision 40
was written to refuse, and this report shipped it. The failure is worth naming
plainly: the rule was applied to the metrics I was suspicious of and not to the
one that flattered.

Mock results are now labelled where they appear, and a repository whose only
results are mock is told that the scorer works and no agent has been measured,
with the step that would change it. `measured_tasks` counts non-mock results, so
the JSON carries the distinction too. `golden-task-eval` says the same thing at
the moment it saves a mock baseline, rather than leaving the reader to discover
it from a filename.

The fix also exposed why three lines of it went missing on the first attempt: the
report program is embedded in a double-quoted shell string, and a double quote
anywhere inside — including in a comment — closes that string and silently
truncates everything after. A comment quoting the section heading did exactly
that. There is now a check that the embedded program contains no double quotes,
because the failure mode is invisible: the script runs, the exit status is zero,
and only the missing output tells you.

Provenance: adopter question, 2026-08-08 — "why is it showing mock baseline?".
Verified this session against that repository's own results: the Agents section
now reads "baseline 1.0 current 1.0 (mock — not your agents)" followed by "the
scorer works — no agent measured yet", and the JSON reports measured_tasks 0 with
is_mock true. A seeded non-mock harness reports measured_tasks 1 and carries no
mock label. `bash scripts/selftest/run.sh` reported "142 passed, 0 failed".

### Amendment (2026-08-08): the factory ships runners for the harnesses it configures

An adopter, told to point `--runner` at a real harness, asked the obvious
question: "we are a harness are we not? couldn't we use what the factory
provides?" They were right. The factory treats `opencode.json` as canonical,
generates the Claude Code and Codex agent files from it, and routes each
harness's model tiers — and then asked the adopter to work out headless
invocation for the same three harnesses themselves. The template shipped an
`example-harness.sh` that exits 1.

`claude.sh`, `codex.sh` and `opencode.sh` now ship, selected by name:
`--harness claude` uses `eval/runners/claude.sh`. `--runner` still overrides, for
a harness the factory does not configure, and `example-harness.sh` remains the
skeleton for that case — now pointing at the three real ones first.

Each runner encodes the permission decision that would otherwise be discovered
the hard way. A headless run has nobody to approve anything, and an "ask"
permission does not fail cleanly: the primary session auto-rejects it, so work is
blocked and the task fails as though the model were incapable. Claude Code runs
under `acceptEdits`, which grants file edits without granting shell; Codex under
`workspace-write`, the narrowest sandbox in which the task is solvable. Neither
bypasses approvals *and* the sandbox — an eval is not a reason to hand over the
machine. opencode runs as the `implementer` role, so what is scored is the agent
the factory actually routes work to.

`mock` stays the default: it calls no model, so the scorer remains provable in CI
without credentials, and a real run costs the adopter money they have not agreed
to spend.

The same question exposed a worse bug. The eval parsed only `--harness=name`, so
`--harness claude` was silently dropped: the run used the mock, printed
"harness=mock", and an adopter reading 1.00 would believe they had measured their
agent. Both spellings are accepted now and an unrecognised argument is an error.
A flag quietly ignored is worse than one rejected — wrong answers are
recoverable, wrong answers that look right are not.

Provenance: adopter question, 2026-08-08. Verified this session by running all
three against the reference task on this machine, with the CLIs installed
(claude 2.1.220, codex-cli 0.145.0, opencode 1.18.11): each scored
"reference-answer: 1/1 passed (score 1.00)" through `golden-task-eval`, and the
Claude run's log confirmed the permission model — Bash denied, Write allowed.
`factory metrics` reported `measured_tasks 1` with `is_mock false` for a real
harness baseline, against 0 and true for the mock.
`bash scripts/selftest/run.sh` reported "149 passed, 0 failed".

### Amendment (2026-08-09): `--html` opens the page

Asking for a page is asking to look at it. `factory metrics --html` wrote the
file and printed its path, leaving the adopter to open it themselves — a step
with no purpose except that nobody had removed it.

It opens now, and the conditions are the interesting part. Launching a browser
is a visible side effect, so it happens only where a human is watching: with a
terminal on stdout, never from CI, a pipe, or a redirect. `--no-open` skips it
for the case where the file is the deliverable. `$BROWSER` is honoured first, as
Linux convention expects, then each platform's opener; none of it is fatal, since
a report that cannot open a window has still produced the report, and the path is
printed either way. The opener is backgrounded, because some hold the terminal
for as long as the browser lives and this should not be a command you remember to
background yourself.

The self-test covers the two cases that could do harm — a browser launched from
CI, or from a run whose output someone is redirecting into a file — rather than
the one that cannot be exercised without a terminal.

Provenance: founder direction, 2026-08-09 — "for factory metrics --html, we
should also open the html file instead of having the user having to open it
manually". Verified this session: a piped run and a `--no-open` run both write
the page and print the open-it-yourself line, while a run under a pty reports
"opening it" and invoked the platform opener.
`bash scripts/selftest/run.sh` reported "157 passed, 0 failed".

### Amendment (2026-08-09): no shipped CI job may require a credential nobody was asked for

An adopter declined the adversarial review lane during an upgrade, and their CI
still failed for want of `OPENROUTER_API_KEY`. Reasonable to read as a bug in the
opt-out; it was not. The language packs' CI workflow ran

    ./scripts/golden-task-eval.sh --harness=opencode

unconditionally, and a real harness needs a provider key. Two features, one
secret name, no relationship — and the shared name is precisely why declining one
looked like it should have silenced the other.

The defect is the unconditional part. `factory init` never asks for this key
unless the review lane is enabled, so every adopter of a language pack inherited
a CI job that could not pass on day one. A gate that fails for a reason the
adopter was never given a chance to fix teaches people to ignore red CI, which
costs more than the check was ever worth.

The job now scores the agents when a key is configured and the scorer when one is
not, saying which it did. Green without credentials, real with them, and never
silent about the difference — the same shape as the eval's own default, where
`mock` is chosen precisely because it calls no model.

The self-test asserts the invariant rather than the symptom: wherever a pack's CI
names a real harness, the key guard must appear too, and the unguarded form must
still be present as the fallback. An earlier attempt grepped for "an unguarded
call" and could not tell it from the guarded one two lines below — a check that
could never pass, which is its own kind of broken.

Provenance: adopter report, 2026-08-09 — "eventhough I said no to adversarial
review when I factory upgraded, the golden eval still warns of the open router
api key not being present". Verified by running the job's exact shell without the
variable set: it printed "No OPENROUTER_API_KEY configured — scoring with the
mock runner", scored the task, and exited 0.
`bash scripts/selftest/run.sh` reported "165 passed, 0 failed".

## Decision 42 (2026-09-06): preserve the final stacked PR when reconciling squash-merged ancestors

What: merge main into PR #65 without rewriting the PR commits. Retain the
final PR implementation in all eight conflicting files. This changes ancestry,
not the enforcement behavior approved by the preceding decisions.

Why: main at 3beb558 and the PR ancestor 427676a have the identical Git tree
`d4bd793b64549ae4cdbc7b6a3405686974319e9f`. PRs #63 and #64 were squash-merged,
so their commit identities differ even though their combined contents match.
The conflicts pit those earlier implementations against the later corrections
on PR #65; taking main would discard those corrections. Preserve the caller
configuration precedence, literal legacy values, diagnostic-aware commit
enumeration, comment parsing, configured test-edit boundary, private event
trimming, runner diagnostics, and corrected fixtures already present on the PR.

Provenance: observed 2026-09-06 via `git rev-parse 427676a^{tree}
origin/main^{tree}` (both returned the tree above), and
`git diff 427676a origin/main` (empty output), after fetching main.
PR requirements and review responses: https://github.com/anoop2811/software-factory-template/pull/65
(read 2026-09-06).

## Decision 43 (2026-09-06): pack dialect gates explain an unsupported shell before parsing Bash syntax

What: the Go, Java, and TypeScript dialect gates reject a non-Bash interpreter
or Bash in POSIX mode with exit 2 and an instruction to execute the script
directly or with bash. The guard uses POSIX syntax and runs before shell
options, shared libraries, or Bash-only constructs. Normal Bash invocation
retains the existing enforcement and exit statuses.

Why: invoking a Bash script through sh overrides its shebang. A BASH_VERSION
check alone misses macOS sh, which is Bash in POSIX mode. A piped enumeration
rewrite would also move the violation counter into a subshell and risk losing
failed checks. Keep the supported interpreter explicit.

Provenance: GitHub issue #67 by @vshanbha, read 2026-09-06:
https://github.com/anoop2811/software-factory-template/issues/67

## Decision 44 (2026-09-06): selftest reports optional fixture omissions and continues

What: selftest skips only fixture groups whose optional pack sources are absent,
prints the group and missing path, and reports a skip count alongside passes
and failures. Pack catalog, dialect, workflow, upgrade, and review-lane fixtures
are optional in an adopted repository. Present fixtures still run and failures
still fail the suite. Core scripts, libraries, and metrics templates remain
required; removing them is not an optional-pack skip.

Why: language pack sources are not copied into adopted repositories, while
packs/review-lane/review-pr.yml is a separate optional capability template. An
adopter removing packs must receive an honest coverage report and the rest of
the selftest, not a raw cp failure that prevents later checks. Template CI also
runs isolated copies without packs and without only the review-lane template,
and proves that a broken present gate is still rejected.

Provenance: GitHub issue #68 by @vshanbha, read 2026-09-06:
https://github.com/anoop2811/software-factory-template/issues/68

## Decision 45 (2026-09-06): native Maven adoption uses the Java pack

See [ADR-0045](adr/0045-maven-adoption.md) for build-tool detection, the Maven
asset overlay, POM preservation, quality-plugin integration, source versions,
and acceptance requirements. The Java pack is beta following Duke42's reported
real adoption with local Maven adaptations in issue #66; native Maven
generation is validated separately from that adopter report.


## Decision 46 (2026-09-06): opt-in budgets and truthful run metadata

What: Add factory budget plan/run/report, with one shared admission controller
for Codex, Claude Code and OpenCode. The acceptance contract is docs/BUDGETS.md.
The controller bounds factory CLI invocations, records local metadata, and labels
client cost estimates separately from authoritative billing. Strict USD ceilings
are refused when they cannot be guaranteed. No background model calls or retries.

Why: the highest-priority roadmap item is developer-controlled cost and visibility.
A limit written in a prompt is not an enforced budget; unknown usage is not zero.
Use existing flat configuration, role routing and generated adapters, plus
standard-library process supervision, instead of a hosted metering dependency.

Provenance: user requested the P0 budgeting/observability item, all three harnesses,
and the unchanged roadmap table with completion percentages, 2026-09-06. Native
CLI and usage sources are recorded in docs/BUDGETS.md. Claude cost reporting is
a client estimate per its official cost-tracking documentation fetched today.

Review follow-up (2026-09-06, PR #71): all post-signal process waits must be
bounded and TimeoutExpired must not skip pipe closure or signal restoration.
If a launched harness cannot be reaped, return timeout with unknown exit/cost
and retain its active reservation until documented recovery confirms exit.
A Codex probe whose cleanup cannot confirm exit must refuse launch. Match the
existing primary CI checkout major tag (v7) in budget acceptance. Python wait
semantics: https://docs.python.org/3/library/subprocess.html#subprocess.Popen.wait
(fetched 2026-09-06); review provenance:
https://github.com/anoop2811/software-factory-template/pull/71#discussion_r3945278692
https://github.com/anoop2811/software-factory-template/pull/71#discussion_r3945278713
https://github.com/anoop2811/software-factory-template/pull/71#discussion_r3945278732


## Decision 47 (2026-09-06): bounded repair loops with local verification evidence

What: implement the existing P1 Loop engineering roadmap item through factory
loop plan/run/status/resume. Default manual mode performs deterministic checks;
explicit bounded mode uses the shared budget controller for implementer and
separate reviewer invocations. All three native harness adapters remain shared.
No background activation, automatic merge, test rewriting or policy weakening.

The versioned acceptance contract is docs/LOOPS.md. Finite attempts and elapsed
time, unchanged-source/repeated-failure detection, checkpoint freshness and
protected/test/governance mutation handoffs prevent indefinite repair. Structured
local evidence identifies the tested content, checks and outcomes; it is not a
trusted attestation and does not replace independent CI or human protected-path
review. Check output used for repair is untrusted data, bounded in size and kept
private; evidence must not claim a pass for skipped checks or unknown exit.

Why: the next user-selected roadmap item is Loop engineering. The discussion of
https://stack72.dev/the-feedback-loop-is-moving-out-of-ci/ (fetched 2026-09-06)
reinforces moving iteration before integration while preserving the independent
verification boundary. Keep feature names/priorities stable and costs opt-in.

Provenance: user authorized merging PR #71 and starting the next feature on
2026-09-06. PR #71 merged as ccb1a3340be51c9c25e2e999d4ed3e74f465edaf.

Decision 47 refinement before policy change: arm this template's own test boundary
with test_file_patterns scripts/selftest/. The loop and native test-edit hook must
use the same POSIX extended-regex semantics as language packs, and hash native
permission/role files even when Git ignores them. Budget deadlines must be checked
inside admission and immediately before spawn, including lock contention.

Decision 47 review follow-up (2026-09-06, PR #72): deterministic native
preflight rejection with readable accounting and no owned active process ends
the loop without an uncertain checkpoint. Retain uncertainty for unreadable
accounting, an active reservation for the current session/task, or an explicitly
reported preflight process whose exit could not be confirmed. Propagate that
probe ownership as a typed error carrying its PID, including when probe parsing
fails before cleanup; an empty budget ledger alone cannot establish probe exit.
Keep the original error visible, and correct the shared configuration citation
to the actual reader and role-routing contract.

Provenance: review comments read 2026-09-06:
https://github.com/anoop2811/software-factory-template/pull/72#discussion_r3945498221
https://github.com/anoop2811/software-factory-template/pull/72#discussion_r3945498232

Review verification additionally reproduced signal/wait OSError during probe
cleanup clearing ownership and admitting another manual controller. Cleanup must
close probe resources despite those errors and retain typed PID ownership when
termination or exit cannot be established. Provenance: observed 2026-09-06 by
independent fault injection into the actual preflight/controller path during
PR #72 review (LOOP-PREFLIGHT-CLEANUP-OSERROR).

## Decision 48 (2026-09-06): specify a compatible Go conversion before implementation

What: draft specs/001-go-runtime-conversion.md using ai-craft's twelve-section
SPEC_TEMPLATE.md at commit 3d6c1bbb8f84116e34519382c826030324a06e77. The
proposal covers staged conversion of factory-owned runtime logic into Go,
with stable command/hook/sourced-library compatibility, safe delivery, existing
state preservation, recoverable upgrades and evidence-gated legacy cleanup.
Existing capability roadmap names and percentages remain unchanged.

Why: the user requested the Go conversion spec after merging PR #72, explicitly
requiring backward compatibility and cleanup as part of conversion. The spec is
a draft for human review; this decision authorizes documentation, not an
implementation, toolchain update, production migration or release. Detailed
proposed alternatives and consequences live in the spec's Decision Record.

Provenance: user direction, 2026-09-06; PR #72 merged as
76952eaa63aebd1ecd282f5ab51dd7c3627cb497, read back from GitHub this session.
Template: https://github.com/anoop2811/ai-craft/blob/3d6c1bbb8f84116e34519382c826030324a06e77/SPEC_TEMPLATE.md
(read from the reference checkout 2026-09-06).

Decision 48 user refinement (2026-09-06): Cobra is required for the Go CLI,
and every conversion slice follows outside-in TDD with Ginkgo v2 and Gomega.
The spec records external acceptance RED before implementation, focused
collaborator RED/GREEN as needed, then refactor, preserving evaluator separation
and the existing command/error/flag contracts. This remains a documentation task.

Decision 48 retention refinement (2026-09-06): the user requested that upgrades
retire superseded factory assets into a local gitignored recovery folder rather
than retaining old implementations indefinitely. Specify private per-migration
backups of replaced/removed owned assets, preserved original paths and restore
metadata, exclusion from all active discovery, and one successful release
transition of default retention. A later distinct release may prune unchanged
eligible backups only after its own required deterministic checks pass, retaining
the immediately preceding installation's recovery set. Failed upgrades, explicit
holds, edits or uncertain ownership prevent automatic deletion and are reported.
This updates the spec only; it moves or deletes no existing factory files.

## Decision 49 (2026-09-06): start Go conversion with characterized contracts

What: begin the user-authorized conversion on the merged Decision 48 spec.
The first reviewable slice records the baseline asset/surface inventory and
independent outside-in Ginkgo/Gomega compatibility cases, and introduces the
Cobra command boundary behind a developer-built candidate rather than switching
installed entrypoints before release/recovery readiness is established. A
command converted in this slice must meet its external behavior before any
production routing changes; unconverted commands retain the canonical scripts.

The inventory covers every tracked baseline asset, its hash/mode, intended
stage, ownership class and convert/retain/retire disposition. It is a reviewed
planning artifact, never permission to delete adopter files. Negative controls
must reject missing/unclassified assets and command-contract drift. Runtime
activation, backup/cleanup and artifact distribution remain gated by Decision
48's target/trust/recovery questions; nothing may silently download or activate
a development candidate in an adopter.

Why: the user requested starting the highest-priority Go conversion, using
Cobra and outside-in TDD with Ginkgo/Gomega, after merging PR #73. Baseline
a380dffb76bb18f1e616504a34775853eedad0ad includes the specification; behavior
comparison remains anchored to 76952ea and v0.1.6 as recorded there. The
configuration-precedence discrepancy EX-001 is pending the user's explicit
choice and must not be silently folded into parity. Progress tables belong in
the chat after each iteration; do not add the new row to FEATURE_ROADMAP.md.

Decision 49 toolchain research before pinning (2026-09-06): Go 1.27.1
(released 2026-09-01, https://go.dev/doc/devel/release), Cobra v1.10.2
(published 2025-12-04, https://github.com/spf13/cobra/releases/tag/v1.10.2),
Ginkgo v2.32.1 (2026-08-10, https://github.com/onsi/ginkgo/releases/tag/v2.32.1)
and Gomega v1.43.0 (2026-08-27, https://github.com/onsi/gomega/releases/tag/v1.43.0)
were read from their authoritative releases this session. Pin these development
dependencies in the factory's own module; do not alter adopter pack selection.
The installed shell dispatcher stays the default throughout this candidate slice.

Decision 49 enforcement refinement: the factory module's Go acceptance and
quality checks join its own check target, while adopted non-factory modules
remain unaffected. Reuse the Go pack dialect gate and lint configuration rather
than copying policy. Extend configured test patterns to _test.go and protect
cmd/internal implementation paths. Template-only CI fetches the baseline Git
history needed by independent inventory comparison.

Quality/action releases checked from official pages/API 2026-09-06 before pinning:
- golangci-lint v2.13.2, published 2026-08-27:
  https://github.com/golangci/golangci-lint/releases/tag/v2.13.2
- gosec v2.29.0, published 2026-08-26:
  https://github.com/securego/gosec/releases/tag/v2.29.0
- govulncheck v1.1.4, published 2025-01-13:
  https://github.com/golang/vuln/releases/tag/v1.1.4
- actions/checkout v7.0.1, published 2026-07-20:
  https://github.com/actions/checkout/releases/tag/v7.0.1
- actions/setup-go v7.0.0, published 2026-07-16:
  https://github.com/actions/setup-go/releases/tag/v7.0.0

Decision 49 lint integration: permit only the blessed Ginkgo/Gomega packages
in revive's dot-imports rule in the shared Go pack config; retain the rule for
all other imports. The installed revive rule documentation explicitly supports
allowedPackages (github.com/mgechev/revive v1.15.0 RULES_DESCRIPTIONS.md, read
2026-09-06). Ignore the developer-only /factory-go binary and document its
colocated build; neither the shell entrypoint nor installed assets change.

Decision 49 review correction: use the Go pack's test filename boundary for
factory test protection, including shell-command payloads with trailing tokens.
An end-of-string-only pattern was observed to allow a test edit followed by
`&& echo done`; this new configuration must preserve the existing hook contract.

Decision 49 execution-edge refinement after observed acceptance RED: retain
normal direct process replacement, and on an exec failure use the baseline's
PATH-selected Bash through a constant positional exec program. Bash supplies
its own no-shebang fallback and platform-specific failure status; no caller
argument is interpolated into shell source. Bash remains a prerequisite for
this candidate stage. The Go dispatcher still selects only canonical routes.

Decision 49 lint review correction: explicitly enable revive default rules
while customizing the two allowed dot-import packages. Pinned linter debug
output showed a custom rule list otherwise drops 22 defaults; retain all 23.

Decision 49 diagnostic characterization: native Bash exec failures preserve
status and error meaning but identify a different invocation/source-line context
in this candidate. Record that raw-framing difference explicitly, strengthen
independent comparison after narrow fixture-context normalization, and leave
production acceptance open rather than silently approving a parity exception.

Decision 49 CI/review follow-up: the first Linux matrix run passed acceptance,
lint and gosec, then govulncheck v1.1.4 panicked inside its old x/tools SSA
builder on Go 1.27.1. Repair scanner/toolchain compatibility without removing
or weakening the vulnerability gate. Source: GitHub Actions run 34071286184,
read 2026-09-06. Simplify the reviewed syscall.Exec boundary by removing its
unreachable success-return branch; a successful exec never returns.

Decision 49 scanner pin correction (2026-09-06): the authoritative Go module
source has golang.org/x/vuln v1.7.0, tagged 2026-08-13 at commit
617f44b718537dccdea1915395650e0529e3b72e:
https://go.googlesource.com/vuln/+/refs/tags/v1.7.0 .
`go list -m -json golang.org/x/vuln@latest` independently reports v1.7.0 and
2026-08-13T18:01:04Z. The earlier GitHub Releases page was stale; use this
current authoritative module tag for the scanner and retain symbol-level
vulnerability scanning on both platforms.

## Decision 50 (2026-09-06): approve precedence correction and initial rollout coverage

What: Anoop answered yes to both implementation questions. Correct EX-001 as
a separately tested prerequisite before Go configuration conversion: explicit
caller values take priority over factory.yaml, then legacy factory.config.
Preserve explicitly set empty values, fixed-key parsing, literal data handling
and unrelated configuration behavior. Observe independent external Ginkgo/
Gomega RED before changing the shared Bash implementation; retain immutable
historical evidence and identify the correction when constructing later Go
parity baselines rather than rewriting historical hashes.

The initial rollout covers v0.1.6 and merged Bash baseline
76952eaa63aebd1ecd282f5ab51dd7c3627cb497. Preserve unsupported older/customized
installations without destructive migration. A manual adopter pilot is required
before Go becomes default. Minimum platform/runners, artifact authenticity and
the detailed pilot evidence/exit criteria remain to be settled at their named
stages; this approval does not authorize default cutover or deletion.

Why: user confirmation in this conversation, 2026-09-06, to both explicit
questions on EX-001 and release coverage/manual pilot. PR #74 remains open;
keep this prerequisite in a separate stacked PR using its existing independent
Go acceptance tooling. Do not merge either PR automatically. The feature table
continues to be reported only in chat after each iteration.

Decision 50 compatibility refinement before implementation: retain the standalone
one-argument legacy loader's existing behavior. During full configuration export,
an optional caller-key snapshot may suppress only assignment to an already-set
caller variable, including empty/readonly values. A matching legacy entry still
exports that variable, preserving the existing sourceable export-bit effect; an
unmatched local caller remains local. No value serialization or shell evaluation
is introduced. Preserve duplicate-key parsing behavior outside the correction.

Decision 50 baseline identity: the official v0.1.6 release API reports
b71ecc32e07ecd87eb330ba8e497c86612f92acd, published 2026-09-06T19:10:47Z,
matching local tag resolution. Source read 2026-09-06:
https://github.com/anoop2811/software-factory-template/releases/tag/v0.1.6 .
Keep this identity alongside the merged Bash baseline for fixture provenance.

Decision 50 baseline correction artifact: derive an inert EX-001 patch
deterministically from the reviewed shared-library diff, with metadata recording
its patch SHA-256, both immutable baseline commits, original blob hash/mode and
corrected blob hash/mode. Independent acceptance must apply it to each exact
historical source and exercise desired precedence; mismatched bytes or corrupt
artifacts must fail. This is compatibility-fixture evidence, not installer
ownership/deletion authority, a runtime fallback or a rewritten historical tree.

Decision 50 integration refinement: all four current CI workflows filter pull
requests to main (for example .github/workflows/go-runtime.yml:8). A separate
stacked PR targeting the foundation branch would receive no CI. Keep the
prerequisite separately committed and reviewed within still-open PR #74 instead,
so its Linux/macOS checks run without weakening or expanding workflow triggers.
The temporary worktree remains isolated from the user's existing checkout.

## Decision 51 (2026-09-06): Go read-only configuration and role bridge

What: after PR #74 merged at 732d2736a192085b2f9cc2f58a51fb523d1a8887,
implement the next independently tested G1 candidate slice: configuration path
resolution, flat-format get/has, role_tier and resolve_tier. Use one Go binary
and Cobra routing. Keep the existing public factory commands and installed
sourceable libraries unchanged while artifact delivery/recovery remain open.

Private local-build protocol: FACTORY_BRIDGE_PROTOCOL=1 selects a separate
Cobra command tree in the same cmd/factory binary; ordinary invocation does not
gain public commands. Supported requests are config file, config get KEY
[DEFAULT], config has KEY, role tier ROLE, and role resolve PROFILE TIER.
Only flat identifier keys ([A-Za-z_][A-Za-z0-9_-]*) are accepted by this private
protocol. Regex-like key invocations in the old sed/grep helpers remain an
explicit characterization boundary before any public adapter replacement.
Unknown protocol versions and malformed private requests refuse with exit 2.
The key selector restriction is not an approved legacy behavior correction.

Candidate sourceable runtime/shell/readers.sh exposes the five existing reader
function names, forwarding only to an explicitly configured absolute local
FACTORY_RUNTIME_BINARY. Missing/nonexecutable runtime refuses; no PATH search,
fallback, download or compilation occurs during a call. The POSIX-compatible
shim forwards literal argv/stdout/status without eval, assignments from Go output
or temporary files. Sourcing it has no I/O or environment mutations.

Preserve the supported baseline semantics: FACTORY_CONFIG selection, Git-root
fallback, first matching key, quoted hashes, unquoted comments/trailing spaces,
empty versus missing get/has behavior, missing/nonregular files, large values,
and case-sensitive standard/economy role mapping. Compare both immutable
baselines (with the approved EX-001 correction where relevant), independent
expected vectors, and the compiled Go boundary before implementation.

Configuration export/legacy loading, config writes, local-hook tokenization,
installed routing, packaging and lifecycle cleanup are separate subsequent
slices. No G0/G1 stage is complete from this read-only slice, and no active
legacy implementation is retired before the approved recovery/activation gates.
The new Go computation is canonical for candidate reader calls and contains no
Bash parser or per-harness policy copies. Existing pinned tooling is unchanged.

Why: user direction to proceed with the next conversion step after merging
PR #74. This bounds the next outside-in TDD cycle while preserving the approved
release coverage, manual pilot and cost-conscious native-call policy. Progress
tables remain in chat; no automatic merge or release is authorized here.

Decision 51 characterization refinement before parser implementation: candidate
whitespace parsing covers C/POSIX ASCII classification. Historical sed/grep
classification varies with locale and platform; non-C locale parity remains an
explicit unresolved activation boundary, not an approved behavior correction.
Do not change the parent locale. Preserve Bash command substitution's removal
of NUL bytes in selected values, after literal key selection and leading-space
removal; a NUL inside a key must not make a different key match. Historical
Linux NUL-warning text and sed/grep versus Go read-error framing remain diagnostic
characterization boundaries. Read errors retain the legacy get default/status 0
and has status 2, with the file and reason on stderr.

Decision 51 enforcement refinement: protect runtime alongside the existing Go
and shell implementation paths. Require the candidate sourceable shim in the
factory-only Go source gate, run POSIX syntax there on both platforms, and add
POSIX shellcheck to the existing provisioned Template CI shellcheck step. These
source-tree checks must not add a runtime or tool requirement to adopters.

Decision 51 shell-state refinement before the corresponding implementation fix:
pass child-only configuration/protocol values through /usr/bin/env with literal
assignment operands, rather than shell prefix assignments that collide with
readonly caller variables. The source candidate already targets Linux/macOS;
/usr/bin/env is an explicit candidate prerequisite, not a PATH runtime search.
Preserve sourceable helper missing-argument behavior under nounset by retaining
the existing required positional expansions; optional defaults remain optional.
Independent regression RED must precede these adapter corrections.

Decision 51 test-portability refinement: characterize the existing missing-
interpreter fixture under the caller-selected Bash as well as the system Bash.
If the immutable baseline and candidate both return 126, admit that observed
platform result alongside 1/127 in the baseline sanity assertion; retain exact
candidate-to-baseline status and normalized diagnostic equality. Do not change
production dispatch or mask a differential failure by selecting a different PATH.

## Decision 52 (2026-09-06): configurable adversarial-review output cap

The advisory OpenRouter review lane's fixed 4096-token cap produced an
incomplete live review with `finish_reason: length`. Raise the repository
default to 8192 and expose `review_max_tokens` / `REVIEW_MAX_TOKENS` with
caller-over-YAML-over-legacy precedence. Accept only decimal values from 1024
through 32768 and reject invalid input before the HTTP request. Keep Anthropic's
4096 request and OpenAI's existing body unchanged. The value is a ceiling, not
a spend guarantee; one request, the diff bound, timeout, provider pin and
truncation refusal remain in force. Record the live failure and authoritative
OpenRouter token semantics in ADR-0057.

## Decision 53 (2026-09-07): candidate runtime artifact verification

Continue G1 with a private, read-only artifact verifier. A versioned inert
manifest names one release identity, one `GOOS/GOARCH` target, one relative
binary path and one SHA-256 digest. Verification rejects duplicate/missing
fields, traversal or absolute paths, symlink/nonregular files, target mismatch,
bad digests and unreadable inputs before any candidate execution. The Cobra
request is `FACTORY_BRIDGE_PROTOCOL=1 runtime verify MANIFEST ROOT TARGET`;
status 0 emits deterministic metadata, status 1 reports I/O/integrity failure,
and status 2 rejects malformed private operands. No public dispatcher,
installer, download, activation, fallback, cleanup or legacy asset changes are
part of this slice. AC 5.2/5.4 and FR-019/020/022 remain incomplete until
authenticated release delivery, all four target artifacts and lifecycle
recovery are separately evidenced.

CI follow-up: construct malformed artifact manifests from test-owned fixture
data rather than reading a path operand back into the fixture writer. Remove
unused fixture parameters without changing assertions or disabling lint rules.
Manifest values must also reject embedded tabs so emitted metadata retains
exactly four tab-separated fields; spaces in artifact filenames remain valid.
Open the binary through an `os.Root` handle confined to the artifact directory
and check the opened file type before hashing. The manifest remains an explicit
caller-selected read-only input and may live outside the artifact directory.

## Decision 54 (2026-09-07): deterministic local runtime selection

Add the explicit private `runtime resolve STORE VERSION TARGET` prerequisite
described in ADR-0059. Resolve one version/target slot and verify both identity
and integrity without execution, fallback, compilation or network access.
Independent acceptance precedes implementation. Release qualification and
authentication remain open questions; this read-only candidate does not
activate adapters, replace legacy assets or claim completed packaging.

## Decision 55 (2026-09-07): source bundles and attested release delivery

Anoop approved Ubuntu 24.04+/macOS 14+ on amd64/arm64 and GitHub build
attestations with offline bundles. Implement ADR-0060: build an explicit source
commit into a deterministic candidate archive, test native packaged binaries,
and attest release archives in a trusted release workflow. Keep local labels
distinct from authentication; trust roots and expected source/workflow identity
come from outside the downloaded artifact. Preserve existing outputs and clean
owned build staging. No adopter activation or legacy deletion occurs here.

## Decision 56 (2026-09-07): authenticated bundle staging

Implement ADR-0061 as the next G1/G4 distribution prerequisite after PR #83.
Stage exact local or attested release archives through an explicit developer
Cobra command. Authenticate private input snapshots before interpreting archives,
validate the complete bundle identity, and publish only into a new private root.
Independent outside-in acceptance precedes implementation. Keep delegated trust
tests distinct from live signing evidence; activation and recovery lifecycle work
remain separate gates. No existing installation assets are replaced in this slice.

Review refinement: keep the source-bundle compiler identity in the shared artifact
contract so packaging and staging cannot drift through separate version literals.
The independent acceptance fixtures retain their explicit expected version.

## Decision 57 (2026-09-07): Go local-hook entry normalization

Implement ADR-0062 as the next G1 sourceable-library slice. Keep caller-shell
field splitting and pathname expansion in the thin adapter; move hook grouping
and comma-field normalization into one Go implementation behind the private
Cobra protocol. Configuration remains literal data, source-time execution is
forbidden, and the candidate retains explicit runtime selection without fallback.
Independent baseline/expected-output Ginkgo acceptance must fail before code.
Public routing and installation lifecycle remain unchanged; this does not
authorize legacy cleanup or claim full G1 compatibility.

Pre-implementation characterization refinement: empty tokens produced by adjacent
non-whitespace IFS delimiters terminate the prior hook and reset flag attachment.
They cannot simply be discarded: `a.sh::--strict:b.sh` with IFS `:` prints only
`a.sh` and `b.sh`. Preserve that boundary in Go and independent acceptance.

Security refinement before correction: review reproduced arithmetic execution of
configuration assigned to an inherited integer scratch variable. Remove named
scratch assignments from the new adapter; use positional data/status transport
and preserve IFS through literal positional slots. Require an independent
marker-file RED/GREEN regression and literal output with inherited attributes.

## Decision 58 (2026-09-07): candidate Go configuration writes

Implement ADR-0063 for `factory_config_set` behind the private Cobra protocol.
Preserve baseline literal replacement/append bytes and unrelated settings, use
a same-directory prepared replacement for admitted regular files, and refuse
unsupported links/modes or invalid requests without fallback. Independent
baseline and failure acceptance must fail before implementation. This explicit
single-writer candidate does not claim installed activation, concurrent legacy
writer exclusion, transactional migration or recovery retention/cleanup.

Review refinement before correction: when publication detects an unexpected
temporary-file identity, cleanup must not delete that unknown replacement by
pathname. Capture and recheck the created inode before cleanup; preserve
mismatches with a visible refusal. A deterministic collaborator regression must
fail before correction. This does not expand the single-writer contract to CAS.

Path characterization refinement before correction: the legacy setter captures
its resolved filename through command substitution, which removes trailing LF.
Preserve that trimming at the setter boundary, including an empty result, without
cleaning embedded path components or changing the independent reader protocol.

Delivery evidence refinement: add writer cases to existing packaged conformance,
prove an immutable pre-writer binary fails them, and exercise the staged writer
on all four native targets with Go/Python absent from runtime PATH.


## Decision 59 (2026-09-07): Go native usage accounting candidate

After merged PR #86, implement ADR-0064's metadata-only accounting boundary for
Codex, Claude Code and OpenCode as the first G2 candidate slice. Preserve native
unknown/null and failure/completeness semantics before porting admission and
process supervision. Use an explicit private Cobra request over decoded event
arrays, an immutable Python normalization oracle, outside-in Ginkgo/Gomega TDD
and native packaged conformance. No installed routing, paid call, active legacy
retirement or upgrade cleanup is authorized by this slice; their existing
migration acceptance gates remain open.


## Decision 60 (2026-09-07): distinguish review transport deadlines from token exhaustion

Implement ADR-0065 after the confirmed curl 180-second cutoff on PR #87. Give the
single request a configurable bounded deadline through an environment/Actions
variable, preserve cost controls and expose safe failure diagnostics. Do not
pretend a timeout returned a model review or claim a provider-side root cause
that the old logs cannot establish. Decision 59 belongs to the separate pending
Go accounting PR #87; this correction is based directly on main.


## Decision 61 (2026-09-08 UTC): select GLM 5.3 Flash for adversarial review

Select `z-ai/glm-5.3-flash` in the repository's
`review_model`. Retain the DeepInfra provider selection, reasoning effort `none`,
8192-token cap, bounded transport deadline and single-request policy. Update
the repository request fixture and self-hosting documentation to match. Native
harness model tiers and adopter defaults are outside this configuration change.

Source fetched 2026-09-08:
https://openrouter.ai/api/v1/models/z-ai/glm-5.3-flash/endpoints confirms the
model ID and a DeepInfra endpoint advertising reasoning and max_tokens support.
This metadata does not establish live review quality or reliability. The trusted
base workflow adopts the setting on new PR events after this change is merged.

This decision supersedes only the DeepSeek model selection in ADR-0055; its
provider-routing contract remains in force. ADR-0057 continues to define the
output cap, and ADR-0065 defines the transport deadline. The requested model
change is tracked in https://github.com/anoop2811/software-factory-template/pull/89.
Dates in this entry and its source check are UTC.


## Decision 62 (2026-09-08 UTC): use supported GLM review reasoning

Supersede Decision 61's retained `none` reasoning setting with `low`, the lowest
documented effort for GLM 5.3 Flash. Keep the model, DeepInfra route, 8192-token
cap, transport deadline and single-request policy. Reasoning shares the output
cap; low effort does not disable thinking or guarantee a complete review.

PR #87 run 34178226454 returned HTTP 400 after 0.130382 seconds, not a timeout.
The exact server error body was not retained. The incompatible setting is a
confirmed configuration defect and a likely explanation, not a proven decoding
of the discarded response. A fresh trusted-base PR event after merge is needed
to establish live success.

Source fetched 2026-09-08 UTC: https://docs.z.ai/guides/capabilities/thinking
states that GLM-5.3/Flash cannot disable thinking and supports low/high/max API
efforts. Listing a reasoning parameter in endpoint metadata does not establish
which values a model accepts. Update the repository request assertion and
self-hosting guidance; keep generic effort support for other models unchanged.


## Decision 63 (2026-09-08 UTC): bound the GLM review output allowance

PR #87 run 34178898119 completed its HTTP request after the low-effort correction
but returned `finish_reason=length`, so no complete review was published. Increase
this repository's configured review_max_tokens from 8192 to 32768, the maximum
accepted by the current factory setting, as requested by the user. Retain GLM
Flash, low reasoning, DeepInfra, the 480-second deadline and one request per run.
Reasoning and final text share this allowance; raising the token ceiling can
increase billed output and does not guarantee completion or quality.

This supersedes only Decision 62's retained repository cap. Keep the shared
runner/adopter default at 8192 and the validated 1024..32768 range from ADR-0057.
Update the real repository request assertion and distinguish the configured cap
from the shared default in self-hosting docs. No automatic retry is introduced.

Observed source: https://github.com/anoop2811/software-factory-template/pull/87#issuecomment-5578045428
(run https://github.com/anoop2811/software-factory-template/actions/runs/34178898119).
Live completion requires a fresh trusted-base PR event after merge.


## Decision 64 (2026-09-08 UTC): Go raw native event parsing candidate

Implement ADR-0066 after PR #87's decoded usage-accounting boundary. Preserve
whole-document-first Python parsing and line fallback before shared accounting,
with independent compiled CLI and packaged conformance. Keep installed runtime
and recovery/retirement gates unchanged. Non-finite numbers, Unicode surrogate
identity and splitlines behavior require explicit parity rather than assuming
Go's default JSON decoder is equivalent.


## Decision 65 (2026-09-08 UTC): allow bounded long review completion

PR #92 run 34190938017 reached the 480-second client deadline with HTTP 200,
first byte at 0.336393 seconds and 1760 bytes received. No complete review was
produced. This confirms the request deadline, not the provider's exact latency
cause or the meaning of its partial bytes.

Raise the review request default/ceiling to 1200 seconds, configurable through
existing REVIEW_TIMEOUT_SECONDS (1..1200); raise both active and generated
workflow jobs to 25 minutes, leaving five minutes for setup and posting. This
supersedes ADR-0065's deadline/ceiling and ten-minute job sizing only. Keep the
15-second connection limit, model, reasoning, token cap, diff cap and one-request
policy. No retry, fallback or claim of provider reliability is introduced.
Longer execution consumes more Actions time; it does not raise the model output
cap. An empty/unset variable uses 1200; smaller explicit values remain supported.
Normalize leading zeroes and bound decimal length before arithmetic.

Source: https://github.com/anoop2811/software-factory-template/pull/92#issuecomment-5579866181.
Decision 64 belongs to the separate Go stream-parsing PR #92. A new PR event
after this correction is merged is required for live trusted-base qualification.


## Decision 66 (2026-09-09 UTC): bounded streaming adversarial review client

Implement ADR-0067 after the transport-only deadline correction. Use the existing
Go/Cobra runtime for an explicitly selected OpenRouter streaming transport,
retaining the shared shell entry point and legacy adopter default. Validate full
completion before publishing, distinguish safe progress from final findings,
and allow an opt-in single retry only for initial HTTP 429/503. Keep the selected
model/provider and output cap; no hidden model/provider failover or ambiguous
request replay. This repository selects the Go client and one retry in its
trusted-base workflow, with Actions variables permitting zero retries. Record
research and independent outside-in failure evidence before claiming delivery.


## Decision 67 (2026-09-10 UTC): Go native harness execution component

Implement ADR-0068 under the approved Go conversion specification. Port native
invocation preparation, local capability checks, process-group supervision and
answer selection for Codex, Claude Code and OpenCode. Exercise the internal
component through independently compiled fake-process acceptance. Do not expose
a public or private launch command that bypasses the controller's locked budget
admission and ownership publication. Record bounded help probing and refusal of
nonpositive allowances as explicit safety tightenings of the immutable baseline.
Installed controller activation, interoperable ledger integration and legacy
retirement retain their existing delivery/recovery gates. No paid native run is
part of implementation or acceptance.


## Decision 68 (2026-09-21 UTC): interoperable Go budget ledger and admission

Implement ADR-0069 under the approved Go runtime conversion specification.
Preserve schema-1 history, exact counters, unknown fields and the persistent
Python flock inode. Add read-only planning/reporting and atomic reservation,
PID publication and finalization as an internal component. Deadline-aware lock
waiting, bounded writes and explicit ambiguous-publication errors prevent
unsafe retry or spending after uncertain storage. Record narrowed input and
filesystem qualification before implementation. No public run route, installed
activation, paid invocation or legacy retirement belongs to this slice.
Decision 67 and ADR-0068 are in the separately open native-execution PR #95;
this independent component is based on main and does not assume its merge.


## Decision 69 (2026-09-22 UTC): compose the Go budget execution controller

Implement ADR-0070 by composing the existing ledger, native supervisor and usage
parser. Keep read-only admission before native probes, recheck deadlines inside
the shared lock, publish child ownership before stdin, and finalize exactly once
with bounded cleanup after cancellation. Return an answer only after confirmed
ownership and durable final metadata. Do not expose a launch route or activate
an installed replacement in this slice. Prefer idiomatic concrete Go composition
over new abstraction frameworks; additional libraries require concrete benefit
and verified permissive open-source licenses. Existing dependencies suffice.


## Decision 70 (2026-09-23 UTC): expose a source-only Go budget command candidate

Implement ADR-0071 using the existing Cobra, ledger and execution controller.
Preserve live admission-plan output, final metadata and answer privacy through a
private compiled command boundary. Characterize plan/report text and JSON against
the immutable Python baseline before implementation. Keep installed shell model
routing, public activation and legacy retirement for their existing gates.


## Decision 71 (2026-09-23 UTC): complete source budget argument compatibility

Implement ADR-0072 with the existing Cobra/pflag command schema and a bounded
legacy argument normalizer. Preserve help/usage channels, statuses, literal
values, unique prefixes and help/error precedence against the immutable Python
oracle. Enumerate permitted human-prose normalization before independent RED
tests; retain machine output and all no-effects requirements. Keep installation,
shell model routing and legacy retirement outside this source-only slice.

Follow-up 2026-09-25 UTC: Linux CI exposed a CPython patch-level difference for
malformed mixed short-help groups. Retain the specified deterministic Go refusal;
ADR-0072's short-help qualification explicitly bounds this source-parity exception
and separates fixed Go assertions from interpreter-dependent oracle observations.
The macOS job exposed further newer-parser differences; pin the CI oracle to
maintained Python 3.12.14 through version-pinned uv, without relaxing those tests
or adding a Python requirement to the Go runtime.

## Decision 72 (2026-09-24 UTC): establish Go loop checkpoint fingerprints

Implement ADR-0073 as the read-only foundation of loop-controller conversion.
Preserve Python's canonical identity, Git/source/policy inputs and explicit
configuration semantics. Reuse shared numeric handling and bound private probe
and file reads; qualify refusals before activation. Add independent compiled-CLI
RED evidence before implementation. Checkpoints, process execution, recovery
transitions and installed cleanup remain separate milestones.

## Decision 73 (2026-09-24 UTC): preserve Go loop checkpoint storage and recovery

Implement ADR-0074 as milestone two of the existing loop conversion package.
Share the proven budget filesystem mechanics, preserve persistent Python lock
interoperability and checkpoint identity, and expose private read/roundtrip and
read-only resume assessment for independent qualification. No process recovery,
counter reset, automatic retry or installed activation follows from assessment.
Keep manual and bounded controllers as subsequent milestones. Require independent
compiled RED and fault-injected publication/locking evidence before completion.

Follow-up 2026-09-25 UTC: a terminal-input cancellation reproduction remained
blocked after SIGTERM. ADR-0074 now requires pre-read refusal of unsupported
nonregular devices, preserving caller ownership and the regular-file caveat.

## Decision 74 (2026-09-25 UTC): execute and resume Go manual loops

Implement ADR-0075 as milestone three of the existing loop conversion package.
Compose stable fingerprints, locked checkpoints and shared process supervision
for deterministic checks with cumulative resume allowances. Require independent
compiled RED, Python parity and lifecycle failure evidence. Keep bounded paid
loops, public argument integration and installed activation/cleanup separate.

## Decision 75 (2026-09-25 UTC): compose Go bounded loops and loop commands

Implement ADR-0076 as the final source milestone of the existing loop package.
Share manual check lifecycle, checkpoint storage and budget admission/execution
for bounded implementation, review and repair. Preserve strict verdicts, finite
allowances, evidence freshness and terminal handoffs. Qualify command semantics
with independent outside-in tests; keep installed activation and cleanup pending.

## Decision 76 (2026-09-26 UTC): compose Go command configuration and model routing

Implement ADR-0077 by composing qualified configuration/export and role helpers
with existing Go budget and loop commands. Preserve wrapper-derived settings,
caller/model precedence, literal argument scanning and three-harness model choices.
Qualify private configured routes through independent shell-oracle and compiled
CLI tests before public dispatch or installed activation. Retain the existing
progress denominator and separate migration/retirement gates.

## Decision 77 (2026-09-26 UTC): route source-built budget and loop commands through Go

Implement ADR-0078 by sharing configured-command orchestration between private
qualification routes and public commands in the locally built binary. Preserve
unaffected script exec boundaries and exact statuses; prohibit implicit fallback
or repeated paid work. Qualify through independent outside-in tests before any
installed dispatch change, while retaining explicit compatibility exceptions.


## Decision 78 (2026-09-26 UTC): assess legacy installation reference files without mutation

Implement ADR-0079 as the first prerequisite of G4 ownership/preview planning.
Compare six budget/loop assets against a compiled immutable reference through
confined read-only observation. Matching bytes never establish installation
ownership, rollback readiness or activation authority. Require independent RED
and negative filesystem evidence, preserve every adopter file, and leave public
preview, trusted action planning and transactional delivery to later milestones.


## Decision 79 (2026-09-26 UTC): plan explicit legacy migration actions conservatively

Implement ADR-0080 by comparing the six assessed legacy paths with an explicit
local target source, reusing the confined observer. List candidate additions,
replacements and explicitly scoped retirements while preserving customizations
and reporting conflicts. Keep origin, authentication, quiescence and recovery
prerequisites visibly blocked; untrusted local metadata never grants ownership.
This closes only target-action planning, not positive provenance or activation.


## Decision 80 (2026-09-27 UTC): authorize explicit adoption of unchanged legacy assets

The user approved explicit legacy adoption and required transparent, bounded
cleanup. ADR-0081 refines historical-ownership-only requirements to permit new
operator-granted management authority over selected unchanged reference files.
Bind confirmation to exact observations and revalidate; never infer consent from
local metadata or claim historical provenance. Preserve modified/unknown files,
keep activation and recovery gates separate, and retain the existing successful-
later-release pruning policy with visible exceptions. No progress credit follows
from the decision before independent implementation qualification and merge.


## Decision 81 (2026-09-27 UTC): expose a scoped read-only Go upgrade preview

Implement ADR-0082 by composing the qualified planner and explicit adoption at
public upgrade --dry-run --source PATH. Reserve preview intent before legacy
dispatch so invalid requests cannot mutate through fallback. Report partial
six-file coverage, exact actions, confirmation and unresolved prerequisites.
Use the current directory without Git/config execution; preserve nonpreview
legacy dispatch and create no local migration state. Complete the existing public
preview milestone only after independent qualification and merge; installation,
recovery and cleanup remain separate work.

Follow-up 2026-09-27 UTC: compiled closed-stdout evidence exposed SIGPIPE exit
before the documented preview status-1 path. ADR-0082 now requires a separate
preview-scoped buffered SIGPIPE notification with deferred Stop, preserving
cancellation and all other routes. Independent RED preceded this correction.


## Decision 82 (2026-09-27 UTC): inspect recovery integrity without granting authority

Implement ADR-0083 as the read-only inventory prerequisite of backup/rollback.
An opt-in public preview inspects bounded private inert recovery records and
saved reference assets. Self-consistent metadata never proves a committed
transaction, restoration eligibility or pruning authority. Preserve current
preview defaults and every activation blocker; no state or backup is created.
Qualify independent outside-in failures, filesystem confinement and resource
limits before granting only the inventory milestone of the existing package.


## Decision 83 (2026-09-27 UTC): exclude recovery copies from active gate discovery

Implement ADR-0084 before durable recovery creation. Ignoring backups is not
sufficient: recursive citation and language-dialect gates still consume them,
and archived docs can satisfy current citations. Prune only the reserved recovery
subtree from source and target discovery, retain active enforcement elsewhere,
and explicitly exclude tracked recovery from the Go gate. Do not claim universal
control over arbitrary user checks or native scanners, or award writer progress
before its remaining prerequisites and implementation qualify.


## Decision 84 (2026-09-28 UTC): implement native Go initialization

Implement ADR-0085 for the user-requested command conversion sequence, beginning
with init. Preserve its installation and pack behavior while keeping independent
outside-in acceptance separate from production. Explicitly retain synchronization,
review-lane and gate subprocess boundaries until their corresponding conversions.
Do not confuse source-native orchestration with installed Go activation or authorize
legacy deletion through initialization. Qualify filesystem and cancellation safety
before merging this command and proceeding to doctor.

Follow-up 2026-09-28 UTC: PR #112 exposed an operating-system argument-size
failure in the trusted-base review script before any model request. Carry the
full prompt and JSON request through private temporary files/stdin, for both
request shapes and every transport, rather than command arguments. Preserve
request contents, provider settings, deadlines and no-fallback behavior. Remove
owned temporary data on exit and exercise a diff above 128 KiB independently.
The trusted-base workflow cannot consume this correction until it is merged;
its advisory success status is not evidence that a model reviewed this PR.

The macOS qualification then exposed the preexisting whole-diff whitespace
substitution in Bash 3 exceeding the test deadline for a permitted 192,226-byte
diff. Use a non-whitespace presence test without constructing a stripped copy;
preserve empty/whitespace-only behavior. Qualify the real /bin/bash interpreter
used by the script shebang, retaining the existing test deadline.


## Decision 85 (2026-09-28 UTC): implement native Go health diagnostics

Implement ADR-0086 after merged native init. Preserve doctor classifications,
proof and adapter checks while moving its orchestration into Go. Generate adapter
comparisons in private scratch state instead of rewriting and restoring adopter
files. Reuse shared configuration and child supervision, keep explicit validation/
synchronization boundaries, and report uncertainty honestly. No installed runtime
activation or legacy cleanup is authorized by a health report. Qualify and merge
this command before beginning the next requested conversion.

Implementation inspection corrected the documented inherited capture bound: the
shared supervisor limits each output stream to 16 MiB (internal/native/prepare.go).
ADR-0085's earlier 32 MiB statement was inaccurate; align it and ADR-0086 with
the unchanged implementation rather than increasing resource limits during a port.

## Decision 86 (2026-09-29 UTC): implement native Go cost reporting

After merged doctor PR #113, implement ADR-0087. Preserve the ordinary cost
report and configuration/event contracts without invoking the old report script.
Reuse configuration grammar and bounded process supervision. Keep clearing a
best-effort unlink that never removes directories or follows a final symlink.
Explicitly correct empty-log double-zero output and shell octal/wrapping
arithmetic: use decimal uint64 estimates with checked multiplication and a
nonzero diagnostic on overflow. Count newline-bearing hook names once instead
of counting their printed lines, and reject binary NUL event data whose shell
interpretation differs by Bash version. Qualify boundaries independently before merge;
do not award installed activation or legacy-retirement credit for this command.

## Decision 87 (2026-09-30 UTC): implement native Go local metrics

After merged report PR #114, implement ADR-0088. Move metrics computation and
text/JSON/HTML rendering from Bash and embedded Python to Go, retaining the
versioned schema, ordinary outputs and local-only behavior. Reuse bounded file
and process components. Make generated HTML publication atomic and refuse
unsafe destinations; preserve browser launch as an explicit detached,
best-effort presentation boundary gated by TTY, CI and --no-open. Document
numeric/resource bounds, stable sorting and per-file malformed-eval fallback
rather than silently inheriting interpreter-dependent failures. Qualify and
merge metrics before starting the next requested command.

Metrics review identified possible output amplification from repeated template
markers or per-task harness metadata. Bound generated JSON/HTML before full
allocation, require one template marker, and cap aggregate eval task rows as
specified in ADR-0088. Input byte limits alone do not bound derived output.

A compiled metrics regression then observed SIGTERM fail to terminate a JSON
write while the reader kept stdout open but stopped draining it. Correct the
shared pipe/socket output cancellation boundary, preserving borrowed descriptor
ownership and restoring its flags, before qualifying metrics. A closed-pipe
test alone did not exercise this blocked-write case.

## Decision 88 (2026-09-30 UTC): implement native Go review-lane management

After merged metrics PR #115, implement ADR-0089. Preserve the advisory lane's
ordinary command behavior with bounded local reads, exact managed-workflow
ownership and config-before-publication ordering. Validate GitHub secret names
before literal substitution, reuse cancellable terminal and publication
components where their contracts match, and route init/doctor lane operations
through the native service without changing process-global cwd. Keep hosted
review/model/provider behavior and installed legacy activation unchanged.
Qualify and merge this command before starting config migration.

## Decision 89 (2026-09-30 UTC): implement native Go config migration

After merged review-lane PR #116, implement ADR-0090. Preserve migration-specific
parsing, YAML precedence and the single legacy recovery name. Prepare and publish
one bounded YAML transformation, then perform a no-replace legacy rename.
Reject unsupported values and mismatched explicit config overrides before
mutation; preserve recovery artifacts and report partial publication honestly.
Reuse the configuration writer without changing its existing single-key
contract. Qualify this last requested native command before declaring the
six-command source implementation complete; installed cutover remains separate.

2026-10-01 UTC qualification refinement: hosted review identified preparatory
operations between the caller's final legacy guard and its rename syscall.
Revalidate the captured snapshot inside the pinned rename operation after those
operations, with a failing replacement regression first. Preserve the trusted,
quiescent directory contract rather than claiming a concurrent-writer atomic
compare-and-swap that a pathname rename does not provide.

Copilot's subsequent performance review found a separate bounded-input weakness:
planning repeatedly scanned/copied YAML for each of up to 4096 settings. Replace
that quadratic work with a selected-key index and accumulated physical edits,
then render once. Preserve frozen duplicate and unterminated-tail semantics and
every intermediate size bound. Qualify representative large valid inputs with
an observed admission handshake and a finite deadline before merging.

Final CI qualification observed the preceding macOS run 36813827308 exceed the
15-minute whole-package allowance while progressing through existing command
environment fixtures, without an assertion failure. Use the existing make
override to allow 20 minutes for the CI suite and 25 minutes for the enclosing
job. Keep local defaults and every operation/performance deadline unchanged.

## Decision 90 (2026-10-01 UTC): create durable locally ignored recovery sets

After merged PR #117, implement ADR-0091 as the next approved conversion slice.
Create private inert copies only for explicitly adopted unchanged reference
assets, establish effective local Git ignoring before payload writes, and require
durable sync and integrity readback before reporting completion. Reuse the
existing confined observer and strict recovery format. Preserve interrupted or
conflicting sets and every active original; repeated creation must not overwrite
evidence or advance retention. Expose a strictly reserved source-built creation
mode while preserving previews and installed legacy dispatch. This closes only
the durable creation milestone after qualification; restoration, complete
installation coverage, activation, retirement and release cutover remain pending.

Disable repository-configured fsmonitor on the fixed Git metadata queries and
qualify a configured executable sentinel, so exclusion checks cannot implicitly
run a user helper. Sanitizing inherited Git variables alone does not override
repository-local configuration.

2026-10-02 UTC qualification refinement: a partial local-exclude append can turn
the intended backup-only pattern into `/.factory`, silently hiding unrelated
runtime files. Preserve narrow ignore semantics even on failure: stage an inert
comment, sync and read back the complete line, then activate its first byte and
sync/read back again. Require a real Git negative control before correction;
incomplete inert evidence may remain, but a broader active rule may not.

PR #118 review refinement, 2026-10-03 UTC: an active local rule surviving a sync
failure is visible evidence, not evidence of durable exclusion. Every creation
and exact reuse must establish the canonical narrow local rule, sync/read back
its validated file and sync `.git/info` before writing backup storage, including
retries and installations already ignored by broader or project rules. Preserve
all existing patterns, incomplete sets and active originals. Add fault regressions
for activation-file and parent-directory sync failures and repeated retry errors.

Preserve legacy operand ownership during dispatch: detached `--source` and
`--ref` values may literally equal the new marker. Only an independent marker
claims creation mode. Exercise explicit wrong confirmation with a correct
environment digest. Qualify the intended private Git fixture modes explicitly
instead of weakening the writer to accommodate inherited creation permissions.
The existing milestone and source-progress denominator remain unchanged.

## Decision 91 (2026-10-03 UTC): plan controlled recovery restoration safely

After merged durable-creation PR #118, implement ADR-0092 as a read-only
restoration prerequisite. Compare only one strict set's selected six-catalog
references against current installed paths, preserve later edits and distinguish
missing candidates from conflicts. Retain metadata/identity validation through
the whole observation and report deterministic ownership, quiescence,
compatibility and activation blockers. Existing recovery manifests are integrity
evidence, not reverse migration ownership; current locks cannot exclude an old
process paused before admission. Therefore active restoration remains gated.
No writes, probes, history mutation, Git changes or legacy execution are allowed.
Preserve creation/preview dispatch and literal legacy option operands. The source
progress denominator remains 30; merged PR #118 earns 60.0%, and this prerequisite
earns no additional controlled-restoration milestone credit.

2026-10-03 UTC review refinement: a changed installed observation must not mark
an otherwise intact backup unsafe. Discard the transient restoration plan and
report one fixed changed-observations diagnostic; preserve assessment/close
failure status-1 precedence. Static recovery exceptions retain their existing
bounded report. Require RED attribution regressions before source correction.

PR #119 review refinement: preserve an already observed assessment error during
the retained installed observer's own ancestry recheck, including a same-read
failure and ancestor replacement. The planner must discard the report with
status 1; successful changed observations remain status 2. Keep the unpinned
historical observer's unsafe classification unchanged. Require a failing
same-observation regression and an explicit historical compatibility control.

Use the production recovery Run entry in planning fault specifications and
remove the new test-only exported wrapper. Shared dispatch already owns the
signal context; do not maintain a second exported entry solely for tests.
## Decision 92 (2026-10-03 UTC): guard cooperating runtime transitions

Adopt ADR-0093 before implementation. Budget preflight precedes ledger locking,
so a shared permanent runtime transition flock must cover preflight through
terminal publication in both Go and legacy controllers. Durable activity
markers survive parent death and remain when child ownership or publication is
uncertain; future exclusive acquisition must refuse rather than infer exit.

Read-only and initially blocked commands remain read-only. Ship the temporary
Python protocol shim through legacy init/upgrade and Go init source inventories;
keep the restricted authenticated binary bundle unchanged. This prerequisite
does not authorize activation, restoration, pruning or unbridged legacy cleanup.
See [ADR 0093](adr/0093-runtime-transition-guard.md) and
specs/001-go-runtime-conversion.md:132.

Implementation refinement, before the help-probe correction: legacy help waits
for its leader through `subprocess.run` without owning a process group. Require
independent descendant RED, then supervised bounded help cleanup before it can
authorize harmless marker removal; typed unconfirmed ownership retains evidence.
This is an explicit unsafe-baseline correction under FR-028, not a new model call.

Further refinement before snapshot correction: independent real public manual
loop diagnostics in both Go and legacy scripts returned successful terminal
checkpoints with empty activity while a snapshot Git helper descendant survived.
Require failing acceptance coverage and reuse supervised command/probe cleanup
for execution snapshot helpers, including successful leaders, before removing
their guard evidence.

2026-10-04 UTC, before the compound-error correction: independent actual-module
fault injection observed guard-close failure replace typed unconfirmed preflight
ownership, dropping its PID and loop uncertainty. Preserve the original direct
error type/PID and report cleanup failure through one shared legacy release
helper; retain the activity marker. Ordinary success plus cleanup failure must
still fail rather than hide the cleanup error.

2026-10-04 UTC, before hosted-review corrections: Go signal acknowledgement plus
leader reaping does not establish owned-group disappearance. Require bounded
absence confirmation in the shared supervisor, within existing cleanup limits,
and a real ready-descendant RED before correction. Preserve typed ownership/PID
on remaining or uninspectable groups. A compound Go loop guard-close failure
must also preserve checkpoint PublicationError/MayHaveCommitted in an error chain;
qualify through an independent compound regression. See ADR 0093's Go refinement.

Qualification clarification, 2026-10-04 UTC: bounded group inspection may retry
non-absence observations within the existing cleanup deadline. Success requires
a later explicit ESRCH before expiry; no permission or inspection error is
absence evidence. Persistent errors and remaining groups retain typed ownership.

## Decision 93 (2026-10-04 UTC): derive restoration from live publication ownership

Adopt ADR 0094 before source changes. An imported v1 reference set cannot prove
that the factory changed a destination. Build a replacement-only live component
whose opaque ownership handle comes from an actual qualified publication. Fresh
exact consent and an inert checked original remain separate inputs; the handle
may restore only its own unchanged after-inode/content/mode under the same
exclusive guard. No new public rollback or activation command is enabled.

Durable fixed pending evidence precedes active mutation. Cooperating Go/Python
guards refuse any such evidence; unfinished close or crash preserves it. Only a
checked durable reversal can clear the exact owned pending entry. Join cleanup
errors without losing primary publication uncertainty. Future consumers retain
target/state/activation and unbridged-quiescence obligations. See
[ADR 0094](adr/0094-live-publication-restoration.md).

Define the remaining backup/rollback third before implementation as three equal
source deliverables: live restoration core, durable forward/reverse lifecycle,
and public compatibility integration. The fixed denominator remains 30; only
the first can earn 1/9 package after qualification and merge, moving 60.0% to
60.4%. This does not close installed rollback or release acceptance.

Pre-implementation clarification: preserve the trusted quiescent namespace
boundary of ADR 0091. Rechecks do not provide atomic compare-and-swap against
hostile same-user writes. Distinguish missing-API compile RED from subsequent
independent runtime RED on interface-only unsupported stubs before behavior.

Before source, explicitly make the live engine filesystem-only: no Git queries,
native probes or subprocesses. Inspect and re-sync existing saved data; do not
reuse creation's Git orchestration. Independent process sentinels start after
fixture backup creation and cover the component's complete lifetime.

Before the prepared-descriptor cleanup correction: the independent paired
actual opened-stage control observed unsafe mode plus actual close/EIO lose
the operational close failure and return refusal status 2. Report operational
status 1 while preserving the safe refusal and pending evidence; keep the
confirmed-close control at status 2. Reuse a private per-operation boundary,
not a forged identity or exported test wrapper. See ADR 0094's refinement.

Before the incomplete-close diagnostic correction: a distinct prepared-abort
control observed actual pending unlink followed by parent-sync EIO, with the
original durably unchanged. Close incorrectly claimed the absent evidence was
retained. Require neutral incomplete-operation wording and local inspection;
the applied-operation control masks the inner wording and was not a diagnostic
RED. Keep all checked durability, ownership and retry requirements unchanged.

Before the live-handle alias correction: an independent actual external API
value copy before first lifecycle use let alias Restore report success and
clear pending while the original handle's named replacement remained active.
Move lifecycle phase, mutex and owned resources behind one private shared state
pointer so legitimate aliases cannot fork authority or cleanup. Refuse nil/zero
handles and retain every actual after-image/durability check. Observe the direct
value-copy regression GREEN; a no-copy prose convention is insufficient.

Before the post-rename observation correction, 2026-10-04 UTC: four independent
evaluator controls performed actual forward/reverse renames and successful
named-stat calls, then reported one-time EIO for target or parent observations
without changing the filesystem. Restore subsequently rejected the unchanged
owned inode after the injected fault cleared. Retain the prepared candidate and
direction before fallible adoption, then reconcile its actual named identity,
complete mode and bytes under the retained guard before checking stale pins.
Reverse retries finish durability without another rename; foreign edits and
changed inputs remain preserved conflicts. Keep private observation boundaries
per operation with actual defaults, not global fault state or imported authority.
See ADR 0094's post-rename observation refinement. This is injected source-test
evidence, not a released incident or spontaneous OS error.

Before the observation-status correction: expanded actual controls passed their
preservation and exact resource/flock release assertions before finding four
status mismatches. Forward/reverse repeated parent EIO returned refusal 2 instead
of operational 1; forward/reverse actual deletion returned operational 1 instead
of conflict 2. Reuse existing assessment classification for observation errors
while retaining refusal for changed metadata. Keep candidate ownership, retry,
typed uncertainty, pending preservation and safe diagnostics unchanged. The
existing status contract remains ADR 0091:56; this grants no new recovery action.

Before the retained storage observation correction, 2026-10-04 UTC: the latest
advisory review identified ordinary writer checks reporting observation EIO as
refusal 2. Two independent actual named-stat-then-EIO controls confirmed this
after candidate promotion and during explicit restoration; preservation, later
checked restoration and exact resource/flock release passed before the status
assertions failed. Split observation errors from metadata mismatches in all
retained writer checks and reuse existing assessment classification. Ordinary
I/O remains operational 1; missing/unsafe names and actual metadata conflicts
remain refusal 2. Keep ownership, cancellation, mutation and retry rules intact.
See ADR 0094's retained storage observation refinement; no new authority is added.

Before the closed pending observation correction, 2026-10-04 UTC: an independent
actual-sync, actual-close and named-stat-then-EIO control completed preservation,
explicit retry without another rename and exact resource/flock release before
its status assertion failed with refusal 2 instead of operational 1. Apply the
same observation classifier to the named pending recheck after its descriptor
closes. Keep missing/unsafe names and actual ownership or metadata conflicts as
refusals, with original durability and explicit retry unchanged. See ADR 0094's
closed pending observation refinement; this grants no additional cleanup action.

## Decision 94 (2026-10-04 UTC): durable live completion and inert records

After merged PR #121, implement ADR 0095's R2.1 source slice. Add durable live
forward Finish and journaled reverse completion while preserving opaque actual
ownership, the permanent exclusive guard and the fixed pending barrier. Keep
records outside immutable v1 sets in a reserved inert namespace under the
already-excluded backup root; qualify its actual Git exclusion/index state before
writing. Record retries use actual retained candidate ownership, not local JSON
as permission. Restart inspection remains read-only with every authority false.

Qualified recovery after interruption is R2.2, with a fresh capability and known
after-image/compatible-state contract; it is not silently credited to journal
inspection. Divide R2 into those two equal deliverables before implementation.
The fixed 30-package denominator remains; merged PR #121 earns 60.4%, and only
qualified/merged R2.1 can earn 60.6%. This source slice enables no public rollback,
installer/default cutover, legacy retirement, retention or release readiness.
Use independent outside-in Ginkgo/Gomega RED before code, shared Go storage and
staging primitives, and verified review-diamond findings. No new dependency or
paid/background invocation is authorized.

Before implementation, constructor ordering refinement: ADR 0095 requires the
actual durable pending barrier before any read-only Git subprocess. Exclusive
guard ownership alone has no durable activity entry, and the existing supervisor
can report unconfirmed query-group ownership. A failed constructor preserves
pending before releasing its guard, including before target mutation. Only inert
filesystem occupancy preflight may precede pending; it launches no process.

Before implementation, exclusion durability refinement: reuse ADR 0091's exact
local `/.factory/backups/` rule invariant without changing ignore files. Pin,
read back and sync the existing rule-bearing exclude file and `.git/info`, then
confirm effective ignoring and untracked journal paths before any record write.
Refuse a missing canonical rule even if a broader project ignore currently hides
the path. Query visibility alone cannot establish durable recovery exclusion.

Before implementation, query ownership refinement: the new constructor must
preserve the supervisor's actual PID and typed native ownership uncertainty in
its returned error, including cancellation. A generic local-Git diagnostic may
not erase that owned state. The pending barrier remains independent evidence
blocking admission; it does not imply that the query group has exited.

Documentation reconciliation, 2026-10-04 UTC: the current source specification
already resolves Q2 platform scope and Q3 artifact trust at
specs/001-go-runtime-conversion.md:481 and
specs/001-go-runtime-conversion.md:482. Correct the older compatibility inventory
wording; actual platform/authentication qualification remains pending, while
specs/001-go-runtime-conversion.md:484 still leaves detailed Q5 pilot criteria
open. Existing approvals are not additional unfinished approval tasks.

Before required-record parser correction, 2026-10-04 UTC: two independent
actual empty-replacement records passed their valid baseline, then inspection
incorrectly accepted an omitted required `after.bytes` field or JSON null.
Preservation and false-authority checks passed before status assertions failed.
Clarify ADR 0095's closed shape: all declared top-level/nested keys are required,
including nullable identity keys; only phase-eligible identity values may be
null. Reject missing fields and null required scalars without changing v1 sets,
inspection authority, live ownership or pending rules. This is source-test
evidence, not a released incident.

Before record-count admission correction, 2026-10-04 UTC: independent correctness
and security controls both observed 63 valid records admit record 64, but 64
valid records incorrectly admit record 65. Namespace validation had exhausted
the retained directory cursor before the admission read. ADR 0095 now explicitly
requires complete bounded counting independent of cursor state, preserving pins
and revalidation rather than weakening the separate 64-record bound. A persistent
independent paired regression must precede correction. Active originals and prior
records remained preserved; this is source-test evidence, not a released incident.

Before inspection-precedence correction, 2026-10-04 UTC: independent reviewer
and persistent spec-writer controls observed real-read/EIO status 1 being reduced
to status 2 after actual record-mode invalidation. The no-invalidation EIO and
metadata-only refusal controls passed. Refine ADR 0095: discard invalid rows and
keep authority false while preserving previously observed I/O priority in the
operation and report Status. Metadata-only conflicts remain refusal 2. This
changes no mutation, restoration or cleanup authority.

Before terminal-direction admission correction, 2026-10-04 UTC: independent
security and verifier controls observed unapplied durable Restore select reverse,
perform a real original-file sync and report EIO, then Apply incorrectly publish
forward. The completed-abort no-mutation control passed. Refine ADR 0095's live
direction invariant explicitly for Apply: refuse new forward work after any
valid terminal direction is latched, preserving explicit chosen-direction retry,
non-latching invalid Finish and legacy journal-free behavior. A persistent
independent regression must precede correction.

Before late Copilot review corrections, 2026-10-04 UTC: review threads
4179354651, 4179354694 and 4179354727 identify accepted root spelling being
compared directly with Git's absolute output, operational errors first observed
in final metadata checks being collapsed into boolean conflicts, and shared
recovery inspection erasing earlier record-read I/O after invalidation. These
claims are under independent runtime qualification; no correction is claimed yet.
Refine ADR 0095 before regression and implementation: normalize only a qualified
no-follow root, retain final metadata failure classification, and preserve I/O
priority while discarding unsafe recovery/publication observations. Private fault
collaborators retain real syscall execution and production defaults. Persistent
independent runtime RED must precede behavior correction. This adds no restart,
restoration, activation, retention or pruning authority.

Independent late-review qualification, 2026-10-04 UTC, before behavioral
correction: actual local Git controls accepted a canonical absolute root but
refused both `.` and an absolute trailing separator after durable pending;
the originals remained unchanged. A real record read followed by reported EIO
returned status 1 alone, but an actual mode mutation reduced it to status 2.
The root/read run selected five cases: two passed and three failed (package
2.038s). Six separate final-only controls covered both inspectors and root
Fstat, record Fstat and record Fstatat. Each performed a successful real syscall
before reporting one injected EIO; preservation, discarded rows, closure and
false authority passed before status 1 assertions failed (six failed, package
2.498s). These are injected source-test observations, not kernel EIO or released
incidents. Narrow private collaborator wiring precedes persistent author RED;
ordinary default behavior and error classification are intentionally unchanged
until those regressions fail.

## Decision 95 (2026-10-04 UTC): grant fresh interrupted publication recovery

After user-merged PR #122 (`0e3c1fce946a0f303743e52861bec7e530ffd44f`), implement
R2.2 under [ADR 0096](adr/0096-interrupted-publication-recovery.md). Independent
correctness/security source review refined the contract before code: existing
exclusive recovery controls, explicit operator quiescence affirmation, independent
known after-reference, actual compatible G2 state and fresh consent constrain a
new capability. Local journals cannot recreate historical authority or authenticate
self-consistent prior forgery. Preserve ordinary pending admission barriers.

The bounded phase matrix finishes an already published image, restores its checked
original or aborts a prepared operation. It does not replay a forward replacement
from the original. Retained budget PIDs need recorded exit evidence; retained loop
process PIDs block. Prepared candidate ctime is descriptive across rename, while
complete current identity is bound by fresh consent and subsequent revalidation.

Independent outside-in runtime RED precedes behavior. Public rollback, complete
installer/cutover, cleanup/retention and release qualification remain pending.
The fixed 30-package source plan reports 60.6% merged; only qualified and merged
R2.2 earns 60.7%. This decision records intended work, not completed qualification.

R2.2 independent qualification sequence before behavior: the actual external client
first failed compilation on the missing API (four cases, zero passed). After the
interface-only unsupported source snapshot, it compiled and completed actual owner
SIGKILL setup, then failed all four runtime recovery controls.

```text
go test -v ./acceptance -run '^TestAcceptance$' -ginkgo.focus 'Interrupted recovery compiled client core' -ginkgo.no-color -count=1
Ran 4 of 2498 Specs in 4.904 seconds
FAIL! -- 0 Passed | 4 Failed
FAIL github.com/anoop2811/software-factory-template/acceptance 5.563s

go test -v ./acceptance -run '^TestAcceptance$' -ginkgo.focus 'Interrupted recovery compiled client core' -ginkgo.no-color -count=1 -ginkgo.succinct
Ran 4 of 2498 Specs in 5.821 seconds
FAIL! -- 0 Passed | 4 Failed
FAIL github.com/anoop2811/software-factory-template/acceptance 6.345s

go test -v ./internal/budget ./internal/loop ./internal/transition -ginkgo.focus 'Interrupted recovery' -ginkgo.no-color -count=1
Budget: 0 Passed | 7 Failed; package 0.777s
Loop: 0 Passed | 8 Failed; package 0.445s
Transition: 12 Passed | 2 Failed; package 1.044s
```

The independent spec-writer ran these with its explicit role, the qualified
interpreter/tool PATH and temporary Go cache; external clients were race-built.
The first two outputs were captured directly by the tool. The component output
was logged to /private/tmp/factory-r22-component-red.log and read back. Twelve
refusal-only guard controls passed unsupported stubs; the two real grant/close
controls failed, preventing a vacuous qualification claim. An initial missing-Go
PATH invocation and a new-test Ginkgo import collision were evaluator issues,
corrected before these runs without changing production behavior or assertions.
This is RED evidence, not completed implementation or release qualification.

The independent assessment evaluator was tightened before qualification: displaced
record/pending inodes are retained outside the inspected namespace, and fresh
eligible proposals precede stale-consent refusal controls. This isolates changed
consent from an unrelated unknown-name blocker. A terminal crash witness also
now permits the constructor's initial publication-directory sync before its JSON
slot exists; it still requires the actual terminal record and real directory sync
before acknowledging the target kill boundary. These are evaluator corrections,
not product defects or relaxed assertions. Broadened crash/fault cases do not
count as isolated production RED while an earlier unsupported API blocks them.

Root independently reran the helper/guard race qualification on 2026-10-04 UTC
and read back /private/tmp/factory-r22-component-green.log:

```text
rtk proxy env FACTORY_AGENT_ROLE=reviewer GOCACHE=/private/tmp/factory-durable-recovery-go-cache go test -race -v ./internal/budget ./internal/loop ./internal/transition -ginkgo.no-color -ginkgo.succinct
Budget: 58/58 SUCCESS! 6.330724583s; package 8.445s
Loop: 90/90 SUCCESS! 10.172179334s; package 11.754s
Transition: 61/61 SUCCESS! 349.934083ms; package 2.175s
```

These include the original 51/82/47 and new 7/8/14 cases. This qualifies only
the helper/guard stage; the recovery engine and crash/fault checks remain pending.

Independent correctness review isolated a recovery-only activity gap. Its real
empty ReadDir ran under a proven held permanent exclusive flock; the evaluator
then created one real retained activity file before acquisition returned. The
guard returned, Check and Close succeeded, and the entry remained. The same
pair without that creation passed. Root read the overlay, raw output and source
and retained this finding against ADR 0096's acquisition contract; ordinary guard
behavior is outside this correction. The intended stable-empty-directory
refinement was committed before the independent author persists RED and before
implementation changes.

```text
rtk proxy env FACTORY_AGENT_ROLE=reviewer GOCACHE=/private/tmp/factory-durable-recovery-go-cache go test -race -count=1 -timeout=120s -overlay=/private/tmp/factory-r22-guard-verifier-_e0yilgk/overlay.json -v ./internal/transition -ginkgo.focus='Reviewer interrupted recovery late activity admission' -ginkgo.no-color -ginkgo.v
actual empty_ReadDir_before_marker=true held_permanent_EX=true late_activity=true returned_guard=true Check=<nil> Close=<nil> preserved_entries=1 primary=<nil>
Ran 2 of 63 Specs in 0.010 seconds
FAIL! -- 1 Passed | 1 Failed
FAIL github.com/anoop2811/software-factory-template/internal/transition 0.524s
```

Raw output: /private/tmp/factory-r22-guard-verifier-_e0yilgk/qualification.log.
This is an actual mutation scheduled by a source-test collaborator, not evidence
of a released incident. A separate missing-errno hypothesis was refuted: the safe
typed guard cause is classified by assessment's existing errno policy, so an
explicit Conflict=false does not itself imply operational status 1.

Two persistent Git-refusal controls then isolated a constructor cleanup defect.
Each first granted against valid Git, later reached real supervised Git with an
eligible fresh proposal, preserved existing pending/journal/current/index/ignore
bytes and closed nonempty captured descriptors plus the actual flock. Missing
exact local exclusion and a force-tracked selected record both ended with status
1 rather than the required refusal 2. Root read the independent tests and log.
ADR 0096 now distinguishes failed-constructor borrowed evidence from incomplete
returned-capability reporting before correction.

```text
rtk proxy env FACTORY_AGENT_ROLE=spec-writer GOCACHE=/private/tmp/factory-durable-recovery-go-cache go test -race -v ./internal/assessment -run '^TestAssessment$' -ginkgo.focus 'Interrupted recovery qualified Git refusal status' -ginkgo.no-color -count=1
Ran 2 of 273 Specs in 1.012 seconds
FAIL! -- 0 Passed | 2 Failed
FAIL github.com/anoop2811/software-factory-template/internal/assessment 1.547s
```

Raw output: /private/tmp/factory-r22-git-refusal-red.log. Independent late-activity
controls also persisted: three selected, one passed and two failed (package
0.508s), including actual marker creation after grant before Check. A separate
directory-Stat taxonomy concern remains unqualified. Its private native-default
observation seam is specified before wiring; no kernel error or correction is
claimed until independently scheduled native operations reach property RED.

The private native-default Stat seam then isolated four operational-status
regressions. Each active/record-directory root.Stat or retained file.Stat actually
succeeded before one reported EIO. Paired eligible grants, nonempty descriptor
closure, actual os.Root closure, unchanged tree/pending and real flock release
passed before the final status assertion received 2 instead of 1. Root read the
tests and raw log before authorizing the classification correction already
specified in ADR 0096.

```text
rtk proxy env FACTORY_AGENT_ROLE=spec-writer GOCACHE=/private/tmp/factory-durable-recovery-go-cache go test -race -v ./internal/assessment -run '^TestAssessment$' -ginkgo.focus 'Interrupted recovery qualified metadata error status' -ginkgo.no-color -count=1
Ran 4 of 277 Specs in 1.673 seconds
FAIL! -- 0 Passed | 4 Failed
FAIL github.com/anoop2811/software-factory-template/internal/assessment 2.174s
```

Raw output: /private/tmp/factory-r22-stat-error-red.log. This is real native
observation followed by an injected report, not kernel EIO or a released incident.

The inherited writer's initial named-directory/file classification was separately
isolated through its existing private collaborator. Six paired proposal/grant
cases performed successful real named observations of .factory, pending and the
selected current file before one reported EIO. Eligible baselines, complete
preservation, nonempty descriptor closure and actual flock release passed before
the final status assertion received 2 instead of 1. Root read the tests and log;
reuse the existing errno policy in the shared helper before any correction,
retaining ordinary missing/unsafe refusals and requalifying existing write paths.

```text
rtk proxy env FACTORY_AGENT_ROLE=spec-writer GOCACHE=/private/tmp/factory-durable-recovery-go-cache go test -race -v ./internal/assessment -run '^TestAssessment$' -ginkgo.focus 'Interrupted recovery qualified named observation error status' -ginkgo.no-color -count=1
Ran 6 of 283 Specs in 2.366 seconds
FAIL! -- 0 Passed | 6 Failed
FAIL github.com/anoop2811/software-factory-template/internal/assessment 2.839s
```

Raw output: /private/tmp/factory-r22-named-error-red.log. This qualifies the
initial lookup error path only; it does not claim a native kernel error.

Further independent review on 2026-10-05 UTC isolated two compound boundaries.
An actual late activity file made Begin refuse, while Guard.Close detected a
validation conflict and successfully released all descriptors/flock. Shared
cleanup nevertheless manufactured status 1. The paired overlay selected two:
one passed and one failed (package 1.163s); root and independent security review
read the actual probe/log and retained the finding. A second overlay displaced
the qualified catalog file before real confined Openat, with separate native
ENOENT and ELOOP outcomes. Both preserved all post-change evidence and closed
actual descriptors/flock, then reported 1 instead of 2. Its three selected cases
had one pass and two failures (package 2.050s). These are actual filesystem
changes and errno observations, not kernel-error injection or released incidents.

Evidence remains in /private/tmp/factory-r22-constructor-late-6hl7tzaw,
qualification.log and open-qualification.log. ADR 0096 specifies guard-conflict
versus real-cleanup precedence across compound errors and existing-policy Openat
classification before persistent independent RED and implementation correction.
The private native-default guard-close seam must retain the old behavior until
its independent real-close-then-reported-error control qualifies precedence.

Exact independent review commands for those retained outputs:

```text
rtk proxy env FACTORY_AGENT_ROLE=reviewer GOCACHE=/private/tmp/factory-durable-recovery-go-cache go test -race -count=1 -timeout=120s -overlay=/private/tmp/factory-r22-constructor-late-6hl7tzaw/overlay.json -v ./internal/assessment -ginkgo.focus='Reviewer interrupted constructor late evidence status' -ginkgo.no-color -ginkgo.v
rtk proxy env FACTORY_AGENT_ROLE=reviewer GOCACHE=/private/tmp/factory-durable-recovery-go-cache go test -race -count=1 -timeout=120s -overlay=/private/tmp/factory-r22-constructor-late-6hl7tzaw/open_overlay.json -v ./internal/assessment -ginkgo.focus='Reviewer interrupted actual open conflict status' -ginkgo.no-color -ginkgo.v
```

Root independently read the second actual probe and native-errno output and
retained it against the existing missing/unsafe-evidence contract. Executable
overlay files are evaluator inputs, not production replacements or shipped tests.

The independent author then persisted eight unchanged cleanup/open controls.
Eligible controls and actual Guard.Close followed by one reported EIO passed.
Late activity with successful native cleanup, and real file/catalog-directory
disappearance or symlink substitution before Openat, failed only their final
status assertion after native errno, preservation and resource-release checks.
Root read the actual log before authorizing the bounded shared-policy correction.

```text
rtk proxy env FACTORY_AGENT_ROLE=spec-writer GOCACHE=/private/tmp/factory-durable-recovery-go-cache go test -race -v ./internal/assessment -run '^TestAssessment$' -ginkgo.focus 'Interrupted recovery qualified (constructor late activity|confined open conflict) status' -ginkgo.no-color -count=1
Ran 8 of 301 Specs in 3.236 seconds
FAIL! -- 3 Passed | 5 Failed
FAIL github.com/anoop2811/software-factory-template/internal/assessment 3.855s
```

Raw output: /private/tmp/factory-r22-close-open-red.log. Compound operational
cleanup precedence already passed before correction and must remain unchanged.

The independent test set is frozen at 105 runnable criteria plus one
subprocess-only witness across five new files; no baseline tests changed.
Final source-qualified affected race output was read back on 2026-10-05 UTC:

```text
rtk proxy env FACTORY_AGENT_ROLE=implementer GOCACHE=/private/tmp/factory-durable-recovery-go-cache PATH="/var/folders/83/yj7qqyt551xbbpvm54tqkcbw0000gn/T/factory-pr113-python-_gp80gpr/venv/bin:$PATH" go test -race -v ./internal/assessment ./internal/budget ./internal/loop ./internal/transition -ginkgo.no-color -ginkgo.succinct
Assessment: 300 Passed | 1 intentional child-witness skip; package 64.844s
Budget: 58/58 SUCCESS! 6.784260875s; package 8.568s
Loop: 90/90 SUCCESS! 12.163744417s; package 14.848s
Transition: 64/64 SUCCESS! 525.79725ms; package 2.664s
```

Raw output: /private/tmp/factory-r22-implementer-final-affected-race.log.
The independent author separately reran the final eight cleanup/open controls
(eight passed, package 5.728s) and eight real compiled clients with race and
qualified Python admission (eight passed, package 13.100s). Native operation
faults and actual SIGKILL remain source-process evidence, not power-loss proof.
Canonical all-package lint independently returned `0 issues.`:

```text
rtk proxy env FACTORY_AGENT_ROLE=spec-writer GOCACHE=/private/tmp/factory-durable-recovery-go-cache GOLANGCI_LINT_CACHE=/private/tmp/factory-durable-recovery-lint-cache /private/tmp/factory-quality-tools/golangci-lint run --config packs/go/.golangci.yml ./...
0 issues.
```

Raw output: /private/tmp/factory-r22-pack-lint-qualified.log. One explained
function-level errorlint exception preserves actual-node error-tree traversal;
first-match errors.As would skip a joined operational sibling. No behavior was
changed for that annotation. Independent correctness/security/test rechecks
reported no surviving findings on eight source and five test hashes. Root
subsequently proved exactly three citation-only comment corrections by restoring
their old text in memory and matching the reviewed SHA256 values.

The full repository/local Go gate and hosted qualification are still separate
pending checks at this recorded checkpoint. No merged credit or release claim.

Post-submission qualification on 2026-10-05 UTC identified an evaluator-only
confinement issue. Hosted Linux at PR #123 head 30e59a5 passed the complete race
suite (acceptance 422.821s; assessment 15.285s), then canonical lint reported
G703 at unchanged publication_test.go:29. The new subprocess witness supplies
its parent-created root and request through environment data; its setup helper
read the selected journal through the older unconstrained fixture reader.
The independent author reproduced that same lint finding without edits:

```text
rtk proxy env FACTORY_AGENT_ROLE=spec-writer GOOS=linux GOARCH=amd64 GOCACHE=/private/tmp/factory-durable-recovery-go-cache GOLANGCI_LINT_CACHE=/private/tmp/factory-durable-recovery-linux-lint-cache /private/tmp/factory-quality-tools/golangci-lint run --config packs/go/.golangci.yml ./internal/assessment
internal/assessment/publication_test.go:29:26: G703: Path traversal via taint analysis (gosec)
1 issues:
* gosec: 1
```

Root read /private/tmp/factory-r22-linux-lint-red-local.log before authorizing
the correction. Confine only the new evaluator helper's selected-journal read
through os.OpenRoot and Root.ReadFile, with checked closure. Preserve the actual
request/record selection, bytes, reference, all behavioral assertions and native
fault/crash controls. Do not edit the baseline helper, suppress the rule, change
production behavior or add a dependency. Requalify affected independent cases
and canonical Linux lint before freezing the evaluator again.

The stronger local invocation with FACTORY_CLI_TEST_RACE=1 reached its existing
30-minute whole-suite allowance while executing the unchanged doctor unsafe-
input matrix. All internal packages passed, but the acceptance package timed out
at 1800.503s and make exited 2. This is not a green full invocation, nor evidence
that the doctor case itself hung. Hosted suite qualification and remaining local
quality checks must be reported separately; no assertion or operation deadline
is relaxed to hide that result.
