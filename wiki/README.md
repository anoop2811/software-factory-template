# wiki/

This directory is agent-maintained. It starts empty on purpose.

As agents work in your repo, the wiki-maintainer role writes short, cited
pages here that summarize what a module does and point at the source — a query
layer over your codebase, not a second copy of it.

Two rules keep it honest:

- **It summarizes and points; it never forks canon.** Your spec source and the
  ADRs stay authoritative. A wiki page that restates them will drift.
- **Every claim carries provenance** — a `file:line`, a fetched URL with a
  date, or `observed YYYY-MM-DD via <action>`. A page without provenance is
  worse than no page: it becomes a stale fact asserted with confidence.

Commit wiki pages like any other file; they are lint-gated at merge.
