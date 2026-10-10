package acceptance_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.yaml.in/yaml/v3"
)

const sourceGateBaselineCommit = "d60b200ac3a221520d183fca8c7738a5cf2bfdbb"

// Independently declared from the reviewed family contract, with neighboring
// descriptions that must remain in base rather than matching a substring.
// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:535
// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:543
var sourceGateCases = []struct {
	Package      string `json:"package"`
	Name         string `json:"name"`
	Installation bool   `json:"installation"`
}{
	{"./acceptance", "Go factory command acceptance Whole installation upgrade public interface serves help", true},
	{"./acceptance", "Go factory command acceptance Whole installation upgrade empty mode operands rejects empty", true},
	{"./acceptance", "Go factory command acceptance Whole installation upgrade semantic consumer recovers interruption", true},
	{"./acceptance", "Go factory command acceptance Installed runtime root separation reports events", true},
	{"./acceptance", "Go factory command acceptance Installed hook-existence library and invocation modes accepts library", true},
	{"./acceptance", "Go factory command acceptance Native Go report core renders events", false},
	{"./acceptance", "Go factory command acceptance Other suite mentions Whole installation upgrade", false},
	{"./acceptance", "Go factory command acceptance Whole installation upgrades unrelated behavior", false},
	{"./acceptance", "Go factory command acceptance Installed runtime root separational unrelated behavior", false},
	{"./acceptance", "Go factory command acceptance Installed hook-existence library and invocation modes-extra unrelated", false},
	{"./internal/native", "Native ownership collaborator acceptance Native inherited terminal job control returns shell prompt", false},
	{"./internal/installation", "Whole installation observation causes Installation post-apply descriptor identity rejects foreign inode", false},
}

// This executable evaluates the actual Make recipe's package/focus/skip operands
// against the independent case universe, records executed commands and can fail
// either partition. It does not execute product tests, builds, models or downloads.
// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:539
const sourceGateRecordingGo = `#!/usr/bin/env python3
import json,os,pathlib,re,sys
args=sys.argv[1:]
record={'args':args,'selected':[],'group':''}
def operand(name):
 for index,value in enumerate(args):
  if value.startswith(name+'='):return value.split('=',1)[1]
  if value==name:return args[index+1]
 return ''
if args and args[0]=='test':
 focus=operand('-ginkgo.focus');skip=operand('-ginkgo.skip')
 record['focus']=focus;record['skip']=skip
 record['group']='installation' if focus else 'base' if skip else 'unpartitioned'
 packages=[value for value in args[1:] if value.startswith('./')]
 with open(os.environ['SOURCE_GATE_CASES']) as source:cases=json.load(source)
 for case in cases:
  included='./...' in packages or case['package'] in packages
  if included and (not focus or re.search(focus,case['name'])) and (not skip or not re.search(skip,case['name'])):
   record['selected'].append(case['package']+' :: '+case['name'])
with open(os.environ['SOURCE_GATE_GO_LOG'],'a') as output:output.write(json.dumps(record)+'\n')
if args and args[0]=='list':print(os.getcwd())
if args and args[0]=='build' and '-o' in args:
 pathlib.Path(operand('-o')).write_bytes(b'test-owned inert build receipt\n')
if record['group'] and record['group']==os.environ.get('SOURCE_GATE_FAIL_GROUP'):
 print('fixture rejected '+record['group'],file=sys.stderr);sys.exit(43)
if '-ginkgo.fail-on-empty' in args and record['group'] and not record['selected']:
 print('fixture rejected empty selection',file=sys.stderr);sys.exit(44)
`

type sourceGateFixture struct {
	root, goLog, qualityLog string
	environment             []string
}

type sourceGateCall struct {
	Args     []string `json:"args"`
	Selected []string `json:"selected"`
	Group    string   `json:"group"`
	Focus    string   `json:"focus"`
	Skip     string   `json:"skip"`
}

func sourceGateMakeFixture() sourceGateFixture {
	GinkgoHelper()
	root, err := filepath.EvalSymlinks(GinkgoT().TempDir())
	Expect(err).NotTo(HaveOccurred())
	fixture := sourceGateFixture{root: root, goLog: filepath.Join(root, "go-calls.jsonl"), qualityLog: filepath.Join(root, "quality-calls")}
	for _, path := range []string{"Makefile", "go.mod"} {
		body, err := os.ReadFile(filepath.Join("..", path))
		Expect(err).NotTo(HaveOccurred())
		writeFixture(filepath.Join(root, path), body, 0600)
	}
	for _, path := range []string{"cmd/factory", "acceptance"} {
		Expect(os.MkdirAll(filepath.Join(root, path), 0700)).To(Succeed())
	}
	writeFixture(filepath.Join(root, "runtime/shell/readers.sh"), []byte("#!/bin/sh\n:\n"), 0600)
	writeFixture(filepath.Join(root, "runtime/shell/config.sh"), []byte("#!/bin/bash\n:\n"), 0600)
	writeFixture(filepath.Join(root, "packs/go/hooks/ginkgo-only-check.sh"), []byte("printf 'dialect\\n' >> \"$SOURCE_GATE_QUALITY_LOG\"\n"), 0600)
	writeFixture(filepath.Join(root, "tools/go"), []byte(sourceGateRecordingGo), 0700)
	for _, name := range []string{"gofmt", "golangci-lint", "gosec", "govulncheck"} {
		body := "#!/bin/sh\nprintf '%s\\n' '" + name + "' >> \"$SOURCE_GATE_QUALITY_LOG\"\n"
		writeFixture(filepath.Join(root, "tools", name), []byte(body), 0700)
	}
	cases, err := json.Marshal(sourceGateCases)
	Expect(err).NotTo(HaveOccurred())
	writeFixture(filepath.Join(root, "cases.json"), cases, 0600)
	repository, err := filepath.Abs("..")
	Expect(err).NotTo(HaveOccurred())
	git := exec.Command("git", "rev-parse", "--absolute-git-dir") // #nosec G204 -- fixed read-only query of the shared source repository.
	git.Dir = repository
	gitDirectory, err := git.Output()
	Expect(err).NotTo(HaveOccurred())
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if key != "PATH" && !strings.HasPrefix(key, "GIT_") && !strings.HasPrefix(key, "SOURCE_GATE_") && !strings.HasPrefix(key, "GO_RUNTIME_TEST_") && key != "MAKEFLAGS" && key != "MFLAGS" {
			fixture.environment = append(fixture.environment, entry)
		}
	}
	fixture.environment = append(fixture.environment,
		"PATH="+filepath.Join(root, "tools")+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GIT_DIR="+strings.TrimSpace(string(gitDirectory)), "GIT_WORK_TREE="+root,
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull,
		"SOURCE_GATE_CASES="+filepath.Join(root, "cases.json"),
		"SOURCE_GATE_GO_LOG="+fixture.goLog, "SOURCE_GATE_QUALITY_LOG="+fixture.qualityLog,
	)
	return fixture
}

func sourceGateRun(fixture sourceGateFixture, script string, extraEnvironment []string) (int, []sourceGateCall, []string) {
	GinkgoHelper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "bash", "-c", script) // #nosec G204 G702 -- reviewed source Make/CI recipe, executed only in the evaluator's inert tools fixture.
	command.Dir = fixture.root
	command.Env = append(append([]string{}, fixture.environment...), extraEnvironment...)
	output, err := command.CombinedOutput()
	Expect(ctx.Err()).NotTo(HaveOccurred(), "source scheduler fixture timed out: %s", output)
	status := 0
	if err != nil {
		var exit *exec.ExitError
		Expect(errors.As(err, &exit)).To(BeTrue(), "%v", err)
		status = exit.ExitCode()
	}
	calls := []sourceGateCall{}
	file, err := os.Open(fixture.goLog)
	if !os.IsNotExist(err) {
		Expect(err).NotTo(HaveOccurred())
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			var call sourceGateCall
			Expect(json.Unmarshal(scanner.Bytes(), &call)).To(Succeed())
			calls = append(calls, call)
		}
		Expect(scanner.Err()).NotTo(HaveOccurred())
		Expect(file.Close()).To(Succeed())
	}
	quality := []string{}
	if body, err := os.ReadFile(fixture.qualityLog); !os.IsNotExist(err) {
		Expect(err).NotTo(HaveOccurred())
		quality = strings.Split(strings.TrimSpace(string(body)), "\n")
	}
	_, _ = fmt.Fprintf(GinkgoWriter, "source scheduler script=%q; status=%d; output=%q; go=%+v; quality=%q\n", script, status, output, calls, quality)
	return status, calls, quality
}

func sourceGateTests(calls []sourceGateCall) []sourceGateCall {
	tests := []sourceGateCall{}
	for _, call := range calls {
		if len(call.Args) > 0 && call.Args[0] == "test" {
			tests = append(tests, call)
		}
	}
	return tests
}

func sourceGateExpected(installation bool) []string {
	expected := []string{}
	for _, row := range sourceGateCases {
		if row.Installation == installation {
			expected = append(expected, row.Package+" :: "+row.Name)
		}
	}
	sort.Strings(expected)
	return expected
}

// Run the actual pinned Go/Ginkgo discovery path using selectors recorded from
// the current Make recipe. Dry-run skips node execution, including BeforeSuite,
// so none of the slow installation/gate/model work runs recursively.
// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:543
func sourceGateNativeDiscovery(call sourceGateCall) (status, selected, total int) {
	GinkgoHelper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	args := []string{"test", "-race", "-count=1", "-timeout=25m", "-run=^TestAcceptance$", "./acceptance", "-ginkgo.dry-run", "-ginkgo.no-color", "-v"}
	if call.Focus != "" {
		args = append(args, "-ginkgo.focus="+call.Focus)
	}
	if call.Skip != "" {
		args = append(args, "-ginkgo.skip="+call.Skip)
	}
	for _, operand := range call.Args {
		if operand == "-ginkgo.fail-on-empty" {
			args = append(args, operand)
		}
	}
	command := exec.CommandContext(ctx, "go", args...) // #nosec G204 -- fixed real acceptance dry-run; recorded selectors remain literal argv and execute no spec bodies.
	command.Dir = ".."
	output, err := command.CombinedOutput()
	Expect(ctx.Err()).NotTo(HaveOccurred(), "native Ginkgo discovery exceeded its finite bound: %s", output)
	if err != nil {
		var exit *exec.ExitError
		Expect(errors.As(err, &exit)).To(BeTrue(), "%v", err)
		status = exit.ExitCode()
	}
	counts := regexp.MustCompile(`Ran ([0-9]+) of ([0-9]+) Specs`).FindAllStringSubmatch(string(output), -1)
	Expect(counts).To(HaveLen(1), "must observe the actual Ginkgo summary, not a recording fixture: %s", output)
	selected, err = strconv.Atoi(counts[0][1])
	Expect(err).NotTo(HaveOccurred())
	total, err = strconv.Atoi(counts[0][2])
	Expect(err).NotTo(HaveOccurred())
	_, _ = fmt.Fprintf(GinkgoWriter, "actual native discovery args=%q; status=%d; selected=%d; total=%d; output=%q\n", args, status, selected, total, output)
	return status, selected, total
}

func sourceGateAssertPartition(call sourceGateCall, installation bool) {
	GinkgoHelper()
	Expect(call.Args).To(ContainElements("-race", "-count=1", "-timeout=25m"))
	Expect(call.Selected).To(ConsistOf(sourceGateExpected(installation)))
	if installation {
		Expect(call.Args).To(ContainElement("./acceptance"))
		Expect(call.Args).NotTo(ContainElement("./..."))
		Expect(call.Focus).NotTo(BeEmpty())
		Expect(call.Skip).To(BeEmpty())
		Expect(call.Group).To(Equal("installation"))
		Expect(call.Args).To(ContainElement("-ginkgo.fail-on-empty"))
	} else {
		Expect(call.Args).To(ContainElement("./..."))
		Expect(call.Skip).NotTo(BeEmpty())
		Expect(call.Focus).To(BeEmpty())
		Expect(call.Group).To(Equal("base"))
	}
}

var _ = Describe("Go source qualification scheduling", func() {
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:535
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:530
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:543
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:549
	It("executes disjoint base and installation criteria with the same selector while retaining every source quality stage", func() {
		selectors := []string{}
		captured := []struct {
			group   string
			calls   []sourceGateCall
			quality []string
		}{}
		for _, group := range []string{"base", "installation"} {
			fixture := sourceGateMakeFixture()
			status, calls, quality := sourceGateRun(fixture, "make go-runtime-source-check GO_RUNTIME_TEST_GROUP="+group, nil)
			Expect(status).To(Equal(0))
			tests := sourceGateTests(calls)
			Expect(tests).To(HaveLen(1))
			captured = append(captured, struct {
				group   string
				calls   []sourceGateCall
				quality []string
			}{group, calls, quality})
		}
		negativeStatus, negativeCount, negativeTotal := sourceGateNativeDiscovery(sourceGateCall{Focus: "^NO_SUCH_SOURCE_QUALIFICATION_SPEC$", Args: []string{"-ginkgo.fail-on-empty"}})
		Expect(negativeStatus).NotTo(Equal(0), "real fail-on-empty must reject an actual zero selection")
		Expect(negativeCount).To(Equal(0))
		Expect(negativeTotal).To(Equal(2655))
		baseStatus, baseCount, baseTotal := sourceGateNativeDiscovery(sourceGateTests(captured[0].calls)[0])
		installationStatus, installationCount, installationTotal := sourceGateNativeDiscovery(sourceGateTests(captured[1].calls)[0])
		Expect(installationStatus).To(Equal(0))
		Expect(baseStatus).To(Equal(0))
		Expect(installationTotal).To(Equal(2655))
		Expect(baseTotal).To(Equal(installationTotal))
		Expect(installationCount).To(Equal(76), "actual pinned Ginkgo must select every installation criterion; a mock-only success is insufficient")
		Expect(baseCount).To(Equal(2579))
		Expect(baseCount + installationCount).To(Equal(baseTotal))
		for _, observed := range captured {
			group, calls, quality := observed.group, observed.calls, observed.quality
			tests := sourceGateTests(calls)
			sourceGateAssertPartition(tests[0], group == "installation")
			if group == "base" {
				selectors = append(selectors, tests[0].Skip)
			} else {
				selectors = append(selectors, tests[0].Focus)
			}
			Expect(quality).To(ContainElements("gofmt", "dialect", "golangci-lint", "gosec", "govulncheck"))
			commands := []string{}
			for _, call := range calls {
				commands = append(commands, call.Args[0])
			}
			Expect(commands).To(ContainElements("list", "vet", "build"))
		}
		Expect(selectors[0]).To(Equal(selectors[1]))
	})

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:535
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:539
	DescribeTable("attempts the complete two-partition union and propagates either partition failure", func(selection, failing string) {
		fixture := sourceGateMakeFixture()
		script := "make go-runtime-source-check"
		if selection != "default" {
			script += " GO_RUNTIME_TEST_GROUP=all"
		}
		status, calls, _ := sourceGateRun(fixture, script, []string{"SOURCE_GATE_FAIL_GROUP=" + failing})
		tests := sourceGateTests(calls)
		Expect(tests).To(HaveLen(2), "all must attempt both partitions even after the first failure")
		sourceGateAssertPartition(tests[0], false)
		sourceGateAssertPartition(tests[1], true)
		Expect(tests[0].Skip).To(Equal(tests[1].Focus))
		union := append(append([]string{}, tests[0].Selected...), tests[1].Selected...)
		Expect(union).To(ConsistOf(append(sourceGateExpected(false), sourceGateExpected(true)...)))
		if failing == "" {
			Expect(status).To(Equal(0))
		} else {
			Expect(status).NotTo(Equal(0), "a real selected runner failure must reach Make's exit status")
		}
	}, Entry("absent group defaults to the healthy complete union", "default", ""), Entry("explicit all keeps the complete union", "all", ""), Entry("base failure still attempts installation and fails", "all", "base"), Entry("installation failure fails the default union", "default", "installation"))

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:529
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:535
	DescribeTable("refuses an invalid explicit selection before any test process", func(group string) {
		fixture := sourceGateMakeFixture()
		status, calls, _ := sourceGateRun(fixture, "make go-runtime-source-check", []string{"GO_RUNTIME_TEST_GROUP=" + group})
		Expect(status).NotTo(Equal(0))
		Expect(sourceGateTests(calls)).To(BeEmpty(), "invalid group cannot become all, base or installation work")
	}, Entry("unknown group", "unrecognized"), Entry("explicit empty group", ""))
})

type sourceGateWorkflowStep struct {
	Name string            `yaml:"name"`
	Uses string            `yaml:"uses"`
	With map[string]any    `yaml:"with"`
	Run  string            `yaml:"run"`
	Env  map[string]string `yaml:"env"`
}

type sourceGateWorkflowJob struct {
	Name     string `yaml:"name"`
	RunsOn   string `yaml:"runs-on"`
	Timeout  int    `yaml:"timeout-minutes"`
	Needs    any    `yaml:"needs"`
	Strategy struct {
		FailFast *bool `yaml:"fail-fast"`
		Matrix   struct {
			OS []string `yaml:"os"`
		} `yaml:"matrix"`
	} `yaml:"strategy"`
	Steps []sourceGateWorkflowStep `yaml:"steps"`
	Env   map[string]string        `yaml:"env"`
}

var _ = Describe("Go source qualification workflow partition", func() {
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:530
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:535
	It("preserves existing OS check names and pinned setup while scheduling installation independently on every same platform", func() {
		baselineCommand := exec.Command("git", "show", sourceGateBaselineCommit+":.github/workflows/go-runtime.yml") // #nosec G204 -- fixed independently identified immutable source workflow.
		baselineCommand.Dir = ".."
		baselineBytes, err := baselineCommand.Output()
		Expect(err).NotTo(HaveOccurred())
		candidateBytes, err := os.ReadFile("../.github/workflows/go-runtime.yml")
		Expect(err).NotTo(HaveOccurred())
		var baseline, candidate struct {
			Jobs map[string]sourceGateWorkflowJob `yaml:"jobs"`
		}
		Expect(yaml.Unmarshal(baselineBytes, &baseline)).To(Succeed())
		Expect(yaml.Unmarshal(candidateBytes, &candidate)).To(Succeed())
		Expect(baseline.Jobs).To(HaveKey("go-runtime"))
		Expect(candidate.Jobs).To(HaveKey("go-runtime"), "existing required matrix check names must retain their job identifier")
		Expect(candidate.Jobs).To(HaveLen(2), "installation needs its own parallel OS matrix rather than the exhausted single job")
		original := baseline.Jobs["go-runtime"]
		for identifier, job := range candidate.Jobs {
			Expect(job.RunsOn).To(Equal(original.RunsOn))
			Expect(job.Timeout).To(Equal(30))
			Expect(job.Strategy).To(Equal(original.Strategy))
			Expect(job.Needs).To(BeNil(), "matrix partitions must be independently scheduled")
			if identifier == "go-runtime" {
				Expect(job.Name).To(Equal(original.Name), "existing display/check names cannot change")
			}
			Expect(job.Steps).To(HaveLen(len(original.Steps)))
			Expect(job.Steps[:len(job.Steps)-1]).To(Equal(original.Steps[:len(original.Steps)-1]), "checkout, Go/Python setup, interpreter qualification and every tool pin must remain unchanged")
			runner := job.Steps[len(job.Steps)-1]
			Expect(runner.Name).To(Equal(original.Steps[len(original.Steps)-1].Name))
			environment := []string{}
			for key, value := range job.Env {
				environment = append(environment, key+"="+value)
			}
			for key, value := range runner.Env {
				environment = append(environment, key+"="+value)
			}
			fixture := sourceGateMakeFixture()
			status, calls, quality := sourceGateRun(fixture, runner.Run, environment)
			Expect(status).To(Equal(0), "actual %s workflow recipe failed in the scheduler fixture", identifier)
			tests := sourceGateTests(calls)
			Expect(tests).To(HaveLen(1))
			sourceGateAssertPartition(tests[0], identifier != "go-runtime")
			Expect(quality).To(ContainElements("gofmt", "dialect", "golangci-lint", "gosec", "govulncheck"))
		}
	})
})
