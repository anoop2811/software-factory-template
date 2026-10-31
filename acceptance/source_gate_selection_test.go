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
	"github.com/onsi/ginkgo/v2/types"
	. "github.com/onsi/gomega"
	"go.yaml.in/yaml/v3"
)

const sourceGateBaselineCommit = "d60b200ac3a221520d183fca8c7738a5cf2bfdbb"

// Independently declared from the reviewed family contract, with neighboring
// descriptions that must remain in base rather than matching a substring.
// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:535
// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:543
var sourceGateCases = []struct {
	Package string `json:"package"`
	Name    string `json:"name"`
	Group   string `json:"group"`
}{
	{"./acceptance", "Go factory command acceptance Whole installation upgrade public interface serves help", "installation"},
	{"./acceptance", "Go factory command acceptance Whole installation upgrade empty mode operands rejects empty", "installation"},
	{"./acceptance", "Go factory command acceptance Whole installation upgrade semantic consumer recovers interruption", "installation"},
	{"./acceptance", "Go factory command acceptance Installed runtime root separation reports events", "installation"},
	{"./acceptance", "Go factory command acceptance Installed hook-existence library and invocation modes accepts library", "installation"},
	{"./acceptance", "Go factory command acceptance Whole installation upgrade release candidate handoff initializes a real selected Go installation", "release-handoff"},
	{"./acceptance", "Go factory command acceptance Whole installation upgrade release candidate handoff recovers interrupted selection", "release-handoff"},
	{"./acceptance", "Go factory command acceptance Supported predecessor recipe compatibility reconstructs predecessor", "historical-recipe"},
	{"./acceptance", "Go factory command acceptance Supported predecessor recipe compatibility", "historical-recipe"},
	{"./acceptance", "Go factory command acceptance Supported predecessor recipe compatibility-extra unrelated", "base"},
	{"./acceptance", "Go factory command acceptance Other suite mentions Supported predecessor recipe compatibility", "base"},
	{"./acceptance", "Go factory command acceptance Qualified obsolete initializer cache retirement restores qualified originals", "historical-recipe"},
	{"./acceptance", "Go factory command acceptance Qualified obsolete initializer cache retirement", "historical-recipe"},
	{"./acceptance", "Go factory command acceptance Qualified obsolete initializer cache retirement-extra unrelated", "base"},
	{"./acceptance", "Go factory command acceptance Other suite mentions Qualified obsolete initializer cache retirement", "base"},
	{"./acceptance", "Go factory command acceptance Go explicit local source initialization installs selected source", "local-init"},
	{"./acceptance", "Go factory command acceptance Go explicit local source initialization", "local-init"},
	{"./acceptance", "Go factory command acceptance Go explicit local source initialization-extra unrelated", "base"},
	{"./acceptance", "Go factory command acceptance Other suite mentions Go explicit local source initialization", "base"},
	{"./acceptance", "Go factory command acceptance Go explicit local source upgrade upgrades selected source", "local-upgrade"},
	{"./acceptance", "Go factory command acceptance Go explicit local source upgrade", "local-upgrade"},
	{"./acceptance", "Go factory command acceptance Go explicit local source upgrade recovers from a durably interrupted selected worker with retained exact images and no compiler forward", "local-recovery"},
	{"./acceptance", "Go factory command acceptance Go explicit local source upgrade recovers from a durably interrupted selected worker with retained exact images and no compiler reverse", "local-recovery"},
	{"./acceptance", "Go factory command acceptance Go explicit local source upgrade recovers from a durably interrupted selected worker with retained exact images and no compiler", "local-recovery"},
	{"./acceptance", "Go factory command acceptance Go explicit local source upgrade recovers from a durably interrupted selected worker with retained exact images and no compiler-extra unrelated", "local-upgrade"},
	{"./acceptance", "Go factory command acceptance Other suite mentions Go explicit local source upgrade recovers from a durably interrupted selected worker with retained exact images and no compiler", "base"},
	{"./acceptance", "Go factory command acceptance Go explicit local source upgrade-extra unrelated", "base"},
	{"./acceptance", "Go factory command acceptance Other suite mentions Go explicit local source upgrade", "base"},
	{"./acceptance", "Go factory command acceptance Go default packaged initialization authenticates exact selected initializer", "default-init"},
	{"./acceptance", "Go factory command acceptance Go default packaged initialization", "default-init"},
	{"./acceptance", "Go factory command acceptance Go default packaged initialization-extra unrelated", "base"},
	{"./acceptance", "Go factory command acceptance Other suite mentions Go default packaged initialization", "base"},
	{"./acceptance", "Go factory command acceptance Go default packaged upgrade authenticates exact selected planner", "default-upgrade"},
	{"./acceptance", "Go factory command acceptance Go default packaged upgrade", "default-upgrade"},
	{"./acceptance", "Go factory command acceptance Go default packaged upgrade-extra unrelated", "base"},
	{"./acceptance", "Go factory command acceptance Other suite mentions Go default packaged upgrade", "base"},
	{"./acceptance", "Go factory command acceptance Native Go report core renders events", "base"},
	{"./acceptance", "Go factory command acceptance Other suite mentions Whole installation upgrade release candidate handoff", "base"},
	{"./acceptance", "Go factory command acceptance Whole installation upgrades unrelated behavior", "base"},
	{"./acceptance", "Go factory command acceptance Installed runtime root separational unrelated behavior", "base"},
	{"./acceptance", "Go factory command acceptance Installed hook-existence library and invocation modes-extra unrelated", "base"},
	{"./internal/native", "Native ownership collaborator acceptance Native inherited terminal job control returns shell prompt", "base"},
	{"./internal/installation", "Whole installation observation causes Installation post-apply descriptor identity rejects foreign inode", "base"},
	{"./internal/acceptance", "Internal acceptance-named collaborator still requires qualification", "base"},
}

// This executable evaluates the actual Make recipe's package/focus/skip operands
// against the independent case universe, records executed commands and can fail
// either partition. It does not execute product tests, builds, models or downloads.
// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:539
const sourceGateRecordingGo = `#!/usr/bin/env python3
import json,os,pathlib,re,subprocess,sys
args=sys.argv[1:]
record={'args':args,'selected':[],'group':''}
module='github.com/anoop2811/software-factory-template'
def operand(*names):
 for index,value in enumerate(args):
  for name in names:
   if value.startswith(name+'='):return value.split('=',1)[1]
   if value==name:return args[index+1]
 return ''
with open(os.environ['SOURCE_GATE_CASES']) as source:cases=json.load(source)
is_runner=bool(args and args[0]=='run' and 'github.com/onsi/ginkgo/v2/ginkgo' in args)
is_helper=bool(args and args[0]=='run' and './internal/qualification/cmd/worker-count' in args)
if is_helper:
 record['group']='worker-plan'
 record['requested']=os.environ.get('GO_RUNTIME_TEST_PROCS','')
 record['maximum']=os.environ.get('GO_RUNTIME_TEST_MAX_PROCS','')
 helper=subprocess.run([os.environ['SOURCE_GATE_WORKER_HELPER']],capture_output=True,check=False)
 record['plan']=helper.stdout.decode('utf-8')

if args and (args[0]=='test' or is_runner):
 focus=operand('-ginkgo.focus','--focus');skip=operand('-ginkgo.skip','--skip')
 record['focus']=focus;record['skip']=skip
 packages=[value if value.startswith('./') else './'+value[len(module)+1:] for value in args[1:] if value.startswith('./') or value.startswith(module+'/')]
 record['group']='installation' if focus else 'base' if skip else 'unpartitioned'
 selected_groups=set()
 for case in cases:
  included='./...' in packages or case['package'] in packages
  if included and (not focus or re.search(focus,case['name'])) and (not skip or not re.search(skip,case['name'])):
   record['selected'].append(case['package']+' :: '+case['name'])
   selected_groups.add(case['group'])
 if './...' not in packages and './acceptance' not in packages:record['group']='internal'
 if focus and selected_groups in [{'release-handoff'},{'historical-recipe'},{'local-init'},{'local-upgrade'},{'local-recovery'},{'default-init'},{'default-upgrade'}]:record['group']=next(iter(selected_groups))
with open(os.environ['SOURCE_GATE_GO_LOG'],'a') as output:output.write(json.dumps(record)+'\n')
if is_helper:
 sys.stdout.buffer.write(helper.stdout);sys.stderr.buffer.write(helper.stderr);sys.exit(helper.returncode)
if 'install' in args:
 package=args[-1].split('@',1)[0];name=package.rsplit('/',1)[-1]
 target=pathlib.Path(os.environ['GOBIN'])/name;target.parent.mkdir(parents=True,exist_ok=True)
 target.write_text("#!/bin/sh\nprintf '%s\\n' 'cached:"+name+"' >> \"$SOURCE_GATE_QUALITY_LOG\"\n");target.chmod(0o700)
if args and args[0]=='list':
 if operand('-f')=='{{.Dir}}':print(os.getcwd())
 else:
  for package in sorted({case['package'] for case in cases}):print(module+'/'+package[2:])
if args and args[0]=='build' and '-o' in args:
 pathlib.Path(operand('-o')).write_bytes(b'test-owned inert build receipt\n')
if is_runner:
 for name in ['--json-report','--junit-report']:
  path=operand(name)
  if path:
   target=pathlib.Path(path);target.parent.mkdir(parents=True,exist_ok=True);target.write_text('test-owned report receipt\n')
if record['group'] and record['group']==os.environ.get('SOURCE_GATE_FAIL_GROUP'):
 print('fixture rejected '+record['group'],file=sys.stderr);sys.exit(43)
if ('-ginkgo.fail-on-empty' in args or '--fail-on-empty' in args) and record['group'] and not record['selected']:
 print('fixture rejected empty selection',file=sys.stderr);sys.exit(44)
`

type sourceGateFixture struct {
	root, goLog, qualityLog string
	environment             []string
}

type sourceGateCall struct {
	Args      []string `json:"args"`
	Selected  []string `json:"selected"`
	Group     string   `json:"group"`
	Focus     string   `json:"focus"`
	Skip      string   `json:"skip"`
	Requested string   `json:"requested"`
	Maximum   string   `json:"maximum"`
	Plan      string   `json:"plan"`
}

var sourceGateWorkerHelperPath string

// per docs/adr/0104-adaptive-qualification-workers.md:18
func sourceGateBuildWorkerHelper() string {
	GinkgoHelper()
	if _, err := os.Stat(sourceGateWorkerHelperPath); os.IsNotExist(err) {
		build := exec.Command("go", "build", "-o", sourceGateWorkerHelperPath, "./internal/qualification/cmd/worker-count") // #nosec G204 -- fixed private package and suite-owned output path.
		build.Dir = ".."
		output, err := build.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "private worker helper setup: %s", output)
	}
	return sourceGateWorkerHelperPath
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
	// per docs/adr/0099-native-factory-orchestration.md:149
	writeFixture(filepath.Join(root, "runtime/shell/helpers.sh"), []byte("#!/bin/sh\n:\n"), 0600)
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
		if key != "PATH" && !strings.HasPrefix(key, "GIT_") && !strings.HasPrefix(key, "SOURCE_GATE_") && !strings.HasPrefix(key, "GO_RUNTIME_TEST_") && key != "GO_RUNTIME_REPORT_DIR" && key != "MAKEFLAGS" && key != "MFLAGS" {
			fixture.environment = append(fixture.environment, entry)
		}
	}
	fixture.environment = append(fixture.environment,
		"PATH="+filepath.Join(root, "tools")+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GIT_DIR="+strings.TrimSpace(string(gitDirectory)), "GIT_WORK_TREE="+root,
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull,
		"SOURCE_GATE_CASES="+filepath.Join(root, "cases.json"),
		"SOURCE_GATE_WORKER_HELPER="+sourceGateBuildWorkerHelper(),
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
	overridden := map[string]bool{}
	for _, entry := range extraEnvironment {
		key, _, _ := strings.Cut(entry, "=")
		overridden[key] = true
	}
	for _, entry := range fixture.environment {
		key, _, _ := strings.Cut(entry, "=")
		if !overridden[key] {
			command.Env = append(command.Env, entry)
		}
	}
	command.Env = append(command.Env, extraEnvironment...)
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
		if len(call.Args) > 0 && (call.Args[0] == "test" || call.Args[0] == "run") && call.Group != "internal" && call.Group != "worker-plan" {
			tests = append(tests, call)
		}
	}
	return tests
}

func sourceGateExpected(group string) []string {
	expected := []string{}
	for _, row := range sourceGateCases {
		if row.Group == group && row.Package == "./acceptance" {
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
func sourceGateNativeDiscovery(call sourceGateCall) (status, selected, total int, identities []string) {
	GinkgoHelper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	reportPath := filepath.Join(GinkgoT().TempDir(), "ginkgo-discovery.json")
	args := []string{"test", "-race", "-count=1", "-timeout=25m", "-run=^TestAcceptance$", "./acceptance", "-ginkgo.dry-run", "-ginkgo.no-color", "-ginkgo.json-report=" + reportPath, "-v"}
	if call.Focus != "" {
		args = append(args, "-ginkgo.focus="+call.Focus)
	}
	if call.Skip != "" {
		args = append(args, "-ginkgo.skip="+call.Skip)
	}
	for _, operand := range call.Args {
		if operand == "-ginkgo.fail-on-empty" || operand == "--fail-on-empty" {
			args = append(args, "-ginkgo.fail-on-empty")
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
	if selected > 0 {
		body, err := os.ReadFile(reportPath)
		Expect(err).NotTo(HaveOccurred())
		var reports []types.Report
		Expect(json.Unmarshal(body, &reports)).To(Succeed())
		Expect(reports).To(HaveLen(1))
		for _, spec := range reports[0].SpecReports {
			if spec.LeafNodeType == types.NodeTypeIt && spec.State.Is(types.SpecStatePassed) {
				identities = append(identities, fmt.Sprintf("%s %s :: %s:%d", reports[0].SuiteDescription, spec.FullText(), filepath.Base(spec.LeafNodeLocation.FileName), spec.LeafNodeLocation.LineNumber))
			}
		}
		Expect(identities).To(HaveLen(selected), "actual report identities must agree with native discovery's summary")
	}
	_, _ = fmt.Fprintf(GinkgoWriter, "actual native discovery args=%q; status=%d; selected=%d; total=%d; output=%q\n", args, status, selected, total, output)
	return status, selected, total, identities
}

func sourceGateOperand(args []string, name string) string {
	for index, argument := range args {
		if value, ok := strings.CutPrefix(argument, name+"="); ok {
			return value
		}
		if argument == name && index+1 < len(args) {
			return args[index+1]
		}
	}
	return ""
}

// per docs/adr/0102-parallel-go-qualification.md:28
func sourceGateAssertInternal(calls []sourceGateCall, required bool) {
	GinkgoHelper()
	internal := []sourceGateCall{}
	for _, call := range calls {
		if call.Group == "internal" {
			internal = append(internal, call)
		}
	}
	if !required {
		Expect(internal).To(BeEmpty())
		return
	}
	Expect(internal).To(HaveLen(1), "base must retain every non-acceptance package in a separate fresh race run")
	Expect(internal[0].Args).To(ContainElements("test", "-race", "-count=1", "-timeout=25m", "-ginkgo.fail-on-empty"))
	expected := []string{}
	for _, row := range sourceGateCases {
		if row.Package != "./acceptance" {
			expected = append(expected, row.Package+" :: "+row.Name)
		}
	}
	Expect(internal[0].Selected).To(ConsistOf(expected), "exclude only the exact root acceptance package, including no omission of internal/acceptance")
	Expect(internal[0].Args).NotTo(ContainElements("./...", "./acceptance", "github.com/anoop2811/software-factory-template/acceptance"))
}

// per docs/adr/0104-adaptive-qualification-workers.md:36
func sourceGateAssertWorkerPlan(calls []sourceGateCall, requested, maximum string) {
	GinkgoHelper()
	plans := []sourceGateCall{}
	for _, call := range calls {
		if call.Group == "worker-plan" {
			plans = append(plans, call)
		}
	}
	Expect(plans).To(HaveLen(1), "one early helper probe selects both worker counts")
	Expect(plans[0].Args).To(Equal([]string{"run", "-mod=readonly", "./internal/qualification/cmd/worker-count"}))
	Expect(plans[0].Requested).To(Equal(requested), "worker settings must cross as literal environment data")
	Expect(plans[0].Maximum).To(Equal(maximum))
	fields := strings.Fields(plans[0].Plan)
	Expect(fields).To(HaveLen(2), "record the actual compiled helper output before Make consumes it")
	base, err := strconv.Atoi(fields[0])
	Expect(err).NotTo(HaveOccurred())
	handoff, err := strconv.Atoi(fields[1])
	Expect(err).NotTo(HaveOccurred())
	Expect(plans[0].Plan).To(Equal(fmt.Sprintf("%d %d\n", base, handoff)), "the helper protocol is one canonical line")
	limit, err := strconv.Atoi(maximum)
	Expect(err).NotTo(HaveOccurred())
	Expect(base).To(BeNumerically(">=", 1))
	Expect(base).To(BeNumerically("<=", limit))
	Expect(handoff).To(Equal(min(base, 2)))
	if requested != "auto" {
		workers, err := strconv.Atoi(requested)
		Expect(err).NotTo(HaveOccurred())
		Expect(base).To(Equal(workers))
	}
	for _, call := range sourceGateTests(calls) {
		expected := base
		if call.Group == "installation" {
			continue
		}
		if call.Group == "release-handoff" || call.Group == "historical-recipe" || call.Group == "local-init" || call.Group == "local-upgrade" || call.Group == "local-recovery" || call.Group == "default-init" || call.Group == "default-upgrade" {
			expected = handoff
		}
		Expect(sourceGateOperand(call.Args, "--procs")).To(Equal(strconv.Itoa(expected)), "%s dispatch must use the exact real helper plan %q", call.Group, plans[0].Plan)
	}
}

func sourceGateAssertPartition(call sourceGateCall, group string) {
	GinkgoHelper()
	Expect(call.Selected).To(ConsistOf(sourceGateExpected(group)))
	Expect(call.Group).To(Equal(group))
	Expect(call.Args).To(ContainElement("./acceptance"))
	Expect(call.Args).NotTo(ContainElement("./..."))
	if group == "installation" {
		Expect(call.Args[0]).To(Equal("test"))
		Expect(call.Args).To(ContainElements("-race", "-count=1", "-timeout=25m", "-ginkgo.fail-on-empty"))
		Expect(call.Skip).NotTo(BeEmpty(), "the shared installation family must exclude its separately qualified handoff criteria")
	} else {
		// per docs/adr/0102-parallel-go-qualification.md:23
		// per docs/adr/0102-parallel-go-qualification.md:40
		Expect(len(call.Args)).To(BeNumerically(">=", 3))
		Expect(call.Args[:3]).To(Equal([]string{"run", "-mod=readonly", "github.com/onsi/ginkgo/v2/ginkgo"}))
		Expect(call.Args).To(ContainElements("--race", "--timeout=25m", "--fail-on-empty", "--keep-going", "--no-color", "--", "-test.count=1", "-test.timeout=25m"))
		workers, err := strconv.Atoi(sourceGateOperand(call.Args, "--procs"))
		Expect(err).NotTo(HaveOccurred())
		Expect(workers).To(BeNumerically(">=", 1))
		workerLimit := 32
		if group == "release-handoff" || group == "historical-recipe" || group == "local-init" || group == "local-upgrade" || group == "local-recovery" || group == "default-init" || group == "default-upgrade" {
			workerLimit = 2
		}
		Expect(workers).To(BeNumerically("<=", workerLimit))
	}
	if group == "base" {
		Expect(call.Skip).NotTo(BeEmpty())
		Expect(call.Focus).To(BeEmpty())
	} else {
		Expect(call.Focus).NotTo(BeEmpty())
		switch group {
		case "local-upgrade":
			Expect(call.Skip).NotTo(BeEmpty(), "ordinary local upgrades must not co-select the dedicated durable recovery family")
		case "release-handoff", "historical-recipe", "local-init", "local-recovery", "default-init", "default-upgrade":
			Expect(call.Skip).To(BeEmpty())
		}
	}
}

var _ = Describe("Go source qualification scheduling", func() {
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:535
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:530
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:543
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:549
	// per docs/adr/0099-native-factory-orchestration.md:155
	// per docs/adr/0101-go-release-handoff.md:560
	// per docs/adr/0121-local-upgrade-readiness-boundary.md:117
	// per docs/adr/0122-default-packaged-go-upgrade.md:180
	It("executes nine disjoint base installation release-handoff historical-recipe local-init local-upgrade local-recovery default-init and default-upgrade groups while retaining every source quality stage", func() {
		callsByGroup := map[string]sourceGateCall{}
		for _, group := range []string{"base", "installation", "release-handoff", "historical-recipe", "local-init", "local-upgrade", "local-recovery", "default-init", "default-upgrade"} {
			fixture := sourceGateMakeFixture()
			status, calls, quality := sourceGateRun(fixture, "make go-runtime-source-check GO_RUNTIME_TEST_GROUP="+group, nil)
			Expect(status).To(Equal(0))
			tests := sourceGateTests(calls)
			Expect(tests).To(HaveLen(1))
			// per docs/adr/0102-parallel-go-qualification.md:23
			// per docs/adr/0102-parallel-go-qualification.md:32
			if group != "installation" {
				Expect(tests[0].Args[0]).To(Equal("run"), "base and handoff must use the pinned Ginkgo CLI rather than serial package execution")
			}
			sourceGateAssertPartition(tests[0], group)
			sourceGateAssertInternal(calls, group == "base")
			sourceGateAssertWorkerPlan(calls, "auto", "4")
			callsByGroup[group] = tests[0]
			if group != "installation" {
				workers, err := strconv.Atoi(sourceGateOperand(tests[0].Args, "--procs"))
				Expect(err).NotTo(HaveOccurred())
				Expect(workers).To(BeNumerically("<=", 4), "automatic default cannot exceed its configured default cap")
				Expect(sourceGateOperand(tests[0].Args, "--json-report")).To(BeEmpty(), "reports remain an explicit local option")
			}
			Expect(quality).To(ContainElements("gofmt", "dialect", "golangci-lint", "gosec", "govulncheck"))
			commands := []string{}
			for _, call := range calls {
				commands = append(commands, call.Args[0])
			}
			Expect(commands).To(ContainElements("list", "vet", "build"))
		}
		// per docs/adr/0121-local-upgrade-readiness-boundary.md:134
		for _, negativeGroup := range []string{"historical-recipe", "local-recovery", "default-upgrade"} {
			for _, empty := range []bool{false, true} {
				fixture := sourceGateMakeFixture()
				environment := []string{"SOURCE_GATE_FAIL_GROUP=" + negativeGroup}
				if empty {
					environment = nil
					cases := []any{}
					for _, row := range sourceGateCases {
						if row.Group != negativeGroup {
							cases = append(cases, row)
						}
					}
					data, err := json.Marshal(cases)
					Expect(err).NotTo(HaveOccurred())
					writeFixture(filepath.Join(fixture.root, "cases.json"), data, 0600)
				}
				status, calls, _ := sourceGateRun(fixture, "make go-runtime-source-check GO_RUNTIME_TEST_GROUP="+negativeGroup, environment)
				Expect(status).NotTo(Equal(0), "explicit selected heavy group must propagate both runner failure and empty selection")
				tests := sourceGateTests(calls)
				Expect(tests).To(HaveLen(1), "refusal must follow the selected runner rather than an unrecognized group")
				Expect(tests[0].Args).To(ContainElement("--fail-on-empty"))
				if empty {
					Expect(tests[0].Selected).To(BeEmpty())
				} else {
					sourceGateAssertPartition(tests[0], negativeGroup)
				}
			}
		}
		// per docs/adr/0102-parallel-go-qualification.md:29
		// per docs/adr/0102-parallel-go-qualification.md:44
		// per docs/adr/0102-parallel-go-qualification.md:139
		for _, workers := range []string{"1", "2", "3"} {
			fixture := sourceGateMakeFixture()
			reports := filepath.Join(fixture.root, `reports "quoted" with spaces`)
			status, calls, _ := sourceGateRun(fixture, "make go-runtime-source-check", []string{"GO_RUNTIME_TEST_PROCS=" + workers, "GO_RUNTIME_REPORT_DIR=" + reports})
			Expect(status).To(Equal(0))
			sourceGateAssertInternal(calls, true)
			sourceGateAssertWorkerPlan(calls, workers, "4")
			for _, call := range sourceGateTests(calls) {
				sourceGateAssertPartition(call, call.Group)
				if call.Group == "installation" {
					continue
				}
				expected := workers
				if (call.Group == "release-handoff" || call.Group == "historical-recipe" || call.Group == "local-init" || call.Group == "local-upgrade" || call.Group == "local-recovery" || call.Group == "default-init" || call.Group == "default-upgrade") && workers == "3" {
					expected = "2"
				}
				Expect(sourceGateOperand(call.Args, "--procs")).To(Equal(expected))
				for _, report := range []struct{ flag, suffix string }{{"--json-report", ".json"}, {"--junit-report", ".xml"}} {
					path := filepath.Join(reports, call.Group+report.suffix)
					Expect(sourceGateOperand(call.Args, report.flag)).To(Equal(path), "each group receives an independent literal report path")
					Expect(os.ReadFile(path)).To(Equal([]byte("test-owned report receipt\n")))
				}
			}
		}
		negativeStatus, negativeCount, negativeTotal, _ := sourceGateNativeDiscovery(sourceGateCall{Focus: "^NO_SUCH_SOURCE_QUALIFICATION_SPEC$", Args: []string{"-ginkgo.fail-on-empty"}})
		Expect(negativeStatus).NotTo(Equal(0), "real fail-on-empty must reject an actual zero selection")
		Expect(negativeCount).To(Equal(0))
		// Forty-nine hosted large-review criteria remain in the base partition.
		// Eleven ordinary local-upgrade, two durable recovery, and ten default-init criteria are independently discovered below.
		// per docs/adr/0118-default-packaged-go-initialization.md:76
		// per docs/adr/0117-explicit-go-local-upgrade.md:94
		// per docs/adr/0109-complete-large-review-inputs.md:72
		// Fourteen default-upgrade criteria gain their own independent family.
		// per docs/adr/0122-default-packaged-go-upgrade.md:180
		// Twenty-six source proof criteria replace two obsolete mapped-script
		// entries and stay in base. per docs/adr/0120-native-source-gate-proof.md:101
		// Seven owned-cache retirement criteria join the measured historical lane.
		// per docs/adr/0123-owned-obsolete-initializer-cache-retirement.md:140
		Expect(negativeTotal).To(Equal(3186))
		allStatus, allCount, allTotal, allIdentities := sourceGateNativeDiscovery(sourceGateCall{Args: []string{"-ginkgo.fail-on-empty"}})
		Expect(allStatus).To(Equal(0))
		Expect(allCount).To(Equal(allTotal))
		Expect(allTotal).To(Equal(3186))
		expectedCounts := map[string]int{"base": 2978, "installation": 98, "release-handoff": 44, "historical-recipe": 19, "local-init": 10, "local-upgrade": 11, "local-recovery": 2, "default-init": 10, "default-upgrade": 14}
		seen, union := map[string]bool{}, []string{}
		for _, group := range []string{"base", "installation", "release-handoff", "historical-recipe", "local-init", "local-upgrade", "local-recovery", "default-init", "default-upgrade"} {
			status, count, total, identities := sourceGateNativeDiscovery(callsByGroup[group])
			Expect(status).To(Equal(0), group)
			Expect(count).To(Equal(expectedCounts[group]), "actual pinned Ginkgo census for %s", group)
			Expect(total).To(Equal(allTotal))
			for _, identity := range identities {
				_, duplicate := seen[identity]
				Expect(duplicate).To(BeFalse(), "actual criterion appeared in two partitions: %s", identity)
				seen[identity] = true
			}
			union = append(union, identities...)
		}
		sort.Strings(union)
		sort.Strings(allIdentities)
		Expect(union).To(Equal(allIdentities), "nine real discovered identity sets must be disjoint and exhaustive")
	})

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:535
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:539
	// per docs/adr/0101-go-release-handoff.md:560
	DescribeTable("attempts the complete nine-partition union and propagates every partition failure", func(selection, failing string) {
		failures := []string{failing}
		if selection == "all" && failing == "" {
			failures = append(failures, "historical-recipe", "local-init", "local-upgrade", "local-recovery", "default-init", "default-upgrade")
		}
		if selection == "default" && failing == "" {
			failures = append(failures, "historical-recipe", "local-recovery", "default-upgrade")
		}
		for _, failing := range failures {
			fixture := sourceGateMakeFixture()
			script := "make go-runtime-source-check"
			if selection != "default" {
				script += " GO_RUNTIME_TEST_GROUP=all"
			}
			status, calls, _ := sourceGateRun(fixture, script, []string{"SOURCE_GATE_FAIL_GROUP=" + failing})
			sourceGateAssertWorkerPlan(calls, "auto", "4")
			tests := sourceGateTests(calls)
			Expect(tests).To(HaveLen(9), "all must attempt all nine partitions even after any earlier failure")
			sourceGateAssertInternal(calls, true)
			seen, union, expected := map[string]bool{}, []string{}, []string{}
			for _, call := range tests {
				Expect(seen).NotTo(HaveKey(call.Group))
				seen[call.Group] = true
				sourceGateAssertPartition(call, call.Group)
				union = append(union, call.Selected...)
				expected = append(expected, sourceGateExpected(call.Group)...)
			}
			Expect(seen).To(HaveKey("base"))
			Expect(seen).To(HaveKey("installation"))
			Expect(seen).To(HaveKey("release-handoff"))
			Expect(seen).To(HaveKey("historical-recipe"))
			Expect(seen).To(HaveKey("local-init"))
			Expect(seen).To(HaveKey("local-upgrade"))
			Expect(seen).To(HaveKey("local-recovery"))
			Expect(seen).To(HaveKey("default-init"))
			Expect(seen).To(HaveKey("default-upgrade"))
			Expect(union).To(ConsistOf(expected))
			if failing == "" {
				Expect(status).To(Equal(0))
			} else {
				Expect(status).NotTo(Equal(0), "a real selected runner failure must reach Make's exit status")
			}
		}
	}, Entry("absent group defaults to the healthy complete union", "default", ""), Entry("explicit all keeps the complete union", "all", ""), Entry("base failure still attempts installation and fails", "all", "base"), Entry("internal package failure still attempts every acceptance partition", "all", "internal"), Entry("installation failure fails the default union", "default", "installation"), Entry("release-handoff failure fails the complete union", "all", "release-handoff"))

	// per docs/adr/0102-parallel-go-qualification.md:29
	// per docs/adr/0104-adaptive-qualification-workers.md:36
	It("refuses invalid worker selection before any qualification work", func() {
		for _, setting := range []string{"GO_RUNTIME_TEST_PROCS", "GO_RUNTIME_TEST_MAX_PROCS"} {
			for _, value := range []string{"", "0", "33", "-1", "01", " 2", "1; exit 0", "$(shell touch WITNESS)"} {
				for _, argument := range []bool{false, true} {
					// GNU Make parses leading assignment whitespace before export; the environment path tests it literally.
					if argument && strings.HasPrefix(value, " ") {
						continue
					}
					fixture := sourceGateMakeFixture()
					witness := filepath.Join(fixture.root, "worker-setting-was-executed")
					literal := strings.ReplaceAll(value, "WITNESS", witness)
					script, environment := "make go-runtime-source-check", []string{setting + "=" + literal}
					if argument {
						// The shell expands only a quoted environment operand; Make receives literal argv.
						script = "make go-runtime-source-check \"$SOURCE_GATE_LITERAL_SETTING\""
						environment = []string{"SOURCE_GATE_LITERAL_SETTING=" + setting + "=" + literal}
					}
					status, calls, quality := sourceGateRun(fixture, script, environment)
					Expect(status).NotTo(Equal(0), "invalid setting %s=%q must be refused", setting, literal)
					Expect(calls).To(HaveLen(1), "only the real private helper may execute while validating malformed settings")
					Expect(calls[0].Group).To(Equal("worker-plan"))
					Expect(calls[0].Args).To(Equal([]string{"run", "-mod=readonly", "./internal/qualification/cmd/worker-count"}), "literal values cross the environment, never executable shell operands")
					Expect(quality).To(BeEmpty())
					_, err := os.Lstat(witness)
					Expect(os.IsNotExist(err)).To(BeTrue(), "Make must not expand $(shell ...) before the helper refuses it")
				}
			}
		}
	})

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:529
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:535
	DescribeTable("refuses an invalid explicit selection before any test process", func(group string) {
		fixture := sourceGateMakeFixture()
		status, calls, _ := sourceGateRun(fixture, "make go-runtime-source-check", []string{"GO_RUNTIME_TEST_GROUP=" + group})
		Expect(status).NotTo(Equal(0))
		Expect(calls).To(BeEmpty(), "invalid group must be refused before invoking even the Go worker-plan helper")
		Expect(sourceGateTests(calls)).To(BeEmpty(), "invalid group cannot become any scheduled qualification work")
	}, Entry("unknown group", "unrecognized"), Entry("explicit empty group", ""))
})

type sourceGateWorkflowStep struct {
	ID   string            `yaml:"id"`
	If   string            `yaml:"if"`
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
			OS      []string         `yaml:"os"`
			Group   []string         `yaml:"group"`
			Include []map[string]any `yaml:"include"`
			Exclude []map[string]any `yaml:"exclude"`
		} `yaml:"matrix"`
	} `yaml:"strategy"`
	Steps []sourceGateWorkflowStep `yaml:"steps"`
	Env   map[string]string        `yaml:"env"`
}

var _ = Describe("Go source qualification workflow partition", func() {
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:530
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:535
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:562
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:587
	// per docs/adr/0101-go-release-handoff.md:560
	// per docs/DECISION_LOG.md:4529
	It("preserves OS check names and pinned setup while scheduling all nine groups independently on both platforms", func() {
		baselineCommand := exec.Command("git", "show", sourceGateBaselineCommit+":.github/workflows/go-runtime.yml") // #nosec G204 -- fixed independently identified immutable source workflow.
		baselineCommand.Dir = ".."
		baselineBytes, err := baselineCommand.Output()
		Expect(err).NotTo(HaveOccurred())
		candidateBytes, err := os.ReadFile("../.github/workflows/go-runtime.yml")
		Expect(err).NotTo(HaveOccurred())
		var baseline, candidate struct {
			Jobs        map[string]sourceGateWorkflowJob `yaml:"jobs"`
			Concurrency struct {
				Group  string `yaml:"group"`
				Cancel string `yaml:"cancel-in-progress"`
			} `yaml:"concurrency"`
		}
		Expect(yaml.Unmarshal(baselineBytes, &baseline)).To(Succeed())
		Expect(yaml.Unmarshal(candidateBytes, &candidate)).To(Succeed())
		// per docs/adr/0101-go-release-handoff.md:608
		for _, path := range []string{"../.github/workflows/go-runtime.yml", "../.github/workflows/runtime-bundles.yml"} {
			body, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			var workflow struct {
				Concurrency struct {
					Group  string `yaml:"group"`
					Cancel string `yaml:"cancel-in-progress"`
				} `yaml:"concurrency"`
			}
			Expect(yaml.Unmarshal(body, &workflow)).To(Succeed())
			Expect(workflow.Concurrency.Group).To(Equal("${{ github.workflow }}-${{ github.event_name == 'pull_request' && github.ref || github.run_id }}"), "only PRs may share a workflow/ref concurrency key")
			Expect(workflow.Concurrency.Cancel).To(Equal("${{ github.event_name == 'pull_request' }}"), "main and called releases must never be canceled")
		}
		Expect(baseline.Jobs).To(HaveKey("go-runtime"))
		Expect(candidate.Jobs).To(HaveKey("go-runtime"), "existing required matrix check names must retain their job identifier")
		Expect(candidate.Jobs).To(HaveLen(2), "installation needs its own parallel OS matrix rather than the exhausted single job")
		original := baseline.Jobs["go-runtime"]
		// Apply only the reviewed compiler/analyzer and action-reference exceptions
		// to the historical baseline; all other setup operands still compare exactly.
		// per docs/DECISION_LOG.md:4548
		Expect(original.Steps[0].Uses).To(Equal("actions/checkout@v7.0.1"))
		Expect(original.Steps[1].Uses).To(Equal("actions/setup-go@v7.0.0"))
		original.Steps[0].Uses = "actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1"
		original.Steps[1].Uses = "actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e"
		Expect(original.Steps[1].With["go-version"]).To(Equal("1.27.1"))
		original.Steps[1].With["go-version"] = "1.27.3"
		// per docs/adr/0101-go-release-handoff.md:606
		original.Steps[1].With["cache-dependency-path"] = "go.sum\ntools/security/go.sum\n"
		Expect(original.Steps[4].Name).To(Equal("Install pinned quality tools"))
		oldLinter := "go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2"
		newLinter := "go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0"
		oldSecurity := "go install github.com/securego/gosec/v2/cmd/gosec@v2.29.0"
		newSecurity := "go -C tools/security install -mod=readonly github.com/securego/gosec/v2/cmd/gosec"
		Expect(strings.Count(original.Steps[4].Run, oldLinter)).To(Equal(1))
		Expect(strings.Count(original.Steps[4].Run, oldSecurity)).To(Equal(1))
		original.Steps[4].Run = strings.Replace(original.Steps[4].Run, oldLinter, newLinter, 1)
		original.Steps[4].Run = strings.Replace(original.Steps[4].Run, oldSecurity, newSecurity, 1)
		for identifier, job := range candidate.Jobs {
			Expect(job.RunsOn).To(Equal(original.RunsOn))
			Expect(job.Timeout).To(Equal(30))
			Expect(job.Strategy.FailFast).To(Equal(original.Strategy.FailFast))
			Expect(job.Strategy.Matrix.OS).To(Equal(original.Strategy.Matrix.OS))
			Expect(job.Strategy.Matrix.Include).To(BeEmpty())
			Expect(job.Strategy.Matrix.Exclude).To(BeEmpty(), "no platform/group pair may be silently skipped")
			Expect(job.Needs).To(BeNil(), "matrix partitions must be independently scheduled")
			groups := []string{"base"}
			if identifier == "go-runtime" {
				Expect(job.Name).To(Equal(original.Name), "existing display/check names cannot change")
				Expect(job.Strategy.Matrix.Group).To(BeEmpty())
			} else {
				Expect(identifier).To(Equal("go-runtime-installation"))
				Expect(job.Name).To(Equal("go-runtime-${{ matrix.group }} (${{ matrix.os }})"))
				Expect(job.Strategy.Matrix.Group).To(ConsistOf("installation", "release-handoff", "historical-recipe", "local-init", "local-upgrade", "local-recovery", "default-init", "default-upgrade"))
				groups = job.Strategy.Matrix.Group
			}
			// per docs/adr/0101-go-release-handoff.md:597
			Expect(job.Steps).To(HaveLen(len(original.Steps) + 3))
			cache, pathStep, install, artifact := sourceGateWorkflowStep{}, sourceGateWorkflowStep{}, sourceGateWorkflowStep{}, sourceGateWorkflowStep{}
			normalized := []sourceGateWorkflowStep{}
			for _, step := range job.Steps {
				switch {
				// per docs/adr/0102-parallel-go-qualification.md:44
				case strings.HasPrefix(step.Uses, "actions/upload-artifact@"):
					Expect(artifact.Uses).To(BeEmpty())
					artifact = step
				case strings.HasPrefix(step.Uses, "actions/cache@"):
					Expect(cache.Uses).To(BeEmpty())
					cache = step
				case strings.Contains(step.Run, "$GITHUB_PATH"):
					Expect(pathStep.Run).To(BeEmpty())
					pathStep = step
				default:
					if step.Name == "Install pinned quality tools" {
						install = step
						step.If, step.Env = "", nil
					}
					normalized = append(normalized, step)
				}
			}
			Expect(artifact.Uses).To(Equal("actions/upload-artifact@cf430e030ddbb5b0abf93d22962f4752f3646cd9"))
			Expect(artifact).To(Equal(job.Steps[len(job.Steps)-1]), "report retention must follow the attempted source runner")
			artifactName, artifactIf := "go-runtime-base-${{ runner.os }}-${{ runner.arch }}", "always()"
			if identifier != "go-runtime" {
				artifactName, artifactIf = "go-runtime-${{ matrix.group }}-${{ runner.os }}-${{ runner.arch }}", "always() && matrix.group != 'installation'"
			}
			Expect(artifact.If).To(Equal(artifactIf), "failed base and handoff runs still attempt artifact publication")
			Expect(artifact.With).To(Equal(map[string]any{"name": artifactName, "path": "${{ runner.temp }}/factory-go-reports/", "if-no-files-found": "warn", "retention-days": 7}))
			Expect(cache.Uses).To(Equal("actions/cache@55cc8345863c7cc4c66a329aec7e433d2d1c52a9"))
			Expect(cache.ID).To(Equal("quality-tools"))
			Expect(cache.If).To(BeEmpty())
			Expect(cache.With).To(HaveLen(2), "no broad restore prefix may impersonate an exact qualified cache hit")
			Expect(cache.With["path"]).To(Equal("${{ runner.temp }}/factory-quality-tools"))
			Expect(cache.With["key"]).To(Equal("factory-quality-v1-${{ runner.os }}-${{ runner.arch }}-go1.27.3-${{ hashFiles('.github/workflows/go-runtime.yml', 'tools/security/go.mod', 'tools/security/go.sum') }}"))
			Expect(pathStep.If).To(BeEmpty(), "PATH must be configured on cold and warm runs")
			Expect(pathStep.Run).NotTo(BeEmpty())
			Expect(install.If).To(Equal("steps.quality-tools.outputs.cache-hit != 'true'"))
			Expect(install.Env).To(Equal(map[string]string{"GOBIN": "${{ runner.temp }}/factory-quality-tools"}))
			Expect(normalized).To(HaveLen(len(original.Steps)))
			Expect(normalized[:len(normalized)-1]).To(Equal(original.Steps[:len(original.Steps)-1]), "only reviewed cache setup exceptions may change checkout, Go/Python setup, interpreter qualification and exact tool pins")
			runner := normalized[len(normalized)-1]
			Expect(runner.If).To(BeEmpty(), "a cache hit never skips source qualification")
			Expect(runner.Name).To(Equal(original.Steps[len(original.Steps)-1].Name))
			for _, group := range groups {
				for _, warm := range []bool{false, true} {
					fixture := sourceGateMakeFixture()
					toolDirectory, pathFile := filepath.Join(fixture.root, "factory-quality-tools"), filepath.Join(fixture.root, "github-path")
					environment := []string{"RUNNER_TEMP=" + fixture.root, "GITHUB_PATH=" + pathFile}
					// per docs/adr/0104-adaptive-qualification-workers.md:54
					workerLimit := "4"
					if warm {
						workerLimit = "1"
					}
					expand := func(value string) string {
						value = strings.ReplaceAll(value, "${{ matrix.group }}", group)
						value = strings.ReplaceAll(value, "${{ vars.GO_RUNTIME_TEST_MAX_PROCS || '4' }}", workerLimit)
						return strings.ReplaceAll(value, "${{ runner.temp }}", fixture.root)
					}
					for key, value := range job.Env {
						environment = append(environment, key+"="+expand(value))
					}
					status, _, _ := sourceGateRun(fixture, pathStep.Run, environment)
					Expect(status).To(Equal(0))
					pathReceipt, err := os.ReadFile(pathFile)
					Expect(err).NotTo(HaveOccurred())
					Expect(string(pathReceipt)).To(Equal(toolDirectory+"\n"), "actual PATH step must select the dedicated directory on both cache outcomes")
					if warm {
						for _, name := range []string{"golangci-lint", "gosec", "govulncheck"} {
							writeFixture(filepath.Join(toolDirectory, name), []byte("#!/bin/sh\nprintf '%s\\n' 'cached:"+name+"' >> \"$SOURCE_GATE_QUALITY_LOG\"\n"), 0700)
						}
					} else {
						installEnvironment := append([]string{}, environment...)
						for key, value := range install.Env {
							installEnvironment = append(installEnvironment, key+"="+expand(value))
						}
						status, _, _ := sourceGateRun(fixture, install.Run, installEnvironment)
						Expect(status).To(Equal(0))
					}
					for key, value := range runner.Env {
						environment = append(environment, key+"="+expand(value))
					}
					environment = append(environment, "PATH="+toolDirectory+string(os.PathListSeparator)+filepath.Join(fixture.root, "tools")+string(os.PathListSeparator)+os.Getenv("PATH"))
					status, calls, quality := sourceGateRun(fixture, expand(runner.Run), environment)
					Expect(status).To(Equal(0), "actual %s/%s workflow recipe failed with warm=%t", identifier, group, warm)
					sourceGateAssertWorkerPlan(calls, "auto", workerLimit)
					tests := sourceGateTests(calls)
					Expect(tests).To(HaveLen(1))
					sourceGateAssertPartition(tests[0], group)
					sourceGateAssertInternal(calls, group == "base")
					if group != "installation" {
						workers, err := strconv.Atoi(sourceGateOperand(tests[0].Args, "--procs"))
						Expect(err).NotTo(HaveOccurred())
						limit, err := strconv.Atoi(workerLimit)
						Expect(err).NotTo(HaveOccurred())
						Expect(workers).To(BeNumerically("<=", limit), "both CI platforms use automatic selection under the repository cap")
						if warm {
							Expect(workers).To(Equal(1), "repository cap one must reach the actual CI runner")
						}
						Expect(sourceGateOperand(tests[0].Args, "--json-report")).To(Equal(filepath.Join(fixture.root, "factory-go-reports", group+".json")))
						Expect(sourceGateOperand(tests[0].Args, "--junit-report")).To(Equal(filepath.Join(fixture.root, "factory-go-reports", group+".xml")))
					}
					Expect(quality).To(ContainElements("gofmt", "dialect", "cached:golangci-lint", "cached:gosec", "cached:govulncheck"), "restored executables must still perform every qualification stage")
					installCalls := [][]string{}
					for _, call := range calls {
						if len(call.Args) > 0 && (call.Args[0] == "install" || call.Args[0] == "-C") {
							installCalls = append(installCalls, call.Args)
						}
					}
					if warm {
						Expect(installCalls).To(BeEmpty())
					} else {
						Expect(installCalls).To(Equal([][]string{{"install", "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0"}, {"-C", "tools/security", "install", "-mod=readonly", "github.com/securego/gosec/v2/cmd/gosec"}, {"install", "golang.org/x/vuln/cmd/govulncheck@v1.7.0"}}))
					}
				}
			}
		}
	})
})
