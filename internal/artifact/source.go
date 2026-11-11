package artifact

// SourceGoVersion is the exact compiler required for newly produced bundles.
// Reader provenance is independently validated instead of matching this pin.
// docs/adr/0149-compiler-provenance-and-security-patches.md:23.
const SourceGoVersion = "go1.27.4"
