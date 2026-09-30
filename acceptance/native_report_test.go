package acceptance_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func reportFixture() (string, string, []string) {
	GinkgoHelper()
	root, cwd := fixture()
	writeFixture(filepath.Join(root, "scripts/hooks/one.sh"), []byte("unused"), 0600)
	var environment []string
	for _, value := range os.Environ() {
		key, _, _ := strings.Cut(value, "=")
		if !strings.HasPrefix(key, "FACTORY_") && !strings.HasPrefix(key, "GIT_") && !strings.HasPrefix(key, "OPENCODE_") && key != "COST_PROFILE" {
			environment = append(environment, value)
		}
	}
	environment = append(environment, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GORACE=atexit_sleep_ms=0")
	return root, cwd, environment
}

func reportRun(root, cwd string, environment []string, args ...string) cliResult {
	return reportProcess(cwd, environment, filepath.Join(root, "factory"), append([]string{"report"}, args...))
}

func reportProcess(cwd string, environment []string, executable string, args []string) cliResult {
	GinkgoHelper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, args...) // #nosec G204 -- compiled test-owned CLI or frozen local oracle with literal arguments.
	command.Dir, command.Env = cwd, environment
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	Expect(ctx.Err()).NotTo(HaveOccurred())
	status := 0
	if err != nil {
		var exited *exec.ExitError
		Expect(errors.As(err, &exited)).To(BeTrue(), "%v", err)
		status = exited.ExitCode()
	}
	return cliResult{stdout.String(), stderr.String(), status}
}

func reportLegacy(root, cwd string, environment []string, args ...string) cliResult {
	GinkgoHelper()
	for _, path := range []string{"scripts/factory-report.sh", "scripts/lib/config.sh"} {
		command := exec.Command("git", "show", "9a3f2e4c61bbc92e4b05fde517cd60e78c4b7165:"+path) // #nosec G204 -- frozen local baseline and fixed fixture paths.
		command.Dir = ".."
		data, err := command.Output()
		Expect(err).NotTo(HaveOccurred())
		writeFixture(filepath.Join(root, path), data, 0700)
	}
	return reportProcess(cwd, environment, "/bin/bash", append([]string{filepath.Join(root, "scripts/factory-report.sh")}, args...))
}

var _ = Describe("Native Go report core", func() {
	// per docs/adr/0087-go-native-report.md:17
	It("reports zero records without the legacy report script or configuration library", func() {
		root, cwd, environment := reportFixture()
		out := reportRun(root, cwd, environment)
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(out.stdout).To(ContainSubstring("Factory report\n"))
		Expect(out.stdout).To(ContainSubstring("Gate blocks recorded:           0"))
		Expect(out.stdout).To(ContainSubstring("No blocks recorded yet"))
		Expect(out.stderr).To(BeEmpty())
	})
	// per docs/adr/0087-go-native-report.md:22
	It("reads the template event log and labels its estimate without a legacy script", func() {
		root, cwd, environment := reportFixture()
		writeFixture(filepath.Join(root, ".factory/events.log"), []byte("2026-09-29\tgate-one\tblocked\n"), 0600)
		out := reportRun(root, cwd, environment, "--ignored")
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(out.stdout).To(ContainSubstring("Gate blocks recorded:           1"))
		Expect(out.stdout).To(ContainSubstring("Estimate (labeled)"))
		Expect(out.stdout).To(ContainSubstring("Review-spend avoided:  ~3000 tokens"))
	})
	// per docs/adr/0087-go-native-report.md:31
	It("clears the selected event log when the first argument requests it and ignores extras", func() {
		root, cwd, environment := reportFixture()
		path := filepath.Join(root, ".factory/events.log")
		writeFixture(path, []byte("event\n"), 0600)
		out := reportRun(root, cwd, environment, "--clear", "--ignored")
		Expect(out).To(Equal(cliResult{"factory report: event log cleared.\n", "", 0}))
		_, err := os.Stat(path)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
})

var _ = Describe("Native Go report frozen parity", func() {
	// per docs/adr/0087-go-native-report.md:71
	DescribeTable("preserves ordinary output and status from the frozen script", func(kind string) {
		root, cwd, environment := reportFixture()
		var args []string
		switch kind {
		case "events":
			writeFixture(filepath.Join(root, ".factory/events.log"), []byte("today\tfirst\twhy\nyesterday\tsecond\tanother reason\n"), 0600)
		case "TSV":
			writeFixture(filepath.Join(root, ".factory/events.log"), []byte("\n \nsolo\n\tleading\tgate\treason\nnow\t\tgate\t\treason\tmore\t\nlast\tgate\ttail"), 0600)
		case "config":
			writeFixture(filepath.Join(cwd, "factory.yaml"), []byte("cost_profile: economy\ncost_profile: ignored\nopencode_default_model: \"literal # model\" # comment\n"), 0600)
			writeFixture(filepath.Join(cwd, "factory.config"), []byte("OPENCODE_FRONTIER_MODEL=legacy-frontier\nOPENCODE_ECONOMY_MODEL='legacy economy'\n"), 0600)
		case "ignored arguments":
			args = []string{"--json", "--help", "--clear", "$(touch SHOULD_NOT_EXIST)"}
		case "environment":
			writeFixture(filepath.Join(cwd, "factory.yaml"), []byte("cost_profile: ignored\nopencode_default_model: yaml-default\nopencode_frontier_model: yaml-frontier\n"), 0600)
			environment = append(environment, "COST_PROFILE=", "OPENCODE_DEFAULT_MODEL=env-default", "OPENCODE_FRONTIER_MODEL=")
		case "relative overrides":
			writeFixture(filepath.Join(cwd, "selected.yaml"), []byte("cost_profile: \"$(touch SHOULD_NOT_EXIST)\"\n"), 0600)
			writeFixture(filepath.Join(cwd, "selected.log"), []byte("today\tgate\trelative override\n"), 0600)
			writeFixture(filepath.Join(root, ".factory/events.log"), []byte("ignored\nignored\n"), 0600)
			environment = append(environment, "FACTORY_CONFIG=selected.yaml", "FACTORY_EVENT_LOG=selected.log", "FACTORY_REVIEW_TOKENS=42")
		case "hook types":
			writeFixture(filepath.Join(root, "scripts/hooks/nested.sh/ignored.sh"), nil, 0600)
			Expect(os.Symlink("missing", filepath.Join(root, "scripts/hooks/dangling.sh"))).To(Succeed())
			writeFixture(filepath.Join(root, "scripts/hooks/.hidden.sh"), nil, 0600)
			writeFixture(filepath.Join(root, "scripts/hooks/not-a-hook.txt"), nil, 0600)
		case "lexical config":
			Expect(os.MkdirAll(filepath.Join(cwd, "real/child"), 0700)).To(Succeed())
			Expect(os.Symlink(filepath.Join(cwd, "real/child"), filepath.Join(cwd, "link"))).To(Succeed())
			writeFixture(filepath.Join(cwd, "real/factory.yaml"), []byte("project_name: fixture\n"), 0600)
			writeFixture(filepath.Join(cwd, "real/factory.config"), []byte("COST_PROFILE=lexical\n"), 0600)
			writeFixture(filepath.Join(cwd, "factory.config"), []byte("COST_PROFILE=wrong\n"), 0600)
			environment = append(environment, "FACTORY_CONFIG="+cwd+"/link/../factory.yaml")
		}
		expected := reportLegacy(root, cwd, environment, args...)
		Expect(expected.status).To(BeZero(), "%+v", expected)
		Expect(os.Remove(filepath.Join(root, "scripts/factory-report.sh"))).To(Succeed())
		Expect(os.Remove(filepath.Join(root, "scripts/lib/config.sh"))).To(Succeed())
		beforeRoot, beforeCWD := nativeInitArtifacts(root), nativeInitArtifacts(cwd)
		actual := reportRun(root, cwd, environment, args...)
		Expect(actual).To(Equal(expected))
		Expect(nativeInitArtifacts(root)).To(Equal(beforeRoot))
		Expect(nativeInitArtifacts(cwd)).To(Equal(beforeCWD))
	}, Entry("missing event log", "missing"), Entry("ordinary events", "events"), Entry("Bash tab fields and unterminated tail", "TSV"), Entry("flat config and legacy fallback", "config"), Entry("ignored arguments", "ignored arguments"), Entry("explicit empty caller precedence", "environment"), Entry("relative config and event overrides", "relative overrides"), Entry("immediate hook entry types", "hook types"), Entry("lexical legacy sibling", "lexical config"))
})
