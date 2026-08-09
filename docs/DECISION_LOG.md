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
