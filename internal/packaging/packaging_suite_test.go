package packaging_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// per docs/adr/0097-installation-source-image-bundles.md:215
func TestPackaging(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Installation Git process ownership")
}
