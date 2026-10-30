# Option termination must survive every parser

Literal argument transport does not prove that the receiving parser will treat
the argument literally. During local initialization, the earlier candidate owner
first captured a target named `--go-candidate` after `--`. After correcting that
owner, the selected initializer still consumed a target named `--pack` as an
option and selected the sibling `--` directory instead.

Test terminated targets that collide with each parser's actual option grammar.
An unknown option-looking target alone did not expose the second defect. Keep
the normalization boundary local rather than changing unrelated callers. The
canonical contract and correction belong in
[ADR 0116](../../docs/adr/0116-explicit-go-local-initialization.md#scope-and-public-contract),
not in this lesson.

Provenance: observed 2026-10-30 UTC through compiled local-init acceptance runs:
`terminator-red.json` recorded two failures in 10.850 seconds;
`initializer-target-red.json` recorded one failure in 43.947 seconds. Independent
native initializer probes also displayed the incorrect target before consent.
The transport boundary is `internal/releasecmd/local_image.go:102`, the init-only
selector owner is `internal/releasecmd/command.go:34`, and the existing downstream
grammar is `internal/initcmd/answers.go:26`. Test evidence lives under
`.factory/qualification/local-init-slice-20261030/` locally.
