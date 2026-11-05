# Source libraries need actual shell capability and complete fixture owners

Sourceable Go adapters must bring their canonical shell bridge files and a real
native owner into copied current-policy fixtures. Keep historical controller
dependencies separate; freezing the current reader globally hides the actual
gate behavior. The governing contract is
[ADR 0136](../../docs/adr/0136-go-source-configuration-entrypoints.md).

For inert Bash loading, qualify path operations as builtins: ordinary caller
functions named `cd`, `pwd` or `printf` can otherwise run during sourcing.
Inherited Bash-named environment variables do not prove Bash syntax support,
and a bare result from `command -v` can name an executable in the current
directory when PATH contains an empty component. Check the actual builtin type.

Provenance: observed 2026-11-05 via independent private source-loader controls
(0/2 before correction), true-Dash cwd-executable control (0/1 before correction)
and the final eight-criterion race run (8 passed, 10.797 seconds). The retained
criterion is `acceptance/source_config_entrypoint_test.go:218`; Decision 151
records commands and outcomes. Its explicit failure branch also catches an IFS
mutation that a bare `set -e` plus intermediate `&&` predicates could miss.
