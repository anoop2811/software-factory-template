package acceptance_test

import (
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// per docs/adr/0072-go-budget-argument-compatibility.md:72
func budgetArgumentParity(root, cwd string, args []string, extra ...string) cliResult {
	GinkgoHelper()
	expected := budgetCLIOracle(root, cwd, args, extra...)
	actual := budgetCLI(root, cwd, args, extra...)
	Expect(actual.status).To(Equal(expected.status), "arguments %q; actual=%+v oracle=%+v", args, actual, expected)
	Expect(actual.stdout != "").To(Equal(expected.stdout != ""), "stdout channel for %q", args)
	Expect(actual.stderr != "").To(Equal(expected.stderr != ""), "stderr channel for %q", args)
	if strings.HasPrefix(expected.stdout, "usage:") {
		Expect(strings.ToLower(actual.stdout)).To(ContainSubstring("usage:"))
		for _, flag := range []string{"--help", "--json", "--session", "--task", "--harness", "--role", "--max-cost-usd", "--prompt-file"} {
			Expect(strings.Contains(actual.stdout, flag)).To(Equal(strings.Contains(expected.stdout, flag)), "help option scope/discoverability: %s", flag)
		}
		if !strings.Contains(expected.stdout, "--session") {
			for _, name := range []string{"plan", "run", "report"} {
				Expect(actual.stdout).To(ContainSubstring(name))
			}
		}
	} else if expected.stdout != "" {
		if strings.HasPrefix(expected.stdout, "{") {
			Expect(budgetJSONComparable(budgetDecode([]byte(actual.stdout)))).To(Equal(budgetJSONComparable(budgetDecode([]byte(expected.stdout)))))
		} else {
			Expect(actual.stdout).To(Equal(expected.stdout))
		}
	}
	if expected.stderr != "" {
		Expect(strings.Contains(strings.ToLower(actual.stderr), "usage:")).To(Equal(strings.Contains(strings.ToLower(expected.stderr), "usage:")), "argument usage versus runtime diagnostic for %q", args)
		Expect(strings.Contains(actual.stderr, "factory budget:") || strings.Contains(strings.ToLower(actual.stderr), "error:")).To(BeTrue(), "error diagnostic must remain present")
	}
	return actual
}

var _ = Describe("G2 budget argument compatibility core", func() {
	// per docs/adr/0072-go-budget-argument-compatibility.md:37
	DescribeTable("exposes help without reading execution state", func(args []string) {
		root, cwd := fixture()
		bad := []byte("PRIVATE_INVALID_HISTORY")
		writeFixture(filepath.Join(root, ".factory/budget.json"), bad, 0600)
		writeFixture(filepath.Join(root, "bin/claude"), []byte("#!/bin/sh\nprintf probed > \"$BUDGET_ARG_PROBE\"\nexit 1\n"), 0755)
		out := budgetArgumentParity(root, cwd, args, "FACTORY_BUDGET_ENABLED=INVALID", "BUDGET_ARG_PROBE="+filepath.Join(cwd, "probe"))
		Expect(out.status).To(BeZero())
		Expect(out.stderr).To(BeEmpty())
		after, err := os.ReadFile(filepath.Join(root, ".factory/budget.json"))
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(bad))
		_, err = os.Stat(filepath.Join(cwd, "probe"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		_, err = os.Stat(filepath.Join(root, ".factory/budget.lock"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("root", []string{"--help"}), Entry("plan", []string{"plan", "-h"}), Entry("run", []string{"run", "--help"}), Entry("report prefix", []string{"report", "--he"}))
	// per docs/adr/0072-go-budget-argument-compatibility.md:22
	DescribeTable("accepts unique scoped long prefixes", func(args []string) { root, cwd := fixture(); budgetArgumentParity(root, cwd, args) }, Entry("plan", []string{"plan", "--s=s", "--t=t", "--ha=codex", "--j"}), Entry("report", []string{"report", "--s=s", "--j"}))
	// per docs/adr/0072-go-budget-argument-compatibility.md:46
	It("preserves usage on the error stream for missing required operands", func() {
		root, cwd := fixture()
		out := budgetArgumentParity(root, cwd, []string{"plan"})
		Expect(out.status).To(Equal(2))
		Expect(out.stdout).To(BeEmpty())
	})
})

func budgetArgumentPlan(extra ...string) []string {
	return append([]string{"plan", "--session=s", "--task=t", "--harness=codex"}, extra...)
}

var _ = Describe("G2 budget argument compatibility matrix", func() {
	// per docs/adr/0072-go-budget-argument-compatibility.md:80
	DescribeTable("matches immutable grammar and precedence without native effects", func(args []string) {
		root, cwd := fixture()
		out := budgetArgumentParity(root, cwd, args, "FACTORY_BUDGET_ENABLED=false")
		Expect(out.stdout + out.stderr).NotTo(ContainSubstring("PRIVATE_OPERAND"))
		_, err := os.Stat(filepath.Join(root, ".factory"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	},
		Entry("root short help", []string{"-h"}), Entry("root unique prefix", []string{"--h"}), Entry("root short repeated help", []string{"-hhh"}), Entry("leaf exact report help", []string{"report", "--help"}), Entry("leaf short repeated help", []string{"plan", "-hhh"}),
		Entry("leaf repeated known help", []string{"plan", "-hh"}), Entry("short mixed group", []string{"plan", "-hj"}), Entry("short attached value", []string{"plan", "-h=false"}), Entry("root attached help", []string{"--help=foo"}), Entry("long attached help", []string{"plan", "--help=false"}),
		Entry("unique role cost and model-free prefixes", []string{"plan", "--s=s", "--t=t", "--ha=codex", "--r=reviewer", "--m=1", "--j"}), Entry("former refused session abbreviation", []string{"report", "--sess=s", "--j"}), Entry("run scoped prefixes", []string{"run", "--s=s", "--t=t", "--ha=claude", "--p=PRIVATE_MISSING", "--j"}), Entry("exact help overrides prefix ambiguity", []string{"plan", "--help"}), Entry("ambiguous prefix", []string{"plan", "--h"}), Entry("ambiguous attached prefix", []string{"plan", "--h=codex"}), Entry("command not abbreviated", []string{"pl", "-h"}),
		Entry("help before later missing value", []string{"plan", "-h", "--session"}), Entry("missing value before help", []string{"plan", "--session", "-h"}), Entry("help before invalid choice", []string{"plan", "-h", "--har=bad"}), Entry("invalid choice before help", []string{"plan", "--har=bad", "-h"}), Entry("ambiguity after help", []string{"plan", "-h", "--h"}), Entry("ambiguity before help", []string{"plan", "--h", "-h"}), Entry("unknown before help", []string{"plan", "--unknown", "-h"}), Entry("unknown after help", []string{"plan", "-h", "--unknown"}), Entry("help before invalid boolean", []string{"plan", "-h", "--json=bad"}), Entry("help before invalid short suffix", []string{"plan", "-h", "-hx"}),
		Entry("root help skips child ambiguity", []string{"--help", "report", "--h"}), Entry("unknown root before child help", []string{"--unknown", "report", "-h"}), Entry("unknown root attached before child help", []string{"--unknown=value", "report", "-h"}), Entry("unknown root separate value becomes bad command", []string{"--unknown", "value", "report", "-h"}), Entry("root option scoped incorrectly", []string{"--json", "report"}),
		Entry("duplicate invalid harness cannot be repaired", budgetArgumentPlan("--harness=bad", "--harness=codex")), Entry("duplicate invalid role cannot be repaired", budgetArgumentPlan("--role=bad", "--role=reviewer")), Entry("duplicate ID last value", budgetArgumentPlan("--session=../bad", "--session=last", "--json")), Entry("duplicate cost last value", budgetArgumentPlan("--max-cost-usd=bad", "--max-cost-usd=1", "--json")), Entry("duplicate boolean", budgetArgumentPlan("--json", "--json")),
		Entry("negative integer value", budgetArgumentPlan("--max-cost-usd", "-1")), Entry("negative decimal value", budgetArgumentPlan("--max-cost-usd", "-.5")), Entry("negative exponent is option looking", budgetArgumentPlan("--max-cost-usd", "-1e3")), Entry("negative trailing point is option looking", budgetArgumentPlan("--max-cost-usd", "-1.")), Entry("negative underscore is option looking", budgetArgumentPlan("--max-cost-usd", "-1_000")), Entry("negative exponent equals literal", budgetArgumentPlan("--max-cost-usd=-1e3")), Entry("lone dash scalar", budgetArgumentPlan("--session", "-")), Entry("option cannot fill scalar", budgetArgumentPlan("--session", "--json")), Entry("option-looking attached scalar", budgetArgumentPlan("--session=--help")), Entry("negative ID scalar", budgetArgumentPlan("--session", "-1")), Entry("empty ID before help deferred", budgetArgumentPlan("--session=", "-h")),
		Entry("bare leaf terminator", []string{"report", "--"}), Entry("leaf terminator hides help", []string{"report", "--", "-h"}), Entry("root terminator does not select command", []string{"--", "report", "-h"}), Entry("unknown private operand", []string{"report", "--PRIVATE_OPERAND"}), Entry("private invalid choice", []string{"plan", "--harness=PRIVATE_OPERAND"}), Entry("literal boolean-looking prompt filename", []string{"run", "--session=s", "--task=t", "--harness=claude", "--prompt-file=--json=true", "--json"}), Entry("private option-looking missing scalar", []string{"plan", "--session", "--PRIVATE_OPERAND"}), Entry("hidden completion with missing value", []string{"report", "__complete", "--session"}), Entry("hidden completion no descriptions", []string{"report", "__completeNoDesc"}))
})

var _ = Describe("G2 budget argument help semantics", func() {
	// per docs/adr/0072-go-budget-argument-compatibility.md:109
	DescribeTable("identifies required options and accepted choices", func(command string) {
		root, cwd := fixture()
		out := budgetArgumentParity(root, cwd, []string{command, "--help"}, "FACTORY_BUDGET_ENABLED=INVALID")
		Expect(out.status).To(BeZero())
		for _, flag := range []string{"--session", "--task", "--harness", "--prompt-file"} {
			required := command != "report" && (flag != "--prompt-file" || command == "run")
			identified := false
			for _, line := range strings.Split(out.stdout, "\n") {
				if strings.Contains(line, flag) && strings.Contains(strings.ToLower(line), "required") {
					identified = true
				}
			}
			Expect(identified).To(Equal(required), "required-option discoverability for %s in %s", flag, out.stdout)
		}
		if command != "report" {
			for _, choice := range []string{"codex", "claude", "opencode", "spec-writer", "implementer", "refactorer", "reviewer", "wiki-maintainer"} {
				Expect(out.stdout).To(ContainSubstring(choice))
			}
		}
	}, Entry("plan", "plan"), Entry("run", "run"), Entry("report", "report"))
})
