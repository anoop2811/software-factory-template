# Go configuration export candidate

This slice moves legacy configuration parsing and export planning into Go under
docs/adr/0056-go-configuration-export-plans.md:5. A Bash adapter applies the plan
to the calling shell, preserving local variables, explicit empty overrides and
export attributes. Installed libraries and native harness routing still use the
existing scripts while artifact delivery and recovery remain unqualified.

## Local use

Build the source candidate with the pinned development toolchain:

```sh
go build -o factory-go ./cmd/factory
FACTORY_RUNTIME_BINARY="$PWD/factory-go"
. runtime/shell/config.sh
factory_config_export
```

`factory_config_load_legacy FILE [PRESERVED_KEYS]` is also available. Like the
existing helper, standalone loading replaces matching values unless their names
appear in the optional space-delimited preserved-key string. Full export instead
snapshots caller-set names before reading either file: caller values, including
empty values, win over nonempty YAML values, which win over legacy assignments.
Empty YAML leaves legacy fallback available. A matching file entry exports a
preserved caller variable; an unmatched local variable stays local.

The new shim is Bash-only, matching the existing export helpers. The separate
`runtime/shell/readers.sh` remains POSIX-sourceable for read-only operations.
Sourcing the export shim assigns the existing `FACTORY_CONFIG_KEYS` constant and
defines functions; it makes no runtime call. Runtime calls require an explicit
absolute executable path and never search PATH, build, download or fall back.

## Data protocol and caller effects

The private Cobra protocol adds `config export [PRESERVED_KEY...]` and
`config legacy FILE [PRESERVED_KEY...]`. It accepts known variable names, not
caller values. The Go process does not infer overrides from its own environment.
It produces ordered assignment/export records using the framing in
docs/adr/0056-go-configuration-export-plans.md:17.

The shim captures successful output and validates the whole frame before applying
it. Unknown names, malformed records, missing termination or a failed runtime
leave caller variables unchanged. Values are data for `printf -v`, never shell
source for `eval`. Literal tabs and trailing whitespace inside quoted values
remain intact. Caller values can contain newlines because they never pass through
the plan. No temporary plan files are created.

After validation, native Bash assignment and export rules still apply. For
example, a standalone attempt to replace a readonly variable retains Bash's
diagnostic and caller-selected error behavior. Validation is not rollback of a
subsequent shell assignment failure.

## Compatibility and remaining work

This candidate covers Bash scalar values and C/POSIX parsing. Non-scalar shell
attributes, customized allowlists, nonregular legacy inputs, read failures and
locale-dependent differences remain characterization boundaries, as recorded in
docs/adr/0056-go-configuration-export-plans.md:65. The candidate explicitly refuses
affected integer, array, nameref and other unsupported variable attributes before
applying any record, because Bash can interpret or redirect their assignments
(docs/adr/0056-go-configuration-export-plans.md:73).
This admission restriction does not approve a public compatibility correction.
It does not complete G0/G1.

NUL-bearing legacy input is also outside the qualified parity scope. Bash 3.2
truncates a record at its first NUL, while modern Bash removes NUL bytes. The
candidate currently follows the latter behavior; independent tests record the
difference rather than treating it as an approved correction. A cutover decision
must resolve it before activation (docs/adr/0056-go-configuration-export-plans.md:97).

Configuration writes and local-hook tokenization remain separate slices. Release
artifacts, authentication, recoverable activation and the manual adopter pilot
must pass before changing installed entrypoints. At that transition, replaced
owned code must move into ignored local recovery storage and follow predecessor
retention rules. This source candidate neither deletes nor backs up active files.
