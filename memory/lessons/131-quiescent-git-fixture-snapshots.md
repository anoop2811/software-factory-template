# Keep Git fixture maintenance quiescent

A complete read-only snapshot can change after fixture preparation because Git
maintenance repacks loose objects. Keep the complete object-database and raw-index
oracles; disable automatic maintenance in the private fixture before add/commit.
Do not hide the changed paths to manufacture the intended semantic RED.

Provenance: observed 2026-11-01 UTC in native-copy qualification: the first report
showed object packing with an unchanged raw index; the subsequent clean RED kept
project, index and runtime snapshots identical. Reports are retained locally as
`inventory-fixture-maintenance-red.json` and `inventory-red.json` under
`.factory/qualification/native-copy-inventory-20261101/`. No GC process identity
was captured. The fixture's explicit controls are recorded in
acceptance/native_copy_inventory_test.go:98.
