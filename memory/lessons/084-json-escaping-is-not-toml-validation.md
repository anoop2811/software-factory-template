# JSON escaping is not TOML validation

Go's JSON string encoder leaves DEL (U+007F) literal, which is forbidden in a
TOML basic string. A syntactically valid canonical JSON description can therefore
produce invalid Codex configuration unless the renderer also escapes DEL. Test
both the emitted representation and exact decoded value; ensure the field under
test actually belongs to the generated file being asserted.

Provenance: observed 2026-10-18 through compiled `factory sync codex` and an
independent TOML parser while verifying the completed PR #126 model review:
https://github.com/anoop2811/software-factory-template/pull/126#issuecomment-6244615935
The governing correction is docs/adr/0099-native-factory-orchestration.md:240;
the durable regression is in `acceptance/orchestration_sync_test.go`.
