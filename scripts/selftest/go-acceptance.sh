#!/bin/bash
set -euo pipefail

# Source-only developer proofs; never installed as factory runtime dependencies.
# docs/adr/0108-default-go-cutover.md:226.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
ROOT="$(cd "$SCRIPT_DIR/../.." && printf '%s/.' "$PWD")"
ROOT=${ROOT%/.}
cd "$ROOT"
if [ ! -f go.mod ] || ! grep -qx 'module github.com/anoop2811/software-factory-template' go.mod || [ ! -d acceptance ]; then
  printf '%s\n' 'go-acceptance: expected the complete factory Go source checkout' >&2
  exit 1
fi
[ "$#" -eq 1 ] || { printf '%s\n' 'go-acceptance: expected one fixed proof selector' >&2; exit 2; }
case "$1" in
  budget)
    go test -race -count=1 -timeout=10m ./internal/budget ./internal/native -ginkgo.fail-on-empty
    focus='G2 ((composed |interoperable )?budget|native (harness|usage)|configured command)'
    ;;
  loop)
    go test -race -count=1 -timeout=10m ./internal/loop -ginkgo.fail-on-empty
    focus='G2 (bounded loop|manual loop|loop|budget and loop paid controller interruption)'
    ;;
  review)
    focus='Native orchestration review|Bounded streaming adversarial review|Budgeted (batched adversarial review|review workflow)|Hosted budgeted adversarial review|Adversarial review large payload boundary'
    ;;
  optional-packs)
    printf '%s\n' 'go-acceptance: native source gate proof and explicitly historical initializer component criteria'
    focus='Go source entry cutover.*fourteen source gates|Native Go init preservation.*optional|Native orchestration init synchronization|Native Go init installed attestation'
    ;;
  java-build-tool)
    printf '%s\n' 'go-acceptance: native initializer artifact criteria with frozen historical source inputs'
    focus='Native Go init artifact compatibility|Native Go init.*(Maven|Java)'
    ;;
  *) printf '%s\n' 'go-acceptance: unknown proof selector' >&2; exit 2 ;;
esac
exec go test -race -count=1 -timeout=10m ./acceptance -ginkgo.focus="$focus" -ginkgo.fail-on-empty -ginkgo.no-color
