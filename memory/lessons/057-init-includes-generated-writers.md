# Initialization includes the writers it invokes

Observed 2026-09-28 while qualifying native init: validating the files copied by
an initializer does not cover the files written later by its adapter generators
and review helper. In particular, Git's discovery ceiling did not stop a target
core.worktree setting from redirecting the helper's workflow destination.

A separate regression replaced an exclusively created publication temporary after
its contents were prepared. Exclusive creation alone did not prove that the named
file still belonged to the operation when it was renamed or cleaned up.

Keep the complete operation boundary in the contract: generated destinations,
effective child configuration/root, and the identity of prepared temporary files.
Do not mistake a green copy test for evidence about those later writes.

Provenance: independent compiled refusal regression in
acceptance/native_init_test.go:789 and deterministic publication regressions in
internal/initcmd/publication_test.go:26; observed RED then GREEN on 2026-09-28.
The canonical decisions and boundaries remain in
[ADR-0085](../../docs/adr/0085-go-native-init.md), not this lesson.
