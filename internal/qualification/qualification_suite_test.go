package qualification

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"testing"
)

func TestQualification(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Adaptive qualification workers")
}
