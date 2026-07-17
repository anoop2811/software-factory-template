# Spec Template

> Adapted from the ai-craft spec format. Markdown-in-git, no tooling, no framework.
> Every spec cites its source. The citation-lint checks that cited files and lines exist.

## Metadata

| Field | Value |
|---|---|
| Spec ID | NNN (zero-padded, sequential) |
| Title | (short, imperative) |
| Stage | (which build stage this belongs to) |
| Status | draft / done / superseded |
| Tier | 1 (permanently human-reviewed) or 2 (spot-checked) |
| Author | (agent + model that wrote it) |
| Date | YYYY-MM-DD |

## 1. Problem

What problem does this feature solve? One paragraph. Cite the spec source if applicable.

## 2. Scope

What's in scope and what's explicitly out of scope. Be precise — this is what the reviewer checks against.

## 3. Design

How does this work? Include:
- Data model (structs, interfaces, types)
- Key algorithms or flows
- API surface (function signatures)
- Error handling strategy

Cite your spec source for every claim, by file and line: `// per docs/SPEC.md:L42`

## 4. Acceptance criteria

Executable criteria, written as Ginkgo Describe/Context/It blocks (prose form is fine for draft). Every criterion must cite the spec line it encodes.

## 5. Security considerations

- What attack surface does this add?
- What invariants must hold? (e.g., idempotent writes, no unauthenticated mutation)
- How are they enforced? (hook, test, CODEOWNERS)

## 6. Testing strategy

- Unit tests (Ginkgo v2 + Gomega)
- Integration tests (if applicable)
- Mutation testing (gremlins)
- What's the golden vector? (if applicable)

## 7. Citations

List every spec doc cited in this spec, with file:line. The citation-lint will verify these resolve.

## 8. Open questions

Anything unresolved. Mark each as OPEN with a clear statement of what's blocking.

## 9. Out of scope

Explicitly list what this spec does NOT address, to prevent scope creep in review.
