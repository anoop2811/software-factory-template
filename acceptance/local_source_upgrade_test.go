package acceptance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/anoop2811/software-factory-template/internal/packaging"
	"github.com/anoop2811/software-factory-template/internal/transition"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const localUpgradePlanMarker = "INDEPENDENT_SELECTED_LOCAL_UPGRADER_PLAN_20261030"
const localUpgradeApplyMarker = "INDEPENDENT_SELECTED_LOCAL_UPGRADER_APPLY_20261030"

type localUpgradeFixture struct {
	image                                   handoffReleaseImage
	beforeSource, source, revision, control string
	legacy                                  installationFixtureData
}

// A source-only proposal marker proves the selected compiled planner runs; the
// invoker was built before that marker. No supplied predecessor code executes.
// per docs/adr/0117-explicit-go-local-upgrade.md:10
func localUpgradePrepare() localUpgradeFixture {
	GinkgoHelper()
	image := handoffBuildImage()
	Expect(string(image.binary)).NotTo(ContainSubstring(localUpgradePlanMarker))
	before := filepath.Join(image.work, "clean predecessor source")
	Expect(os.Rename(filepath.Join(image.work, "exact-source"), before)).To(Succeed())
	var err error
	before, err = filepath.EvalSymlinks(before)
	Expect(err).NotTo(HaveOccurred())
	imageGit(before, "config", "gc.auto", "0")
	imageGit(before, "config", "maintenance.auto", "false")
	repository, err := filepath.Abs("..")
	Expect(err).NotTo(HaveOccurred())
	imageGit(before, "fetch", "--quiet", "--no-tags", repository, "b71ecc32e07ecd87eb330ba8e497c86612f92acd", "76952eaa63aebd1ecd282f5ab51dd7c3627cb497")
	source := filepath.Join(image.work, "selected source 'quoted' $(not-code)")
	imageGit(image.work, "clone", "--quiet", "--shared", before, source)
	source, err = filepath.EvalSymlinks(source)
	Expect(err).NotTo(HaveOccurred())
	imageGit(source, "config", "gc.auto", "0")
	imageGit(source, "config", "maintenance.auto", "false")
	path := filepath.Join(source, "internal/installation/proposal.go")
	data, err := os.ReadFile(path)
	Expect(err).NotTo(HaveOccurred())
	needle := `Checks: []string{"candidate-help", "installed-assets", "native-gate-proof", "state-compatibility"}`
	marked := strings.Replace(string(data), needle, `Checks: []string{"candidate-help", "installed-assets", "native-gate-proof", "state-compatibility", "`+localUpgradePlanMarker+`"}`, 1)
	Expect(marked).NotTo(Equal(string(data)))
	writeFixture(path, []byte(marked), 0644)
	control := filepath.Join(image.work, "selected-worker-control.json")
	transaction := filepath.Join(source, "internal/installation/transaction.go")
	transactionData, err := os.ReadFile(transaction)
	Expect(err).NotTo(HaveOccurred())
	needle = "owner.journalImage, err = saveJournal(ctx, owner.tree, owner.record, owner.journalImage)\n\treturn err"
	paused := strings.Replace(string(transactionData), needle, "owner.journalImage, err = saveJournal(ctx, owner.tree, owner.record, owner.journalImage)\n\tif err == nil { err = owner.independentLocalUpgradePause(ctx) }\n\treturn err", 1)
	Expect(paused).NotTo(Equal(string(transactionData)))
	paused = strings.Replace(paused, "func (capability *Capability) Complete(ctx context.Context) error {", "func (capability *Capability) Complete(ctx context.Context) error {\n _, _ = os.Stderr.WriteString("+fmt.Sprintf("%q", localUpgradeApplyMarker+"\n")+")", 1)
	publication := "\tnewPending := owner.pending == nil"
	admitted := strings.Replace(paused, publication, "\tif err := owner.independentLocalUpgradeAdmission(ctx); err != nil { return err }\n"+publication, 1)
	Expect(admitted).NotTo(Equal(paused))
	writeFixture(transaction, []byte(admitted), 0644)
	hook := strings.ReplaceAll(localUpgradePauseSource, "INDEPENDENT_CONTROL_PATH", fmt.Sprintf("%q", control))
	writeFixture(filepath.Join(source, "internal/installation/independent_local_upgrade_fixture.go"), []byte(hook), 0644)
	imageGit(source, "add", "internal/installation/proposal.go", "internal/installation/transaction.go", "internal/installation/independent_local_upgrade_fixture.go")
	imageGit(source, "commit", "--quiet", "-m", "independent selected upgrade planner witness")
	revision := strings.TrimSpace(imageGit(source, "rev-parse", "HEAD"))
	Expect(revision).NotTo(Equal(image.revision))
	Expect(imageGit(source, "status", "--porcelain", "--untracked-files=all")).To(BeEmpty())
	legacy := installationLegacyFixture(image.binary, image.references["v0.1.6"], image.archivePath, image.revision)
	AddReportEntry("local-upgrade.source", map[string]string{"before_source": before, "source": source, "revision": revision, "invoker_revision": image.revision}, ReportEntryVisibilityNever)
	// Commit-triggered detached Git maintenance changes loose-object storage while
	// a preview runs. Disable it before any adopter commit; keep the complete
	// .git byte/mode preservation witness rather than dropping its paths.
	imageGit(legacy.root, "config", "gc.auto", "0")
	imageGit(legacy.root, "config", "maintenance.auto", "false")
	return localUpgradeFixture{image, before, source, revision, control, legacy}
}

func localUpgradeArgs(source, before string) []string {
	args := []string{"upgrade", "--local", "--source", source}
	if before != "" {
		args = append(args, "--before-source", before)
	}
	for _, input := range installationInputs {
		args = append(args, "--project-input", input)
	}
	return args
}

func localUpgradeBundle(source, revision, version, output string, selectedCompiler ...string) packaging.Result {
	GinkgoHelper()
	compiler, err := exec.LookPath("go")
	Expect(err).NotTo(HaveOccurred())
	if len(selectedCompiler) == 1 {
		compiler = selectedCompiler[0]
	}
	compiler, err = filepath.EvalSymlinks(compiler)
	Expect(err).NotTo(HaveOccurred())
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	bundle, err := packaging.Build(ctx, packaging.Options{Source: source, Revision: revision, Version: version, Target: runtime.GOOS + "/" + runtime.GOARCH, Output: output, Compiler: compiler, Installation: true})
	Expect(err).NotTo(HaveOccurred())
	return bundle
}

func localUpgradeWholeArgs(image packaging.Result, profile string) []string {
	args := []string{"upgrade", "--installation", "--profile", profile, "--source", image.Archive, "--version", image.Version, "--revision", image.Revision, "--target", image.Target, "--local"}
	for _, input := range installationInputs {
		args = append(args, "--project-input", input)
	}
	return args
}

const localUpgradeAnswers = "Installation Project\ninstallation-project\n@factory-fixture\nfixture-user\ninternal\ndocs\n\ninherit\nstandard\nn\ny\n"

type localUpgradeGoFixture struct {
	f            handoffBootstrapFixture
	driver, root string
	before       packaging.Result
	runtime      []byte
}

func (fixture localUpgradeFixture) goPredecessor(versioned bool, alternateCompiler ...bool) localUpgradeGoFixture {
	GinkgoHelper()
	setup, _ := handoffNativeFixture(fixture.image)
	cache := filepath.Join(setup.root, "original predecessor cache")
	Expect(os.MkdirAll(cache, 0700)).To(Succeed())
	cache, err := filepath.EvalSymlinks(cache)
	Expect(err).NotTo(HaveOccurred())
	version := "local-" + fixture.image.revision
	if versioned {
		version = handoffVersion
	}
	var compiler []string
	if len(alternateCompiler) == 1 && alternateCompiler[0] {
		localUpgradeCompiler(setup, fixture.image, "build", "drift")
		compiler = []string{filepath.Join(setup.root, "host tools/go")}
	}
	before := localUpgradeBundle(fixture.beforeSource, fixture.image.revision, version, filepath.Join(cache, "package"), compiler...)
	archive, err := os.ReadFile(before.Archive)
	Expect(err).NotTo(HaveOccurred())
	entries, _ := imageDecodeArchive(archive)
	var executable []byte
	for _, entry := range entries {
		if strings.HasSuffix(entry.name, "/bin/factory-runtime") {
			executable = entry.body
		}
	}
	Expect(executable).NotTo(BeEmpty())
	original := handoffReleaseImage{executable, archive, before.Archive, fixture.image.revision, fixture.image.work, fixture.image.references}
	transport, program := handoffNativeFixture(original)
	root := filepath.Join(transport.cwd, "original Go adopter")
	Expect(os.Mkdir(root, 0700)).To(Succeed())
	imageGit(root, "init", "--quiet")
	args := []string{"init", root, "--go-candidate", handoffVersion, "--pack", "none"}
	if !versioned {
		args = []string{"--factory-local-init", "1", cache, fixture.beforeSource, fixture.image.revision, before.SHA256, "--", root, "--pack", "none"}
	}
	initialized := handoffNativeProcess(transport.cwd, transport.environment, localUpgradeAnswers, program, args...)
	Expect(initialized.status).To(Equal(0), "%+v", initialized)
	f, driver := localInitSurface(fixture.image)
	return localUpgradeGoFixture{f, driver, root, before, executable}
}

func localUpgradeBefore(args []string, before packaging.Result) []string {
	return append(args, "--before-source", before.Archive, "--before-version", before.Version, "--before-revision", before.Revision, "--before-target", before.Target, "--before-local")
}

func localUpgradeMigration(root string) string {
	GinkgoHelper()
	paths, err := filepath.Glob(filepath.Join(root, ".factory/installation-transactions/*.json"))
	Expect(err).NotTo(HaveOccurred())
	Expect(paths).To(HaveLen(1))
	return strings.TrimSuffix(filepath.Base(paths[0]), ".json")
}

func localUpgradeCompiler(f handoffBootstrapFixture, image handoffReleaseImage, stage, mode string) string {
	GinkgoHelper()
	compiler, err := exec.LookPath("go")
	Expect(err).NotTo(HaveOccurred())
	compiler, err = filepath.EvalSymlinks(compiler)
	Expect(err).NotTo(HaveOccurred())
	data := strings.Replace(localCompilerProcessFixture, `if stage==c.Stage&&c.Mode=="overflow"{`, `if stage==c.Stage&&c.Mode=="fail"{fmt.Fprintln(os.Stderr,"independent local upgrade compiler failure");os.Exit(58)}
 if stage==c.Stage&&c.Mode=="overflow"{`, 1)
	data = strings.Replace(data, "command:=exec.Command(c.Real,os.Args[1:]...)", `if stage=="build"{file,err:=os.OpenFile(c.Ready+".builds",os.O_APPEND|os.O_CREATE|os.O_WRONLY,0600);if err!=nil{os.Exit(16)};if _,err=file.WriteString("build\n");err!=nil{os.Exit(17)};if file.Close()!=nil{os.Exit(18)}}
 if c.Mode=="drift"&&stage=="build"{os.Args=append(os.Args[:2],append([]string{"-ldflags=-buildid=INDEPENDENT_PRIOR_COMPILER_BYTES"},os.Args[2:]...)...)}
 command:=exec.Command(c.Real,os.Args[1:]...)`, 1)
	source := filepath.Join(f.root, "upgrade-compiler.go")
	wrapper := filepath.Join(f.root, "upgrade-compiler")
	writeFixture(source, []byte(data), 0600)
	built := imageProcess(image.work, nil, compiler, "build", "-trimpath", "-buildvcs=false", "-o", wrapper, source)
	Expect(built.status).To(Equal(0), "%+v", built)
	return localInitCompiler(f, wrapper, compiler, stage, mode)
}

// Evaluator-only code is committed into the selected fixture, never production.
// The READY receipt follows a real durable journal save after two published rows;
// cancellation targets that selected worker, not its outer inherited-stream waiter.
// per docs/adr/0117-explicit-go-local-upgrade.md:78
const localUpgradePauseSource = `package installation
import("context";"encoding/json";"os";"syscall";"time")
func(owner *transaction)independentLocalUpgradeAdmission(ctx context.Context)error{
 data,err:=os.ReadFile(INDEPENDENT_CONTROL_PATH);if os.IsNotExist(err){return nil};if err!=nil{return err}
 var c struct{Root,Ready string};if err=json.Unmarshal(data,&c);err!=nil{return err}
 if owner.root!=c.Root||owner.request.Recover!=""||owner.request.Rollback!=""{return nil}
 group,err:=syscall.Getpgid(0);if err!=nil{return err}
 receipt:=map[string]any{"pid":os.Getpid(),"group":group,"root":owner.root,"migration":owner.record.MigrationID,"phase":"publisher-admitted","source":owner.request.Source.Archive,"before":owner.request.Before.Archive,"at":time.Now().UTC().Format(time.RFC3339Nano)}
 data,err=json.Marshal(receipt);if err!=nil{return err};if err=os.WriteFile(c.Ready+".admitted.tmp",data,0600);err!=nil{return err};return os.Rename(c.Ready+".admitted.tmp",c.Ready+".admitted")
}
func(owner *transaction)independentLocalUpgradePause(ctx context.Context)error{
 data,err:=os.ReadFile(INDEPENDENT_CONTROL_PATH);if os.IsNotExist(err){return nil};if err!=nil{return err}
 var c struct{Root,Ready string};if err=json.Unmarshal(data,&c);err!=nil{return err}
 if owner.root!=c.Root||owner.request.Recover!=""||owner.request.Rollback!=""||owner.record.Phase!="applying"{return nil}
 applied:=0;for _,row:=range owner.record.Entries{if row.Phase=="applied"{applied++}};if applied<2{return nil}
 group,err:=syscall.Getpgid(0);if err!=nil{return err}
 receipt:=map[string]any{"pid":os.Getpid(),"group":group,"root":owner.root,"migration":owner.record.MigrationID,"phase":owner.record.Phase,"applied":applied,"source":owner.request.Source.Archive,"before":owner.request.Before.Archive,"at":time.Now().UTC().Format(time.RFC3339Nano)}
 data,err=json.Marshal(receipt);if err!=nil{return err};if err=os.WriteFile(c.Ready+".tmp",data,0600);err!=nil{return err};if err=os.Rename(c.Ready+".tmp",c.Ready);err!=nil{return err}
 for{select{case<-ctx.Done():return ctx.Err();case<-time.After(10*time.Millisecond):if _,err:=os.Stat(c.Ready+".release");err==nil{return nil}}}
}
`

type localUpgradeWorkerReceipt struct {
	PID, Group                                 int
	Root, Migration, Phase, Source, Before, At string
	Applied                                    int
}

func localUpgradeRecoveryArgs(stderr string) []string {
	GinkgoHelper()
	_, rest, found := strings.Cut(stderr, "Recovery preview (no changes):\n")
	Expect(found).To(BeTrue(), "actual interrupted child must emit recoverable input advice: %s", stderr)
	line, _, _ := strings.Cut(rest, "\n")
	Expect(line).NotTo(BeEmpty())
	// The output is the product's fully single-quoted shell vector. Decode only
	// this one line, then use exec literal operands for all actual recovery calls.
	command := exec.Command("/bin/sh", "-c", "set -- "+line+"; printf '%s\\0' \"$@\"") // #nosec G204 G702 -- decode the exact emitted fully quoted command from an owned fixture; subsequent recovery uses literal exec operands.
	output, err := command.Output()
	Expect(err).NotTo(HaveOccurred())
	args := strings.Split(strings.TrimSuffix(string(output), "\x00"), "\x00")
	Expect(args).To(ContainElement("--dry-run"))
	Expect(args).NotTo(ContainElement("--confirm"))
	Expect(args).NotTo(ContainElement("--quiescent"))
	Expect(args[0]).To(BeAnExistingFile())
	Expect(args[1:3]).To(Equal([]string{"upgrade", "--installation"}))
	Expect(args).To(ContainElement("--local"))
	return args
}

func localUpgradeWithoutPreview(args []string) []string {
	result := []string{}
	for _, arg := range args {
		if arg != "--dry-run" && arg != "--json" {
			result = append(result, arg)
		}
	}
	return result
}

var _ = Describe("Go explicit local source upgrade", Ordered, ContinueOnFailure, func() {
	var fixture localUpgradeFixture
	BeforeAll(func() { fixture = localUpgradePrepare() })

	// per docs/adr/0117-explicit-go-local-upgrade.md:26
	// per docs/adr/0117-explicit-go-local-upgrade.md:73
	It("upgrades a genuine legacy installation with fresh consent and restores exact originals through rollback", func() {
		f, driver := localInitSurface(fixture.image)
		legacy := installationCloneLegacyFixture(fixture.image.binary, fixture.legacy)
		root := legacy.root
		preserved := map[string][]byte{"docs/operator-note.md": []byte("operator customization\n"), ".factory/events.log": []byte("2026-10-30T00:00:00Z\toperator\tkeep history\n")}
		for path, data := range preserved {
			writeFixture(filepath.Join(root, filepath.FromSlash(path)), data, 0600)
		}
		imageGit(root, "add", "--all")
		imageGit(root, "commit", "--quiet", "-m", "adopter application and history")
		head, index := imageGit(root, "rev-parse", "HEAD"), imageGit(root, "ls-files", "--stage", "-z")
		original, err := os.ReadFile(filepath.Join(root, "factory"))
		Expect(err).NotTo(HaveOccurred())
		config, err := os.ReadFile(filepath.Join(root, "factory.yaml"))
		Expect(err).NotTo(HaveOccurred())
		before, cache := installationSnapshot(root), installationSnapshot(f.environment["FACTORY_RELEASE_DIR"])
		args := localUpgradeArgs(fixture.source, "")
		proposal := handoffProposal(handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, args...), "--dry-run", "--json")...))
		Expect(proposal["authentication"]).To(Equal("local-source"))
		Expect(proposal["profile"]).To(Equal("v0.1.6"))
		Expect(proposal["checks"]).To(ContainElement(localUpgradePlanMarker))
		Expect(proposal["blockers"]).To(BeEmpty())
		Expect(installationSnapshot(root)).To(Equal(before))
		confirmed := append(append([]string{}, args...), "--confirm", proposal["proposal_digest"].(string))
		refused := handoffNativeProcess(root, f.environment, "", driver, confirmed...)
		Expect(refused.status).To(Equal(2), "%+v", refused)
		Expect(installationSnapshot(root)).To(Equal(before))
		applied := handoffNativeProcess(root, f.environment, "", driver, append(confirmed, "--quiescent")...)
		Expect(applied.status).To(Equal(0), "%+v", applied)
		Expect(applied.stderr).To(ContainSubstring(localUpgradeApplyMarker))
		localInitDescriptor(root, fixture.revision)
		for path, data := range preserved {
			Expect(os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))).To(Equal(data))
		}
		Expect(os.ReadFile(filepath.Join(root, "factory.yaml"))).To(Equal(config))
		Expect(imageGit(root, "rev-parse", "HEAD")).To(Equal(head))
		Expect(imageGit(root, "ls-files", "--stage", "-z")).To(Equal(index))
		ids := retentionBackupIDs(root)
		Expect(ids).To(HaveLen(1))
		saved := filepath.Join(root, ".factory/backups", ids[0], "assets/factory")
		info, err := os.Stat(saved)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode().Perm() & 0111).To(BeZero())
		Expect(os.ReadFile(saved)).To(Equal(original))
		Expect(imageProcess(root, nil, "git", "check-ignore", "--no-index", "-q", "--", saved).status).To(Equal(0))
		after := localUpgradeBundle(fixture.source, fixture.revision, "local-"+fixture.revision, filepath.Join(f.root, "exact after image"))
		localInitPoisonCompiler(f)
		ordinary := handoffNativeProcess(root, f.environment, "", filepath.Join(root, "factory"), "selftest")
		Expect(ordinary.status).To(Equal(0), "%+v", ordinary)
		reverse := append(localUpgradeWholeArgs(after, "v0.1.6"), "--rollback", ids[0])
		rollback := handoffProposal(handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, reverse...), "--dry-run", "--json")...))
		// Fresh legacy projection deliberately executes the original metrics path,
		// which requires Python. Go selftest above ran with Python poisoned; only
		// this legacy rollback phase receives the evaluator's provisioned tool.
		python, err := exec.LookPath("python3")
		Expect(err).NotTo(HaveOccurred())
		python, err = filepath.EvalSymlinks(python)
		Expect(err).NotTo(HaveOccurred())
		pythonLink := filepath.Join(f.root, "host tools/python3")
		Expect(os.Remove(pythonLink)).To(Succeed())
		Expect(os.Symlink(python, pythonLink)).To(Succeed())
		restored := handoffNativeProcess(root, f.environment, "", driver, append(reverse, "--confirm", rollback["proposal_digest"].(string), "--quiescent")...)
		Expect(restored.status).To(Equal(0), "%+v", restored)
		Expect(os.ReadFile(filepath.Join(root, "factory"))).To(Equal(original))
		Expect(os.ReadFile(filepath.Join(root, "factory.yaml"))).To(Equal(config))
		for path, data := range preserved {
			Expect(os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))).To(Equal(data))
		}
		Expect(installationSnapshot(f.environment["FACTORY_RELEASE_DIR"])).To(Equal(cache))
		localInitNoAcquisition(f)
	})
	// per docs/adr/0117-explicit-go-local-upgrade.md:34
	DescribeTable("upgrades an exact Go predecessor and restores its original native identity", func(versioned bool) {
		before := fixture.goPredecessor(versioned)
		f, driver, root := before.f, before.driver, before.root
		modern := []byte("{\"schema\":1,\"runs\":[],\"note\":\"preserve Go history\"}\n")
		writeFixture(filepath.Join(root, ".factory/loops.json"), modern, 0600)
		writeFixture(filepath.Join(root, "docs/operator-go-note.md"), []byte("operator Go content\n"), 0600)
		writeFixture(filepath.Join(root, ".factory/backups/operator-held/witness"), []byte("do not advance release retention\n"), 0600)
		original, err := os.ReadFile(filepath.Join(root, ".factory/installation.current"))
		Expect(err).NotTo(HaveOccurred())
		snapshot := installationSnapshot(root)
		args := localUpgradeArgs(fixture.source, fixture.beforeSource)
		preview := handoffProposal(handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, args...), "--dry-run", "--json", "--")...))
		Expect(preview["profile"]).To(Equal("go-hybrid-v1"))
		Expect(preview["authentication"]).To(Equal("local-source"))
		Expect(preview["checks"]).To(ContainElement(localUpgradePlanMarker))
		Expect(preview["blockers"]).To(BeEmpty())
		Expect(installationSnapshot(root)).To(Equal(snapshot))
		applied := handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, args...), "--confirm", preview["proposal_digest"].(string), "--quiescent", "--")...)
		Expect(applied.status).To(Equal(0), "%+v", applied)
		Expect(applied.stderr).To(ContainSubstring(localUpgradeApplyMarker))
		localInitDescriptor(root, fixture.revision)
		Expect(os.ReadFile(filepath.Join(root, ".factory/loops.json"))).To(Equal(modern))
		Expect(os.ReadFile(filepath.Join(root, ".factory/backups/operator-held/witness"))).To(Equal([]byte("do not advance release retention\n")))
		after := localUpgradeBundle(fixture.source, fixture.revision, "local-"+fixture.revision, filepath.Join(f.root, "original after image"))
		reverse := localUpgradeBefore(localUpgradeWholeArgs(after, "go-hybrid-v1"), before.before)
		reverse = append(reverse, "--rollback", localUpgradeMigration(root))
		localInitPoisonCompiler(f)
		proposal := handoffProposal(handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, reverse...), "--dry-run", "--json")...))
		restored := handoffNativeProcess(root, f.environment, "", driver, append(reverse, "--confirm", proposal["proposal_digest"].(string), "--quiescent")...)
		Expect(restored.status).To(Equal(0), "%+v", restored)
		Expect(os.ReadFile(filepath.Join(root, ".factory/bin/factory-runtime"))).To(Equal(before.runtime))
		Expect(os.ReadFile(filepath.Join(root, ".factory/installation.current"))).To(Equal(original))
		Expect(os.ReadFile(filepath.Join(root, ".factory/loops.json"))).To(Equal(modern))
		healthy := handoffNativeProcess(root, f.environment, "", filepath.Join(root, "factory"), "selftest")
		Expect(healthy.status).To(Equal(0), "%+v", healthy)
		localInitNoAcquisition(f)
	}, Entry("versioned Go image", true), Entry("local Go image", false))

	// per docs/adr/0117-explicit-go-local-upgrade.md:34
	It("refuses missing or mismatched predecessor inputs and preserves required customized entries", func() {
		before := fixture.goPredecessor(false)
		f, driver, root := before.f, before.driver, before.root
		snapshot := installationSnapshot(root)
		missing := handoffNativeProcess(root, f.environment, "", driver, append(localUpgradeArgs(fixture.source, ""), "--dry-run", "--json")...)
		Expect(missing.status).To(Equal(2), "%+v", missing)
		Expect(missing.stderr).To(ContainSubstring("--before-source"))
		Expect(installationSnapshot(root)).To(Equal(snapshot))
		wrong := handoffNativeProcess(root, f.environment, "", driver, append(localUpgradeArgs(fixture.source, fixture.source), "--dry-run", "--json")...)
		Expect(wrong.status).To(Equal(2), "%+v", wrong)
		Expect(wrong.stderr).To(ContainSubstring("predecessor"))
		Expect(installationSnapshot(root)).To(Equal(snapshot))
		// Attached flag-looking data stays literal when the parent forwards it
		// into the selected worker; all grammar flags precede the final delimiter.
		literalBefore := filepath.Join(root, "--literal-before")
		Expect(os.Symlink(fixture.beforeSource, literalBefore)).To(Succeed())
		literalSnapshot := installationSnapshot(root)
		literalArgs := append(localUpgradeArgs(fixture.source, ""), "--before-source=--literal-before", "--dry-run", "--json", "--")
		literal := handoffProposal(handoffNativeProcess(root, f.environment, "", driver, literalArgs...))
		Expect(literal["blockers"]).To(BeEmpty())
		Expect(literal["checks"]).To(ContainElement(localUpgradePlanMarker))
		Expect(installationSnapshot(root)).To(Equal(literalSnapshot))
		Expect(os.Remove(literalBefore)).To(Succeed())
		dirtyBefore := filepath.Join(f.root, "dirty predecessor source")
		imageGit(f.root, "clone", "--quiet", "--shared", fixture.beforeSource, dirtyBefore)
		writeFixture(filepath.Join(dirtyBefore, "untracked-before-witness"), []byte("dirty before checkout\n"), 0600)
		dirtySnapshot := installationSnapshot(dirtyBefore)
		beforeDirty := handoffNativeProcess(root, f.environment, "", driver, append(localUpgradeArgs(fixture.source, dirtyBefore), "--dry-run", "--json")...)
		Expect(beforeDirty.status).To(Equal(1), "%+v", beforeDirty)
		Expect(beforeDirty.stderr).To(Or(ContainSubstring("dirty"), ContainSubstring("uncommitted")))
		Expect(installationSnapshot(dirtyBefore)).To(Equal(dirtySnapshot))
		Expect(installationSnapshot(root)).To(Equal(snapshot))
		path := ".factory/assets/scaffold/scripts/lib/loop.py"
		custom := []byte("operator maintained required cache; never execute\n")
		writeFixture(filepath.Join(root, filepath.FromSlash(path)), custom, 0600)
		edited := installationSnapshot(root)
		out := handoffNativeProcess(root, f.environment, "", driver, append(localUpgradeArgs(fixture.source, fixture.beforeSource), "--dry-run", "--json")...)
		Expect(out.status).To(Equal(2), "%+v", out)
		Expect(out.stderr).To(ContainSubstring("--installation"))
		proposal := installationDecodeProposal(cliResult{stdout: out.stdout})
		found := false
		for _, raw := range proposal["blockers"].([]any) {
			if raw.(map[string]any)["path"] == path {
				found = true
			}
		}
		Expect(found).To(BeTrue())
		Expect(out.stderr).To(ContainSubstring("--installation"))
		Expect(installationSnapshot(root)).To(Equal(edited))
		// Genuine initialization is required: adapters, runtime.manifest and the
		// descriptor all bind runtime bytes. A two-file digest mutation would be
		// an incoherent installation and cannot prove the original-archive escape.
		driftBefore := fixture.goPredecessor(false, true)
		driftF, driftDriver, driftRoot := driftBefore.f, driftBefore.driver, driftBefore.root
		Expect(driftBefore.runtime).NotTo(Equal(before.runtime))
		driftSnapshot := installationSnapshot(driftRoot)
		driftRefusal := handoffNativeProcess(driftRoot, driftF.environment, "", driftDriver, append(localUpgradeArgs(fixture.source, fixture.beforeSource), "--dry-run", "--json")...)
		Expect(driftRefusal.status).To(Equal(2), "%+v", driftRefusal)
		Expect(driftRefusal.stderr).To(ContainSubstring("--installation"))
		driftProposal := installationDecodeProposal(cliResult{stdout: driftRefusal.stdout})
		Expect(driftProposal["blockers"]).To(ContainElement(HaveKeyWithValue("path", ".factory/bin/factory-runtime")))
		Expect(installationSnapshot(driftRoot)).To(Equal(driftSnapshot))
		after := localUpgradeBundle(fixture.source, fixture.revision, "local-"+fixture.revision, filepath.Join(driftF.root, "archive escape after"))
		escape := localUpgradeBefore(localUpgradeWholeArgs(after, "go-hybrid-v1"), driftBefore.before)
		escapePreview := handoffProposal(handoffNativeProcess(driftRoot, driftF.environment, "", driftDriver, append(escape, "--dry-run", "--json")...))
		Expect(escapePreview["blockers"]).To(BeEmpty(), "retaining the actual original archive remains a usable explicit escape")
		Expect(installationSnapshot(driftRoot)).To(Equal(driftSnapshot))
		localInitNoAcquisition(f)
		localInitNoAcquisition(driftF)
		legacy := installationCloneLegacyFixture(fixture.image.binary, fixture.legacy)
		legacyBefore := installationSnapshot(legacy.root)
		extra := handoffNativeProcess(legacy.root, f.environment, "", driver, append(localUpgradeArgs(fixture.source, fixture.beforeSource), "--dry-run", "--json")...)
		Expect(extra.status).To(Equal(2), "%+v", extra)
		Expect(installationSnapshot(legacy.root)).To(Equal(legacyBefore))
	})

})

// Independent ordered groups let the bounded two-worker CI plan share this
// heavy family. Each group owns its source and legacy initializer fixtures.
// per docs/adr/0117-explicit-go-local-upgrade.md:94
var _ = Describe("Go explicit local source upgrade", Ordered, ContinueOnFailure, func() {
	var fixture localUpgradeFixture
	BeforeAll(func() { fixture = localUpgradePrepare() })

	// per docs/adr/0117-explicit-go-local-upgrade.md:26
	It("binds fresh consent to literal local source and reports repeatable public apply instructions", func() {
		f, driver := localInitSurface(fixture.image)
		compilerReceipt := localUpgradeCompiler(f, fixture.image, "build", "healthy")
		legacy := installationCloneLegacyFixture(fixture.image.binary, fixture.legacy)
		root := legacy.root
		args := localUpgradeArgs(fixture.source, "")
		before := installationSnapshot(root)
		first := handoffProposal(handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, args...), "--dry-run", "--json")...))
		second := handoffProposal(handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, args...), "--dry-run", "--json")...))
		Expect(second["proposal_digest"]).To(Equal(first["proposal_digest"]), "fresh private acquisition paths cannot invalidate unchanged local-image consent")
		text := handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, args...), "--dry-run")...)
		Expect(text.status).To(Equal(0), "%+v", text)
		Expect(text.stdout).To(ContainSubstring(first["proposal_digest"].(string)))
		Expect(text.stdout).To(ContainSubstring("--confirm"))
		Expect(text.stdout).To(ContainSubstring("--quiescent"))
		Expect(text.stdout).NotTo(ContainSubstring("--factory-local-upgrade"))
		Expect(text.stdout).NotTo(ContainSubstring("curl"))
		stale := handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, args...), "--confirm", strings.Repeat("0", 64), "--quiescent")...)
		Expect(stale.status).To(Equal(2), "%+v", stale)
		Expect(installationSnapshot(root)).To(Equal(before))
		builds, err := os.ReadFile(compilerReceipt + ".builds")
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.Split(strings.TrimSpace(string(builds)), "\n")).To(HaveLen(4), "each of two JSON previews, text preview and stale apply builds the after-image exactly once; selected child must never recompile it")
		localInitNoAcquisition(f)
	})

	// per docs/adr/0117-explicit-go-local-upgrade.md:29
	It("validates local upgrade selectors before work and preserves nonlocal assessment routing", func() {
		f, driver := localInitSurface(fixture.image)
		localInitPoisonCompiler(f)
		legacy := installationCloneLegacyFixture(fixture.image.binary, fixture.legacy)
		root := legacy.root
		before := installationSnapshot(root)
		cases := [][]string{{"--local", "--local"}, {"--local=true"}, {"--local", "--source"}, {"--local", "--source="}, {"--local", "--source", fixture.source, "--source", fixture.source}, {"--local", "--before-source"}, {"--local", "--before-source="}, {"--local", "--before-source", fixture.beforeSource, "--before-source", fixture.beforeSource}, {"--local", "--go-candidate", handoffVersion}, {"--go-candidate", handoffVersion, "--local"}, {"--local", "--installation"}, {"--local", "--ref", "main"}, {"--local", "--dry-run", "--confirm", strings.Repeat("a", 64), "--quiescent"}, {"--local", "--json", "--confirm", strings.Repeat("a", 64), "--quiescent"}, {"--local", "--confirm", strings.Repeat("a", 64)}, {"--local", "--quiescent"}}
		for _, args := range cases {
			out := handoffNativeProcess(root, f.environment, "", driver, append([]string{"upgrade"}, args...)...)
			Expect(out.status).To(Equal(2), "args=%q: %+v", args, out)
			Expect(installationSnapshot(root)).To(Equal(before))
			localInitNoAcquisition(f)
		}
		bareHelp := handoffNativeProcess(root, f.environment, "", driver, "upgrade", "--local", "--help")
		Expect(bareHelp.status).To(Equal(0), "bare local help must not let Cobra consume ambient process argv: %+v", bareHelp)
		help := handoffNativeProcess(root, f.environment, "", driver, "upgrade", "--local", "--source", filepath.Join(f.cwd, "missing source"), "--help")
		Expect(help.status).To(Equal(0), "%+v", help)
		Expect(help.stdout).To(ContainSubstring("--before-source"))
		Expect(installationSnapshot(root)).To(Equal(before))
		localInitNoAcquisition(f)
		ordinary := handoffNativeProcess(root, f.environment, "", driver, "upgrade", "--dry-run", "--source", fixture.source, "--json")
		Expect(ordinary.status).To(Equal(2), "the existing assessment remains conservative for unproven ownership: %+v", ordinary)
		var assessment map[string]any
		Expect(json.Unmarshal([]byte(ordinary.stdout), &assessment)).To(Succeed())
		Expect(assessment["mode"]).To(Equal("dry_run"))
		Expect(assessment["coverage"]).To(Equal("partial"))
		plan := assessment["plan"].(map[string]any)
		Expect(plan["scope"]).To(Equal("g2-budget-loop-six"))
		Expect(plan["source_authentication"]).To(Equal("unverified_local"))
		Expect(plan["applicable"]).To(BeFalse())
		Expect(ordinary.stdout).NotTo(ContainSubstring(localUpgradePlanMarker))
		Expect(installationSnapshot(root)).To(Equal(before))
		localInitNoAcquisition(f)
		developer, _ := localInitSurface(fixture.image)
		sourceProgram := filepath.Join(fixture.source, "factory-go")
		writeFixture(sourceProgram, fixture.image.binary, 0700)
		Expect(imageGit(fixture.source, "status", "--porcelain", "--untracked-files=all")).To(BeEmpty())
		implicit := handoffNativeProcess(root, developer.environment, "", sourceProgram, "upgrade", "--local")
		Expect(implicit.status).To(Equal(0), "bare local selection defaults to a read-only preview from the invoked checkout: %+v", implicit)
		Expect(implicit.stdout).To(ContainSubstring("proposal_digest="))
		Expect(installationSnapshot(root)).To(Equal(before))
		localInitNoAcquisition(developer)

	})

	// per docs/adr/0117-explicit-go-local-upgrade.md:22
	It("preserves source project and cache on dirty checkout or actual compiler failure", func() {
		for _, kind := range []string{"tracked", "untracked", "compiler"} {
			f, driver := localInitSurface(fixture.image)
			legacy := installationCloneLegacyFixture(fixture.image.binary, fixture.legacy)
			root := legacy.root
			source := fixture.source
			if kind != "compiler" {
				source = filepath.Join(f.root, "dirty selected source")
				imageGit(f.root, "clone", "--quiet", "--shared", fixture.source, source)
				if kind == "tracked" {
					path := filepath.Join(source, "go.mod")
					body, err := os.ReadFile(path)
					Expect(err).NotTo(HaveOccurred())
					writeFixture(path, append(body, '\n'), 0644)
				} else {
					writeFixture(filepath.Join(source, "operator-untracked.go"), []byte("package operator\n"), 0600)
				}
				localInitPoisonCompiler(f)
			} else {
				localUpgradeCompiler(f, fixture.image, "build", "fail")
			}
			before, sourceBefore, cache := installationSnapshot(root), installationSnapshot(source), installationSnapshot(f.environment["FACTORY_RELEASE_DIR"])
			out := handoffNativeProcess(root, f.environment, "", driver, append(localUpgradeArgs(source, ""), "--dry-run", "--json")...)
			Expect(out.status).To(Equal(1), "kind=%s: %+v", kind, out)
			if kind == "compiler" {
				Expect(out.stderr).To(ContainSubstring("independent local upgrade compiler failure"))
			} else {
				Expect(out.stderr).To(Or(ContainSubstring("dirty"), ContainSubstring("uncommitted")))
			}
			Expect(installationSnapshot(root)).To(Equal(before))
			Expect(installationSnapshot(source)).To(Equal(sourceBefore))
			Expect(installationSnapshot(f.environment["FACTORY_RELEASE_DIR"])).To(Equal(cache))
			localInitNoAcquisition(f)
		}
	})

	// per docs/adr/0117-explicit-go-local-upgrade.md:83
	DescribeTable("cancels only the public caller after an actual compiler readiness handshake", func(signal syscall.Signal) {
		f, driver := localInitSurface(fixture.image)
		legacy := installationCloneLegacyFixture(fixture.image.binary, fixture.legacy)
		root := legacy.root
		ready := localUpgradeCompiler(f, fixture.image, "version", "hold")
		before, sourceBefore := installationSnapshot(root), installationSnapshot(fixture.source)
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		command := handoffCommand(ctx, root, f.environment, "", driver, append(localUpgradeArgs(fixture.source, ""), "--dry-run", "--json")...)
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
		Expect(command.Start()).To(Succeed())
		finished := make(chan error, 1)
		go func() { finished <- command.Wait() }()
		DeferCleanup(func() { _ = command.Process.Kill() })
		Eventually(func() bool {
			select {
			case err := <-finished:
				StopTrying(fmt.Sprintf("upgrade exited before compiler READY: %v stderr=%q", err, stderr.String())).Now()
			default:
			}
			_, err := os.Stat(ready)
			return err == nil
		}, 30*time.Second).Should(BeTrue())
		receipt := localCompilerReadiness(ready, "version", time.Second)
		DeferCleanup(localCompilerCleanup, receipt)
		Expect(receipt.Group).To(Equal(receipt.Leader))
		Expect(command.Process.Signal(signal)).To(Succeed())
		var returned error
		select {
		case returned = <-finished:
		case <-time.After(6 * time.Second):
			localCompilerCleanup(receipt)
			_ = command.Process.Kill()
			Eventually(finished, 6*time.Second).Should(Receive(&returned))
			Fail("upgrade caller exceeded bounded compiler cancellation")
		}
		Expect(returned).To(HaveOccurred())
		Expect(command.ProcessState.ExitCode()).To(Equal(1), "%s", stderr.String())
		Expect(stdout.String()).To(BeEmpty())
		uncertain := strings.Contains(stderr.String(), "ownership is unconfirmed")
		if localCompilerAlive(receipt) {
			Expect(uncertain).To(BeTrue())
		}
		entries, err := os.ReadDir(f.environment["TMPDIR"])
		Expect(err).NotTo(HaveOccurred())
		if !uncertain {
			Expect(entries).To(BeEmpty())
		}
		AddReportEntry("local-upgrade.compiler-cancellation", map[string]any{"signal": signal.String(), "receipt": receipt, "uncertain": uncertain, "alive": localCompilerAlive(receipt)}, ReportEntryVisibilityNever)
		Expect(installationSnapshot(root)).To(Equal(before))
		Expect(installationSnapshot(fixture.source)).To(Equal(sourceBefore))
		localInitNoAcquisition(f)
	}, Entry("SIGINT", syscall.SIGINT), Entry("SIGTERM", syscall.SIGTERM))

	// per docs/adr/0117-explicit-go-local-upgrade.md:59
	It("uses direct installed exclusive admission and preserves pending barriers", func() {
		before := fixture.goPredecessor(false)
		f, root := before.f, before.root
		installed := filepath.Join(root, "factory")
		args := localUpgradeArgs(fixture.source, fixture.beforeSource)
		preview := handoffProposal(handoffNativeProcess(root, f.environment, "", installed, append(append([]string{}, args...), "--dry-run", "--json")...))
		snapshot := installationSnapshot(root)
		lease, err := transition.ReadOnly(context.Background(), root)
		Expect(err).NotTo(HaveOccurred())
		refused := handoffNativeProcess(root, f.environment, "", installed, append(append([]string{}, args...), "--confirm", preview["proposal_digest"].(string), "--quiescent")...)
		Expect(refused.status).To(Equal(2), "competing reader must prevent publication: %+v", refused)
		Expect(installationSnapshot(root)).To(Equal(snapshot))
		Expect(lease.Close(context.Background())).To(Succeed())
		applied := handoffNativeProcess(root, f.environment, "", installed, append(args, "--confirm", preview["proposal_digest"].(string), "--quiescent")...)
		Expect(applied.status).To(Equal(0), "installed upgrade must not retain its own shared lease: %+v", applied)
		localInitPoisonCompiler(f)
		noSource := handoffNativeProcess(root, f.environment, "", installed, "upgrade", "--local", "--dry-run", "--json")
		Expect(noSource.status).To(Equal(2), "%+v", noSource)
		writeFixture(filepath.Join(root, ".factory/runtime-publication.pending"), []byte("operator pending witness\n"), 0600)
		pending := installationSnapshot(root)
		blocked := handoffNativeProcess(root, f.environment, "", filepath.Join(root, ".factory/bin/factory-runtime"), "--factory-local-upgrade", "1")
		Expect(blocked.status).NotTo(Equal(0))
		Expect(blocked.stderr).To(ContainSubstring("admission"))
		Expect(installationSnapshot(root)).To(Equal(pending))
		localInitNoAcquisition(f)
	})

	// per docs/adr/0117-explicit-go-local-upgrade.md:78
	DescribeTable("recovers from a durably interrupted selected worker with retained exact images and no compiler", func(direction string) {
		before := fixture.goPredecessor(false)
		f, driver, root := before.f, before.driver, before.root
		modern := []byte("{\"schema\":1,\"runs\":[],\"note\":\"interrupt witness\"}\n")
		writeFixture(filepath.Join(root, ".factory/loops.json"), modern, 0600)
		originalDescriptor, err := os.ReadFile(filepath.Join(root, ".factory/installation.current"))
		Expect(err).NotTo(HaveOccurred())
		args := localUpgradeArgs(fixture.source, fixture.beforeSource)
		preview := handoffProposal(handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, args...), "--dry-run", "--json")...))
		ready := filepath.Join(f.root, "actual-selected-worker.json")
		control, err := json.Marshal(map[string]string{"Root": root, "Ready": ready})
		Expect(err).NotTo(HaveOccurred())
		writeFixture(fixture.control, control, 0600)
		DeferCleanup(func() { Expect(os.Remove(fixture.control)).To(Succeed()) })
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		command := handoffCommand(ctx, root, f.environment, "", driver, append(args, "--confirm", preview["proposal_digest"].(string), "--quiescent")...)
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
		command.Cancel = func() error { return command.Process.Signal(syscall.SIGTERM) }
		Expect(command.Start()).To(Succeed())
		// Acquisition and preflight share the unchanged whole-command deadline;
		// publication gets its own unchanged allowance only after actual admission.
		// per docs/adr/0121-local-upgrade-readiness-boundary.md:25
		finished := make(chan struct{})
		var returned error
		go func() { returned = command.Wait(); close(finished) }()
		phase := "awaiting publisher admission"
		var admission, receipt localUpgradeWorkerReceipt
		readReceipt := func(path string, target *localUpgradeWorkerReceipt) bool {
			data, err := os.ReadFile(path)
			if os.IsNotExist(err) {
				return false
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(json.Unmarshal(data, target)).To(Succeed())
			return true
		}
		groupAbsent := func(value localUpgradeWorkerReceipt) (bool, error) {
			if value.PID <= 0 || value.Group != value.PID {
				return false, errors.New("no admitted selected process group")
			}
			err := syscall.Kill(-value.Group, 0)
			if errors.Is(err, syscall.ESRCH) {
				return true, nil
			}
			return false, err
		}
		defer func() {
			// Snapshot phase first; inspect captured output only after Wait, and
			// retain every owned input if process-group absence is unproven.
			admissionData, admissionErr := os.ReadFile(ready + ".admitted")
			durableData, durableErr := os.ReadFile(ready)
			owned := admission
			if owned.PID == 0 && admissionErr == nil {
				_ = json.Unmarshal(admissionData, &owned)
			}
			absentBefore, probeBefore := groupAbsent(owned)
			deadline, _ := ctx.Deadline()
			// On success this sample follows later recovery; a negative value is
			// not the duration of the already-reaped original command.
			remaining := time.Until(deadline).Seconds()
			waited := false
			select {
			case <-finished:
				waited = true
			default:
				_ = command.Process.Signal(syscall.SIGTERM)
				select {
				case <-finished:
					waited = true
				case <-time.After(10 * time.Second):
					// Destructive fallback requires the original live leader's
					// current group identity, not just a remembered numeric PGID.
					group, err := syscall.Getpgid(owned.PID)
					if owned.PID > 0 && owned.Group == owned.PID && err == nil && group == owned.Group {
						_ = syscall.Kill(-group, syscall.SIGKILL)
					}
					_ = command.Process.Kill()
					select {
					case <-finished:
						waited = true
					case <-time.After(10 * time.Second):
					}
				}
			}
			absentAfter, probeAfter := groupAbsent(owned)
			if !waited || !absentAfter || probeAfter != nil {
				reason := fmt.Sprintf("local upgrade custody uncertain: waited=%t group=%d absent=%t probe=%v", waited, owned.Group, absentAfter, probeAfter)
				for _, path := range []string{f.root, root, fixture.image.work} {
					holdFixtureCleanup(path, reason)
				}
			}
			observation := map[string]any{
				"phase": phase, "caller_pid": command.Process.Pid,
				"admission": string(admissionData), "admission_present": admissionErr == nil,
				"durable_ready": string(durableData), "durable_ready_present": durableErr == nil,
				"group_absent_before_cleanup": absentBefore, "group_probe_before_cleanup": fmt.Sprint(probeBefore),
				"group_absent_after_wait": absentAfter, "group_probe_after_wait": fmt.Sprint(probeAfter),
				"context_seconds_remaining_at_fixture_cleanup_entry": remaining, "caller_wait_confirmed": waited,
			}
			if waited {
				observation["caller_wait"] = fmt.Sprint(returned)
				observation["stdout_after_wait"] = stdout.String()
				observation["stderr_after_wait"] = stderr.String()
			}
			AddReportEntry("local-upgrade.publisher-boundary", observation, ReportEntryVisibilityNever)
			Expect(waited).To(BeTrue(), "native caller must be reaped before fixture disposal")
			Expect(probeAfter).NotTo(HaveOccurred(), "selected process-group absence must be observable")
			Expect(absentAfter).To(BeTrue(), "the whole recorded group must settle, including descendants of a reaped leader")
		}()
		waitForReceipt := func(path string, target *localUpgradeWorkerReceipt) bool {
			select {
			case <-finished:
				StopTrying(fmt.Sprintf("caller exited before %s: %v stderr=%q", phase, returned, stderr.String())).Now()
			default:
			}
			return readReceipt(path, target)
		}
		deadline, ok := ctx.Deadline()
		Expect(ok).To(BeTrue())
		Eventually(func() bool { return waitForReceipt(ready+".admitted", &admission) }, time.Until(deadline)).WithContext(ctx).Should(BeTrue())
		Expect(admission.PID).NotTo(Equal(command.Process.Pid), "admission must belong to the selected worker")
		Expect(admission.Group).To(Equal(admission.PID))
		Expect(admission.Root).To(Equal(root))
		Expect(admission.Phase).To(Equal("publisher-admitted"))
		Expect(admission.Source).To(BeAnExistingFile())
		Expect(admission.Before).To(BeAnExistingFile())
		phase = "awaiting selected worker durable READY"
		Eventually(func() bool { return waitForReceipt(ready, &receipt) }, 60*time.Second).WithContext(ctx).Should(BeTrue())
		Expect(receipt.PID).To(Equal(admission.PID), "the admitted publisher must produce the durable witness")
		Expect(receipt.Group).To(Equal(admission.Group))
		Expect(receipt.Migration).To(Equal(admission.Migration))
		Expect(receipt.Source).To(Equal(admission.Source))
		Expect(receipt.Before).To(Equal(admission.Before))
		Expect(receipt.PID).NotTo(Equal(command.Process.Pid), "signal actual selected transaction worker, not inherited-stream waiter")
		Expect(receipt.Group).To(Equal(receipt.PID))
		phase = "durable READY observed"
		Expect(receipt.Root).To(Equal(root))
		Expect(receipt.Phase).To(Equal("applying"))
		Expect(receipt.Applied).To(BeNumerically(">=", 2))
		Expect(receipt.Source).To(BeAnExistingFile())
		Expect(receipt.Before).To(BeAnExistingFile())
		Expect(filepath.Join(root, ".factory/runtime-publication.pending")).To(BeAnExistingFile())
		journalPath := filepath.Join(root, ".factory/installation-transactions", receipt.Migration+".json")
		journalData, err := os.ReadFile(journalPath)
		Expect(err).NotTo(HaveOccurred())
		var journal map[string]any
		Expect(json.Unmarshal(journalData, &journal)).To(Succeed())
		Expect(journal["phase"]).To(Equal("applying"))
		actualApplied := 0
		for _, raw := range journal["entries"].([]any) {
			if raw.(map[string]any)["phase"] == "applied" {
				actualApplied++
			}
		}
		Expect(actualApplied).To(BeNumerically(">=", 2), "READY must follow actual durable publication, not a sleep")
		Expect(syscall.Kill(receipt.PID, syscall.SIGTERM)).To(Succeed())
		select {
		case <-finished:
		case <-time.After(10 * time.Second):
			Fail("selected worker SIGTERM did not return bounded recovery advice")
		}
		var exit *exec.ExitError
		Expect(errors.As(returned, &exit)).To(BeTrue())
		Expect(exit.ExitCode()).To(Equal(1), "%s", stderr.String())
		Expect(stderr.String()).To(ContainSubstring(localUpgradeApplyMarker))
		replay := localUpgradeRecoveryArgs(stderr.String())
		Expect(replay).To(ContainElement("--before-local"))
		Expect(replay).To(ContainElement(receipt.Source))
		Expect(replay).To(ContainElement(receipt.Before))
		relative, err := filepath.Rel(root, replay[0])
		Expect(err).NotTo(HaveOccurred())
		Expect(relative).To(HavePrefix(".." + string(filepath.Separator)))
		localInitPoisonCompiler(f)
		unchanged := installationSnapshot(root)
		// The printed vector chooses reverse by default; require it literally replayable.
		advice := handoffNativeProcess(root, f.environment, "", replay[0], append(append([]string{}, replay[1:]...), "--json")...)
		Expect(advice.status).To(Equal(0), "%+v", advice)
		Expect(installationSnapshot(root)).To(Equal(unchanged))
		handoffProposal(advice)
		for i, arg := range replay {
			if arg == "--direction" {
				replay[i+1] = direction
			}
		}
		recovery := handoffProposal(handoffNativeProcess(root, f.environment, "", replay[0], append(append([]string{}, replay[1:]...), "--json")...))
		Expect(installationSnapshot(root)).To(Equal(unchanged))
		fresh := localUpgradeWithoutPreview(replay[1:])
		complete := handoffNativeProcess(root, f.environment, "", replay[0], append(fresh, "--confirm", recovery["proposal_digest"].(string), "--quiescent")...)
		Expect(complete.status).To(Equal(0), "%+v", complete)
		if direction == "forward" {
			localInitDescriptor(root, fixture.revision)
		} else {
			Expect(os.ReadFile(filepath.Join(root, ".factory/bin/factory-runtime"))).To(Equal(before.runtime))
			Expect(os.ReadFile(filepath.Join(root, ".factory/installation.current"))).To(Equal(originalDescriptor))
		}
		_, err = os.Lstat(filepath.Join(root, ".factory/runtime-publication.pending"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		Expect(os.ReadFile(filepath.Join(root, ".factory/loops.json"))).To(Equal(modern))
		healthy := handoffNativeProcess(root, f.environment, "", filepath.Join(root, "factory"), "selftest")
		Expect(healthy.status).To(Equal(0), "%+v", healthy)
		localInitNoAcquisition(f)
		AddReportEntry("local-upgrade.selected-worker-recovery", map[string]any{"receipt": receipt, "signal": "SIGTERM to selected worker only", "direction": direction, "emitted_argv": localUpgradeRecoveryArgs(stderr.String()), "status": complete.status, "compiler": "poisoned during all recovery"}, ReportEntryVisibilityNever)
	}, Entry("forward", "forward"), Entry("reverse", "reverse"))

	// per docs/adr/0117-explicit-go-local-upgrade.md:78
	It("retains replayable qualified inputs after a genuine preparation-only publication refusal", func() {
		f, driver := localInitSurface(fixture.image)
		legacy := installationCloneLegacyFixture(fixture.image.binary, fixture.legacy)
		root := legacy.root
		path := filepath.Join(root, ".gitignore")
		data, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		exposed := "\n!/.factory/\n/.factory/bin/\n/.factory/assets/\n/.factory/installation.current\n/.factory/installation-transactions/\n/.factory/backups/\n!/.factory/bin/\n"
		writeFixture(path, append(data, []byte(exposed)...), 0600)
		Expect(imageProcess(root, nil, "git", "check-ignore", "--no-index", "-q", "--", ".factory/bin/factory-runtime").status).To(Equal(1))
		args := localUpgradeArgs(fixture.source, "")
		preview := handoffProposal(handoffNativeProcess(root, f.environment, "", driver, append(append([]string{}, args...), "--dry-run", "--json")...))
		Expect(preview["blockers"]).To(BeEmpty())
		refused := handoffNativeProcess(root, f.environment, "", driver, append(args, "--confirm", preview["proposal_digest"].(string), "--quiescent")...)
		Expect(refused.status).To(Equal(2), "%+v", refused)
		Expect(refused.stderr).To(ContainSubstring("not effectively ignored"))
		id := localUpgradeMigration(root)
		data, err = os.ReadFile(filepath.Join(root, ".factory/installation-transactions", id+".json"))
		Expect(err).NotTo(HaveOccurred())
		var record map[string]any
		Expect(json.Unmarshal(data, &record)).To(Succeed())
		Expect(record["phase"]).To(Equal("prepared"))
		replay := localUpgradeRecoveryArgs(refused.stderr)
		localInitPoisonCompiler(f)
		unchanged := installationSnapshot(root)
		out := handoffNativeProcess(root, f.environment, "", replay[0], append(append([]string{}, replay[1:]...), "--json")...)
		Expect(out.status).To(Equal(0), "%+v", out)
		handoffProposal(out)
		Expect(installationSnapshot(root)).To(Equal(unchanged))
		localInitNoAcquisition(f)
		AddReportEntry("local-upgrade.preparation-recovery", map[string]any{"phase": record["phase"], "emitted_argv": replay, "preview_status": out.status}, ReportEntryVisibilityNever)
	})

})
