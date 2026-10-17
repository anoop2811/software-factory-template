# Budgeted batched adversarial review

**Status:** Approved scope; qualification pending
**Author(s):** Anoop Gopalakrishnan (direction); Codex (specification)
**Date:** 2026-10-17
**Story/Ticket:** Decision 99; user-authorized implementation and model review

## 1. Overview

Review large pull requests through bounded batches with measurable coverage.

## 2. Problem

The existing size guard skipped PR #126. Posting a notice appeared successful.
Observed evidence is in docs/adr/0100-budgeted-batched-adversarial-review.md:8.

## 3. User Stories

Developers see exact reviewed changes; maintainers choose cost/routing limits;
unchanged, context-matching completed work can be reused.

## 4. Acceptance Criteria

Independently prove large-diff coverage, strict input validation, fragmentation,
budgets, authenticated cache, failure handling and exact-head publication.
Required controls: docs/adr/0100-budgeted-batched-adversarial-review.md:89.

## 5. Functional Requirements

One shared Go command plans, reviews and reports complete diff/manifest input.
Canonical contract: docs/adr/0100-budgeted-batched-adversarial-review.md:23.

## 6. Non-Functional Requirements

Bound execution and costs; keep credentials private and PR content inert.
No new module or language dependency. Preserve all three harnesses.

## 7. Data Model

Reports identify revisions, policy/context, batch/file/fragment coverage,
reservations, nullable usage and explicit incomplete reasons.
Schema: docs/adr/0100-budgeted-batched-adversarial-review.md:76.

## 8. API Contract

Use Cobra and the existing strict Go stream transport. Configurable limits and
opt-in routing: docs/adr/0100-budgeted-batched-adversarial-review.md:49.

## 9. Out of Scope

Automatic fixes/merges, silent model substitution, unbounded replay/spend,
or guarantees of accuracy, external uptime or provider billing.

## 10. Open Questions

None blocks implementation. External failure remains explicit until completion.

## 11. Decision Record

Decision 99 and ADR 0100 precede code. Separate test authors, implementers and
reviewers follow outside-in TDD and the repository review-diamond recipe.

## 12. Success Metrics

Independent checks and hosted CI pass; a >200000-byte fixture receives complete
coverage; matching cache avoids calls; the final change receives real model review.
