# Carry active file identity into the next recovery intent

A file rename can succeed before the journal records the installed inode. If
the first recovery accepts that file using its staged identity, a later
recovery may still need that proof. Persist the qualified active identity
when recording the new intent, before staging another replacement.
Equal contents alone cannot identify the published file.

Observed 2026-11-23 in the [macOS CI migration run](https://github.com/anoop2811/software-factory-template/actions/runs/45107131014/job/137473530766).
The second reverse preview refused the factory's published Codex adapter after
both the forward upgrade and its reverse recovery were interrupted. See
docs/DECISION_LOG.md:9151 for the qualification and refusal rules.
