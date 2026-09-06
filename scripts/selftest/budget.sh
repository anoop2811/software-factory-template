#!/bin/bash
set -euo pipefail

# Deterministic subprocess acceptance for docs/BUDGETS.md:121.
# Fake harness executables only: this suite never makes a paid model request.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec python3 "$SCRIPT_DIR/budget/acceptance.py" "$(cd "$SCRIPT_DIR/../.." && pwd)"
