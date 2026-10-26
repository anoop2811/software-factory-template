package acceptance_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The native npm boundary runs an optional dependency lifecycle in the fixture,
// including the interpreter fallback that installation must not enable.
// per docs/DECISION_LOG.md:4873
func handoffNPMLifecycleFixture(f handoffBootstrapFixture) {
	GinkgoHelper()
	lifecycle := filepath.Join(f.root, "optional native addon", "install.sh")
	writeFixture(lifecycle, []byte(`#!/bin/sh
set -eu
printf 'optional native install\n' >> "$HANDOFF_RECORD_DIR/npm-lifecycle"
python3 --version || :
`), 0700)
	writeFixture(filepath.Join(f.root, "host tools", "npm"), []byte(`#!/bin/sh
set -eu
[ "${1-}" = install ] || exit 97
[ -f package.json ] || exit 97
ignore_scripts=false
for argument do
  case "$argument" in --ignore-scripts|--ignore-scripts=true) ignore_scripts=true ;; esac
done
if [ "$ignore_scripts" = false ]; then
  /bin/sh "$HANDOFF_NPM_LIFECYCLE"
fi
printf '%s\n' "$PWD" >> "$HANDOFF_RECORD_DIR/npm-installed"
`), 0700)
	f.environment["HANDOFF_NPM_LIFECYCLE"] = lifecycle
}

// per docs/DECISION_LOG.md:4879
func handoffNPMLifecyclePreserved(f handoffBootstrapFixture, target string) {
	GinkgoHelper()
	installed, err := os.ReadFile(filepath.Join(f.records, "npm-installed"))
	Expect(err).NotTo(HaveOccurred(), "optional dependency installation must still run")
	Expect(string(installed)).To(Equal(filepath.Join(target, ".opencode") + "\n"))
	lifecycle, lifecycleErr := os.ReadFile(filepath.Join(f.records, "npm-lifecycle"))
	forbidden, forbiddenErr := os.ReadFile(filepath.Join(f.records, "forbidden-tool"))
	Expect(os.IsNotExist(forbiddenErr)).To(BeTrue(), "Go-selected initialization attempted a forbidden tool: %s; lifecycle trace: %s", forbidden, lifecycle)
	Expect(os.IsNotExist(lifecycleErr)).To(BeTrue(), "Go-selected initialization must not run optional package lifecycle scripts: %s", lifecycle)
}
