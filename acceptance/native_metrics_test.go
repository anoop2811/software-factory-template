package acceptance_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func metricsFixture() (string, string, []string) {
	GinkgoHelper()
	root, cwd, environment := reportFixture()
	writeFixture(filepath.Join(cwd, "scripts/hooks/local.sh"), []byte("#!/bin/sh\nexit 0\n"), 0600)
	writeFixture(filepath.Join(root, "templates/metrics.html"), []byte("<!doctype html><script>const DATA = /*__FACTORY_METRICS_JSON__*/null;</script>\n"), 0600)
	bin := filepath.Join(filepath.Dir(root), "metrics-bin")
	writeFixture(filepath.Join(bin, "python3"), []byte("#!/bin/sh\nprintf called > \"$METRICS_PYTHON_MARKER\"\nexit 97\n"), 0700)
	for i, value := range environment {
		if strings.HasPrefix(value, "PATH=") {
			environment[i] = "PATH=" + bin + string(os.PathListSeparator) + strings.TrimPrefix(value, "PATH=")
		}
	}
	environment = append(environment, "NO_COLOR=1", "METRICS_PYTHON_MARKER="+filepath.Join(cwd, "PYTHON_CALLED"))
	return root, cwd, environment
}

func metricsRun(root, cwd string, environment []string, args ...string) cliResult {
	return reportProcess(cwd, environment, filepath.Join(root, "factory"), append([]string{"metrics"}, args...))
}

func metricsLegacy(root, cwd string, environment []string, args ...string) cliResult {
	GinkgoHelper()
	for _, path := range []string{"scripts/factory-metrics.sh", "scripts/lib/config.sh", "scripts/lib/color.sh"} {
		command := exec.Command("git", "show", "fb68fb43602abcc0838e5089a1da9604c022530c:"+path) // #nosec G204 -- frozen local baseline, fixed fixture assets.
		command.Dir = ".."
		data, err := command.Output()
		Expect(err).NotTo(HaveOccurred())
		writeFixture(filepath.Join(root, path), data, 0700)
	}
	// The oracle intentionally uses Python; the candidate fixture remains poisoned.
	oracleEnv := append([]string(nil), environment...)
	for i, value := range oracleEnv {
		if strings.HasPrefix(value, "PATH=") {
			oracleEnv[i] = "PATH=" + os.Getenv("PATH")
		}
	}
	return reportProcess(cwd, oracleEnv, "/bin/bash", append([]string{filepath.Join(root, "scripts/factory-metrics.sh")}, args...))
}

func metricsJSON(text string) map[string]any {
	GinkgoHelper()
	var data map[string]any
	Expect(json.Unmarshal([]byte(text), &data)).To(Succeed())
	return data
}

var _ = Describe("Native Go metrics core", func() {
	// per docs/adr/0088-go-native-metrics.md:16
	It("emits the metrics JSON schema without the legacy script or Python", func() {
		root, cwd, environment := metricsFixture()
		out := metricsRun(root, cwd, environment, "--json")
		Expect(out.status).To(BeZero(), "%+v", out)
		var data map[string]any
		Expect(json.Unmarshal([]byte(out.stdout), &data)).To(Succeed())
		Expect(data["schema"]).To(Equal("factory.metrics/v1"))
		Expect(data["window_days"]).To(Equal(float64(30)))
		_, err := os.Stat(filepath.Join(cwd, "PYTHON_CALLED"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
	// per docs/adr/0088-go-native-metrics.md:35
	It("prints the local text report without the legacy script or Python", func() {
		root, cwd, environment := metricsFixture()
		out := metricsRun(root, cwd, environment)
		Expect(out.status).To(BeZero(), "%+v", out)
		for _, text := range []string{"Factory metrics", "Enforcement", "Loop health", "Verification discipline", "Agents", "Nothing above left this machine"} {
			Expect(out.stdout).To(ContainSubstring(text))
		}
		_, err := os.Stat(filepath.Join(cwd, "PYTHON_CALLED"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
	// per docs/adr/0088-go-native-metrics.md:96
	It("publishes self-contained HTML without the legacy script or Python", func() {
		root, cwd, environment := metricsFixture()
		out := metricsRun(root, cwd, environment, "--html", "--no-open")
		Expect(out.status).To(BeZero(), "%+v", out)
		page, err := os.ReadFile(filepath.Join(cwd, ".factory/metrics.html"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(page)).To(ContainSubstring("factory.metrics/v1"))
		Expect(string(page)).NotTo(ContainSubstring("/*__FACTORY_METRICS_JSON__*/null"))
		_, err = os.Stat(filepath.Join(cwd, "PYTHON_CALLED"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
})

var _ = Describe("Native Go metrics frozen parity", func() {
	// per docs/adr/0088-go-native-metrics.md:57
	It("matches controlled Git history and Unicode verification claims from a nested cwd", func() {
		root, cwd, environment := metricsFixture()
		stamp := time.Now().UTC().AddDate(0, 0, -1).Format(time.RFC3339)
		environment = append(environment, "GIT_AUTHOR_DATE="+stamp, "GIT_COMMITTER_DATE="+stamp)
		doctorGit(cwd, environment, "init", "-q", "-b", "metrics-main")
		doctorGit(cwd, environment, "config", "user.name", "Fixture One")
		doctorGit(cwd, environment, "config", "user.email", "fixture@example.invalid")
		writeFixture(filepath.Join(cwd, "a.txt"), []byte("one"), 0600)
		doctorGit(cwd, environment, "add", "a.txt")
		doctorGit(cwd, environment, "commit", "-q", "-m", "feat: base")
		doctorGit(cwd, environment, "branch", "metrics-side")
		writeFixture(filepath.Join(cwd, "a.txt"), []byte("two"), 0600)
		doctorGit(cwd, environment, "add", "a.txt")
		doctorGit(cwd, environment, "commit", "-q", "-m", "fix: fixed fixed", "-m", "`go test` -> PASS")
		doctorGit(cwd, environment, "checkout", "-q", "metrics-side")
		writeFixture(filepath.Join(cwd, "b.txt"), []byte("other"), 0600)
		doctorGit(cwd, environment, "add", "b.txt")
		doctorGit(cwd, environment, "-c", "user.name=Fixture Two", "commit", "-q", "-m", "Revert works")
		doctorGit(cwd, environment, "checkout", "-q", "metrics-main")
		doctorGit(cwd, environment, "merge", "-q", "--no-ff", "-m", "chore: merge", "metrics-side")
		doctorGit(cwd, environment, "commit", "-q", "--allow-empty", "-m", "docs: αfixedβ worksé fixed_")
		writeFixture(filepath.Join(cwd, "chosen.yaml"), []byte("test_file_patterns: FIRST\ntest_file_patterns:\n"), 0600)
		writeFixture(filepath.Join(cwd, "chosen.events"), []byte(time.Now().UTC().Format("2006-01-02")+"\tgate\treason\n"), 0600)
		environment = append(environment, "FACTORY_CONFIG=chosen.yaml", "FACTORY_EVENT_LOG=chosen.events")
		nested := filepath.Join(cwd, "nested")
		Expect(os.Mkdir(nested, 0700)).To(Succeed())
		expected := metricsLegacy(root, nested, environment, "--json")
		Expect(expected.status).To(BeZero(), "%+v", expected)
		Expect(os.Remove(filepath.Join(root, "scripts/factory-metrics.sh"))).To(Succeed())
		actual := metricsRun(root, nested, environment, "--json")
		Expect(actual.status).To(BeZero(), "%+v", actual)
		data := metricsJSON(actual.stdout)
		Expect(data).To(Equal(metricsJSON(expected.stdout)))
		Expect(data["loop"]).To(Equal(map[string]any{"commits": float64(5), "merges": float64(1), "reverts": float64(1), "authors": float64(2), "files_touched": float64(2), "files_reworked": float64(1)}))
		Expect(data["verification"]).To(Equal(map[string]any{"claim_commits": float64(2), "cited_commits": float64(1)}))
	})

	// per docs/adr/0088-go-native-metrics.md:18
	DescribeTable("preserves option precedence and ordinary days parsing", func(args []string, jsonOutput bool) {
		root, cwd, environment := metricsFixture()
		expected := metricsLegacy(root, cwd, environment, args...)
		Expect(expected.status).To(BeZero(), "%+v", expected)
		Expect(os.Remove(filepath.Join(root, "scripts/factory-metrics.sh"))).To(Succeed())
		actual := metricsRun(root, cwd, environment, args...)
		Expect(actual.status).To(BeZero(), "%+v", actual)
		Expect(actual.stderr).To(Equal(expected.stderr))
		if jsonOutput {
			Expect(metricsJSON(actual.stdout)).To(Equal(metricsJSON(expected.stdout)))
		} else {
			Expect(actual.stdout).To(Equal(expected.stdout))
		}
	}, Entry("last format JSON", []string{"--html", "--json"}, true), Entry("last days wins", []string{"--days", "2", "--days=7", "--json"}, true), Entry("leading zeros decimal", []string{"--days=0007", "--json"}, true), Entry("invalid falls back", []string{"--days=invalid", "--json"}, true), Entry("empty falls back", []string{"--days=", "--json"}, true), Entry("option token consumed as days", []string{"--days", "--json"}, false), Entry("zero days", []string{"--days=0", "--json"}, true))

	// per docs/adr/0088-go-native-metrics.md:20
	DescribeTable("preserves argument error statuses", func(args []string, status int, diagnostic string) {
		root, cwd, environment := metricsFixture()
		out := metricsRun(root, cwd, environment, args...)
		Expect(out.status).To(Equal(status), "%+v", out)
		Expect(out.stdout).To(BeEmpty())
		if diagnostic != "" {
			Expect(out.stderr).To(ContainSubstring(diagnostic))
		}
	}, Entry("missing days operand", []string{"--days"}, 1, ""), Entry("unknown", []string{"--unknown"}, 2, "unknown argument"), Entry("no-open text", []string{"--no-open"}, 2, "--no-open applies to --html only"), Entry("no-open final JSON", []string{"--html", "--no-open", "--json"}, 2, "--no-open applies to --html only"))

	// per docs/adr/0088-go-native-metrics.md:121
	DescribeTable("matches the frozen ordinary report", func(kind string) {
		root, cwd, environment := metricsFixture()
		args := []string{"--json"}
		switch kind {
		case "empty text":
			args = nil
		case "scaffold text", "tasks text":
			path := filepath.Join(cwd, "eval/golden-tasks")
			if kind == "tasks text" {
				path = filepath.Join(path, "task-one")
			}
			Expect(os.MkdirAll(path, 0700)).To(Succeed())
			args = nil
		case "mock text":
			writeFixture(filepath.Join(cwd, "eval/results/mock-baseline.json"), []byte(`{"harness":"mock","tasks":[{"task":"mock only","score":1.0}]}`), 0600)
			args = nil
		case "events":
			cutoff := time.Now().UTC().AddDate(0, 0, -30).Format("2006-01-02")
			old := time.Now().UTC().AddDate(0, 0, -31).Format("2006-01-02")
			writeFixture(filepath.Join(cwd, ".factory/events.log"), []byte(old+"T00:00Z\told\tx\n"+cutoff+"T00:00Z\tz\tx\n"+cutoff+"T00:01Z\tz\tx\n"+cutoff+"T00:02Z\tz\tx\n"+cutoff+"T02:00Z\tb\tx\n"+cutoff+"T02:00Z\ta\tx\n"+cutoff+"T02:00Z\t\tx\n"), 0600)
		case "hooks":
			writeFixture(filepath.Join(cwd, "scripts/hooks/reporter.sh"), []byte("factory_log_event\nexit 1\n"), 0600)
			writeFixture(filepath.Join(cwd, "scripts/hooks/mute.sh"), []byte("  exit 9\n"), 0600)
			writeFixture(filepath.Join(cwd, "scripts/hooks/exempt.sh"), []byte("# factory: no-block-event\nexit 2\n"), 0600)
			writeFixture(filepath.Join(cwd, "scripts/hooks/.hidden.sh"), []byte("factory_log_event\nexit 1\n"), 0600)
			writeFixture(filepath.Join(cwd, "scripts/hooks/directory.sh/ignored.sh"), nil, 0600)
			writeFixture(filepath.Join(cwd, "factory.yaml"), []byte("test_file_patterns: literal\ncitation_prefix:\nprotected_paths: src\ncheck_command: make check\n"), 0600)
		case "eval", "eval text":
			writeFixture(filepath.Join(cwd, "eval/results/a-baseline.json"), []byte(`{"harness":"mock","inputs_fingerprint":"a","tasks":[{"task":"mock one","score":1.0}]}`), 0600)
			writeFixture(filepath.Join(cwd, "eval/results/b-baseline.json"), []byte(`{"harness":"real","inputs_fingerprint":"b","tasks":[{"task":"real one","score":0.25},{"task":"null score","score":null}]}`), 0600)
			writeFixture(filepath.Join(cwd, "eval/results/b-current.json"), []byte(`{"inputs_fingerprint":"changed","tasks":[{"task":"real one","score":0.5},{"task":"real one","score":0.75}]}`), 0600)
			Expect(os.MkdirAll(filepath.Join(cwd, "eval/golden-tasks/task-one"), 0700)).To(Succeed())
			if kind == "eval text" {
				args = nil
			}
		}
		expected := metricsLegacy(root, cwd, environment, args...)
		Expect(expected.status).To(BeZero(), "%+v", expected)
		Expect(os.Remove(filepath.Join(root, "scripts/factory-metrics.sh"))).To(Succeed())
		beforeRoot, beforeCWD := nativeInitArtifacts(root), nativeInitArtifacts(cwd)
		actual := metricsRun(root, cwd, environment, args...)
		Expect(actual.status).To(Equal(expected.status), "%+v", actual)
		Expect(actual.stderr).To(Equal(expected.stderr))
		if len(args) == 0 {
			Expect(actual.stdout).To(Equal(expected.stdout))
		} else {
			Expect(metricsJSON(actual.stdout)).To(Equal(metricsJSON(expected.stdout)))
		}
		Expect(nativeInitArtifacts(root)).To(Equal(beforeRoot))
		Expect(nativeInitArtifacts(cwd)).To(Equal(beforeCWD))
	}, Entry("empty JSON", "empty"), Entry("event windows groups and friction", "events"), Entry("gate instrumentation and configuration", "hooks"), Entry("eval mock real null duplicate and stale", "eval"), Entry("eval text score presentation", "eval text"), Entry("missing eval scaffold advice", "empty text"), Entry("missing eval task advice", "scaffold text"), Entry("missing baseline advice", "tasks text"), Entry("mock-only disclosure", "mock text"))
})
