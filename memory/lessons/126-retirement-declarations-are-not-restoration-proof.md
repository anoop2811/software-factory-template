# Retirement declarations are not restoration proof

A declared retirement path can name an optional file that was never owned or
changed. Its presence in a transaction, or its recorded action label, does not
prove that a canonical original exists for restoration. Keep preserved unowned
entries outside forward mutation custody while checking every actual restoration
against fresh images and saved originals.

Provenance: observed 2026-10-31 UTC through compiled public R1 controls. A matching
journal/manifest with one retired before-image cleared incorrectly allowed reverse
preview; a preserved unsafe cache also blocked three forward recovery modes.
The controls recorded 0 passed/2 failed in 178.656 seconds, then 2 passed/0 failed
in 256.725 seconds, including the valid exact-rollback control.

The canonical requirements are
`docs/adr/0123-owned-obsolete-initializer-cache-retirement.md:150` and
`docs/adr/0123-owned-obsolete-initializer-cache-retirement.md:158`.
