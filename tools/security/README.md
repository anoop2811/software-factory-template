# Factory development security scanner

This module builds the existing gosec engine with the analyzer dependency needed
to read Go 1.27 compiler data. Current builds use Go 1.27.4 under
[ADR 0149](../../docs/adr/0149-compiler-provenance-and-security-patches.md).
It is separate from the factory runtime module
and is not installed into adopter projects. See
docs/adr/0098-whole-installation-upgrade-and-rollback.md:587.

From the repository root:

```sh
go -C tools/security install -mod=readonly github.com/securego/gosec/v2/cmd/gosec
gosec ./...
```

Keep the scanner engine, analyzer dependency and checksums pinned. A scan with
parser errors or a nonzero exit is incomplete even when its issue count is zero.
