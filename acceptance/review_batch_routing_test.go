package acceptance_test

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Budgeted batched adversarial review routing", func() {
	// per docs/adr/0100-batched-review.md:68
	It("keeps the default strict provider and exact model without enabling fallback", func() {
		root, cwd := fixture()
		batchWriteInput(cwd, batchAdded([]string{"a.go"}, 1))
		service := batchServer(cwd)
		result := batchRun(root, cwd, filepath.Join(cwd, "out"), service, nil, nil)
		Expect(result.status).To(Equal(0), result.stderr)
		Expect(service.observed()).To(HaveLen(1))
		var request map[string]any
		Expect(json.Unmarshal(service.observed()[0].body, &request)).To(Succeed())
		Expect(request["model"]).To(Equal("fixture/model"))
		Expect(request["provider"]).To(Equal(map[string]any{"order": []any{"deepinfra"}, "allow_fallbacks": false, "require_parameters": true}))
	})
	// per docs/adr/0100-batched-review.md:68
	It("allows only the explicitly approved same-model fallback providers under both price ceilings", func() {
		root, cwd := fixture()
		batchWriteInput(cwd, batchAdded([]string{"a.go"}, 1))
		service := batchServer(cwd)
		result := batchRun(root, cwd, filepath.Join(cwd, "out"), service, nil, []string{"REVIEW_FALLBACK_PROVIDERS=together,fireworks", "REVIEW_MAX_PROMPT_PRICE=0.50", "REVIEW_MAX_COMPLETION_PRICE=1.50"})
		Expect(result.status).To(Equal(0), result.stderr)
		Expect(service.observed()).To(HaveLen(1))
		var request map[string]any
		Expect(json.Unmarshal(service.observed()[0].body, &request)).To(Succeed())
		Expect(request["model"]).To(Equal("fixture/model"))
		Expect(request).NotTo(HaveKey("models"))
		provider := request["provider"].(map[string]any)
		Expect(provider["order"]).To(Equal([]any{"deepinfra", "together", "fireworks"}))
		Expect(provider["only"]).To(Equal([]any{"deepinfra", "together", "fireworks"}))
		Expect(provider["allow_fallbacks"]).To(BeTrue())
		Expect(provider["require_parameters"]).To(BeTrue())
		prices := provider["max_price"].(map[string]any)
		prompt, err := strconv.ParseFloat(fmt.Sprint(prices["prompt"]), 64)
		Expect(err).NotTo(HaveOccurred())
		completion, err := strconv.ParseFloat(fmt.Sprint(prices["completion"]), 64)
		Expect(err).NotTo(HaveOccurred())
		Expect(prompt).To(Equal(0.5))
		Expect(completion).To(Equal(1.5))
	})
	// per docs/adr/0100-batched-review.md:68
	DescribeTable("rejects fallback without a valid bounded routing and price policy before HTTP", func(environment []string) {
		root, cwd := fixture()
		batchWriteInput(cwd, batchAdded([]string{"a.go"}, 1))
		service := batchServer(cwd)
		result := batchRun(root, cwd, filepath.Join(cwd, "out"), service, nil, environment)
		Expect(result.status).NotTo(Equal(0))
		Expect(service.observed()).To(BeEmpty())
	},
		Entry("both ceilings absent", []string{"REVIEW_FALLBACK_PROVIDERS=together"}),
		Entry("completion ceiling absent", []string{"REVIEW_FALLBACK_PROVIDERS=together", "REVIEW_MAX_PROMPT_PRICE=0.5"}),
		Entry("negative price", []string{"REVIEW_FALLBACK_PROVIDERS=together", "REVIEW_MAX_PROMPT_PRICE=-0.5", "REVIEW_MAX_COMPLETION_PRICE=1"}),
		Entry("too many fallback providers", []string{"REVIEW_FALLBACK_PROVIDERS=together,fireworks,novita,another", "REVIEW_MAX_PROMPT_PRICE=0.5", "REVIEW_MAX_COMPLETION_PRICE=1"}),
	)
})
