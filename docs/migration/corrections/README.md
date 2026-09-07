# Explicit baseline corrections

These files record approved corrections used by compatibility fixtures. They
are inert test evidence, not installation scripts, runtime fallbacks or authority
to overwrite an adopter's files. The immutable asset inventory and historical
Git blobs remain unchanged.

## EX-001: caller configuration precedence

[Decision 50](../../DECISION_LOG.md#decision-50-2026-09-06-approve-precedence-correction-and-initial-rollout-coverage)
records Anoop's approval of this prerequisite. The
[conversion spec](../../../specs/001-go-runtime-conversion.md) records the same
choice and the remaining stage gates.

When a caller supplies a supported variable, legacy configuration must not
replace its value. Caller values, including explicit empty values, take priority
over YAML, then legacy configuration. A matching legacy assignment retains its
existing export effect for a caller's local variable. Readonly values are never
assigned over. Standalone one-argument legacy loading, duplicate-key behavior,
unknown-key rejection and literal parsing retain their existing contracts.

[EX-001.json](EX-001.json) identifies both immutable sources (v0.1.6 and the
merged Bash baseline), the original executable Git mode and SHA-256, and the
patch and corrected-source SHA-256 values. Their configuration source happens
to be identical; both release identities remain independently exercised.
[EX-001.patch](EX-001.patch) was derived deterministically from the minimal
reviewed `scripts/lib/config.sh` diff against the foundation base.

The acceptance evaluator checks source identity, mode and patch scope before
applying the correction to a private fixture. It then checks the corrected hash
and the desired behavior. Damaged source or correction metadata must not produce
a passing fixture. Historical tests separately retain the original observed bug;
future Go parity must explicitly choose the corrected fixture and cannot pretend
the historical release already behaved this way.

Run the configuration acceptance from the factory module root:

```sh
go test ./acceptance -count=1 -ginkgo.focus=EX001 -ginkgo.no-color
```

This fixture coverage does not complete G0/G1, qualify an entire historical
installation, or implement upgrade backup/cleanup. The first rollout remains
limited to the two approved baselines, with preservation of unsupported older or
customized installations and a mandatory manual pilot before default cutover.
