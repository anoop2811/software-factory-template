# ADR 0118: select a packaged Go runtime for ordinary Go initialization

Status: accepted implementation contract; independent public RED precedes code

Date: 2026-10-30 UTC

## Complete function

Ordinary `factory init` invoked through the Go CLI selects an authenticated
packaged Go initializer. The source shell launcher and public installer's active
v0.1.6 pin stay with their separately scheduled activation. Prepare the literal
default `v0.2.0`; an unpublished or unavailable release refuses without changing
the target. This preparation does not claim that v0.2.0 has been published.

Reuse candidate acquisition, publisher verification, exact-runtime handoff,
qualified initialization, owned cleanup and gate proofs. Do not construct a
second initializer, trust an arbitrary colocated binary, compile source during
ordinary initialization or fall back to scripts after a selected Go failure.

## Selection and argument boundaries

Existing explicit candidate and local selectors retain their owners and ignore
ambient release defaults. For ordinary Go init, select in this order:

1. One explicit `--ref TAG` before the first user `--`.
2. A nonempty caller `FACTORY_REF`.
3. The validated installed release version, for an admitted installed caller.
4. The declared `v0.2.0` default for a source-built caller.

Explicit refs and environment refs must name an exact stable Go release at or
after v0.2.0. Moving aliases, old Bash tags, malformed values, duplicate refs and
implicit prerelease selection refuse. A validated installed prerelease in the
Go v0.2.0-or-newer family may initialize its own exact authenticated version;
explicit `--go-candidate` remains the prerelease override. An installed
`local-*` or unsupported nonrelease label must explicitly choose local source
or an official release instead of attempting a GitHub download for that label.
Version labels do not replace publisher verification.

Carry the version from the admitted `installedlayout.Context.Descriptor` into
the CLI context. Do not reread an unvalidated descriptor or let an environment
variable claim installed identity. The existing direct whole-upgrade route must
continue to avoid retaining a shared lease; it supplies no initializer version.

Parse selectors only before the first user terminator, and prepend the resolved
candidate selector before forwarding initializer arguments. A terminated target
such as `--ref`, `--source` or `--go-candidate` stays a literal target. Separate
private protocol framing from user option termination. Fix the authenticated
initializer's selector/help scan at this same boundary so it cannot consume a
terminated target during the default handoff. Keep upgrade parsing unchanged.

Ordinary help describes the default and supported overrides without resolving
assets, running tools or touching the target. Validate malformed explicit
selectors before help; ambient defaults and installed local labels do not stop
the help-only path. Preserve initializer option validation, literal values,
working-directory behavior, inherited streams, consent and classified outcomes.

## Compatibility qualification

Independent outside-in Ginkgo/Gomega scenarios first fail against unchanged
main through the real compiled Go CLI. Then exercise authenticated default and
ref/environment precedence, admitted installed-version reuse, explicit selector
priority, clean failure/decline, unavailable release, help and malformed input,
literal terminated targets, exact selected runtime and poisoned compiler/Python.
Go initialization must publish the qualified selection and prove its gates.

Ordinary source initialization previously exposed the unqualified native
initializer through the public CLI, while installed ordinary init refused.
Update those intentional contract expectations. Retain the existing initializer
prompt/data/pack/publication/TTY/ownership coverage as explicitly classified
historical component qualification, using a guarded test-only initcmd executable
and immutable historical asset declarations where required. The public shipping
CLI has no legacy escape flag or hidden environment bypass. Do not delete tests,
weaken source/child custody, make historical programs current dependencies, or
claim those component cases prove current packaged delivery.

The new public family proves current behavior through actual candidate images.
Keep it in a dedicated `default-init` qualification partition if necessary to
preserve headroom in the existing heavy handoff job. Prove disjoint full identity
coverage, empty-selection refusal and all/default failure propagation before
changing CI wiring. Retain every existing race and quality stage and platform.

## Independent parallel boundaries

This PR changes ordinary Go initialization only. Default upgrade, minimal cache,
obsolete-asset retirement, source selftest, source bootstrap and release
publication remain independently reviewed functions. The inventory and source
selftest branches may progress in parallel with explicit ownership; integrate
their changes and repeat relevant qualification before dependent merges.
