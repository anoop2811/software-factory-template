package loop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// promptReadContext changes state only after the initial admission check and
// first bounded read, avoiding timing-dependent cancellation of filesystem I/O.
type promptReadContext struct {
	context.Context
	calls   int
	failure error
}

func (c *promptReadContext) Err() error {
	c.calls++
	if c.calls >= 3 {
		return c.failure
	}
	return nil
}

var _ = Describe("Bounded loop prompt cancellation", func() {
	// per docs/adr/0076-go-bounded-loop-controller.md:154
	DescribeTable("preserves context error identity observed after reading starts", func(failure error) {
		path := filepath.Join(GinkgoT().TempDir(), "prompt")
		Expect(os.WriteFile(path, []byte(strings.Repeat("private prompt", 8192)), 0600)).To(Succeed())
		ctx := &promptReadContext{Context: context.Background(), failure: failure}
		prompt, err := loadLoopPrompt(ctx, path)
		Expect(ctx.calls).To(BeNumerically(">=", 3))
		Expect(prompt).To(BeEmpty())
		Expect(err).To(MatchError(failure))
		Expect(errors.Is(err, failure)).To(BeTrue())
	}, Entry("cancellation", context.Canceled), Entry("deadline", context.DeadlineExceeded))
})
