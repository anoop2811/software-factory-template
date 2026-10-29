#!/bin/bash
set -euo pipefail

# Source-only native Ginkgo/Gomega proof runner.
# docs/adr/0108-default-go-cutover.md:226.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
exec "$SCRIPT_DIR/go-acceptance.sh" budget
