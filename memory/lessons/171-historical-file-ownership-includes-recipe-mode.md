# Historical file ownership includes recipe mode

Exact bytes alone do not describe a historical installation. A copied workflow
can retain the private staging mode of its producing initializer. Qualify bytes
and mode together from that recipe, and carry both through restoration and
retention. Normalizing an immutable fixture's permissions would hide the defect.

Provenance: observed 2026-11-12 in PR #180's
[historical-recipe Linux job](https://github.com/anoop2811/software-factory-template/actions/runs/43232587338/job/131231521909),
which refused genuine copied-Go workflows because it expected 0644 rather than
their observed 0600. The decision and qualification boundary are in
docs/adr/0148-transactional-installed-ci-migration.md:332. The public historical
rebootstrap criterion is in acceptance/predecessor_recipe_compatibility_test.go.

The next public run also exposed an earlier initializer vector missing from
recognition after the mode refusal was removed. Preserve genuine producer
fixtures through the complete operation: a later synthetic image cannot prove
an earlier generation's compatibility. See the immutable-source boundary and
observed regression receipts in docs/adr/0148-transactional-installed-ci-migration.md:381.
