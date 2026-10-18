# Budgeted batched adversarial review

**Status:** Approved
**Authors:** Anoop Gopalakrishnan; Codex
**Date:** 2026-10-17
**Story:** Decision 99

## 1. Overview

Bounded batches make large-PR review coverage measurable.

## 2. Problem

PR #126 exceeded the old guard; a skip looked successful.

## 3. User Stories

Developers inspect coverage; maintainers control spend and reuse matching work.

## 4. Acceptance Criteria

Validate inputs, budgets, cache, failures and publication independently:
docs/adr/0100-batched-review.md:89.

## 5. Functional Requirements

The shared Go command reviews complete diff/manifest inputs:
docs/adr/0100-batched-review.md:23.

## 6. Non-Functional Requirements

Bound cost; isolate credentials; preserve three harnesses without new modules.

## 7. Data Model

Revision/coverage/usage schema:
docs/adr/0100-batched-review.md:76.

## 8. API Contract

Cobra and existing strict transport; limits/routing:
docs/adr/0100-batched-review.md:49.

## 9. Out of Scope

Auto-fixes/merges, silent model changes, unlimited spend and uptime guarantees.

## 10. Open Questions

None blocks implementation.

## 11. Decision Record

Decision 99 precedes code; independent authors use outside-in TDD/review-diamond.

## 12. Success Metrics

CI passes; large-input coverage/cache checks pass; final change is model-reviewed.
