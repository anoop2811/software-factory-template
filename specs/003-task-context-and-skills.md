# Task context and on-demand skills

**Spec 003 · Draft for maintainer review · 2026-11-24**

Direction from Anoop Gopalakrishnan. Draft by Codex. Decision 183. Implementation progress 0%.

## 1. Overview

Let a developer answer “what project material will this agent receive for my task?” before starting a run.
A **source** supplies evidence, such as a requirement, contract, test or code excerpt. A **skill**
supplies a named procedure the developer can select, such as rehearsing an upgrade. Both remain in the
team's chosen files. The factory prepares a bounded, inspectable context for Codex, Claude Code and
OpenCode. [Roadmap](../docs/FEATURE_ROADMAP.md#L19)

## 2. Problem

An API team might keep its contract in OpenAPI YAML, its examples in Gherkin and its release procedure in
a handbook. Requiring the team to rewrite those materials into the factory's twelve-section format would
create a second source of truth. Loading the entire handbook into every run would also make it harder to
see which instructions mattered or how much context they consumed. The twelve-section convention governs
specifications *for this factory*. Adopters choose the structure, names and language of their own
material.

## 3. User Stories

| As a developer working on… | I want to… | So that… |
|---|---|---|
| A small repository | Give the factory one existing design note and a test file | I can try a preview without creating a catalog, registering a provider or starting an agent. |
| A Go service and TypeScript client in a monorepo | Select the service's spec, a shared API contract and one migration skill | Another team's similarly named spec stays out of my task. The same setup also works for other languages. |
| A regulated product | Reference requirement `PAY-42` in a locally materialized export, with its upstream revision | A reviewer can trace the exact local excerpt to the revision the producer identifies as approved. |
| A team with its own spec generator | Produce catalog entries from our existing tool | Our format can participate without a factory patch, a native plugin or disclosure of our service credentials. |
| A team that plans in Aha! and delivers in Jira, or uses Linear | Start with my assigned ticket and see its acceptance criteria and the customer objective it supports | I can explain the change in my team's terms and trace each excerpt to its owner. |
| A cross-harness handoff | Preview the material once and give the same selection to a Codex implementer or a Claude Code or OpenCode reviewer | Switching harnesses preserves the cited evidence while each role keeps its own permissions. |

## 4. Acceptance Criteria

Outside-in Ginkgo scenarios, with Gomega assertions against the public CLI and the harness input boundary.
Each row identifies the functional requirement it proves.

| ID | Given / When | Then |
|---|---|---|
| AC1 · [FR1–2](#fr1 "specs/003-task-context-and-skills.md:54") | Given an ordinary repo with a design note and an untracked non-ignored test, preview both by path from a subdirectory, twice | Both runs produce the same root-relative citations, bytes and digest. No catalog, state file, credentials, agent process or network call is needed. |
| AC2 · [FR2–3](#fr2 "specs/003-task-context-and-skills.md:58") | Given two teams with a local ID `refund` and an external producer's catalog for `PAY-42`, select one qualified ID plus the exported requirement | Only those entries appear. The export has a local path, line range, content hash and upstream revision. An exporter built independently of factory code passes offline validation. Duplicate namespaces and missing IDs fail with a diagnostic. |
| AC3 · [FR3–4](#fr3 "specs/003-task-context-and-skills.md:62") | Given an on-call procedure and a migration procedure, list skills, then select only the migration skill | Listing shows metadata only. The selected body appears once in the preview. An embedded command and a link to another file are displayed as text and cause zero executions or file inclusions. |
| AC4 · [FR5–6](#fr5 "specs/003-task-context-and-skills.md:76") | Given a service scope, a shared explicitly selected contract, tied search matches and a byte cap, run local retrieval | The shared contract survives. Only eligible in-scope matches follow in deterministic order. Inspecting the preview accounts for every considered candidate. An exact fit passes. One byte under the required size blocks delivery. A search match pointing to an untracked draft is omitted and explicit selection can include it. |
| AC5 · [FR6–8](#fr6 "specs/003-task-context-and-skills.md:85") | Preview a request, then change selected bytes without committing, retarget its catalog entry, or change an input used by a selected summary | A managed run with the old digest refuses before budget reservation or harness launch. Restoring identical inputs restores the digest. A captured harness input contains exactly the approved context bytes even if the file changes after validation. |
| AC6 · [FR7–8](#fr7 "specs/003-task-context-and-skills.md:95") | Run the same approved request against each of three instrumented CLI substitutes | All receive the same context bytes exactly once. Existing task instructions, role restrictions, hook preflight and budget admission still apply. A blocked budget causes zero agent launches. With a near-limit prompt, reject combined input overflow without a reservation or truncation, including when only the Codex role prefix tips it over. |
| AC7 · [FR1, FR6–8](#fr1 "specs/003-task-context-and-skills.md:54") | Request an ignored credential fixture, escaping path, symlink, hard-linked file, special file, malformed catalog, unsupported version or unreadable required range | The command fails with a useful reason, emits no partial payload and never prints or records fixture contents. A filename containing terminal controls displays safely. A mock delimiter inside source text decodes as text. A legacy run without context options exercises its existing behavior. |
| AC8 · [FR9](#fr9 "specs/003-task-context-and-skills.md:112") | Given an original spec and an optional summary, preview first without summaries, then explicitly request the summary, then modify the original | First preview includes the requested original, second labels the summary and cites the original, third rejects the stale binding. Changing only the range in the original locator also blocks. Explicit summary plus search for the same ID includes only the summary. Each preview makes zero model requests. |
| AC9 · [FR10](#fr10 "specs/003-task-context-and-skills.md:119") | Export the fixtures in [Work-item mappings](#work-item-mappings) for one assigned ticket, then preview the emitted request offline | For both Aha!–Jira profiles and for Linear, the preview contains the expected records, native labels and revision-bound citations in order. Exercise a custom Jira type, an optional absent parent, an ambiguous counterpart and a ticket with four siblings. Check export failures separately from factory preview failures. |

## 5. Functional Requirements

- <a id="fr1"></a>**FR1 · Opt in and preview.** An explicit context command works within a Git worktree. Resolve relative
  paths from that worktree's root even when invoked below it. Show each selected item, selection reason,
  source range, omissions, byte count and digest. Preview and list are read-only. Init, upgrade,
  interactive clients and managed runs with no context flags retain their current behavior.
- <a id="fr2"></a>**FR2 · Bring your own specs.** Accept regular UTF-8 text by path, optionally with a 1-based inclusive
  line range. Markdown, plain text, source code, JSON, YAML and Gherkin are examples. Read the working
  copy. Teams choose directories and filenames. A direct file needs no front matter, parser, catalog, Git
  commit or language pack.
- <a id="fr3"></a>**FR3 · Plug in a catalog.** A request may use zero or more explicitly named local catalog files. Any
  team tool may produce the versioned data contract in §7. IDs are qualified by catalog namespace. Resolve
  only the configured local paths. A remote spec system can export individual files or one snapshot with
  ranges. Its producer supplies the upstream reference and immutable revision for each externally
  attributed item. Report those values as producer-supplied provenance. Local hashes establish which bytes
  the factory read. Upstream approval and recency are facts the producer must establish. Ship versioned,
  machine-readable request and catalog schemas and accepted/rejected examples with the catalog slice. An
  independently written exporter must pass without importing factory packages.
- <a id="fr4"></a>**FR4 · Select skills on demand.** Catalogs list a skill's ID, description, scope and body path. Listing
  reads metadata without opening procedure bodies. Only an explicit skill selection loads its
  instructions. Register a runbook or `SKILL.md` with a catalog entry of kind `skill` pointing to its path.
  Read its text verbatim, including any front matter. The catalog supplies listing metadata. Additional resources
  require their own selections. The factory treats procedure text as task material and grants no tool
  access, shell execution or change of role through it.
- <a id="fr5"></a>**FR5 · Choose local retrieval.** Search is explicit. Match the query against cataloged source IDs,
  descriptions and tags, using sequences of Unicode letters or digits as words, compared with Unicode
  simple case folding. Require at least one matching word. Rank by number of distinct query words matched,
  descending, then qualified ID ascending. Use the requested repo-relative directory scope and descendant
  scopes by path component. The root scope covers the worktree. Explicit selections may name a shared item
  outside the search scope. Only catalog metadata is searched. Matching entries supply their declared
  file/range as context. A file without a catalog entry is available by explicit path only. Record an untracked, missing or
  ineligible optional match as omitted without exposing its body. Search never activates a skill. A valid
  search with no eligible matches adds nothing and reports why. Delivery of an empty selection is blocked.
- <a id="fr6"></a>**FR6 · Bound and explain.** Direct files, named sources and named skills are required. Pack these first
  in request order, then whole optional search matches by rank. If an optional match is too large, record
  the omission and try the next. If required items exceed the cap, block and identify their sizes.
  Deduplicate identical original path/range/kind or identical summary representation, retaining distinct
  citations and reasons. If search finds an explicitly selected ID, keep the explicit representation.
  Conflicting explicit original and summary choices for the same ID are an error in either order. Distinct
  representations from different IDs remain separate. The cap applies to exact UTF-8 bytes of the
  serialized context delivered to the harness, including its framing and citations. Report omission
  diagnostics outside that payload. The proposed default is 32 KiB, configurable per request up to 1 MiB.
  Provider token counts and billing have separate units. Recheck the cap when merging another citation.
- <a id="fr7"></a>**FR7 · Bind a run to its preview.** Preview returns a versioned SHA-256 digest binding the effective
  request, catalog declarations used, selected source identities and exact payload bytes. Recompute before
  admission. On mismatch, request a fresh preview and leave the budget untouched. Pass the same validated
  in-memory bytes through to launch, without reopening paths. Run attempts, time, concurrency, cost
  estimates and role preflights stay with the existing controllers. Attach context once at the task-input
  boundary and keep the harness's own instructions and native framing intact. Check the combined task,
  context and harness-specific framing against the native input limit before reserving a run. Refuse
  unsupported delivery or oversized combined input without truncation.
- <a id="fr8"></a>**FR8 · Preserve local boundaries.** Read eligible tracked files and explicitly named non-ignored
  untracked files. Apply Git's ignore rules to untracked inputs. Always exclude Git metadata and the
  factory's private `.factory/` state. Apply the same containment, type and eligibility checks to
  catalogs, requests, originals and summary files. Refuse absolute paths, traversal, symlink components,
  hard-linked input files (another name may be outside the checkout), binary/NUL or invalid UTF-8 content and nonregular files. Bind eligibility
  checks to the opened file to tolerate concurrent replacements safely. Reject unknown schema versions,
  ambiguous IDs, duplicate keys and out-of-range selectors. Raw task material goes only to the explicit
  preview output or selected harness. Any extra context fields in logs or budget records contain only
  digests and counts. Tracked text can contain secrets, so the operator reviews what will be sent.
- <a id="fr9"></a>**FR9 · Optional summaries.** An explicitly requested summary may replace its named source. Bind it to
  the source’s qualified ID, path, range, full-file SHA-256 and upstream identity where present. Retain
  both original and summary citations. A missing or stale explicitly requested summary blocks, including
  when its locator changes within an otherwise identical export. The default is original text. Teams can
  provide human-written or generated summaries through the catalog. Generating a summary with a model is a
  separate explicit action through an enabled budgeted run. Preview never calls a model or silently
  summarizes to fit.
- <a id="fr10"></a>**FR10 · Bring your team's work-item mappings.** A team may use a project-specific
  mapping to export a ticket and its product context. The producer chooses native record types, fields
  and links and emits an ordinary v1 catalog and request as described in §7. The factory applies FR3,
  FR6 and FR8 to those files. Ship worked producer examples with the catalog slice. Direct-file users
  need no mapping.

## 6. Non-Functional Requirements

Use one Go selector and serializer with a Cobra entry point. Exercise normal entry points with outside-in
Ginkgo/Gomega tests before implementing each slice (specs/001-go-runtime-conversion.md:637). Local
operations require no accounts, network, background index or extra runtime in the adopter project. Keep
stable governance and role instructions ahead of volatile task context (docs/COST_AND_TOKENS.md:89). Bound
read sizes, candidate counts and diagnostics independently of output size. Publish those limits and test
refusal at each boundary. Do not suggest token or dollar savings without measured evidence.

## 7. Data Model

A **request** specifies ordered required selections, optional catalogs, search, summary choices and byte
limit. A **catalog** maps names to bounded pieces of existing project material. A **preview** contains a
printable payload, separate selection diagnostics and a digest. Relative paths in all three refer to the
checkout root. There is no special meaning to a directory named `specs/`.

Example catalog at `team/payments.context.json`, produced by hand or by a team's exporter:

```json
{
  "version": 1,
  "namespace": "payments",
  "entries": [
    {"id": "refund", "kind": "source", "description": "Refund API contract", "path": "apis/openapi.yaml", "lines": [40, 58], "scope": "services/payments", "tags": ["refund"]},
    {"id": "PAY-42", "kind": "source", "description": "Approved refund requirement", "path": "exports/approved.txt", "lines": [12, 16], "origin": {"ref": "acme:requirements/PAY-42", "revision": "rev-18"}},
    {"id": "rehearse", "kind": "skill", "description": "Rehearse a payment migration", "path": "handbook/rehearse.md", "scope": "services/payments"}
  ]
}
```

`version`, `namespace` and `entries` are required. Each entry requires `id`, `kind`, `description` and
`path`. IDs and namespaces match `[A-Za-z0-9][A-Za-z0-9._-]*`, case-sensitive. A namespace is unique
across loaded catalogs. IDs are unique within it. Omitting `lines` selects the whole file and omitting
`scope` means `.`. Optional `tags` is an array of strings. An optional `origin` requires both fields shown above and
is never dereferenced. A source’s optional `summary` contains a local `path` and a `for` object supplied
when the summary is produced. `for` records the qualified `id`, original `path`, resolved `lines` (inclusive range, null only for an
empty file, as in a citation), full-file `sha256`, and the original `origin` object or null. Require equality with the
current source identity before using a summary. Skills cannot define summaries. Unknown fields in a v1
request or catalog are errors to catch misspelled selectors.

Example request at `tasks/refund.json`:

```json
{
  "version": 1,
  "catalogs": ["team/payments.context.json"],
  "select": [
    {"source": "payments:PAY-42"},
    {"file": "services/payments/refund_test.go"},
    {"skill": "payments:rehearse"}
  ],
  "search": {"query": "refund", "scope": "services/payments"},
  "max_bytes": 32768
}
```

Each selection names exactly one `file`, `source` or `skill`. A file may add `lines`. A source may add
`use_summary: true`. Catalogs and search are optional. A request needs at least one selection or a
nonempty search. A line ends at LF or at EOF. Keep original bytes including CRLF when extracting a range.
A trailing LF does not create an extra selectable line. Hash the full raw file, including line endings.
Local paths and line numbers remain usable when the producer's service is offline.

### Work-item mappings

A project's exporter reads its own mapping and a local snapshot. The mapping names the native types and
fields that supply the assignment, acceptance criteria and customer objective. It distinguishes a
parent or grouping link within a tracker from an explicit link to the same deliverable in another
tracker. Resolve links by source instance and record ID. Type mappings describe which records *can*
correspond. An actual record link identifies the pair. Matching titles never establish a link.
The exporter owns its configuration syntax. The factory reads only the resulting v1 catalog and request.

| Worked producer profile | Relationship and field choices |
|---|---|
| Aha! Feature ↔ Jira Epic | Product tracks one customer capability per Aha! Feature. Engineering delivers it through several Jira Stories or Tasks under the linked Epic. Select the assigned ticket's acceptance criteria, the Epic's scope, the linked Feature's product requirement and its Initiative's objective. An Aha! Epic (formerly Master Feature) can sit between Initiative and Feature when the workspace uses that level. |
| Aha! Feature ↔ Jira Story | Product already writes Features at story granularity. Link the selected Story directly to its Aha! Feature. Its parent Jira Epic corresponds to the Aha! Epic (Master Feature). For a team that calls the story-level type `Delivery Slice`, configure that project's type identifier and keep its native label in the excerpt. |
| Linear | Select the assigned Issue's acceptance criteria, its Project's scope and the linked Initiative's objective. For a Sub-issue, the profile may also select its parent Issue. A standalone Issue with no optional links produces a ticket-only request. |

These are example choices a team can change. Aha! describes both Jira mappings and supports workspace
terminology. Linear describes how Initiatives group Projects and Issues can have Sub-issues.
[Aha! Jira mappings](https://support.aha.io/aha-roadmaps/integrations/jira/recommended-jira-mappings~7444658099150945189) ·
[Aha! terminology](https://support.aha.io/release-notes/2020-release-notes/june-26-2020~7603835117453681135) ·
[Linear Initiatives](https://linear.app/docs/initiatives) · [Linear sub-issues](https://linear.app/docs/parent-and-sub-issues).

For the first profile, use Jira Story `PAY-142` (“Show the refund date”), its parent Epic `PAY-100`,
the Epic's linked Aha! Feature `REF-12` and that Feature's directly linked Initiative `INIT-3`.
Four other tickets share `PAY-100`. The example producer writes only the chosen fields to a separate
local excerpt for each record. Keep the native type label in the text. Use a credential-free record
reference such as `acme-jira:PAY-142` and the snapshot's immutable upstream revision in `origin`.
Seed the input with a private note on the same JSON line as the criteria and a signed URL. Assert
that neither the note nor the URL token appears in the exported files, preview or diagnostics. Given catalogs
with namespaces `jira` and `aha`, the exporter emits this request:

```json
{"version":1,"catalogs":["exports/jira.json","exports/aha.json"],"select":[{"source":"jira:PAY-142"},{"source":"jira:PAY-100"},{"source":"aha:REF-12"},{"source":"aha:INIT-3"}]}
```

Preview includes exactly those four selections in that order. The siblings and unselected fields
stay out. Use the same ticket key in the second fixture with type `Delivery Slice`: its explicit
counterpart is now `REF-12`, and the Epic's counterpart is Aha! Master Feature `MF-7`. Assert the
ordered IDs `jira:PAY-142`, `aha:REF-12`, `jira:PAY-100`, `aha:MF-7` and each record's citation.
In the Linear fixture select `linear:ENG-8` (Issue), `linear:PROJECT-2` and `linear:INIT-5` in that
order. Remove the optional Project link and assert that only `linear:ENG-8` remains.

An absent optional link ends that branch. If a required counterpart is absent, a referenced target
is missing from the snapshot, or two records satisfy a single-valued link, the example exporter names
the unresolved record or link and produces no request. Give two candidate Features the same title.
A single explicit link selects its target. Removing that required link causes export to fail.
A request with an unresolved catalog ID or an exceeded byte cap fails at factory preview under FR3
and FR6. Offline preview needs no tracker credentials, network or model calls.

The v1 payload is UTF-8 canonical JSON, with an ordered `items` array. Each item has `kind`,
`representation` (`original` or `summary`), `citations` and `text`. Citations identify the local path,
range, full-file SHA-256, and the qualified ID and upstream identity when applicable. Use keys `path`,
`lines`, `sha256` and, when present, `id` and `origin`. A whole-file citation has its actual line
range, or null for an empty file. A summary also cites
its own path and hash. Structural JSON escaping keeps embedded text from introducing a second item. Escape
terminal control and bidirectional formatting characters when displaying untrusted values to a human,
including filenames and diagnostics. Machine output preserves the underlying text for decoding.

Use [RFC 8785](https://www.rfc-editor.org/rfc/rfc8785.html) for canonical JSON bytes, without a trailing
newline. The digest is lowercase hex SHA-256 over the canonical object with keys `version`, `request`,
`declarations` and `payload`. The normalized request carries `version: 1`, `catalogs` and `select`
(empty arrays when omitted), and `max_bytes` with its effective value. Preserve selection order.
Omit absent search, optional ranges and false `use_summary`. An active search defaults its scope to
`.`. Declarations are selected catalog entries with qualified IDs, ordered by ID. The payload in the
digest input is the decoded object. Exclude machine paths, time and run identifiers.

**File-only conformance example.** A file `docs/refund.txt` contains the six ASCII letters `Refund`
followed by one LF. Preview only that file using the defaults. Declarations are empty. The canonical
payload below is 206 bytes, excluding the Markdown newline. The preview digest is
`328b03ac859bc1c82a7ced94101057820f98ffa00b695520c12986eee954e988`.

```json
{"items":[{"citations":[{"lines":[1,1],"path":"docs/refund.txt","sha256":"53cac814f776c9c4b85666d5569ecc2c0439ff9474a48a0d17e5f01728dee921"}],"kind":"source","representation":"original","text":"Refund\n"}]}
```

The command below reproduces only this ASCII/integer example. Use RFC 8785 canonicalization for general
inputs. Save the JSON line above as `payload.json` and run the command locally.

```sh
jq -cjS '{version:1,request:{version:1,catalogs:[],select:[{file:"docs/refund.txt"}],max_bytes:32768},declarations:[],payload:.}' payload.json | shasum -a 256
```

## 8. API Contract

Proposed public surface, using the existing `./factory` entry point:

```sh
./factory context preview --file design/refunds.md --file tests/refunds.feature
./factory context skills --catalog team/payments.context.json
./factory context validate --catalog team/payments.context.json
./factory context preview --request tasks/refund.json --json
./factory budget run --harness codex --session billing --task refund --prompt-file task.md \
  --context-request tasks/refund.json --expect-context DIGEST_FROM_PREVIEW
```

`--file` is repeatable shorthand for a file-only request. `--request` is mutually exclusive with it.
`context skills` accepts repeatable catalogs. Exporters can use `context validate --catalog PATH` offline
to check schemas, ID uniqueness and local source bindings before sharing a catalog. A clean validation
returns 0 and a malformed fixture returns 2. Validation never executes the producer. The run flags are a
pair and require the existing explicit budget opt-in. Substitute `claude` or `opencode` for the harness.
The developer's task prompt remains separate. A successful JSON preview returns `version`, `digest`,
`payload`, `payload_bytes`, `included` and `omitted`. Each `included` record carries `citations`,
`reason` (`explicit` or `search`) and an `item_index` into the payload. Each `omitted` record identifies
the candidate and a machine-readable reason such as `byte_limit` or `ineligible`. Count payload bytes
after decoding the JSON string. Human and JSON output refer to the same payload. Use exit 0 for success, 2
for invalid/blocked requests or stale consent and 1 for operational failure. A failure exposes a reason
and no partial payload. Existing budget command exit behavior applies once admitted.

## 9. Out of Scope

Provider-specific API clients and authentication, arbitrary executable plugins, remote URL fetching, a
shared vector database, cross-session memory, and modifying the permissions of a native harness belong to
separate designs. A producer may use any of these upstream to make its own local export. This contract
covers the material the factory adds and the consuming factory-managed run. Direct interactive launches
and automatic selection inside repair loops are outside the first delivery.
Tracker write-back, two-way synchronization and a factory-wide work-item hierarchy are outside this spec.

## 10. Open Questions

The proposed 32 KiB default and 1 MiB maximum need maintainer approval. Which real adopter can supply a
representative second catalog for qualification? The test suite must include two independently structured
catalogs regardless. Neither choice requires a particular spec template, vendor or language.

## 11. Decision Record

[Decision 183](../docs/DECISION_LOG.md#decision-183-2026-11-24-utc-draft-a-portable-task-context-and-skills-contract) records the draft. Use a data-only interchange so a team can add a spec source without
installing code in all three clients. Keep explicit files as the smallest useful entry point. A catalog is
optional until a team needs stable names, scopes, reusable skills or third-party exports. Prefer the
existing budget controller for every paid launch (docs/BUDGETS.md:13). Approval of this document
authorizes separately reviewed implementation slices. Each slice includes the CLI behavior, documentation
and acceptance evidence (docs/BOUNDED_DELIVERY.md:10).

## 12. Success Metrics

Measure progress by accepted slices, with each milestone earning one of five equally weighted units. Until
a slice is merged with passing evidence, the feature remains at its prior percentage.

| Cumulative | Observable result required |
|---|---|
| 20% | Local preview of explicit files and a catalog-selected skill. Exact byte cap, failure privacy and compatibility cases pass. |
| 40% | Two independent catalog producers, monorepo scopes, external provenance and on-demand skill listing pass. The offline work-item fixtures in AC9 pass with project-specific mappings. |
| 60% | Deterministic local retrieval, deduplication, omissions and source-bound optional summaries pass. |
| 80% | Digest-bound delivery and stale-input refusal pass for all three harness adapters, including budget and permission preservation. |
| 100% | Source and installed entry points pass the cross-harness fixture suite, existing adopters retain default behavior, and a disposable multi-team adopter rehearsal records the exact revision, commands and outcomes. Record live harness observations separately from fixture results. |

Release status and paid live testing are reported separately. A documentation merge earns 0 implementation units.
