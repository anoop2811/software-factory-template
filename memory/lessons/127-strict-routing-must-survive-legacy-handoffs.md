# Strict routing must survive legacy handoffs

A strict outer parser does not protect a request if a later legacy parser can
reinterpret its original arguments. In the installer, post-terminator refs and
padded or empty verb operands reached source execution after legacy word
splitting. Keep malformed Go requests outside that fallback, and refuse legacy
operand forms that the retained controller cannot preserve.

Provenance: observed 2026-10-31 UTC through the compiled R2 selector controls.
The collected-path RED recorded thirteen cached-source executions with status 97
across thirty-eight vectors; the repaired installer passed those same refusal
controls. See `docs/adr/0124-ordinary-authenticated-go-installer-bootstrap.md:164`
and `docs/adr/0124-ordinary-authenticated-go-installer-bootstrap.md:173` for the
canonical contract.
