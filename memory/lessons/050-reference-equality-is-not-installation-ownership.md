# Reference equality is not installation ownership

A file can match known factory bytes without evidence of how it entered an
adopter's repository. Local version or ownership metadata can also be edited.
A read-only reference comparison should report those observations without
silently turning them into authority to replace or delete the file.

Keep that boundary explicit in result fields and acceptance tests. Later action
planning must establish prior origin and target intent, and apply must revalidate
under its own exclusion/recovery protocol; a successful assessment is not an
applicable migration plan.

Provenance: docs/adr/0079-go-installation-reference-assessment.md:38 defines the
reference fingerprints; docs/adr/0079-go-installation-reference-assessment.md:42
excludes local ownership claims;
docs/adr/0079-go-installation-reference-assessment.md:68 limits success to reference
assessment; specs/001-go-runtime-conversion.md:176 requires preservation when
ownership is uncertain.
