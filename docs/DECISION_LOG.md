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
