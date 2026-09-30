# Use the configured quality gate

Scoped default lint success does not establish compliance with the installed
language pack. Run the repository's configured command, including its config
path, before reporting lint completion. Check temporary interpreter environments
can import their standard library before spending a full compatibility run on
them; an executable left behind in temporary storage is not a usable runtime.

Provenance: observed 2026-09-29 while qualifying PR #113. Linux job
[109432328311](https://github.com/anoop2811/software-factory-template/actions/runs/36576197451/job/109432328311)
passed tests but rejected five findings under `packs/go/.golangci.yml` after
scoped default lint had passed. Commit `3861100` satisfied the configured lint;
the final [platform run](https://github.com/anoop2811/software-factory-template/actions/runs/36577869980)
passed. The first local full run selected a temporary Python executable whose
standard library and virtual-environment metadata were no longer present.
Recreating the pinned interpreter removed those startup failures; the subsequent
local suite reached its timeout, so no local full-gate pass was claimed.

The canonical quality command remains `make go-runtime-source-check`; this
lesson records an observed qualification mistake, not a second tool policy.
