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

This isn't a convention you have to remember: `scripts/hooks/wiki-lint.sh`
enforces it. Every content page must cite a source and every cross-reference
must resolve, or the build fails (this index page is exempt from the citation
rule) — it runs in CI, in `make check`, and shows up in
`factory doctor`. That is the "lint" half of the pattern; writing and querying
the pages is the agent's job.
