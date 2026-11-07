# Keep historical input inventories separate from current initialization

Retiring runtime files can change a historical regression's expected output even
after its source bytes have been pinned. The initializer component uses current
engine code but its archived scaffold comparison deliberately expects the older
asset set. Freezing only the files does not freeze the input selection.

Keep that specific historical inventory explicit and test-only, while exercising
the unmodified shipping candidate against the current source without retired
files. Do not freeze the whole engine or weaken the old artifact comparison to
accommodate the new inventory. Current and historical evidence serve different
contracts.

Provenance: observed 2026-11-06 through the source loop retirement inventory RED:
the actual copy gate refused missing `scripts/lib/budget.py` before removal from
the current required inventory. The historical initializer retained its original
artifact assertions through the inventory-only build overlay in
`acceptance/init_component_driver_test.go:43`; the independent current-source
journey removes the retired files before initialization in
`acceptance/source_entrypoints_test.go:630`. The decision and limits are canonical
in `docs/adr/0140-go-source-loop-and-python-retirement.md:85`.

Audit both filenames and module names plus import-path construction. An embedded
`import runtime_transition` received a separate live `scripts/lib` argument and
escaped the initial filename search. Observed 2026-11-06 in the three failing
interrupted-recovery criteria of [hosted run 42238772848](https://github.com/anoop2811/software-factory-template/actions/runs/42238772848);
`docs/adr/0140-go-source-loop-and-python-retirement.md:95` records its explicit
private-oracle disposition. Passing source-entrypoint checks do not cover this
separate base-family recovery consumer.
