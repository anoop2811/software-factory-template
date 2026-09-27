package bridge

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/anoop2811/software-factory-template/internal/assessment"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
)

func TestAssessmentOutput(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Assessment output boundary")
}

type adoptionFaultWriter struct {
	bytes.Buffer
	failWrite bool
	writes    int
}

func (w *adoptionFaultWriter) Write(data []byte) (int, error) {
	w.writes++
	if w.failWrite {
		return 0, errors.New("PRIVATE_WRITER_ERROR")
	}
	return w.Buffer.Write(data)
}
func (w *adoptionFaultWriter) Flush() error { return errors.New("PRIVATE_FLUSH_ERROR") }

var _ = Describe("Adoption output failure", func() {
	// per docs/adr/0081-explicit-legacy-asset-adoption.md:75
	DescribeTable("refuses write or flush failures without a successful status", func(proposal, writeFailure bool) {
		writer := &adoptionFaultWriter{failWrite: writeFailure}
		command := &cobra.Command{}
		command.SetOut(writer)
		status := 0
		var result any = assessment.Proposal{}
		code := 0
		if !proposal {
			result = assessment.AdoptedPlan{}
			code = 2
		}
		err := writeAssessment(context.Background(), command, result, code, &status)
		Expect(err).To(MatchError("cannot write assessment"))
		Expect(status).To(Equal(1))
		Expect(writer.writes).To(Equal(1))
		Expect(err.Error()).NotTo(ContainSubstring("PRIVATE_"))
		if writeFailure {
			Expect(writer.Len()).To(BeZero())
		} else {
			Expect(writer.String()).To(HaveSuffix("\n"))
		}
	}, Entry("proposal write", true, true), Entry("proposal flush", true, false), Entry("adopted plan write", false, true), Entry("adopted plan flush", false, false))
})
