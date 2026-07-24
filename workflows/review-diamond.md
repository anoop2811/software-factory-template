# Workflow: review-diamond

> Review a change across dimensions, verify each finding, and converge on one review.

A node is a bounded job (a role); an edge carries data. Plumbing (dedupe, merge)
is an edge — plain code, not an agent. Findings pass a verifier before they count.

## review
- role: reviewer
- kind: fanout
- over: the changed files, one lens each — correctness, security, tests

## verify
- role: reviewer
- kind: verify
- of: review — try to refute each finding; keep only what survives

## merge
- role: code
- kind: edge
- note: dedupe and rank the surviving findings — plain code, no agent

## synthesize
- role: reviewer
- kind: agent
- of: merge — write the single review from the ranked findings
