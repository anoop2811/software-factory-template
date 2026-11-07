# ADR 0141: native public selftest and explicit development regressions

Status: accepted; locally qualified; hosted qualification and merge pending.
Decision: 156. Implements R3 group 7 after merged PR #171.

## Context

The public Go selftest already proves fourteen shipped gates with violation and
healthy controls. The remaining source script owns a broader development suite:
218 previously observed checks across 34 families, including current pack and
website checks and deliberately historical controller regressions. Fourteen
native gate pairs do not replace that broader evidence. R3 has 31/32 paths
merged; this group closes its last public source boundary without deleting tests.

## Public transport

Route `scripts/selftest/run.sh` to its physical checkout's `factory selftest`.
Reuse the qualified source adapter transport, with owner traversal `../..`.
Preserve caller cwd, literal argv, environment, streams and status. Refuse forged
scalar Bash source metadata, a symlinked adapter file and a missing/non-executable owner.
Do not select a PATH factory, parse policy in shell or fall back to development
regressions. The source launcher alone owns development compilation. Installed
adapters continue using their shipped Go binary without a compiler or Python.

Reuse `installedproof` and existing CLI dispatch. No new CLI command, runtime
dependency, installation recipe or native engine is needed for this routing.
Any discovered runtime correction requires independent reproduction and a
recorded decision before production edits.

## Development coverage and installation boundary

Move the complete general controller to `scripts/selftest/development.sh`.
Preserve all assertions, diagnostic/summary grammar and existing immutable
`1754de23de95a7e3c734bedd86894295e823c988` and
`f3a0b6520917e104883cc89b123d7244fa0c46a7` inputs. Adjust only relocation-related
references, including its self-inspection and optional-pack wrapper. Keep current
policy, pack and website subjects current. No additional historical public
runner is needed: nested historical upgrades skip doctor and other intentional
attestation stubs retain the public path.

Classify F01-F12, F16, F18, F23-F24, F27-F29 and F33-F34 as retained current
development coverage. Classify F13-F15, F17, F19-F22, F25-F26 and F30-F32 as
RETAINED-HISTORICAL, recording their deliberate current inputs where applicable.
The existing comparison map remains guidance, not a claim of native equivalence.
Preserve B01-B32, L01-L40 and the recorded review-controller dispositions.

Keep `make selftest` on the public path and the Makefile unchanged. Set this
repository's `factory.yaml` check command to the development runner. Existing
`make check` already executes that configured command after public selftest;
standalone protected-path checks also retain development coverage. Adopter
configuration is generated separately and does not inherit this selfhost command.
CI explicitly runs public native proof and development regressions as separate
steps. The development runner is not added to initializer inventory, installed
roles, required hooks or native proof assets. Its presence in a complete source
payload does not make it an installed runtime executable.

## Independent qualification

The spec-writer owns tests and relocation of the development controller. Observe
RED against the previous public runner before implementing the new transport.
Reuse qualified transport helpers with the nested adapter path and exercise the
real source launcher/native proof. Keep existing constant-pass/constant-fail
controls and snapshot/restoration assertions; do not infer coverage from counts.
Prove Make/configured-check separation with bounded recorder fixtures. Run the
complete relocated suite and optional-pack checks independently. Audit current
versus immutable references and actual acceptance partition discovery.

Use disposable complete checkouts for compilation, enforcement and tests.
Preserve the operator's ignored binary and unrelated edits in the primary repo.
Run affected source/native criteria, enclosing source gates and independent
review, then record exact-head hosted qualification separately. R3 closes only
after qualification and merge. R4 integrated rehearsal and R5-R7 release,
platform/pilot and stable activation obligations remain separate.
