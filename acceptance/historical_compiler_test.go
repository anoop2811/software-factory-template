package acceptance_test

import (
	"debug/buildinfo"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Historical executable setup must not relabel a current-compiler build.
// per docs/adr/0149-compiler-provenance-and-security-patches.md:63
// per docs/adr/0149-compiler-provenance-and-security-patches.md:69
func buildHistoricalRuntime(source, destination string, args ...string) {
	GinkgoHelper()
	compiler := os.Getenv("FACTORY_HISTORICAL_GO")
	Expect(filepath.IsAbs(compiler)).To(BeTrue(), "set FACTORY_HISTORICAL_GO to an absolute Go 1.27.3 compiler path for immutable predecessor qualification")
	environment := map[string]string{"GOTOOLCHAIN": "local", "GOROOT": "", "GOENV": "off", "GOWORK": "off", "GOFLAGS": "", "GOEXPERIMENT": "", "GOOS": runtime.GOOS, "GOARCH": runtime.GOARCH}
	version := imageProcess(source, environment, compiler, "version")
	Expect(version.status).To(BeZero(), "historical compiler is unavailable: %+v", version)
	Expect(strings.Fields(version.stdout)).To(Equal([]string{"go", "version", "go1.27.3", runtime.GOOS + "/" + runtime.GOARCH}), "FACTORY_HISTORICAL_GO must be the native Go 1.27.3 compiler; never substitute the current compiler")
	built := imageProcess(source, environment, compiler, args...)
	Expect(built.status).To(BeZero(), "immutable historical build: %+v", built)
	info, err := buildinfo.ReadFile(destination)
	Expect(err).NotTo(HaveOccurred())
	Expect(info.GoVersion).To(Equal("go1.27.3"), "actual historical executable must match its recorded compiler provenance")
}
