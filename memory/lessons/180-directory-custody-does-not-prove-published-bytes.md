# Directory custody does not prove published bytes

A retained output directory and one expected filename do not prove that the
published executable still contains authenticated bytes. Replacement, symlink,
in-place content and permission changes can all preserve the directory and name.
Retain the checked leaf as well, then validate its named identity and digest
before closing publication custody. Preserve foreign replacements on refusal.

Provenance: observed 2026-11-13 in the review-client bootstrap review and four
failing mutation controls in internal/releasecmd/review_client_output_test.go:164.
The corrected check is internal/releasecmd/review_client_output.go:224, using
internal/installationfs/files.go:108. Canonical scope and receipt/status semantics
remain in docs/adr/0153-authenticated-review-client-bootstrap.md:71.
