package installationimage_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// per docs/adr/0097-installation-source-image-bundles.md:173
func TestInstallationImage(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Installation source image schema")
}
