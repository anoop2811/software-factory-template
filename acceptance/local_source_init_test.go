package acceptance_test

import (
	"bytes"
	"context"
	"crypto/sha256"
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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const localInitSelectedMarker = "INDEPENDENT_SELECTED_LOCAL_INIT_20261030"

// Only developer compilation receives the evaluator's provisioned compiler.
// Private HOME cache links preserve the packager's hermetic environment policy.
// per docs/adr/0116-explicit-go-local-initialization.md:30
func localInitSurface(image handoffReleaseImage) (handoffBootstrapFixture, string) {
	GinkgoHelper()
	f, driver := handoffNativeFixture(image)
	compiler, err := exec.LookPath("go")
	Expect(err).NotTo(HaveOccurred())
	compiler, err = filepath.EvalSymlinks(compiler)
	Expect(err).NotTo(HaveOccurred())
	path := filepath.Join(f.root, "host tools", "go")
	Expect(os.Remove(path)).To(Succeed())
	Expect(os.Symlink(compiler, path)).To(Succeed())
	result := imageProcess(image.work, nil, compiler, "env", "-json", "GOCACHE", "GOMODCACHE")
	Expect(result.status).To(Equal(0), "%+v", result)
	var caches map[string]string
	Expect(json.Unmarshal([]byte(result.stdout), &caches)).To(Succeed())
	build := filepath.Join(f.environment["HOME"], ".cache", "go-build")
	if runtime.GOOS == "darwin" {
		build = filepath.Join(f.environment["HOME"], "Library", "Caches", "go-build")
	}
	for destination, source := range map[string]string{build: caches["GOCACHE"], filepath.Join(f.environment["HOME"], "go", "pkg", "mod"): caches["GOMODCACHE"]} {
		Expect(filepath.IsAbs(source)).To(BeTrue())
		Expect(os.MkdirAll(filepath.Dir(destination), 0700)).To(Succeed())
		Expect(os.Symlink(source, destination)).To(Succeed())
	}
	f.environment["GOTOOLCHAIN"], f.environment["GOPROXY"], f.environment["GOSUMDB"] = "local", "off", "off"
	f.environment["FACTORY_REF"] = "main"
	temporary := filepath.Join(f.root, "local temporary")
	Expect(os.MkdirAll(temporary, 0700)).To(Succeed())
	f.environment["TMPDIR"] = temporary
	cache := filepath.Join(f.root, "foreign persistent cache")
	writeFixture(filepath.Join(cache, "operator-witness"), []byte("private existing factory state\n"), 0600)
	f.environment["FACTORY_RELEASE_DIR"] = cache
	return f, driver
}

func localInitPoisonCompiler(f handoffBootstrapFixture) {
	GinkgoHelper()
	path := filepath.Join(f.root, "host tools", "go")
	Expect(os.Remove(path)).To(Succeed(), "remove the fixture link, never overwrite the real compiler")
	writeFixture(path, []byte("#!/bin/sh\nprintf go > \"$HANDOFF_RECORD_DIR/forbidden-tool\"\nexit 97\n"), 0700)
}

func localInitNoAcquisition(f handoffBootstrapFixture) {
	GinkgoHelper()
	for _, name := range []string{"api", "release-view", "release-download", "trusted-root", "verify", "forbidden-tool"} {
		_, err := os.Lstat(filepath.Join(f.records, name))
		Expect(os.IsNotExist(err)).To(BeTrue(), "no release/model/forbidden-tool call: %s", name)
	}
}

func localInitDescriptor(root, revision string) {
	GinkgoHelper()
	data, err := os.ReadFile(filepath.Join(root, ".factory/installation.current"))
	Expect(err).NotTo(HaveOccurred())
	var descriptor map[string]any
	Expect(json.Unmarshal(data, &descriptor)).To(Succeed())
	Expect(descriptor["kind"]).To(Equal("go-hybrid-v1"))
	Expect(descriptor["version"]).To(Equal("local-" + revision))
	Expect(descriptor["revision"]).To(Equal(revision))
	Expect(descriptor["target"]).To(Equal(runtime.GOOS + "/" + runtime.GOARCH))
	program, err := os.ReadFile(filepath.Join(root, ".factory/bin/factory-runtime"))
	Expect(err).NotTo(HaveOccurred())
	Expect(len(program)).To(BeNumerically(">", 2))
	Expect(string(program[:2])).NotTo(Equal("#!"))
	Expect(descriptor["runtime_sha256"]).To(Equal(fmt.Sprintf("%x", sha256.Sum256(program))))
}

func localInitCompiler(f handoffBootstrapFixture, wrapper, compiler, stage, mode string) string {
	GinkgoHelper()
	path := filepath.Join(f.root, "host tools", "go")
	Expect(os.Remove(path)).To(Succeed())
	data, err := os.ReadFile(wrapper)
	Expect(err).NotTo(HaveOccurred())
	writeFixture(path, data, 0700)
	ready := filepath.Join(f.root, "compiler-ready.json")
	control, err := json.Marshal(localCompilerControl{compiler, stage, mode, ready, "both"})
	Expect(err).NotTo(HaveOccurred())
	writeFixture(filepath.Join(filepath.Dir(path), "control.json"), control, 0600)
	return ready
}

var _ = Describe("Go explicit local source initialization", Ordered, ContinueOnFailure, func() {
	var image handoffReleaseImage
	var source, revision, compiler, wrapper string
	BeforeAll(func() {
		image = handoffBuildImage()
		Expect(string(image.binary)).NotTo(ContainSubstring(localInitSelectedMarker), "the invoking build must predate the selected initializer")
		source = filepath.Join(image.work, "local source 'quoted' $(not-code) with spaces")
		Expect(os.Rename(filepath.Join(image.work, "exact-source"), source)).To(Succeed())
		var err error
		source, err = filepath.EvalSymlinks(source)
		Expect(err).NotTo(HaveOccurred())
		repository, err := filepath.Abs("..")
		Expect(err).NotTo(HaveOccurred())
		imageGit(source, "fetch", "--quiet", "--no-tags", repository, "b71ecc32e07ecd87eb330ba8e497c86612f92acd", "76952eaa63aebd1ecd282f5ab51dd7c3627cb497")
		path := filepath.Join(source, "internal/initcmd/command.go")
		original, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		changed := strings.Replace(string(original), `message := "factory-init:`, `message := "`+localInitSelectedMarker+`\nfactory-init:`, 1)
		Expect(changed).NotTo(Equal(string(original)))
		writeFixture(path, []byte(changed), 0644)
		imageGit(source, "add", "internal/initcmd/command.go")
		imageGit(source, "commit", "--quiet", "-m", "independent selected initializer witness")
		revision = strings.TrimSpace(imageGit(source, "rev-parse", "HEAD"))
		Expect(revision).NotTo(Equal(image.revision))
		Expect(imageGit(source, "status", "--porcelain", "--untracked-files=all")).To(BeEmpty())
		compiler, err = exec.LookPath("go")
		Expect(err).NotTo(HaveOccurred())
		compiler, err = filepath.EvalSymlinks(compiler)
		Expect(err).NotTo(HaveOccurred())
		fixture := strings.Replace(localCompilerProcessFixture, `if stage==c.Stage&&c.Mode=="overflow"{`, `if stage==c.Stage&&c.Mode=="fail"{fmt.Fprintln(os.Stderr,"independent local compiler failure");os.Exit(58)}
 if stage==c.Stage&&c.Mode=="overflow"{`, 1)
		Expect(fixture).NotTo(Equal(localCompilerProcessFixture))
		fixturePath := filepath.Join(image.work, "local-init-compiler.go")
		writeFixture(fixturePath, []byte(fixture), 0600)
		wrapper = filepath.Join(image.work, "local-init-compiler")
		built := imageProcess(image.work, nil, compiler, "build", "-trimpath", "-buildvcs=false", "-o", wrapper, fixturePath)
		Expect(built.status).To(Equal(0), "%+v", built)
		AddReportEntry("local-init.source", map[string]string{"path": source, "revision": revision, "invoker_revision": image.revision}, ReportEntryVisibilityNever)
	})

	// per docs/adr/0116-explicit-go-local-initialization.md:16
	// per docs/adr/0116-explicit-go-local-initialization.md:92
	DescribeTable("runs the selected committed initializer and installs a native local identity without release acquisition", func(explicit bool, literalTarget string) {
		f, driver := localInitSurface(image)
		target := filepath.Join(f.cwd, literalTarget)
		args := []string{"init", "--local", "--pack", "none"}
		if explicit {
			args = append(args, "--source", source)
		} else {
			driver = filepath.Join(source, "factory-go")
			writeFixture(driver, image.binary, 0700)
			Expect(imageGit(source, "status", "--porcelain", "--untracked-files=all")).To(BeEmpty(), "ignored developer executable must not dirty committed input")
		}
		args = append(args, "--", literalTarget)
		before, cache := installationSnapshot(source), installationSnapshot(f.environment["FACTORY_RELEASE_DIR"])
		out := handoffNativeProcess(f.cwd, f.environment, nativeInitAnswers("y"), driver, args...)
		Expect(out.status).To(Equal(0), "%+v", out)
		Expect(out.stdout).To(ContainSubstring(localInitSelectedMarker), "selected source must own initialization, not just the payload")
		Expect(out.stderr).To(ContainSubstring("local-source"))
		Expect(out.stderr).To(ContainSubstring(revision))
		Expect(out.stderr).To(ContainSubstring("no publisher authentication"))
		AddReportEntry("local-init.initializer-result", map[string]any{"args": args, "status": out.status, "stdout": out.stdout, "stderr": out.stderr}, ReportEntryVisibilityNever)
		localInitDescriptor(target, revision)
		Expect(installationSnapshot(source)).To(Equal(before))
		Expect(installationSnapshot(f.environment["FACTORY_RELEASE_DIR"])).To(Equal(cache))
		localInitPoisonCompiler(f)
		ordinary := handoffNativeProcess(target, f.environment, "", filepath.Join(target, "factory"), "selftest")
		Expect(ordinary.status).To(Equal(0), "%+v", ordinary)
		if explicit {
			installedBefore := installationSnapshot(target)
			missingSource := handoffNativeProcess(target, f.environment, "", filepath.Join(target, "factory"), "init", filepath.Join(f.cwd, "another project"), "--local", "--pack", "none")
			Expect(missingSource.status).To(Equal(2), "installed local init requires an explicit source: %+v", missingSource)
			Expect(installationSnapshot(target)).To(Equal(installedBefore))
			pending := filepath.Join(target, ".factory/runtime-publication.pending")
			writeFixture(pending, []byte("independent pending-publication witness\n"), 0600)
			blockedBefore := installationSnapshot(target)
			blocked := handoffNativeProcess(target, f.environment, "", filepath.Join(target, ".factory/bin/factory-runtime"), "--factory-local-init", "1")
			Expect(blocked.status).NotTo(Equal(0), "%+v", blocked)
			Expect(blocked.stderr).To(ContainSubstring("admission"), "installed admission must precede private protocol parsing: %+v", blocked)
			Expect(installationSnapshot(target)).To(Equal(blockedBefore))
		}
		localInitNoAcquisition(f)
		_, err := os.Lstat(filepath.Join(target, ".git"))
		Expect(os.IsNotExist(err)).To(BeTrue(), "initialization keeps its existing no-git-init behavior")
	}, Entry("explicit literal source directory", true, "--go-candidate=literal 'quoted' $(not-code) local project"), Entry("invoked clean developer checkout", false, "--go-candidate"), Entry("literal initializer-option target", true, "--pack"))

	// per docs/adr/0116-explicit-go-local-initialization.md:23
	It("refuses malformed or conflicting local selectors and serves local help without inspecting source or writing a target", func() {
		f, driver := localInitSurface(image)
		localInitPoisonCompiler(f)
		target := filepath.Join(f.cwd, "must remain absent")
		before, cache := installationSnapshot(f.cwd), installationSnapshot(f.environment["FACTORY_RELEASE_DIR"])
		cases := [][]string{
			{"--local", "--local"}, {"--local=true"}, {"--local=false"},
			{"--local", "--source"}, {"--local", "--source="}, {"--local", "--source", ""},
			{"--local", "--source", source, "--source", source}, {"--local", "--source=" + source, "--source=" + source},
			{"--local", "--go-candidate", handoffVersion}, {"--go-candidate", handoffVersion, "--local"},
			{"--local", "--source", source, "--ref", "main"}, {"--local", "--installation"},
			{"--source", source}, {"--source=" + source}, {"--local", "--source", "bad\nsource"},
		}
		receipts := []string{}
		for _, suffix := range cases {
			out := handoffNativeProcess(f.cwd, f.environment, "", driver, append([]string{"init", target}, suffix...)...)
			Expect(out.status).To(Equal(2), "selectors=%q: %+v", suffix, out)
			Expect(out.stdout).NotTo(ContainSubstring("Proceed?"))
			Expect(installationSnapshot(f.cwd)).To(Equal(before))
			Expect(installationSnapshot(f.environment["FACTORY_RELEASE_DIR"])).To(Equal(cache))
			localInitNoAcquisition(f)
			receipts = append(receipts, fmt.Sprintf("%q status=%d", suffix, out.status))
		}
		for _, help := range []string{"--help", "-h"} {
			out := handoffNativeProcess(f.cwd, f.environment, "", driver, "init", target, "--local", "--source", filepath.Join(f.cwd, "missing source"), help)
			Expect(out.status).To(Equal(0), "%+v", out)
			Expect(out.stdout).To(ContainSubstring("--local"))
			Expect(out.stdout).To(ContainSubstring("--source"))
			Expect(installationSnapshot(f.cwd)).To(Equal(before))
			localInitNoAcquisition(f)
		}
		missingInvoker := "factory-independent-missing-invoker-name"
		runMissingInvoker := func(args ...string) cliResult {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			command := handoffCommand(ctx, f.cwd, f.environment, "", driver, args...)
			command.Args[0] = missingInvoker
			var stdout, stderr bytes.Buffer
			command.Stdout, command.Stderr = &stdout, &stderr
			err := command.Run()
			Expect(ctx.Err()).NotTo(HaveOccurred())
			status := 0
			if err != nil {
				var exit *exec.ExitError
				Expect(errors.As(err, &exit)).To(BeTrue(), "%v", err)
				status = exit.ExitCode()
			}
			return cliResult{stdout.String(), stderr.String(), status}
		}
		unresolved := runMissingInvoker("init", target, "--local", "--pack", "none")
		AddReportEntry("local-init.missing-invoker", map[string]any{"argv0": missingInvoker, "status": unresolved.status, "stderr": unresolved.stderr}, ReportEntryVisibilityNever)
		Expect(unresolved.status).To(Equal(1), "a source-built dispatcher lookup failure is operational, not installed-context usage: %+v", unresolved)
		Expect(unresolved.stderr).To(ContainSubstring("locate dispatcher"))
		Expect(unresolved.stderr).To(ContainSubstring(missingInvoker))
		Expect(unresolved.stderr).NotTo(ContainSubstring("installed local init requires"))
		for _, help := range []string{"--help", "-h"} {
			out := runMissingInvoker("init", target, "--local", help)
			Expect(out.status).To(Equal(0), "local help must not resolve the default source: %+v", out)
			Expect(out.stdout).To(ContainSubstring("--local"))
		}
		Expect(installationSnapshot(f.cwd)).To(Equal(before))
		Expect(installationSnapshot(f.environment["FACTORY_RELEASE_DIR"])).To(Equal(cache))
		localInitNoAcquisition(f)
		AddReportEntry("local-init.selector-refusals", receipts, ReportEntryVisibilityNever)
	})

	// per docs/adr/0116-explicit-go-local-initialization.md:16
	It("refuses tracked and untracked dirty source before compilation and preserves the project and foreign cache", func() {
		for _, tracked := range []bool{true, false} {
			f, driver := localInitSurface(image)
			localInitPoisonCompiler(f)
			dirty := filepath.Join(f.root, "dirty source")
			imageGit(image.work, "clone", "--quiet", "--shared", source, dirty)
			if tracked {
				path := filepath.Join(dirty, "go.mod")
				body, err := os.ReadFile(path)
				Expect(err).NotTo(HaveOccurred())
				writeFixture(path, append(body, '\n'), 0644)
			} else {
				writeFixture(filepath.Join(dirty, "operator-untracked.go"), []byte("package operator\n"), 0600)
			}
			target := filepath.Join(f.cwd, "must remain absent")
			before, caller, cache := installationSnapshot(dirty), installationSnapshot(f.cwd), installationSnapshot(f.environment["FACTORY_RELEASE_DIR"])
			out := handoffNativeProcess(f.cwd, f.environment, nativeInitAnswers("y"), driver, "init", target, "--local", "--source", dirty, "--pack", "none")
			Expect(out.status).To(Equal(1), "tracked=%t: %+v", tracked, out)
			Expect(strings.ToLower(out.stderr)).To(Or(ContainSubstring("dirty"), ContainSubstring("uncommitted")))
			Expect(installationSnapshot(dirty)).To(Equal(before))
			Expect(installationSnapshot(f.cwd)).To(Equal(caller))
			Expect(installationSnapshot(f.environment["FACTORY_RELEASE_DIR"])).To(Equal(cache))
			localInitNoAcquisition(f)
		}
	})

	// per docs/adr/0116-explicit-go-local-initialization.md:79
	It("propagates real compiler failure without publishing a target or taking custody of persistent cache", func() {
		f, driver := localInitSurface(image)
		localInitCompiler(f, wrapper, compiler, "build", "fail")
		target := filepath.Join(f.cwd, "must remain absent")
		before, caller, cache := installationSnapshot(source), installationSnapshot(f.cwd), installationSnapshot(f.environment["FACTORY_RELEASE_DIR"])
		out := handoffNativeProcess(f.cwd, f.environment, nativeInitAnswers("y"), driver, "init", target, "--local", "--source", source, "--pack", "none")
		Expect(out.status).To(Equal(1), "%+v", out)
		Expect(out.stderr).To(ContainSubstring("independent local compiler failure"))
		Expect(out.stdout).To(BeEmpty())
		Expect(installationSnapshot(source)).To(Equal(before))
		Expect(installationSnapshot(f.cwd)).To(Equal(caller))
		Expect(installationSnapshot(f.environment["FACTORY_RELEASE_DIR"])).To(Equal(cache))
		localInitNoAcquisition(f)
		entries, err := os.ReadDir(f.environment["TMPDIR"])
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(BeEmpty(), "confirmed failure cleans only its own transient workspaces")
	})

	// per docs/adr/0116-explicit-go-local-initialization.md:70
	DescribeTable("reconciles parent-only signals after an actual compiler descendant holds both captures", func(signal syscall.Signal) {
		f, driver := localInitSurface(image)
		ready := localInitCompiler(f, wrapper, compiler, "version", "hold")
		target := filepath.Join(f.cwd, "must remain absent")
		before, caller, cache := installationSnapshot(source), installationSnapshot(f.cwd), installationSnapshot(f.environment["FACTORY_RELEASE_DIR"])
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		command := handoffCommand(ctx, f.cwd, f.environment, nativeInitAnswers("y"), driver, "init", target, "--local", "--source", source, "--pack", "none")
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
		Expect(command.Start()).To(Succeed())
		finished := make(chan error, 1)
		go func() { finished <- command.Wait() }()
		DeferCleanup(func() { _ = command.Process.Kill() })
		Eventually(func() bool {
			select {
			case err := <-finished:
				StopTrying(fmt.Sprintf("local init exited before compiler READY: error=%v stdout=%q stderr=%q", err, stdout.String(), stderr.String())).Now()
			default:
			}
			_, err := os.Stat(ready)
			return err == nil
		}, 30*time.Second).Should(BeTrue())
		receipt := localCompilerReadiness(ready, "version", time.Second)
		DeferCleanup(localCompilerCleanup, receipt)
		Expect(receipt.Group).To(Equal(receipt.Leader), "compiler must have its own supervised process group")
		AddReportEntry("local-init.compiler-ready", receipt, ReportEntryVisibilityNever)
		Expect(command.Process.Signal(signal)).To(Succeed(), "signal only the CLI parent PID")
		var returned error
		select {
		case returned = <-finished:
		case <-time.After(6 * time.Second):
			localCompilerCleanup(receipt)
			_ = command.Process.Kill()
			Eventually(finished, 6*time.Second).Should(Receive(&returned))
			Fail("local init remained blocked after parent-only cancellation")
		}
		Expect(returned).To(HaveOccurred())
		Expect(command.ProcessState.ExitCode()).To(Equal(1), "%s", stderr.String())
		Expect(stdout.String()).To(BeEmpty())
		uncertain := strings.Contains(stderr.String(), "ownership is unconfirmed")
		AddReportEntry("local-init.cancellation", map[string]any{"signal": signal.String(), "ownership_unconfirmed": uncertain, "owned_process_alive": localCompilerAlive(receipt), "stderr": stderr.String()}, ReportEntryVisibilityNever)
		if localCompilerAlive(receipt) {
			Expect(uncertain).To(BeTrue(), "surviving owned processes require actionable ownership uncertainty")
		}
		entries, err := os.ReadDir(f.environment["TMPDIR"])
		Expect(err).NotTo(HaveOccurred())
		if uncertain {
			Expect(entries).NotTo(BeEmpty())
			Expect(stderr.String()).To(ContainSubstring(f.environment["TMPDIR"]))
		} else {
			Expect(entries).To(BeEmpty())
			Expect(localCompilerAlive(receipt)).To(BeFalse())
		}
		Expect(installationSnapshot(source)).To(Equal(before))
		Expect(installationSnapshot(f.cwd)).To(Equal(caller))
		Expect(installationSnapshot(f.environment["FACTORY_RELEASE_DIR"])).To(Equal(cache))
		localInitNoAcquisition(f)
	}, Entry("SIGINT", syscall.SIGINT), Entry("SIGTERM", syscall.SIGTERM))

	// per docs/adr/0116-explicit-go-local-initialization.md:36
	It("retains ordinary source initialization and authenticated candidate routing with the compiler poisoned", func() {
		ordinarySource, target, environment := nativeInitFixture()
		for _, entry := range environment {
			if value, ok := strings.CutPrefix(entry, "PATH="); ok {
				writeFixture(filepath.Join(strings.Split(value, string(os.PathListSeparator))[0], "go"), []byte("#!/bin/sh\nexit 97\n"), 0700)
			}
		}
		ordinary := nativeInitRun(ordinarySource, target, nativeInitAnswers("y"), environment, []string{"--pack", "none"})
		Expect(ordinary.status).To(Equal(0), "%+v", ordinary)
		Expect(ordinary.stdout).NotTo(ContainSubstring(localInitSelectedMarker))
		_, err := os.Lstat(filepath.Join(target, ".factory/installation.current"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		f, driver := handoffNativeFixture(image)
		candidateTarget := filepath.Join(f.cwd, "candidate project")
		candidate := handoffNativeProcess(f.cwd, f.environment, nativeInitAnswers("y"), driver, "init", candidateTarget, "--go-candidate", handoffVersion, "--pack", "none")
		Expect(candidate.status).To(Equal(0), "%+v", candidate)
		body, err := os.ReadFile(filepath.Join(candidateTarget, ".factory/installation.current"))
		Expect(err).NotTo(HaveOccurred())
		var descriptor map[string]any
		Expect(json.Unmarshal(body, &descriptor)).To(Succeed())
		Expect(descriptor["version"]).To(Equal(handoffVersion))
		Expect(descriptor["revision"]).To(Equal(image.revision))
		Expect(candidate.stdout).NotTo(ContainSubstring(localInitSelectedMarker))
		_, err = os.Stat(filepath.Join(f.records, "forbidden-tool"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	// per docs/adr/0116-explicit-go-local-initialization.md:54
	// per docs/adr/0116-explicit-go-local-initialization.md:96
	It("accepts the exact private local image handoff and refuses malformed or mismatched custody without rebuild or foreign-cache removal", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		cache := filepath.Join(image.work, "evaluator local cache")
		Expect(os.MkdirAll(cache, 0700)).To(Succeed())
		var physicalErr error
		cache, physicalErr = filepath.EvalSymlinks(cache)
		Expect(physicalErr).NotTo(HaveOccurred())
		writeFixture(filepath.Join(cache, "operator-witness"), []byte("caller-owned cache witness\n"), 0600)
		built, err := packaging.Build(ctx, packaging.Options{Source: source, Revision: revision, Target: runtime.GOOS + "/" + runtime.GOARCH, Output: filepath.Join(cache, "package"), Version: "local-" + revision, Compiler: compiler, Installation: true})
		Expect(err).NotTo(HaveOccurred())
		archive, err := os.ReadFile(built.Archive)
		Expect(err).NotTo(HaveOccurred())
		entries, _ := imageDecodeArchive(archive)
		var selected []byte
		for _, entry := range entries {
			if strings.HasSuffix(entry.name, "/bin/factory-runtime") {
				selected = entry.body
			}
		}
		Expect(selected).NotTo(BeEmpty())
		selectedDriver := filepath.Join(image.work, "exact selected executable")
		writeFixture(selectedDriver, selected, 0700)
		f, invoker := handoffNativeFixture(image)
		target := filepath.Join(f.cwd, "private protocol target")
		caller := installationSnapshot(f.cwd)
		args := []string{"--factory-local-init", "1", cache, source, revision, built.SHA256, "--", target, "--pack", "none"}
		defaultCwd := filepath.Join(f.root, "private default target cwd")
		literalCwd := filepath.Join(f.root, "private literal target cwd")
		Expect(os.MkdirAll(defaultCwd, 0700)).To(Succeed())
		Expect(os.MkdirAll(literalCwd, 0700)).To(Succeed())
		defaultPhysical, err := filepath.EvalSymlinks(defaultCwd)
		Expect(err).NotTo(HaveOccurred())
		literalPhysical, err := filepath.EvalSymlinks(literalCwd)
		Expect(err).NotTo(HaveOccurred())
		positive := []struct {
			name, cwd, target string
			args              []string
		}{
			{"explicit target", f.cwd, target, args},
			{"trailing delimiter default target", defaultCwd, defaultPhysical, append(append([]string{}, args[:7]...), "--pack", "none", "--")},
			{"known initializer option as literal target", literalCwd, filepath.Join(literalPhysical, "--java-build-tool=literal"), append(append([]string{}, args[:7]...), "--pack", "none", "--", "--java-build-tool=literal")},
		}
		positiveReceipts := []string{}
		for _, test := range positive {
			before := installationSnapshot(test.cwd)
			declined := handoffNativeProcess(test.cwd, f.environment, nativeInitAnswers("n"), selectedDriver, test.args...)
			Expect(declined.status).To(Equal(1), "%s: %+v", test.name, declined)
			Expect(declined.stdout).To(ContainSubstring("Aborted."))
			Expect(declined.stdout).To(ContainSubstring("Target dir:   "+test.target+"\n"), "%s must preserve the exact intended target: %+v", test.name, declined)
			Expect(installationSnapshot(test.cwd)).To(Equal(before))
			localInitNoAcquisition(f)
			positiveReceipts = append(positiveReceipts, fmt.Sprintf("%s target=%q status=%d", test.name, test.target, declined.status))
		}
		AddReportEntry("local-init.private-target-declines", positiveReceipts, ReportEntryVisibilityNever)
		Expect(installationSnapshot(f.cwd)).To(Equal(caller))
		witness, err := os.ReadFile(filepath.Join(cache, "operator-witness"))
		Expect(err).NotTo(HaveOccurred())
		foreign := filepath.Join(image.work, "foreign private cache")
		writeFixture(filepath.Join(foreign, "operator-witness"), witness, 0600)
		foreignBefore := installationSnapshot(foreign)
		cases := []struct {
			name, program string
			args          []string
			diagnostic    string
		}{
			{"missing protocol fields", selectedDriver, []string{"--factory-local-init", "1"}, "invalid private local init protocol"},
			{"unsupported protocol", selectedDriver, append([]string{"--factory-local-init", "2"}, args[2:]...), "invalid private local init protocol"},
			{"foreign cache root", selectedDriver, append(append([]string{}, args[:2]...), append([]string{foreign}, args[3:]...)...), "local archive custody"},
			{"missing physical source", selectedDriver, append(append([]string{}, args[:3]...), append([]string{filepath.Join(image.work, "missing source")}, args[4:]...)...), "local source identity"},
			{"archive digest mismatch", selectedDriver, append(append([]string{}, args[:5]...), append([]string{strings.Repeat("0", 64)}, args[6:]...)...), "local archive digest mismatch"},
			{"actual self mismatch", invoker, append([]string{}, args...), "selected initializer digest mismatch"},
		}
		receipts := []string{}
		for _, test := range cases {
			out := handoffNativeProcess(f.cwd, f.environment, nativeInitAnswers("y"), test.program, test.args...)
			Expect(out.status).NotTo(Equal(0), "%s: %+v", test.name, out)
			Expect(out.stderr).To(ContainSubstring(test.diagnostic), "%s must reach its own validation phase: %+v", test.name, out)
			Expect(out.stdout).NotTo(ContainSubstring("Proceed?"))
			Expect(installationSnapshot(f.cwd)).To(Equal(caller))
			Expect(installationSnapshot(foreign)).To(Equal(foreignBefore))
			Expect(os.ReadFile(filepath.Join(cache, "operator-witness"))).To(Equal(witness))
			Expect(os.ReadFile(built.Archive)).To(Equal(archive))
			localInitNoAcquisition(f)
			receipts = append(receipts, fmt.Sprintf("%s status=%d", test.name, out.status))
		}
		AddReportEntry("local-init.private-handoff-refusals", receipts, ReportEntryVisibilityNever)
	})
})
