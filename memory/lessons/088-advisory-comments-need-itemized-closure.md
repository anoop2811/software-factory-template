# Close advisory findings individually

Observed 2026-10-18 via GitHub PR128: the GraphQL reviewThreads query returned an
empty array while the advisory review remained visible as an issue comment:
https://github.com/anoop2811/software-factory-template/pull/128#issuecomment-6245733918
The user correctly flagged that the visible findings still lacked individual
closure, despite a general response and passing CI.

An advisory issue comment has no review-thread resolution state. Record each
numbered finding with its correction or complete-source refutation and evidence.
Keep the personal reply short and place the detailed ledger in collapsed sections.
Do not treat completed review coverage or an empty reviewThreads array as proof
that every reported claim has been addressed. The canonical verification rule is
docs/FACTORY_RULES.md:79; the PR contract is
docs/adr/0101-go-release-handoff.md:571.
