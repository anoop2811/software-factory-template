package releasecmd

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// per docs/adr/0148-transactional-installed-ci-migration.md:145
func TestReleaseCommands(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Release command presentation")
}
