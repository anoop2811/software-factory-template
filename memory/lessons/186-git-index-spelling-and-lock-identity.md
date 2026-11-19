# Git can index a lock under another spelling

On a case-insensitive volume, a literal `git ls-files` lookup can return empty
for a file that is already tracked. When enforcing a private lock, check the
index spellings and whether each one identifies the physical lock. A repository
on a case-sensitive volume can legitimately contain distinct files.

Observed on macOS on 2026-11-19 UTC in a disposable repository. After adding
`.Factory/Runtime-Transition.lock`, the lowercase literal lookup returned
empty. The `:(icase,literal).factory/runtime-transition.lock` pathspec returned
the indexed spelling, and `os.path.samefile` confirmed both names identified
the same file. The installation requirement is in docs/DECISION_LOG.md:9059.
