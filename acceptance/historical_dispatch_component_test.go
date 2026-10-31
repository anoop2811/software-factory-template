package acceptance_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const historicalDispatcherRevision = "00f6ef995ec4aab4c3fac2f65ca0b31008a849d7"

var historicalDispatcherOnce sync.Once
var historicalDispatcherProgram []byte

// Build the actual final mapped dispatcher from an independently fixed trusted
// Git object. This executable is historical evidence, never the current public
// CLI, an installed role, a shipping escape flag or duplicated dispatcher code.
// per docs/adr/0122-default-packaged-go-upgrade.md:95
func historicalDispatcherBytes() []byte {
	GinkgoHelper()
	historicalDispatcherOnce.Do(func() {
		repository, err := filepath.Abs("..")
		Expect(err).NotTo(HaveOccurred())
		work := GinkgoT().TempDir()
		source := filepath.Join(work, "immutable dispatcher source")
		imageGit(repository, "clone", "--quiet", "--shared", "--no-checkout", repository, source)
		imageGit(source, "checkout", "--quiet", "--detach", historicalDispatcherRevision)
		Expect(strings.TrimSpace(imageGit(source, "rev-parse", "HEAD"))).To(Equal(historicalDispatcherRevision))
		output := filepath.Join(work, "historical-factory")
		args := []string{"build", "-trimpath", "-buildvcs=false", "-o", output, "./cmd/factory"}
		if os.Getenv("FACTORY_CLI_TEST_RACE") == "1" {
			args = append([]string{"build", "-race"}, args[1:]...)
		}
		env := map[string]string{"GOTOOLCHAIN": "local", "GOPROXY": "off", "GOSUMDB": "off", "GOWORK": "off", "GOENV": "off", "GOFLAGS": ""}
		built := imageProcess(source, env, "go", args...)
		Expect(built.status).To(Equal(0), "trusted immutable dispatcher component build: %+v", built)
		historicalDispatcherProgram, err = os.ReadFile(output)
		Expect(err).NotTo(HaveOccurred())
		AddReportEntry("historical-dispatcher.component", map[string]any{"revision": historicalDispatcherRevision, "race": os.Getenv("FACTORY_CLI_TEST_RACE") == "1", "runtime_claim": "historical component only"}, ReportEntryVisibilityNever)
	})
	return historicalDispatcherProgram
}

// Only explicitly classified obsolete dispatcher assertions use these immutable
// bytes. Current assessment/adoption/recovery fixtures keep the current runtime.
// per docs/adr/0122-default-packaged-go-upgrade.md:196
func useHistoricalDispatcher(root string) {
	GinkgoHelper()
	writeFixture(filepath.Join(root, "factory"), historicalDispatcherBytes(), 0755)
}

func historicalDispatcherFixture() (string, string) {
	root, cwd := fixtureDirectories()
	useHistoricalDispatcher(root)
	writeFixture(filepath.Join(root, "legacy-factory"), legacyDispatcher, 0755)
	return root, cwd
}
