package evalcmd

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/anoop2811/software-factory-template/internal/native"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestGoldenDiagnostics(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Golden diagnostic privacy")
}

// Merely offering this method must not make arbitrary error content public.
// per docs/adr/0139-go-source-budget-entrypoint.md:130
type privateGoldenDiagnosticError struct{}

func (privateGoldenDiagnosticError) Error() string          { return "PRIVATE_ERROR" }
func (privateGoldenDiagnosticError) SafeDiagnostic() string { return "PRIVATE_FORGED_DIAGNOSTIC" }

var _ = Describe("Golden ownership diagnostic privacy", func() {
	// per docs/adr/0139-go-source-budget-entrypoint.md:128
	// per docs/adr/0139-go-source-budget-entrypoint.md:130
	It("reports only concrete native ownership facts while preserving generic failure and retained work", func() {
		ownership := &native.OwnershipError{ProcessPID: 4242}
		retained := &retainedGoldenWorkError{path: "/retained fixture/quoted \"work\"\n"}
		token := strings.Repeat("a", 64)
		private := errors.New("PRIVATE_RUNNER_STDERR PRIVATE_ENVIRONMENT " + token)
		for _, item := range []struct {
			name      string
			cause     error
			wantFacts bool
		}{
			{"arbitrary error", errors.Join(private, retained), false},
			{"forged diagnostic method", errors.Join(privateGoldenDiagnosticError{}, private, retained), false},
			{"wrapped native evidence", fmt.Errorf("PRIVATE_WRAPPER: %w", errors.Join(ownership, private, retained)), true},
		} {
			var diagnostic bytes.Buffer
			Expect(goldenError(&diagnostic, item.cause)).To(Equal(1), item.name)
			text := diagnostic.String()
			Expect(text).To(HavePrefix("golden-task-eval: evaluation could not complete\n"), item.name)
			Expect(text).To(ContainSubstring(fmt.Sprintf("golden-task-eval: retained work directory %q\n", retained.path)), item.name)
			Expect(text).NotTo(ContainSubstring("PRIVATE_"), item.name)
			Expect(text).NotTo(ContainSubstring(token), item.name)
			if item.wantFacts {
				Expect(text).To(ContainSubstring(ownership.SafeDiagnostic()), item.name)
			} else {
				Expect(text).To(Equal("golden-task-eval: evaluation could not complete\n"+fmt.Sprintf("golden-task-eval: retained work directory %q\n", retained.path)), item.name)
			}
		}
	})
})
