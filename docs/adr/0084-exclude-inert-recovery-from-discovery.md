# ADR-0084: Exclude inert recovery from factory-owned discovery

Status: Accepted prerequisite; backup creation and activation remain pending.
Date: 2026-09-27
Decision: 83

## Problem and scope

Before writing recovery copies, enforce specs/001-go-runtime-conversion.md:216
and specs/001-go-runtime-conversion.md:337 at existing factory-owned discovery
boundaries. An ignored backup script currently triggers citation lint; archived
documentation can satisfy an active citation when docs_root is the repository
root. Ignored Java/TypeScript backup tests also trigger current dialect gates.
Go tracked-file discovery must explicitly exclude the reserved root even for
force-tracked files; ignoring alone does not establish that boundary.

This prerequisite changes retained validation gates, not their implementation
language. No new Go feature, dependency, tool version, runtime activation, recovery
writer, Git index change or backup is introduced. The Go conversion estimate stays
50% until the existing durable-recovery-creation milestone actually qualifies.

## Exclusion contract

The reserved recovery subtree is exactly .factory/backups at the installation
root, including the node itself and all descendants. Exclude it before recursive
traversal or payload reads from:
- scripts/citation-lint.sh source enumeration
- scripts/citation-lint.sh document target resolution
- packs/java/hooks/junit5-only-check.sh test enumeration
- packs/typescript/hooks/vitest-only-check.sh test enumeration
- packs/go/hooks/ginkgo-only-check.sh tracked test enumeration

Use literal root-relative boundaries, not substring matches for backups or
.factory. Continue to check .factory/active and .factory/backups-other, and a
nested application directory named .factory/backups outside the reserved root.
Preserve existing command/root conventions, skip settings, dialect checks and
other exclusion rules. Add a small anchored prune to recursive find scans;
Git-backed enumeration must use an explicit root-relative exclusion rather than
rely on .gitignore. Keep installed gates self-contained, with no new helper-copy
requirements or language-runtime dependency.

Citation target search must also exclude recovery when docs_root is '.', an
absolute root or an ancestor containing the installation. Normalize the configured
document root through its physical directory for that lookup, with literal-safe
find patterns when path components contain glob metacharacters. A docs_root that
resolves into the reserved recovery subtree refuses with a clear fixed diagnostic
and nonzero status, without reading archived documents. This is an intentional
safety restriction: recovery cannot be configured as current canonical spec input.
Ordinary document roots and citations outside recovery remain checked. Existing
source argument/filename limitations outside this change are not silently broadened
or claimed corrected. PR_BODY source behavior stays unchanged.

Find traversals prune the directory node itself so unreadable backup children,
FIFOs or archived symlinks cannot affect the result. Files directly named
.factory/backups are also excluded; this scanner rule is not a claim that such
storage is eligible for a writer. Git exclusion does not untrack user files;
the eventual writer must separately refuse tracked recovery paths.

## Evidence boundary

Existing Go and Python loop source snapshots explicitly exclude .factory and
runtime source packaging uses a narrow allowlist. Adapter sync uses fixed active
inputs; normal local hooks require executable files while saved copies are 0600.
No dedicated factory-owned RAG/context indexer exists yet. Record these as audited
boundaries, not a claim that arbitrary user commands or external native scanners
can be constrained by a generic flag. Their supported configuration and exclusion
evidence remain prerequisites for public backup creation/activation.

## Qualification

Independent outside-in Ginkgo/Gomega tests must first fail with archived invalid
citations, archived target-only documents, Java/Jest imports and force-tracked Go
stdlib tests. Run the real scripts in isolated repositories using installed paths.
Then demonstrate unchanged enforcement on active equivalents, near-prefix paths
and nested ordinary application paths. Cover ignored/unignored recovery, empty
recovery, mixed active/recovery content, unreadable children, source/target
lookup, physical docs_root aliases and unusual root path characters. No model
call, installation, recovery creation or deletion is a validation step.
Require shell syntax/static checks, existing selftests, independent correctness
and security review, and current-head CI before merge. Progress remains in the
existing 30-package plan; this prerequisite earns no separate percentage credit.

The reserved boundary is the physical scan/installation root plus the literal
.factory/backups path. Do not follow a symlink at the reserved node to redefine
which external or active tree is excluded. Such a node is ineligible recovery
storage and the future writer must refuse it. A configured docs_root alias into
the real reserved subtree is refused through physical root normalization.

Physical-root capture must preserve trailing newline bytes in valid directory
names. Ordinary shell command substitution strips them and can move the apparent
reserved boundary, allowing archived documents to satisfy an active citation.
Qualify both installation-root and docs_root capture; preserve bytes with a
sentinel and remove only the command terminator before constructing the literal
prune pattern. This requirement concerns the new normalization boundary and does
not claim to correct unrelated legacy source-list filename parsing.
