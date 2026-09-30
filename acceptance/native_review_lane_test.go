package acceptance_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const reviewLaneHeader = "# Managed by: factory review-lane. Remove with: ./factory review-lane disable"

func reviewLaneFixture() (string, string, []string) {
	GinkgoHelper()
	root, cwd, base := reportFixture()
	var environment []string
	for _, value := range base {
		key, _, _ := strings.Cut(value, "=")
		if !strings.HasPrefix(key, "REVIEW_") && !strings.HasPrefix(key, "CLAUDE_") && !strings.HasPrefix(key, "CODEX_") && key != "MODEL_PROVIDER" {
			environment = append(environment, value)
		}
	}
	writeFixture(filepath.Join(cwd, "factory.yaml"), []byte("project_name: lane-fixture\n"), 0600)
	writeFixture(filepath.Join(root, "packs/review-lane/review-pr.yml"), []byte("name: advisory review\nsecret: __REVIEW_API_KEY_SECRET__\n"), 0600)
	bin := filepath.Join(filepath.Dir(root), "lane-bin")
	writeFixture(filepath.Join(bin, "gh"), []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$LANE_GH_CALLS\"\nexit 1\n"), 0700)
	for i, value := range environment {
		if strings.HasPrefix(value, "PATH=") {
			environment[i] = "PATH=" + bin + string(os.PathListSeparator) + strings.TrimPrefix(value, "PATH=")
		}
	}
	environment = append(environment, "NO_COLOR=1", "LANE_GH_CALLS="+filepath.Join(cwd, "gh-calls"))
	return root, cwd, environment
}

func reviewLaneRun(root, cwd string, environment []string, args ...string) cliResult {
	return reportProcess(cwd, environment, filepath.Join(root, "factory"), append([]string{"review-lane"}, args...))
}

func reviewLaneLegacy(root, cwd string, environment []string, args ...string) cliResult {
	GinkgoHelper()
	for _, path := range []string{"scripts/factory-review-lane.sh", "scripts/lib/config.sh", "scripts/lib/color.sh"} {
		command := exec.Command("git", "show", "315e4f90250260ca2933ade74063cdbd532cc5ad:"+path) // #nosec G204 -- fixed local oracle revision and fixture assets.
		command.Dir = ".."
		data, err := command.Output()
		Expect(err).NotTo(HaveOccurred())
		writeFixture(filepath.Join(root, path), data, 0700)
	}
	return reportProcess(cwd, environment, "/bin/bash", append([]string{filepath.Join(root, "scripts/factory-review-lane.sh")}, args...))
}

func reviewLaneFakeGH(root string, status string) {
	GinkgoHelper()
	body := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$LANE_GH_CALLS\"\nprintf '%s' \"$LANE_GH_OUTPUT\"\nexit " + status + "\n"
	writeFixture(filepath.Join(filepath.Dir(root), "lane-bin/gh"), []byte(body), 0700)
}

var _ = Describe("Native Go review-lane core", func() {
	// per docs/adr/0089-go-native-review-lane.md:17
	It("reports default status without a legacy review-lane script", func() {
		root, cwd, environment := reviewLaneFixture()
		out := reviewLaneRun(root, cwd, environment)
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(out.stdout).To(ContainSubstring("review lane: off\n"))
		Expect(out.stdout).To(ContainSubstring("workflow:  not installed"))
	})
	// per docs/adr/0089-go-native-review-lane.md:72
	It("enables a managed workflow and records configuration without the legacy script", func() {
		root, cwd, environment := reviewLaneFixture()
		out := reviewLaneRun(root, cwd, environment, "enable", "FIXTURE_API_KEY")
		Expect(out.status).To(BeZero(), "%+v", out)
		workflow, err := os.ReadFile(filepath.Join(cwd, ".github/workflows/adversarial-review.yml"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(workflow)).To(HavePrefix(reviewLaneHeader + "\n"))
		Expect(string(workflow)).To(ContainSubstring("secret: FIXTURE_API_KEY"))
		config, err := os.ReadFile(filepath.Join(cwd, "factory.yaml"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(config)).To(ContainSubstring("review_lane: \"on\""))
	})
	// per docs/adr/0089-go-native-review-lane.md:85
	It("disables its managed workflow and records off without the legacy script", func() {
		root, cwd, environment := reviewLaneFixture()
		path := filepath.Join(cwd, ".github/workflows/adversarial-review.yml")
		writeFixture(path, []byte(reviewLaneHeader+"\nname: fixture\n"), 0644)
		out := reviewLaneRun(root, cwd, environment, "disable")
		Expect(out.status).To(BeZero(), "%+v", out)
		_, err := os.Stat(path)
		Expect(os.IsNotExist(err)).To(BeTrue())
		config, err := os.ReadFile(filepath.Join(cwd, "factory.yaml"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(config)).To(ContainSubstring("review_lane: \"off\""))
	})
})

var _ = Describe("Native Go review-lane frozen parity", func() {
	// per docs/adr/0089-go-native-review-lane.md:23
	DescribeTable("retains nested-root config routing, lexical fallback and inert caller values", func(emptyCaller bool) {
		root, cwd, environment := reviewLaneFixture()
		doctorGit(cwd, environment, "init", "-q")
		Expect(os.MkdirAll(filepath.Join(cwd, "real/child"), 0700)).To(Succeed())
		Expect(os.Symlink(filepath.Join(cwd, "real/child"), filepath.Join(cwd, "link"))).To(Succeed())
		writeFixture(filepath.Join(cwd, "real/factory.yaml"), []byte("review_lane: on\nreview_lane: off\nmodel_provider: openai\nreview_model: $(touch LANE_INJECTION)\n"), 0600)
		writeFixture(filepath.Join(cwd, "real/factory.config"), []byte("REVIEW_API_KEY_SECRET=SIBLING_KEY\ntouch LANE_INJECTION\n"), 0600)
		environment = append(environment, "FACTORY_CONFIG=link/../factory.yaml")
		if emptyCaller {
			environment = append(environment, "REVIEW_MODEL=", "REVIEW_API_KEY_SECRET=")
		}
		outside := filepath.Join(filepath.Dir(root), "readonly-workflow")
		writeFixture(outside, []byte("USER_WORKFLOW"), 0600)
		workflow := filepath.Join(cwd, ".github/workflows/adversarial-review.yml")
		Expect(os.MkdirAll(filepath.Dir(workflow), 0700)).To(Succeed())
		Expect(os.Symlink(outside, workflow)).To(Succeed())
		nested := filepath.Join(cwd, "nested")
		Expect(os.Mkdir(nested, 0700)).To(Succeed())
		expected := reviewLaneLegacy(root, nested, environment, "status")
		Expect(expected.status).To(BeZero(), "%+v", expected)
		Expect(os.Remove(filepath.Join(root, "scripts/factory-review-lane.sh"))).To(Succeed())
		before := nativeInitArtifacts(cwd)
		actual := reviewLaneRun(root, nested, environment, "status")
		Expect(actual).To(Equal(expected))
		Expect(actual.stdout).To(ContainSubstring("review lane: on"))
		Expect(actual.stdout).To(ContainSubstring("workflow:  installed"))
		if emptyCaller {
			Expect(actual.stdout).To(ContainSubstring("<frontier tier for openai>"))
			Expect(actual.stdout).To(ContainSubstring("OPENAI_API_KEY"))
		} else {
			Expect(actual.stdout).To(ContainSubstring("$(touch LANE_INJECTION)"))
			Expect(actual.stdout).To(ContainSubstring("SIBLING_KEY"))
		}
		Expect(nativeInitArtifacts(cwd)).To(Equal(before))
		data, err := os.ReadFile(outside)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(Equal("USER_WORKFLOW"))
	}, Entry("parsed literal values", false), Entry("explicit empty caller wins", true))

	// per docs/adr/0089-go-native-review-lane.md:29
	DescribeTable("retains provider frontier and secret fallbacks", func(provider, frontier string) {
		root, cwd, environment := reviewLaneFixture()
		oracleRoot, oracleCWD, oracleEnv := reviewLaneFixture()
		config := "project_name: fixture\n"
		if provider != "" {
			config += "model_provider: " + provider + "\n"
		}
		config += frontier
		for _, dir := range []string{cwd, oracleCWD} {
			writeFixture(filepath.Join(dir, "factory.yaml"), []byte(config), 0600)
		}
		expected := reviewLaneLegacy(oracleRoot, oracleCWD, oracleEnv, "enable")
		Expect(expected.status).To(BeZero(), "%+v", expected)
		actual := reviewLaneRun(root, cwd, environment, "enable")
		Expect(actual).To(Equal(expected))
		Expect(nativeInitArtifacts(cwd)).To(Equal(nativeInitArtifacts(oracleCWD)))
	}, Entry("default OpenRouter", "", ""), Entry("Anthropic", "anthropic", ""), Entry("OpenAI", "openai", ""), Entry("unknown provider default", "other", ""), Entry("configured frontier", "anthropic", "claude_frontier_model: configured-frontier\n"))

	// per docs/adr/0089-go-native-review-lane.md:38
	It("matches the secret first field case-insensitively with exactly one lookup", func() {
		root, cwd, environment := reviewLaneFixture()
		writeFixture(filepath.Join(cwd, "factory.yaml"), []byte("review_lane: on\nreview_api_key_secret: fixture_Key\n"), 0600)
		reviewLaneFakeGH(root, "0")
		environment = append(environment, "LANE_GH_OUTPUT=FIXTURE_KEY yesterday\n")
		out := reviewLaneRun(root, cwd, environment, "pending")
		Expect(out).To(Equal(cliResult{"", "", 0}))
		calls, err := os.ReadFile(filepath.Join(cwd, "gh-calls"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(calls)).To(Equal("secret list\n"))
	})

	// per docs/adr/0089-go-native-review-lane.md:19
	It("returns the legacy usage and status for an unknown command without querying GitHub", func() {
		root, cwd, environment := reviewLaneFixture()
		out := reviewLaneRun(root, cwd, environment, "--help", "ignored")
		Expect(out).To(Equal(cliResult{"", "usage: factory review-lane [status|enable|disable|pending|secret-name]\n", 2}))
		_, err := os.Stat(filepath.Join(cwd, "gh-calls"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	// per docs/adr/0089-go-native-review-lane.md:103
	DescribeTable("matches ordinary output and resulting artifacts", func(kind string) {
		root, cwd, environment := reviewLaneFixture()
		oracleRoot, oracleCWD, oracleEnv := reviewLaneFixture()
		args := []string{"status", "ignored"}
		config := "project_name: fixture\n"
		ghOutput := ""
		ghStatus := "0"
		switch kind {
		case "empty":
			args = []string{"", "ignored"}
		case "secret":
			args = []string{"secret-name", "ignored"}
			config += "model_provider: anthropic\n"
		case "enable":
			args = []string{"enable", "FIXTURE_KEY", "ignored"}
			config += "model_provider: openai\nreview_model: chosen-model\n"
		case "disable":
			args = []string{"disable", "ignored"}
			config += "review_lane: on\n"
		case "pending off":
			args = []string{"pending", "ignored"}
		case "pending set", "pending missing", "pending failed":
			args = []string{"pending", "ignored"}
			config += "review_lane: on\nreview_api_key_secret: FIXTURE_KEY\n"
			ghOutput = "OTHER_KEY yesterday\n"
			if kind == "pending set" {
				ghOutput = "FIXTURE_KEY yesterday\n"
			}
			if kind == "pending failed" {
				ghStatus = "7"
			}
		}
		for _, pair := range [][2]string{{root, cwd}, {oracleRoot, oracleCWD}} {
			writeFixture(filepath.Join(pair[1], "factory.yaml"), []byte(config), 0600)
			reviewLaneFakeGH(pair[0], ghStatus)
			if kind == "disable" {
				writeFixture(filepath.Join(pair[1], ".github/workflows/adversarial-review.yml"), []byte(reviewLaneHeader+"\nname: prior\n"), 0644)
			}
		}
		environment = append(environment, "LANE_GH_OUTPUT="+ghOutput)
		oracleEnv = append(oracleEnv, "LANE_GH_OUTPUT="+ghOutput)
		expected := reviewLaneLegacy(oracleRoot, oracleCWD, oracleEnv, args...)
		Expect(expected.status).To(BeZero(), "%+v", expected)
		actual := reviewLaneRun(root, cwd, environment, args...)
		Expect(actual).To(Equal(expected))
		Expect(nativeInitArtifacts(cwd)).To(Equal(nativeInitArtifacts(oracleCWD)))
		calls, err := os.ReadFile(filepath.Join(cwd, "gh-calls"))
		if strings.HasPrefix(kind, "pending ") && kind != "pending off" {
			Expect(err).NotTo(HaveOccurred())
			Expect(string(calls)).To(Equal("secret list\n"))
		} else {
			Expect(os.IsNotExist(err)).To(BeTrue())
		}
	}, Entry("status", "status"), Entry("empty command status", "empty"), Entry("provider secret name", "secret"), Entry("enable", "enable"), Entry("disable", "disable"), Entry("off pending silent", "pending off"), Entry("known secret", "pending set"), Entry("missing secret", "pending missing"), Entry("failed gh unverified", "pending failed"))
})
