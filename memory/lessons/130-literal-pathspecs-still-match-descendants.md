# Literal pathspecs still match descendants

Literal Git pathspecs disable wildcard interpretation but still select indexed
descendants. A file-presence gate must compare each required filename with the
exact NUL-delimited names returned by Git; a successful exit alone is insufficient.

Provenance: observed 2026-11-01 UTC via
`rtk proxy env GIT_OPTIONAL_LOCKS=0 git --literal-pathspecs -c core.fsmonitor=false ls-files --error-unmatch -- scripts/hooks`:
exit 0 returned hook children, without an exact `scripts/hooks` entry. The
independent gate regression also observed an indexed directory's child falsely
certifying an untracked regular parent input before repair. Canonical contract:
docs/adr/0125-native-initializer-copy-inventory-gate.md:160.
