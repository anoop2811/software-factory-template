# Tar EOF is not gzip integrity evidence

When a tar reader reaches its end marker, the surrounding gzip stream can still
have unread data and an unchecked trailer. Closing the gzip reader alone does
not validate its checksum. Drain the bounded stream to EOF, and separately reject
extra members when the archive contract permits only one gzip stream.

Provenance: Go gzip Reader.Close and Reader.Multistream documentation, fetched
2026-09-07: https://pkg.go.dev/compress/gzip#Reader.Close and
https://pkg.go.dev/compress/gzip#Reader.Multistream. The factory contract is
docs/adr/0061-runtime-bundle-staging.md:55; the acceptance oracle starts with a
valid three-file tar before adding an invalid tail so missing files cannot make
that check pass vacuously.
