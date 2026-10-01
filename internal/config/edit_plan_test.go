package config

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// per docs/adr/0090-go-native-config-migration.md:123
func TestEditPlan(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Flat configuration edit plan oracle")
}

var _ = Describe("Indexed flat configuration editing", func() {
	// per docs/adr/0090-go-native-config-migration.md:42
	// per docs/adr/0090-go-native-config-migration.md:126
	DescribeTable("matches the established reader and physical-line setter after each conceptual edit", func(initial string, conditional bool) {
		ctx := context.Background()
		keys := []string{"a", "b", "ew", "new", "9key", "_key", "config_migrated"}
		plan, err := NewEditPlan(ctx, []byte(initial), keys)
		Expect(err).NotTo(HaveOccurred())
		oracle := []byte(initial)
		edits := []struct{ key, value string }{
			{"b", "appended"}, {"ew", "joined"}, {"new", "later"},
			{"a", ""}, {"a", "filled"}, {"a", "again"},
			{"9key", " leading value # data"}, {"_key", "literal $(touch INERT)"},
			{"b", "single'quote\tvalue"}, {"config_migrated", "yes"},
		}
		compareReads := func(step int) {
			for _, key := range keys {
				want, err := GetBytes(ctx, oracle, key, "FALLBACK")
				Expect(err).NotTo(HaveOccurred())
				got, err := plan.Get(ctx, key, "FALLBACK")
				Expect(err).NotTo(HaveOccurred())
				Expect(got).To(Equal(want), "initial=%q conditional=%t step=%d key=%s", initial, conditional, step, key)
			}
		}
		compareReads(-1)
		for step, edit := range edits {
			want, err := GetBytes(ctx, oracle, edit.key, "")
			Expect(err).NotTo(HaveOccurred())
			got, err := plan.Get(ctx, edit.key, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(Equal(want), "initial=%q conditional=%t step=%d", initial, conditional, step)
			if !conditional || want == "" || edit.key == "config_migrated" {
				oracle, err = RewriteBytes(ctx, oracle, edit.key, edit.value)
				Expect(err).NotTo(HaveOccurred())
				Expect(plan.Set(ctx, edit.key, edit.value)).To(Succeed())
			}
			actual, err := plan.Bytes(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(actual)).To(Equal(string(oracle)), "initial=%q conditional=%t step=%d", initial, conditional, step)
			compareReads(step)
		}
	},
		Entry("empty direct edits", "", false), Entry("empty migration edits", "", true),
		Entry("opaque lines direct edits", "# retained\n\n untouched\n", false), Entry("opaque lines migration edits", "# retained\n\n untouched\n", true),
		Entry("duplicate configured direct edits", "a: first\na: second\nb: mine\n", false), Entry("duplicate configured migration edits", "a: first\na: second\nb: mine\n", true),
		Entry("blank first duplicate direct edits", "a: # blank\na: second\n", false), Entry("blank first duplicate migration edits", "a: # blank\na: second\n", true),
		Entry("quoted blank duplicate direct edits", "a: \"\"\na: \"second\"\n", false), Entry("quoted blank duplicate migration edits", "a: \"\"\na: \"second\"\n", true),
		Entry("unquoted unterminated tail direct edits", "a:", false), Entry("unquoted unterminated tail migration edits", "a:", true),
		Entry("quoted unterminated tail direct edits", "a: \"\"", false), Entry("quoted unterminated tail migration edits", "a: \"\"", true),
		Entry("key created by tail concatenation direct edits", "n", false), Entry("key created by tail concatenation migration edits", "n", true),
		Entry("completion replaces merged tail direct edits", "config_migrated: old", false), Entry("completion replaces merged tail migration edits", "config_migrated: old", true),
		Entry("indented keys remain opaque direct edits", " a: nested\n\ta: nested\n", false), Entry("indented keys remain opaque migration edits", " a: nested\n\ta: nested\n", true),
		Entry("ordinary first values direct edits", "a: \t leading # note\nb: \"quoted # value\" trailing\n", false), Entry("ordinary first values migration edits", "a: \t leading # note\nb: \"quoted # value\" trailing\n", true),
		Entry("unmatched reader quote direct edits", "a: \"unmatched # tail\n", false), Entry("unmatched reader quote migration edits", "a: \"unmatched # tail\n", true),
		Entry("blank completion tail direct edits", "config_migrated:", false), Entry("blank completion tail migration edits", "config_migrated:", true),
	)
})
