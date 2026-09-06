#!/bin/bash
set -euo pipefail

# Deterministic subprocess acceptance for docs/LOOPS.md:5; no paid model calls.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec python3 "$SCRIPT_DIR/loop/acceptance.py" "$(cd "$SCRIPT_DIR/../.." && pwd)"
