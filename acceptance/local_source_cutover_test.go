package acceptance_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Only an explicitly local developer operation receives the harness compiler.
// Installed commands below restore the compiler poison before execution.
// per docs/adr/0108-default-go-cutover.md:53
func localSourceSurface(image handoffReleaseImage) (handoffBootstrapFixture, string) {
	GinkgoHelper()
	f, driver := handoffNativeFixture(image)
	compiler, err := exec.LookPath("go")
	Expect(err).NotTo(HaveOccurred())
	compiler, err = filepath.EvalSymlinks(compiler)
	Expect(err).NotTo(HaveOccurred())
	info, err := os.Stat(compiler)
	Expect(err).NotTo(HaveOccurred())
	Expect(info.Mode().IsRegular()).To(BeTrue())
	goPath := filepath.Join(f.root, "host tools", "go")
	Expect(os.Remove(goPath)).To(Succeed())
	Expect(os.Symlink(compiler, goPath)).To(Succeed())
	cache := imageProcess(image.work, nil, compiler, "env", "-json", "GOCACHE", "GOMODCACHE")
	Expect(cache.status).To(Equal(0), "%+v", cache)
	var directories map[string]string
	Expect(json.Unmarshal([]byte(cache.stdout), &directories)).To(Succeed())
	for _, key := range []string{"GOCACHE", "GOMODCACHE"} {
		Expect(filepath.IsAbs(directories[key])).To(BeTrue())
		f.environment[key] = directories[key]
	}
	// The hermetic packager deliberately ignores ambient GO* variables. Provision
	// the private developer HOME's standard cache locations instead of weakening it.
	// per docs/adr/0108-default-go-cutover.md:73
	buildCache := filepath.Join(f.environment["HOME"], ".cache", "go-build")
	if runtime.GOOS == "darwin" {
		buildCache = filepath.Join(f.environment["HOME"], "Library", "Caches", "go-build")
	}
	for destination, cache := range map[string]string{
		buildCache: directories["GOCACHE"],
		filepath.Join(f.environment["HOME"], "go", "pkg", "mod"): directories["GOMODCACHE"],
	} {
		Expect(os.MkdirAll(filepath.Dir(destination), 0700)).To(Succeed())
		Expect(os.Symlink(cache, destination)).To(Succeed())
	}
	f.environment["GOTOOLCHAIN"], f.environment["GOPROXY"], f.environment["GOSUMDB"] = "local", "off", "off"
	f.environment["FACTORY_REF"] = "main"
	return f, driver
}

// per docs/adr/0108-default-go-cutover.md:73
func localSourceNoNetwork(f handoffBootstrapFixture) {
	GinkgoHelper()
	for _, name := range []string{"api", "release-view", "release-download", "trusted-root", "verify", "forbidden-tool"} {
		_, err := os.Lstat(filepath.Join(f.records, name))
		Expect(os.IsNotExist(err)).To(BeTrue(), "local-source cannot imply publisher authentication or invoke a forbidden tool: %s", name)
	}
}

// per docs/adr/0108-default-go-cutover.md:47
func localSourcePoisonCompiler(f handoffBootstrapFixture) {
	GinkgoHelper()
	path := filepath.Join(f.root, "host tools", "go")
	Expect(os.Remove(path)).To(Succeed(), "remove only the fixture symlink before writing a poison, preserving the actual compiler")
	writeFixture(path, []byte("#!/bin/sh\nprintf go > \"$HANDOFF_RECORD_DIR/forbidden-tool\"\nexit 97\n"), 0700)
}

// per docs/adr/0108-default-go-cutover.md:70
func localSourceInstalled(root, revision string) {
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
	Expect(string(program[:2])).NotTo(Equal("#!"), "the installed implementation must be a native executable")
}

// per docs/adr/0108-default-go-cutover.md:63
func localSourceUpgradeArgs(source string) []string {
	args := []string{"upgrade", "--local", "--source", source}
	for _, value := range installationInputs {
		args = append(args, "--project-input", value)
	}
	return args
}

// per docs/adr/0108-default-go-cutover.md:80
func localSourceAdvice(output, heading string) string {
	GinkgoHelper()
	_, remaining, found := strings.Cut(output, heading+"\n")
	Expect(found).To(BeTrue(), "a concrete command must follow %q: %s", heading, output)
	command := strings.TrimSpace(strings.SplitN(remaining, "\n", 2)[0])
	Expect(command).NotTo(BeEmpty())
	return command
}

// Shell performs only word expansion here; set never executes the printed
// program or its confirmation arguments. The fixture source path is controlled.
// per docs/adr/0108-default-go-cutover.md:80
func localSourceCommandOperands(f handoffBootstrapFixture, root, command string) []string {
	GinkgoHelper()
	writeFixture(filepath.Join(f.root, "host tools", "not-code"), []byte("#!/bin/sh\nprintf evaluated > \"$HANDOFF_RECORD_DIR/forbidden-tool\"\n"), 0700)
	consumer := handoffNativeProcess(root, f.environment, "", "/bin/sh", "-c", "set -- "+command+"\nprintf '%s\\000' \"$@\"")
	Expect(consumer.status).To(Equal(0), "%+v", consumer)
	Expect(consumer.stderr).To(BeEmpty())
	operands := strings.Split(strings.TrimSuffix(consumer.stdout, "\x00"), "\x00")
	Expect(len(operands)).To(BeNumerically(">", 1))
	return operands
}

// Synthetic release identities are hermetic authenticated-transport fixtures,
// never a claim that these tags were published.
// per docs/adr/0108-default-go-cutover.md:28
func cutoverReleaseAssets(f handoffBootstrapFixture, image handoffReleaseImage, version string) {
	GinkgoHelper()
	writeFixture(filepath.Join(f.environment["HANDOFF_ASSETS"], "factory-bootstrap-"+version+"-"+f.target), image.binary, 0600)
	writeFixture(filepath.Join(f.environment["HANDOFF_ASSETS"], "factory-installation-"+version+"-"+f.target+".tar.gz"), handoffImageVersion(image, version), 0600)
}

var _ = Describe("Go local source cutover", Ordered, ContinueOnFailure, func() {
	var image handoffReleaseImage
	var source string
	var prepared installationFixtureData
	BeforeAll(func() {
		image = handoffBuildImage()
		source = filepath.Join(image.work, "local source $(not-code) with spaces")
		Expect(os.Rename(filepath.Join(image.work, "exact-source"), source)).To(Succeed())
		physical, err := filepath.EvalSymlinks(source)
		Expect(err).NotTo(HaveOccurred())
		source = physical
		// The canonical packager reads these immutable recognition objects; the
		// isolated fixture's new source commit does not carry historical ancestry.
		// per docs/adr/0108-default-go-cutover.md:128
		repository, err := filepath.Abs("..")
		Expect(err).NotTo(HaveOccurred())
		imageGit(source, "fetch", "--quiet", "--no-tags", repository, "b71ecc32e07ecd87eb330ba8e497c86612f92acd", "76952eaa63aebd1ecd282f5ab51dd7c3627cb497")
		for _, revision := range []string{"b71ecc32e07ecd87eb330ba8e497c86612f92acd", "76952eaa63aebd1ecd282f5ab51dd7c3627cb497"} {
			imageGit(source, "cat-file", "-e", revision+"^{tree}")
		}
		Expect(strings.TrimSpace(imageGit(source, "rev-parse", "HEAD"))).To(Equal(image.revision))
		prepared = installationLegacyFixture(image.binary, image.references["v0.1.6"], image.archivePath, image.revision)
	})

	// per docs/adr/0108-default-go-cutover.md:62
	// per docs/adr/0108-default-go-cutover.md:67
	DescribeTable("packages committed local source into a qualified Go installation without network acquisition", func(explicit bool) {
		f, driver := localSourceSurface(image)
		target := filepath.Join(f.cwd, "literal $(not-code) local project")
		args := []string{"init", target, "--local", "--pack", "none"}
		if explicit {
			args = append(args, "--source", source)
		} else {
			driver = filepath.Join(source, "factory-go")
			writeFixture(driver, image.binary, 0700)
			Expect(imageGit(source, "status", "--porcelain", "--untracked-files=all")).To(BeEmpty(), "the colocated developer executable must remain ignored")
		}
		result := handoffNativeProcess(f.cwd, f.environment, nativeInitAnswers("y"), driver, args...)
		Expect(result.status).To(Equal(0), "%+v", result)
		Expect(result.stderr).To(ContainSubstring("local-source"))
		Expect(result.stderr).To(ContainSubstring(image.revision))
		localSourceInstalled(target, image.revision)
		localSourcePoisonCompiler(f)
		ordinary := handoffNativeProcess(target, f.environment, "", filepath.Join(target, "factory"), "selftest")
		Expect(ordinary.status).To(Equal(0), "%+v", ordinary)
		localSourceNoNetwork(f)
		_, err := os.Lstat(filepath.Join(target, ".git"))
		Expect(os.IsNotExist(err)).To(BeTrue(), "local init retains the existing no-git-init behavior")
	}, Entry("explicit source directory", true), Entry("invoked developer checkout", false))

	// per docs/adr/0108-default-go-cutover.md:63
	// per docs/adr/0108-default-go-cutover.md:80
	It("uses a fresh whole proposal and quiescent consent for a real v0.1.6 local upgrade", func() {
		f, driver := localSourceSurface(image)
		legacy := installationCloneLegacyFixture(image.binary, prepared)
		application := []byte("operator application and history\n")
		writeFixture(filepath.Join(legacy.root, "application.txt"), application, 0644)
		writeFixture(filepath.Join(legacy.root, "scripts/operator-custom.sh"), []byte("#!/bin/sh\nprintf '%s\\n' operator\n"), 0755)
		imageGit(legacy.root, "add", "--all")
		imageGit(legacy.root, "commit", "--quiet", "-m", "local cutover adopter history")
		head, index := imageGit(legacy.root, "rev-parse", "HEAD"), imageGit(legacy.root, "ls-files", "--stage", "-z")
		config, err := os.ReadFile(filepath.Join(legacy.root, "factory.yaml"))
		Expect(err).NotTo(HaveOccurred())
		args := localSourceUpgradeArgs(source)
		before := installationSnapshot(legacy.root)
		proposal := handoffProposal(handoffNativeProcess(legacy.root, f.environment, "", driver, append(args, "--dry-run", "--json")...))
		Expect(proposal["authentication"]).To(Equal("local-source"))
		Expect(proposal["profile"]).To(Equal("v0.1.6"))
		Expect(proposal["blockers"]).To(BeEmpty())
		Expect(installationSnapshot(legacy.root)).To(Equal(before))
		confirmed := append(append([]string(nil), args...), "--confirm", proposal["proposal_digest"].(string))
		missingQuiescence := handoffNativeProcess(legacy.root, f.environment, "", driver, confirmed...)
		Expect(missingQuiescence.status).To(Equal(2))
		Expect(installationSnapshot(legacy.root)).To(Equal(before), "digest alone cannot authorize mutation")
		result := handoffNativeProcess(legacy.root, f.environment, "", driver, append(confirmed, "--quiescent")...)
		Expect(result.status).To(Equal(0), "%+v", result)
		localSourceInstalled(legacy.root, image.revision)
		Expect(os.ReadFile(filepath.Join(legacy.root, "application.txt"))).To(Equal(application))
		Expect(os.ReadFile(filepath.Join(legacy.root, "factory.yaml"))).To(Equal(config))
		Expect(imageGit(legacy.root, "rev-parse", "HEAD")).To(Equal(head))
		Expect(imageGit(legacy.root, "ls-files", "--stage", "-z")).To(Equal(index))
		ids := retentionBackupIDs(legacy.root)
		Expect(ids).To(HaveLen(1))
		saved := filepath.Join(legacy.root, ".factory/backups", ids[0], "assets/factory")
		info, err := os.Stat(saved)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode().Perm()&0111).To(BeZero(), "the retained legacy executable must be inert")
		ignored := imageProcess(legacy.root, nil, "git", "check-ignore", "--no-index", "--", saved)
		Expect(ignored.status).To(Equal(0), "%+v", ignored)
		localSourcePoisonCompiler(f)
		ordinary := handoffNativeProcess(legacy.root, f.environment, "", filepath.Join(legacy.root, "factory"), "selftest")
		Expect(ordinary.status).To(Equal(0), "%+v", ordinary)
		localSourceNoNetwork(f)
	})

	// per docs/adr/0108-default-go-cutover.md:72
	It("refuses tracked dirty source without presenting it as a committed installation", func() {
		f, driver := localSourceSurface(image)
		dirty := filepath.Join(f.root, "dirty source")
		clone := imageProcess(image.work, nil, "git", "clone", "--quiet", "--no-hardlinks", source, dirty)
		Expect(clone.status).To(Equal(0), "%+v", clone)
		module, err := os.ReadFile(filepath.Join(dirty, "go.mod"))
		Expect(err).NotTo(HaveOccurred())
		writeFixture(filepath.Join(dirty, "go.mod"), append(module, '\n'), 0644)
		target := filepath.Join(f.cwd, "must remain absent")
		before := installationSnapshot(f.cwd)
		result := handoffNativeProcess(f.cwd, f.environment, nativeInitAnswers("y"), driver, "init", target, "--local", "--source", dirty, "--pack", "none")
		Expect(result.status).NotTo(Equal(0), "%+v", result)
		Expect(strings.ToLower(result.stderr)).To(Or(ContainSubstring("dirty"), ContainSubstring("uncommitted")))
		Expect(installationSnapshot(f.cwd)).To(Equal(before))
		localSourceNoNetwork(f)
	})

	// per docs/adr/0108-default-go-cutover.md:28
	It("reuses a validated installed release for ordinary initialization without a compiler", func() {
		f, driver := handoffNativeFixture(image)
		defaultReleasePolicyMetadata(f)
		f.environment["HANDOFF_METADATA_PRERELEASE"] = "false"
		const version = "v0.2.1"
		cutoverReleaseAssets(f, image, version)
		first, second := filepath.Join(f.cwd, "installed parent"), filepath.Join(f.cwd, "another governed project")
		initial := handoffNativeProcess(f.cwd, f.environment, nativeInitAnswers("y"), driver, "init", first, "--go-candidate", version, "--pack", "none")
		Expect(initial.status).To(Equal(0), "%+v", initial)
		result := handoffNativeProcess(first, f.environment, nativeInitAnswers("y"), filepath.Join(first, "factory"), "init", second, "--pack", "none")
		Expect(result.status).To(Equal(0), "%+v", result)
		data, err := os.ReadFile(filepath.Join(second, ".factory/installation.current"))
		Expect(err).NotTo(HaveOccurred())
		var descriptor map[string]any
		Expect(json.Unmarshal(data, &descriptor)).To(Succeed())
		Expect(descriptor["version"]).To(Equal(version), "installed init must not silently choose the prepared v0.2.0 target")
		Expect(descriptor["revision"]).To(Equal(image.revision))
		_, err = os.Lstat(filepath.Join(f.records, "forbidden-tool"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	// per docs/adr/0108-default-go-cutover.md:32
	It("refuses an implicit default downgrade from a newer validated Go installation", func() {
		f, driver := handoffNativeFixture(image)
		defaultReleasePolicyMetadata(f)
		f.environment["HANDOFF_METADATA_PRERELEASE"] = "false"
		cutoverReleaseAssets(f, image, "v0.3.0")
		cutoverReleaseAssets(f, image, "v0.2.0")
		target := filepath.Join(f.cwd, "newer installed release")
		initial := handoffNativeProcess(f.cwd, f.environment, nativeInitAnswers("y"), driver, "init", target, "--go-candidate", "v0.3.0", "--pack", "none")
		Expect(initial.status).To(Equal(0), "%+v", initial)
		data, err := os.ReadFile(filepath.Join(target, ".factory/installation.current"))
		Expect(err).NotTo(HaveOccurred())
		var descriptor map[string]any
		Expect(json.Unmarshal(data, &descriptor)).To(Succeed())
		Expect(descriptor["version"]).To(Equal("v0.3.0"))
		before := installationSnapshot(target)
		result := handoffNativeProcess(target, f.environment, "", filepath.Join(target, "factory"), "upgrade", "--dry-run", "--json")
		Expect(result.status).To(Equal(2), "an available older prepared release cannot become an implicit downgrade: status=%d stderr=%s", result.status, result.stderr)
		Expect(strings.ToLower(result.stderr)).To(Or(ContainSubstring("downgrade"), ContainSubstring("newer")))
		Expect(result.stdout).NotTo(ContainSubstring("proposal_digest"))
		Expect(installationSnapshot(target)).To(Equal(before))
		_, err = os.Lstat(filepath.Join(f.records, "forbidden-tool"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	// per docs/adr/0108-default-go-cutover.md:63
	// per docs/adr/0108-default-go-cutover.md:71
	It("prints a replayable local-source apply command with its literal source path", func() {
		f, driver := localSourceSurface(image)
		legacy := installationCloneLegacyFixture(image.binary, prepared)
		args := localSourceUpgradeArgs(source)
		before := installationSnapshot(legacy.root)
		jsonPreview := handoffProposal(handoffNativeProcess(legacy.root, f.environment, "", driver, append(args, "--dry-run", "--json")...))
		text := handoffNativeProcess(legacy.root, f.environment, "", driver, append(args, "--dry-run")...)
		Expect(text.status).To(Equal(0), "%+v", text)
		Expect(text.stdout).To(ContainSubstring("local-source"))
		Expect(text.stdout).NotTo(ContainSubstring("network acquisition"))
		command := localSourceAdvice(text.stdout, "Apply with:")
		Expect(command).NotTo(ContainSubstring("curl"))
		operands := localSourceCommandOperands(f, legacy.root, command)
		invoker, err := filepath.EvalSymlinks(driver)
		Expect(err).NotTo(HaveOccurred())
		Expect(filepath.IsAbs(operands[0])).To(BeTrue())
		Expect(operands[0]).To(Equal(invoker), "apply advice must retain the actual selected Go invoker")
		program := operands[0]
		operands = operands[1:]
		Expect(operands[0]).To(Equal("upgrade"))
		Expect(operands).To(ContainElement("--local"))
		Expect(operands).NotTo(ContainElement("--go-candidate"))
		Expect(handoffFlag(operands, "--source")).To(Equal(source))
		Expect(handoffFlag(operands, "--confirm")).To(Equal(jsonPreview["proposal_digest"]))
		Expect(operands).To(ContainElement("--quiescent"))
		var inputs []string
		for index, operand := range operands {
			if operand == "--project-input" && index+1 < len(operands) {
				inputs = append(inputs, operands[index+1])
			}
		}
		Expect(inputs).To(ConsistOf(installationInputs), "apply advice must retain every explicit transformation input")
		var readonly []string
		for index := 0; index < len(operands); index++ {
			if operands[index] == "--confirm" {
				index++
				continue
			}
			if operands[index] != "--quiescent" {
				readonly = append(readonly, operands[index])
			}
		}
		replayed := handoffProposal(handoffNativeProcess(legacy.root, f.environment, "", program, append(readonly, "--dry-run", "--json")...))
		Expect(replayed["proposal_digest"]).To(Equal(jsonPreview["proposal_digest"]))
		Expect(replayed["authentication"]).To(Equal("local-source"))
		Expect(installationSnapshot(legacy.root)).To(Equal(before))
		localSourceNoNetwork(f)
	})

	// per docs/adr/0108-default-go-cutover.md:70
	// per docs/adr/0101-go-release-handoff.md:579
	It("runs its printed read-only local recovery command against the actual retained journal", func() {
		f, driver := localSourceSurface(image)
		legacy := installationCloneLegacyFixture(image.binary, prepared)
		ignore, err := os.ReadFile(filepath.Join(legacy.root, ".gitignore"))
		Expect(err).NotTo(HaveOccurred())
		ignore = append(ignore, []byte("\n!/.factory/\n/.factory/bin/\n/.factory/assets/\n/.factory/installation.current\n/.factory/installation-transactions/\n/.factory/backups/\n!/.factory/bin/\n")...)
		writeFixture(filepath.Join(legacy.root, ".gitignore"), ignore, 0644)
		actualIgnore := imageProcess(legacy.root, nil, "git", "check-ignore", "--no-index", "-q", "--", ".factory/bin/factory-runtime")
		Expect(actualIgnore.status).To(Equal(1), "the explicit final negation must expose the private runtime")
		args := localSourceUpgradeArgs(source)
		proposal := handoffProposal(handoffNativeProcess(legacy.root, f.environment, "", driver, append(args, "--dry-run", "--json")...))
		Expect(proposal["blockers"]).To(BeEmpty())
		refused := handoffNativeProcess(legacy.root, f.environment, "", driver, append(args, "--confirm", proposal["proposal_digest"].(string), "--quiescent")...)
		Expect(refused.status).To(Equal(2), "%+v", refused)
		Expect(refused.stderr).To(ContainSubstring("not effectively ignored"))
		journals, err := filepath.Glob(filepath.Join(legacy.root, ".factory/installation-transactions/*.json"))
		Expect(err).NotTo(HaveOccurred())
		Expect(journals).To(HaveLen(1))
		body, err := os.ReadFile(journals[0])
		Expect(err).NotTo(HaveOccurred())
		var journal map[string]any
		Expect(json.Unmarshal(body, &journal)).To(Succeed())
		Expect(journal["phase"]).To(Equal("prepared"))
		command := localSourceAdvice(refused.stderr, "Recovery preview (no changes):")
		before := installationSnapshot(legacy.root)
		localSourcePoisonCompiler(f)
		preview := handoffNativeProcess(legacy.root, f.environment, "", "/bin/sh", "-c", command)
		Expect(preview.status).To(Or(Equal(0), Equal(2)), "%+v", preview)
		Expect(preview.stderr).To(BeEmpty())
		Expect(preview.stdout).NotTo(BeEmpty(), "the printed read-only command must inspect the actual retained transaction")
		Expect(installationSnapshot(legacy.root)).To(Equal(before))
		operands := localSourceCommandOperands(f, legacy.root, command)
		Expect(operands).To(ContainElement("--dry-run"))
		jsonPreview := handoffNativeProcess(legacy.root, f.environment, "", operands[0], append(operands[1:], "--json")...)
		Expect(jsonPreview.status).To(Or(Equal(0), Equal(2)), "%+v", jsonPreview)
		decoded := installationDecodeProposal(jsonPreview)
		Expect(decoded["profile"]).To(Equal("v0.1.6"))
		Expect(command).To(ContainSubstring("--local"))
		Expect(command).NotTo(ContainSubstring("--attestation"))
		Expect(command).NotTo(ContainSubstring("--trusted-root"))
		Expect(installationSnapshot(legacy.root)).To(Equal(before), "the printed command must perform actual read-only recovery, not replay the transaction")
		localSourceNoNetwork(f)
	})
})
