# Verifier filenames can carry format semantics

A byte-identical private copy is insufficient when a consumer selects its parser
from the filename. Preserve that format discriminator without returning to the
mutable original or weakening signature checks. Use one shared snapshot-name
function for both writing and invoking the verifier.

Provenance: observed 2026-10-24 in published release run
https://github.com/anoop2811/software-factory-template/actions/runs/40103396983.
All four native jobs authenticated their subjects, then candidate staging lost the
bundle suffix and GitHub CLI refused it. The same signed archive and unchanged
bundle authenticated with `.jsonl` but failed with no suffix in a local real-CLI
probe. The authoritative parser dispatches on `filepath.Ext`:
https://github.com/cli/cli/blob/4f2cfe92e3499f8581bf39db77e0a1431fa7b55c/pkg/cmd/attestation/verification/attestation.go.
Decision 110 in docs/DECISION_LOG.md:4639 defines the correction and independent
regression boundary. These local observations do not qualify a replacement release.
