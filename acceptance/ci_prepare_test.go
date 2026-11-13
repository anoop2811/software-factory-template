package acceptance_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// per docs/adr/0143-ci-checkout-preparation.md:29
// per docs/adr/0143-ci-checkout-preparation.md:178
var _ = Describe("Whole installation upgrade release candidate handoff CI checkout preparation", SpecPriority(0), Label("handoff-ci-surface"), func() {
	// per docs/adr/0143-ci-checkout-preparation.md:29
	// per docs/adr/0143-ci-checkout-preparation.md:37
	It("owns help, exact selectors and explicit disposable-checkout consent before acquisition", func() {
		root, cwd, marker := handoffSurfaceFixture()
		driver := filepath.Join(root, "factory")
		env := map[string]string{"PATH": filepath.Join(root, "tools") + string(os.PathListSeparator) + os.Getenv("PATH"), "HANDOFF_EXECUTION_MARKER": marker, "FACTORY_BRIDGE_PROTOCOL": "", "CI": "1"}
		before := nativeInitArtifacts(cwd)
		for _, args := range [][]string{{"ci", "--help"}, {"ci", "prepare", "--help"}, {"ci", "prepare", "--go-candidate", "v9.8.0", "--origin-target", runtime.GOOS + "/" + runtime.GOARCH, "--ephemeral", "--help"}} {
			out := imageProcess(cwd, env, driver, args...)
			Expect(out.status).To(BeZero(), "help must be owned by the native CI command: %+v", out)
			Expect(out.stdout).To(ContainSubstring("prepare"))
			Expect(out.stdout).To(ContainSubstring("--ephemeral"))
			Expect(out.stderr).To(BeEmpty())
		}
		target := runtime.GOOS + "/" + runtime.GOARCH
		valid := []string{"--go-candidate", handoffVersion, "--origin-target", target, "--ephemeral"}
		// per docs/adr/0143-ci-checkout-preparation.md:188
		cases := []struct {
			args   []string
			reason string
		}{
			{nil, "missing CI consent or selection"},
			{[]string{"--go-candidate", handoffVersion, "--origin-target", target}, "missing CI consent or selection"},
			{[]string{"--go-candidate", handoffVersion, "--ephemeral"}, "missing CI consent or selection"},
			{[]string{"--go-candidate", "main", "--origin-target", target, "--ephemeral"}, "invalid CI release"},
			{[]string{"--go-candidate", "latest", "--origin-target", target, "--ephemeral"}, "invalid CI release"},
			{[]string{"--go-candidate", handoffVersion, "--origin-target", "windows/amd64", "--ephemeral"}, "invalid CI origin target"},
			{append(append([]string{}, valid...), "--go-candidate", handoffVersion), "duplicate CI argument"},
			{append(append([]string{}, valid...), "--origin-target", target), "duplicate CI argument"},
			{append(append([]string{}, valid...), "--ephemeral"), "duplicate CI argument"},
			{append(append([]string{}, valid...), "--source", cwd), "unknown CI argument"},
			{append(append([]string{}, valid...), "--unknown"), "unknown CI argument"},
			{[]string{"--go-candidate=" + handoffVersion, "--origin-target=", "--ephemeral"}, "invalid CI origin target"},
		}
		for _, testcase := range cases {
			out := imageProcess(cwd, env, driver, append([]string{"ci", "prepare"}, testcase.args...)...)
			Expect(out.status).To(Equal(2), "args %q: %+v", testcase.args, out)
			Expect(strings.ToLower(out.stderr)).To(ContainSubstring("ci"))
			Expect(out.stderr).To(ContainSubstring(testcase.reason), "the refusal must identify the invalid selection without echoing caller inputs")
			Expect(out.stderr).To(ContainSubstring("require an exact release, origin target and --ephemeral"))
			Expect(nativeInitArtifacts(cwd)).To(Equal(before))
			_, err := os.Lstat(marker)
			Expect(os.IsNotExist(err)).To(BeTrue(), "invalid request must not acquire or execute a release")
		}
	})
})

type ciPrepareProject struct {
	image      handoffReleaseImage
	repository string
	selected   []string
	private    []string
}

// Build the real producer's image, initialize it through the authenticated public
// CLI, then commit only Git-visible project inputs. No selected private state is copied.
// per docs/adr/0143-ci-checkout-preparation.md:126
func ciPrepareProjectFixture(patches ...func(string)) ciPrepareProject {
	return ciPrepareProjectCustomized(nil, patches...)
}

// Existing callers retain the original simplified application fixture. Workflow
// qualification can preserve the actual initialized Makefile before committing.
// per docs/adr/0145-installed-ci-workflow-rendering.md:99
func ciPrepareProjectCustomized(customize func(string, []byte, map[string]string), patches ...func(string)) ciPrepareProject {
	GinkgoHelper()
	image := handoffBuildImage(patches...)
	repositorySource, err := filepath.Abs("..")
	Expect(err).NotTo(HaveOccurred())
	imageGit(filepath.Join(image.work, "exact-source"), "fetch", "--quiet", "--no-tags", repositorySource, "b71ecc32e07ecd87eb330ba8e497c86612f92acd", "76952eaa63aebd1ecd282f5ab51dd7c3627cb497")
	bundle := localUpgradeBundle(filepath.Join(image.work, "exact-source"), image.revision, handoffVersion, filepath.Join(image.work, "packaged-ci-image"))
	data, err := os.ReadFile(bundle.Archive)
	Expect(err).NotTo(HaveOccurred())
	entries, _ := imageDecodeArchive(data)
	image.binary, image.archive, image.archivePath = entries[0].body, data, bundle.Archive
	f, driver := handoffNativeFixture(image)
	repository := filepath.Join(f.cwd, "initialized public project")
	out := handoffCooperativeProcess(f.cwd, f.environment, nativeInitAnswers("y"), []string{f.root}, driver, "init", repository, "--go-candidate", handoffVersion, "--pack", "none")
	Expect(out.status).To(BeZero(), "actual packaged initialization must establish the source of the committed public assets: %+v", out)
	var initializedMakefile []byte
	if customize != nil {
		initializedMakefile, err = os.ReadFile(filepath.Join(repository, "Makefile"))
		Expect(err).NotTo(HaveOccurred())
	}
	descriptor, err := os.ReadFile(filepath.Join(repository, ".factory/installation.current"))
	Expect(err).NotTo(HaveOccurred())
	var selection struct {
		Assets []struct {
			Path string `json:"path"`
		} `json:"assets"`
	}
	Expect(json.Unmarshal(descriptor, &selection)).To(Succeed())
	selected := []string{}
	private := []string{}
	for _, asset := range selection.Assets {
		if !strings.HasPrefix(asset.Path, ".factory/") {
			selected = append(selected, asset.Path)
		} else {
			private = append(private, asset.Path)
		}
	}
	Expect(selected).To(ContainElement("factory"))
	configPath := filepath.Join(repository, "factory.yaml")
	config, err := os.ReadFile(configPath)
	Expect(err).NotTo(HaveOccurred())
	lines := strings.Split(string(config), "\n")
	found := false
	for i, line := range lines {
		if strings.HasPrefix(line, "protected_paths:") {
			lines[i] = `protected_paths: "internal/billing internal/auth"`
		}
		if strings.HasPrefix(line, "check_command:") {
			lines[i] = "check_command: ./check-application.sh"
			found = true
		}
	}
	Expect(found).To(BeTrue())
	writeFixture(configPath, []byte(strings.Join(lines, "\n")), 0644)
	writeFixture(filepath.Join(repository, "check-application.sh"), []byte("#!/bin/sh\nprintf 'application check\\n' >> \"$CI_PREPARE_CHECK_LOG\"\nexit \"${CI_PREPARE_CHECK_STATUS:-0}\"\n"), 0755)
	writeFixture(filepath.Join(repository, "Makefile"), []byte(".PHONY: check check-drift\ncheck:\n\t./check-application.sh\ncheck-drift:\n\t@:\n"), 0644)
	writeFixture(filepath.Join(repository, "application.txt"), []byte("application-owned content\n"), 0644)
	writeFixture(filepath.Join(repository, ".github/workflows/application.yml"), []byte("name: Preserve application CI\non: workflow_dispatch\njobs: {}\n"), 0644)
	if customize != nil {
		customize(repository, initializedMakefile, f.environment)
	}
	imageGit(repository, "init", "--quiet")
	// per docs/adr/0148-transactional-installed-ci-migration.md:544
	imageGit(repository, "config", "--local", "gc.auto", "0")
	imageGit(repository, "config", "--local", "maintenance.auto", "false")
	imageGit(repository, "add", "--all")
	imageGit(repository, "commit", "--quiet", "-m", "test: commit public initialized project")
	tracked := strings.Split(strings.TrimSuffix(imageGit(repository, "ls-files"), "\n"), "\n")
	Expect(tracked).To(ContainElements("factory", ".factory-version", "factory.yaml"))
	for _, path := range tracked {
		Expect(path).NotTo(HavePrefix(".factory/"), "private installed custody must be absent from the Git project")
	}
	return ciPrepareProject{image, repository, selected, private}
}

// per docs/adr/0143-ci-checkout-preparation.md:127
func (p ciPrepareProject) checkout() (handoffBootstrapFixture, string, string) {
	GinkgoHelper()
	f, driver := handoffNativeFixture(p.image)
	root := filepath.Join(f.cwd, "fresh checkout")
	imageGit(f.cwd, "clone", "--quiet", "--no-local", "--config=gc.auto=0", "--config=maintenance.auto=false", p.repository, root)
	imageGit(root, "update-ref", "refs/remotes/origin/main", "HEAD")
	for _, path := range []string{".factory/bin", ".factory/assets", ".factory/installation.current", ".factory/runtime-publication.pending"} {
		_, err := os.Lstat(filepath.Join(root, path))
		Expect(os.IsNotExist(err)).To(BeTrue(), "Git clone unexpectedly carries %s", path)
	}
	f.environment["CI_PREPARE_CHECK_LOG"] = filepath.Join(f.records, "application-check")
	// Keep an unknown private leaf as well as ordinary untracked application data.
	writeFixture(filepath.Join(root, "notes/operator-note"), []byte("keep untracked application data\n"), 0600)
	writeFixture(filepath.Join(root, ".factory/operator-note"), []byte("keep unknown private data\n"), 0600)
	return f, driver, root
}

// Keep complete Git snapshots deterministic without ignoring maintenance artifacts.
// per docs/adr/0148-transactional-installed-ci-migration.md:544
func ciPrepareRequireQuiescentGit(root string) {
	GinkgoHelper()
	Expect(strings.TrimSpace(imageGit(root, "config", "--local", "--get", "gc.auto"))).To(Equal("0"))
	Expect(strings.TrimSpace(imageGit(root, "config", "--local", "--get", "maintenance.auto"))).To(Equal("false"))
}

func ciPrepareArguments(target string) []string {
	return []string{"ci", "prepare", "--go-candidate", handoffVersion, "--origin-target", target, "--ephemeral"}
}

// A refusal is meaningful only if the new public operation owns it, and every
// byte, mode and original filesystem identity survives the refused publication.
// per docs/adr/0143-ci-checkout-preparation.md:139
func ciPrepareRefusal(root, driver string, f handoffBootstrapFixture, args []string) cliResult {
	GinkgoHelper()
	unchanged := installationPreservationWitness(root)
	out := handoffNativeProcess(root, f.environment, "", driver, args...)
	Expect(out.status).NotTo(BeZero(), "%+v", out)
	Expect(out.stderr).NotTo(ContainSubstring("unknown command"), "the intended CI owner must refuse: %+v", out)
	unchanged()
	return out
}

// per docs/adr/0143-ci-checkout-preparation.md:115
func ciPreparePublicPaths(root string) []string {
	GinkgoHelper()
	paths := []string{}
	Expect(filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if relative == ".git" || relative == ".factory" {
			return filepath.SkipDir
		}
		if !entry.IsDir() {
			paths = append(paths, filepath.ToSlash(relative))
		}
		return nil
	})).To(Succeed())
	sort.Strings(paths)
	return paths
}

// The expected private entries come from the authenticated initialized project,
// plus preexisting unknown data and the descriptor excluded from its own digest.
// Check before health commands can intentionally create separate runtime state.
// per docs/adr/0143-ci-checkout-preparation.md:193
func ciPreparePrivateWitness(root string, selected []string) func() {
	GinkgoHelper()
	entries := func() map[string]fs.FileMode {
		GinkgoHelper()
		found := map[string]fs.FileMode{}
		Expect(filepath.WalkDir(filepath.Join(root, ".factory"), func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			found[filepath.ToSlash(relative)] = entry.Type()
			return nil
		})).To(Succeed())
		return found
	}
	expected := entries()
	for _, path := range append(append([]string{}, selected...), ".factory/installation.current") {
		Expect(path).To(HavePrefix(".factory/"))
		expected[path] = 0
		for parent := filepath.Dir(path); parent != "."; parent = filepath.Dir(parent) {
			expected[filepath.ToSlash(parent)] = fs.ModeDir
		}
	}
	return func() {
		GinkgoHelper()
		Expect(entries()).To(Equal(expected), "preparation may create only selected private files and required parents; private backups, staging debris and empty extra directories are forbidden")
	}
}

// per docs/adr/0143-ci-checkout-preparation.md:178
var _ = Describe("Whole installation upgrade release candidate handoff CI checkout preparation", Ordered, SpecPriority(2), ContinueOnFailure, Label("handoff-ci-project"), func() {
	var project ciPrepareProject
	BeforeAll(func() { project = ciPrepareProjectFixture() })

	// per docs/adr/0143-ci-checkout-preparation.md:126
	// per docs/adr/0143-ci-checkout-preparation.md:115
	It("authenticates a cloned initialized project and runs installed checks without compiler or Python", func() {
		f, driver, root := project.checkout()
		privatePublished := ciPreparePrivateWitness(root, project.private)
		beforePaths := ciPreparePublicPaths(root)
		head, index, ignore := imageGit(root, "rev-parse", "HEAD"), imageGit(root, "ls-files", "--stage", "-z"), imageGit(root, "show", "HEAD:.gitignore")
		preserved := map[string][]byte{}
		for _, path := range []string{"application.txt", "factory.yaml", "Makefile", ".github/workflows/application.yml", "notes/operator-note", ".factory/operator-note"} {
			data, err := os.ReadFile(filepath.Join(root, path))
			Expect(err).NotTo(HaveOccurred())
			preserved[path] = data
		}
		missing := handoffNativeProcess(root, f.environment, "", filepath.Join(root, "factory"), "help")
		Expect(missing.status).NotTo(BeZero(), "a plain clone must not already have private runtime custody")
		out := handoffCooperativeProcess(root, f.environment, "", []string{f.root, root}, driver, ciPrepareArguments(runtime.GOOS+"/"+runtime.GOARCH)...)
		AddReportEntry("ci-prepare.actual-result", map[string]any{"status": out.status, "stdout": out.stdout, "stderr": out.stderr}, ReportEntryVisibilityNever)
		Expect(out.status).To(BeZero(), "%+v", out)
		privatePublished()
		defaultInitDescriptor(root, handoffVersion, project.image.revision, project.image.binary)
		proof := handoffNativeProcess(root, f.environment, "", filepath.Join(root, "factory"), "selftest")
		Expect(proof.status).To(BeZero(), "%+v", proof)
		Expect(proof.stdout).To(ContainSubstring("0 failed"))
		for _, status := range []string{"0", "23"} {
			f.environment["CI_PREPARE_CHECK_STATUS"] = status
			check := handoffNativeProcess(root, f.environment, "", filepath.Join(root, "factory"), "check")
			if status == "0" {
				Expect(check.status).To(BeZero(), "%+v", check)
			} else {
				Expect(check.status).NotTo(BeZero(), "%+v", check)
			}
		}
		Expect(os.ReadFile(f.environment["CI_PREPARE_CHECK_LOG"])).To(Equal([]byte("application check\napplication check\n")))
		for path, data := range preserved {
			Expect(os.ReadFile(filepath.Join(root, path))).To(Equal(data), path)
		}
		Expect(imageGit(root, "rev-parse", "HEAD")).To(Equal(head))
		Expect(imageGit(root, "ls-files", "--stage", "-z")).To(Equal(index))
		Expect(os.ReadFile(filepath.Join(root, ".gitignore"))).To(Equal([]byte(ignore)))
		Expect(ciPreparePublicPaths(root)).To(Equal(beforePaths), "preparation must not create adjacent backups or other public cruft")
		_, err := os.Lstat(filepath.Join(root, ".factory/runtime-publication.pending"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		_, err = os.Lstat(filepath.Join(f.records, "forbidden-tool"))
		Expect(os.IsNotExist(err)).To(BeTrue(), "installed behavior must not invoke Go/Python/network/archive helpers")
		trace, err := os.ReadFile(filepath.Join(f.records, "trace"))
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.Count(string(trace), "release-download\n")).To(Equal(1), "same-target preparation must reuse one acquisition")
		verify := handoffRecorded(f, "verify")
		Expect(handoffFlag(verify, "--source-digest")).To(Equal(project.image.revision))
		Expect(handoffFlag(verify, "--source-ref")).To(Equal("refs/tags/" + handoffVersion))
		Expect(handoffFlag(verify, "--signer-workflow")).To(Equal("anoop2811/software-factory-template/.github/workflows/runtime-release.yml"))
		// Existing complete custody is not a repair target, even for the same release.
		repeated := ciPrepareRefusal(root, driver, f, ciPrepareArguments(runtime.GOOS+"/"+runtime.GOARCH))
		Expect(strings.ToLower(repeated.stderr)).To(ContainSubstring("fresh"))
	})

	// per docs/adr/0143-ci-checkout-preparation.md:45
	It("refuses unauthenticated or mismatched release and tracked runtime bindings without project mutation", func() {
		for _, kind := range []string{"authentication", "archive subject", "marker tag", "marker revision", "launcher digest"} {
			f, driver, root := project.checkout()
			switch kind {
			case "authentication":
				f.environment["HANDOFF_SCENARIO"] = "authentication-fail"
			case "archive subject":
				f.environment["HANDOFF_SCENARIO"] = "wrong-archive-subject"
			case "marker tag", "marker revision":
				path := filepath.Join(root, ".factory-version")
				data, err := os.ReadFile(path)
				Expect(err).NotTo(HaveOccurred())
				if kind == "marker tag" {
					data = bytes.Replace(data, []byte(handoffVersion), []byte("v9.8.1-rc.1"), 1)
				} else {
					data = bytes.Replace(data, []byte(project.image.revision), []byte(strings.Repeat("a", 40)), 1)
				}
				writeFixture(path, data, 0644)
				imageGit(root, "add", ".factory-version")
				imageGit(root, "commit", "--quiet", "-m", "test: mismatched tracked marker")
			case "launcher digest":
				path := filepath.Join(root, "factory")
				data, err := os.ReadFile(path)
				Expect(err).NotTo(HaveOccurred())
				digest := fmt.Sprintf("%x", sha256.Sum256(project.image.binary))
				Expect(string(data)).To(ContainSubstring(digest))
				writeFixture(path, bytes.ReplaceAll(data, []byte(digest), []byte(strings.Repeat("a", 64))), 0755)
				imageGit(root, "add", "factory")
				imageGit(root, "commit", "--quiet", "-m", "test: mismatched tracked runtime")
			}
			ciPrepareRefusal(root, driver, f, ciPrepareArguments(runtime.GOOS+"/"+runtime.GOARCH))
		}
	})

	// per docs/adr/0143-ci-checkout-preparation.md:64
	// per docs/adr/0143-ci-checkout-preparation.md:72
	It("preserves customized, dirty, unsafe and already-owned checkout inputs on refusal", func() {
		ciPrepareRequireQuiescentGit(project.repository)
		for _, kind := range []string{"customized", "missing", "nonexecutable", "symlink", "hardlink", "dirty worktree", "dirty index", "private bin", "private assets", "private backup", "private transaction", "descriptor", "pending", "metadata ambiguity"} {
			f, driver, root := project.checkout()
			ciPrepareRequireQuiescentGit(root)
			gate := filepath.Join(root, "scripts/hooks/direct-main-push-block.sh")
			switch kind {
			case "customized":
				writeFixture(gate, []byte("#!/bin/sh\nexit 0\n"), 0755)
				imageGit(root, "add", "scripts/hooks/direct-main-push-block.sh")
				imageGit(root, "commit", "--quiet", "-m", "test: customize owned gate")
			case "missing":
				imageGit(root, "rm", "--quiet", "scripts/hooks/direct-main-push-block.sh")
				imageGit(root, "commit", "--quiet", "-m", "test: remove owned gate")
			case "nonexecutable":
				Expect(os.Chmod(gate, 0644)).To(Succeed())
				imageGit(root, "add", "scripts/hooks/direct-main-push-block.sh")
				imageGit(root, "commit", "--quiet", "-m", "test: remove executable mode")
			case "symlink":
				outside := filepath.Join(f.root, "outside gate")
				writeFixture(outside, []byte("outside original\n"), 0600)
				Expect(os.Remove(gate)).To(Succeed())
				Expect(os.Symlink(outside, gate)).To(Succeed())
				imageGit(root, "add", "scripts/hooks/direct-main-push-block.sh")
				imageGit(root, "commit", "--quiet", "-m", "test: link owned gate")
			case "hardlink":
				Expect(os.Link(gate, filepath.Join(root, "unknown-hardlink"))).To(Succeed())
			case "dirty worktree":
				writeFixture(filepath.Join(root, "application.txt"), []byte("uncommitted application change\n"), 0644)
			case "dirty index":
				writeFixture(filepath.Join(root, "application.txt"), []byte("staged application change\n"), 0644)
				imageGit(root, "add", "application.txt")
			case "private bin", "private assets", "private backup", "private transaction":
				names := map[string]string{"private bin": "bin", "private assets": "assets", "private backup": "backups", "private transaction": "installation-transactions"}
				Expect(os.Mkdir(filepath.Join(root, ".factory", names[kind]), 0700)).To(Succeed())
			case "descriptor":
				writeFixture(filepath.Join(root, ".factory/installation.current"), []byte("unknown private descriptor\n"), 0600)
			case "pending":
				writeFixture(filepath.Join(root, ".factory/runtime-publication.pending"), []byte("unknown pending owner\n"), 0600)
			case "metadata ambiguity":
				path := filepath.Join(root, "factory.yaml")
				data, err := os.ReadFile(path)
				Expect(err).NotTo(HaveOccurred())
				writeFixture(path, append(data, []byte("project_name: conflicting-project\n")...), 0644)
				imageGit(root, "add", "factory.yaml")
				imageGit(root, "commit", "--quiet", "-m", "test: ambiguous project metadata")
			}
			ciPrepareRefusal(root, driver, f, ciPrepareArguments(runtime.GOOS+"/"+runtime.GOARCH))
		}
	})

	// per docs/adr/0143-ci-checkout-preparation.md:83
	It("refuses a selected private asset exposed by selective ignores despite ignored representative probes", func() {
		f, driver, root := project.checkout()
		metrics := ""
		for _, path := range project.private {
			if strings.HasSuffix(path, "/templates/metrics.html") {
				Expect(metrics).To(BeEmpty(), "the authenticated descriptor must identify one metrics asset")
				metrics = path
			}
		}
		Expect(metrics).NotTo(BeEmpty())
		ignorePath := filepath.Join(root, ".gitignore")
		data, err := os.ReadFile(ignorePath)
		Expect(err).NotTo(HaveOccurred())
		parts := strings.Split(metrics, "/")
		for index := 1; index < len(parts); index++ {
			parent := strings.Join(parts[:index], "/")
			data = append(data, []byte("\n!/"+parent+"/\n/"+parent+"/*\n")...)
		}
		data = append(data, []byte("!/"+metrics+"\n")...)
		writeFixture(ignorePath, data, 0644)
		imageGit(root, "add", ".gitignore")
		imageGit(root, "commit", "--quiet", "-m", "test: selectively expose one private selected asset")
		for _, path := range []string{".factory/bin/factory-runtime", ".factory/assets/scaffold/factory", ".factory/installation.current", ".factory/installation-transactions/ignore-proof", ".factory/backups/ignore-proof"} {
			probe := imageProcess(root, f.environment, "git", "check-ignore", "--no-index", "-q", "--", path)
			Expect(probe.status).To(BeZero(), "representative probe %s must remain ignored: %+v", path, probe)
		}
		probe := imageProcess(root, f.environment, "git", "check-ignore", "--no-index", "-q", "--", metrics)
		Expect(probe.status).To(Equal(1), "the actual selected asset must be exposed: %+v", probe)
		ciPrepareRefusal(root, driver, f, ciPrepareArguments(runtime.GOOS+"/"+runtime.GOARCH))
	})

	// per docs/adr/0143-ci-checkout-preparation.md:105
	It("admits clean initialized gitlinks and refuses hidden staged or filtered child changes without executing helpers", func() {
		for _, kind := range []string{"clean", "staged gitlink", "filtered child"} {
			f, driver, root := project.checkout()
			origin := filepath.Join(f.root, "submodule origin")
			Expect(os.Mkdir(origin, 0700)).To(Succeed())
			imageGit(origin, "init", "--quiet")
			writeFixture(filepath.Join(origin, "tracked.txt"), []byte("old content\n"), 0644)
			imageGit(origin, "add", "tracked.txt")
			imageGit(origin, "commit", "--quiet", "-m", "test: first child commit")
			imageGit(root, "-c", "protocol.file.allow=always", "submodule", "add", "--quiet", origin, "application-child")
			imageGit(root, "commit", "--quiet", "-m", "test: clean initialized gitlink")
			child := filepath.Join(root, "application-child")
			marker := filepath.Join(f.records, "submodule-filter-executed")
			switch kind {
			case "staged gitlink":
				writeFixture(filepath.Join(child, "tracked.txt"), []byte("new content\n"), 0644)
				imageGit(child, "add", "tracked.txt")
				imageGit(child, "commit", "--quiet", "-m", "test: staged child commit")
				imageGit(root, "add", "application-child")
				imageGit(root, "config", "diff.ignoreSubmodules", "all")
				Expect(imageProcess(root, f.environment, "git", "diff", "--cached", "--quiet").status).To(BeZero(), "fixture must exercise the configured staged-gitlink blind spot")
				Expect(imageProcess(root, f.environment, "git", "diff", "--cached", "--quiet", "--ignore-submodules=none").status).To(Equal(1))
			case "filtered child":
				writeFixture(filepath.Join(child, ".gitattributes"), []byte("tracked.txt filter=fixture\n"), 0644)
				imageGit(child, "add", ".gitattributes")
				imageGit(child, "commit", "--quiet", "-m", "test: child filter attributes")
				imageGit(root, "add", "application-child")
				imageGit(root, "commit", "--quiet", "-m", "test: admit child attribute commit")
				filter := filepath.Join(f.root, "child-filter")
				writeFixture(filter, []byte("#!/bin/sh\nprintf executed > \"$CI_PREPARE_FILTER_MARKER\"\n/bin/cat\n"), 0700)
				imageGit(child, "config", "filter.fixture.clean", "'"+strings.ReplaceAll(filter, "'", "'\"'\"'")+"'")
				f.environment["CI_PREPARE_FILTER_MARKER"] = marker
				// Same length defeats a size-only dirty check; changed mtime forces content inspection.
				writeFixture(filepath.Join(child, "tracked.txt"), []byte("new content\n"), 0644)
				Expect(os.Chtimes(filepath.Join(child, "tracked.txt"), time.Unix(1, 0), time.Unix(1, 0))).To(Succeed())
			}
			if kind == "clean" {
				head, index := imageGit(root, "rev-parse", "HEAD"), imageGit(root, "ls-files", "--stage", "-z")
				out := handoffCooperativeProcess(root, f.environment, "", []string{f.root, root}, driver, ciPrepareArguments(runtime.GOOS+"/"+runtime.GOARCH)...)
				Expect(out.status).To(BeZero(), "%+v", out)
				defaultInitDescriptor(root, handoffVersion, project.image.revision, project.image.binary)
				Expect(imageGit(root, "rev-parse", "HEAD")).To(Equal(head))
				Expect(imageGit(root, "ls-files", "--stage", "-z")).To(Equal(index))
			} else {
				ciPrepareRefusal(root, driver, f, ciPrepareArguments(runtime.GOOS+"/"+runtime.GOARCH))
			}
			_, err := os.Lstat(marker)
			Expect(os.IsNotExist(err)).To(BeTrue(), "preparation must not execute submodule-local filters")
		}
	})

	// per docs/adr/0143-ci-checkout-preparation.md:45
	// per docs/adr/0143-ci-checkout-preparation.md:135
	It("reprojects an authenticated inert foreign origin to the actual host owner without executing foreign bytes", func() {
		f, driver, root := project.checkout()
		privatePublished := ciPreparePrivateWitness(root, project.private)
		foreignOS := "linux"
		if runtime.GOOS == "linux" {
			foreignOS = "darwin"
		}
		foreignTarget := foreignOS + "/" + runtime.GOARCH
		// per docs/adr/0148-transactional-installed-ci-migration.md:643
		producer := defaultInitFixtures{
			invoker: project.image, source: filepath.Join(project.image.work, "exact-source"), revision: project.image.revision,
		}
		producing := time.Now()
		bundle := producer.packageImage(handoffVersion, foreignTarget, filepath.Join(project.image.work, "foreign-origin-image"))
		AddReportEntry("ci-prepare.foreign-image-setup", map[string]any{
			"producer": producer.packager, "source": producer.source, "revision": bundle.Revision,
			"version": bundle.Version, "target": bundle.Target, "archive": bundle.Archive,
			"sha256": bundle.SHA256, "producer_seconds": time.Since(producing).Seconds(),
		}, ReportEntryVisibilityNever)
		archive, err := os.ReadFile(bundle.Archive)
		Expect(err).NotTo(HaveOccurred())
		foreign := []byte("#!/bin/sh\nprintf foreign-executed > \"$CI_PREPARE_FOREIGN_MARKER\"\nexit 91\n")
		originImage := project.image
		originImage.archive = archive
		originArchive := handoffImageBinary(originImage, foreign, handoffVersion)
		foreignName := strings.ReplaceAll(foreignTarget, "/", "-")
		writeFixture(filepath.Join(f.environment["HANDOFF_ASSETS"], "factory-bootstrap-"+handoffVersion+"-"+foreignName), foreign, 0600)
		writeFixture(filepath.Join(f.environment["HANDOFF_ASSETS"], "factory-installation-"+handoffVersion+"-"+foreignName+".tar.gz"), originArchive, 0600)
		ciPrepareMultiTargetTransport(f)
		f.environment["CI_PREPARE_FOREIGN_MARKER"] = filepath.Join(f.records, "foreign-executed")
		hostPublic := ciPrepareProjectDigest(root, project.selected, project.image.binary, foreign)
		decision := []byte("# Decision Log\n\n## Decision 158: Project an inert foreign runtime\n\nAccepted only for this disposable qualification project.\n")
		writeFixture(filepath.Join(root, "docs/DECISION_LOG.md"), decision, 0644)
		imageGit(root, "add", "--all")
		imageGit(root, "commit", "--quiet", "-m", "test: project foreign runtime for Decision 158")
		head, index, paths := imageGit(root, "rev-parse", "HEAD"), imageGit(root, "ls-files", "--stage", "-z"), ciPreparePublicPaths(root)
		config, err := os.ReadFile(filepath.Join(root, "factory.yaml"))
		Expect(err).NotTo(HaveOccurred())
		out := handoffCooperativeProcess(root, f.environment, "", []string{f.root, root}, driver, ciPrepareArguments(foreignTarget)...)
		Expect(out.status).To(BeZero(), "%+v", out)
		privatePublished()
		defaultInitDescriptor(root, handoffVersion, project.image.revision, project.image.binary)
		Expect(os.ReadFile(filepath.Join(root, "docs/DECISION_LOG.md"))).To(Equal(decision))
		for path, data := range hostPublic {
			Expect(os.ReadFile(filepath.Join(root, path))).To(Equal(data), path)
		}
		Expect(imageGit(root, "rev-parse", "HEAD")).To(Equal(head))
		Expect(imageGit(root, "ls-files", "--stage", "-z")).To(Equal(index))
		Expect(os.ReadFile(filepath.Join(root, "factory.yaml"))).To(Equal(config))
		Expect(ciPreparePublicPaths(root)).To(Equal(paths), "reprojection must not leave adjacent backup files")
		for _, command := range []string{"selftest", "check"} {
			result := handoffNativeProcess(root, f.environment, "", filepath.Join(root, "factory"), command)
			Expect(result.status).To(BeZero(), "%s through the actual host launcher: %+v", command, result)
		}
		for _, marker := range []string{"foreign-executed", "forbidden-tool"} {
			_, err := os.Lstat(filepath.Join(f.records, marker))
			Expect(os.IsNotExist(err)).To(BeTrue(), marker)
		}
		verification, err := os.ReadFile(filepath.Join(f.records, "all-verification-arguments"))
		Expect(err).NotTo(HaveOccurred())
		for target, binary := range map[string][]byte{f.target: project.image.binary, foreignName: foreign} {
			Expect(string(verification)).To(ContainSubstring("factory-bootstrap-" + handoffVersion + "-" + target))
			Expect(string(verification)).To(ContainSubstring(fmt.Sprintf("%x", sha256.Sum256(binary))))
			Expect(string(verification)).To(ContainSubstring("factory-installation-" + handoffVersion + "-" + target + ".tar.gz"))
		}
		for _, archive := range [][]byte{project.image.archive, originArchive} {
			Expect(string(verification)).To(ContainSubstring(fmt.Sprintf("%x", sha256.Sum256(archive))))
		}
	})
})

// The foreign origin is deliberately inert. Only independently enumerated
// generated runtime-digest bindings are projected; Selection is not the oracle.
// per docs/adr/0143-ci-checkout-preparation.md:135
func ciPrepareProjectDigest(root string, selected []string, before, after []byte) map[string][]byte {
	GinkgoHelper()
	oldDigest, newDigest := fmt.Sprintf("%x", sha256.Sum256(before)), fmt.Sprintf("%x", sha256.Sum256(after))
	originals := map[string][]byte{}
	changed := []string{}
	for _, path := range selected {
		data, err := os.ReadFile(filepath.Join(root, path))
		Expect(err).NotTo(HaveOccurred())
		originals[path] = data
		count := bytes.Count(data, []byte(oldDigest))
		if count == 0 {
			continue
		}
		expected := 1
		if path == "factory" {
			expected = 2
		}
		Expect(count).To(Equal(expected), "only exact generated digest bindings may be replaced in %s", path)
		info, err := os.Stat(filepath.Join(root, path))
		Expect(err).NotTo(HaveOccurred())
		writeFixture(filepath.Join(root, path), bytes.ReplaceAll(data, []byte(oldDigest), []byte(newDigest)), info.Mode().Perm())
		changed = append(changed, path)
	}
	Expect(changed).To(ConsistOf("factory", "eval/runners/claude.sh", "eval/runners/codex.sh", "eval/runners/example-harness.sh", "eval/runners/mock.sh", "eval/runners/opencode.sh", "scripts/adversarial-review.sh", "scripts/factory-budget.sh", "scripts/factory-doctor.sh", "scripts/factory-init.sh", "scripts/factory-loop.sh", "scripts/factory-metrics.sh", "scripts/factory-migrate-config.sh", "scripts/factory-report.sh", "scripts/factory-review-lane.sh", "scripts/factory-upgrade.sh", "scripts/golden-task-eval.sh", "scripts/harness-structural-eval.sh", "scripts/lib/budget-config.sh", "scripts/lib/color.sh", "scripts/lib/config.sh", "scripts/lib/events.sh", "scripts/lib/hookspath.sh", "scripts/lib/roles.sh", "scripts/lib/timing.sh", "scripts/pre-push-check.sh", "scripts/prereq-check.sh", "scripts/selftest/run.sh", "scripts/sync-claude.sh", "scripts/sync-codex.sh", "scripts/sync-opencode.sh"))
	return originals
}

// per docs/adr/0143-ci-checkout-preparation.md:45
func ciPrepareMultiTargetTransport(f handoffBootstrapFixture) {
	GinkgoHelper()
	path := filepath.Join(f.root, "host tools", "gh")
	data, err := os.ReadFile(path)
	Expect(err).NotTo(HaveOccurred())
	script := string(data)
	old := `    destination=
    while [ "$#" -gt 0 ]; do
      case "$1" in --dir) destination="$2"; shift 2 ;; *) shift ;; esac
    done`
	newText := `    destination= requested_assets=
    while [ "$#" -gt 0 ]; do
      case "$1" in
        --dir) destination="$2"; shift 2 ;;
        --pattern) requested_assets="$requested_assets $2"; shift 2 ;;
        *) shift ;;
      esac
    done`
	Expect(strings.Count(script, old)).To(Equal(1))
	script = strings.Replace(script, old, newText, 1)
	old = `for asset in "factory-bootstrap-$selected_version-$HANDOFF_TARGET" "factory-installation-$selected_version-$HANDOFF_TARGET.tar.gz" runtime-attestations.jsonl; do`
	Expect(strings.Count(script, old)).To(Equal(1))
	script = strings.Replace(script, old, `for asset in $requested_assets; do`, 1)
	script = strings.Replace(script, "  verify)\n", "  verify)\n    printf '%s\\n' \"$@\" >> \"$HANDOFF_RECORD_DIR/all-verification-arguments\"\n", 1)
	writeFixture(path, []byte(script), 0700)
}

// Instrument only test-owned committed source. The owner receipt does not alter
// routing, and the scheduling gate waits after the real pending rename and fsync.
// per docs/adr/0143-ci-checkout-preparation.md:143
func ciPrepareInstrumentedSource(source string) {
	GinkgoHelper()
	path := filepath.Join(source, "cmd/factory/main.go")
	data, err := os.ReadFile(path)
	Expect(err).NotTo(HaveOccurred())
	old := "func main() { os.Exit(run(context.Background(), os.Args[1:])) }"
	Expect(strings.Count(string(data), old)).To(Equal(1))
	body := `func main() {
	if receipt := os.Getenv("CI_PREPARE_OWNER_RECEIPT"); receipt != "" {
		if err := os.WriteFile(receipt, []byte("selected authenticated CI owner\n"), 0600); err != nil { os.Exit(98) }
	}
	os.Exit(run(context.Background(), os.Args[1:]))
}`
	writeFixture(path, []byte(strings.Replace(string(data), old, body, 1)), 0644)
	path = filepath.Join(source, "internal/initcmd/files.go")
	data, err = os.ReadFile(path)
	Expect(err).NotTo(HaveOccurred())
	old = "\tpublished = true\n\treturn parent.Sync()"
	Expect(strings.Count(string(data), old)).To(Equal(1))
	body = `	published = true
	if err := parent.Sync(); err != nil { return err }
	if a.path == ".factory/runtime-publication.pending" {
		if receipt := os.Getenv("CI_PREPARE_PENDING_RECEIPT"); receipt != "" {
			if err := os.WriteFile(receipt, []byte(strconv.Itoa(os.Getpid())), 0600); err != nil { return err }
			<-ctx.Done()
			return ctx.Err()
		}
	}
	return nil`
	writeFixture(path, []byte(strings.Replace(string(data), old, body, 1)), 0644)
}

// per docs/adr/0143-ci-checkout-preparation.md:178
var _ = Describe("Whole installation upgrade release candidate handoff CI checkout preparation", Ordered, SpecPriority(2), ContinueOnFailure, Label("handoff-ci-ownership"), func() {
	var project ciPrepareProject
	BeforeAll(func() { project = ciPrepareProjectFixture(ciPrepareInstrumentedSource) })

	// per docs/adr/0143-ci-checkout-preparation.md:53
	// per docs/adr/0143-ci-checkout-preparation.md:143
	It("hands a differing caller to the authenticated host owner and preserves checkout when that owner lacks CI", func() {
		f, driver, root := project.checkout()
		Expect(project.image.binary).NotTo(Equal(candidateBinary))
		writeFixture(driver, candidateBinary, 0700)
		receipt := filepath.Join(f.records, "selected-owner")
		f.environment["CI_PREPARE_OWNER_RECEIPT"] = receipt
		out := handoffCooperativeProcess(root, f.environment, "", []string{f.root, root}, driver, ciPrepareArguments(runtime.GOOS+"/"+runtime.GOARCH)...)
		Expect(out.status).To(BeZero(), "%+v", out)
		Expect(os.ReadFile(receipt)).To(Equal([]byte("selected authenticated CI owner\n")), "the independently compiled invoker cannot write this selected-owner receipt")
		defaultInitDescriptor(root, handoffVersion, project.image.revision, project.image.binary)
		AddReportEntry("ci-prepare.instrumented-owner", "authenticated selected image executed before successful installed admission", ReportEntryVisibilityNever)

		// This native image preserves initialization but deliberately predates the CI
		// entrypoint. No historical release is falsely claimed to have been rebuilt.
		unsupported := ciPrepareProjectFixture(func(source string) {
			path := filepath.Join(source, "cmd/factory/main.go")
			data, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			old := "func main() { os.Exit(run(context.Background(), os.Args[1:])) }"
			Expect(strings.Count(string(data), old)).To(Equal(1))
			body := `func main() {
	if len(os.Args) > 1 && os.Args[1] == "ci" {
		fmt.Fprintln(os.Stderr, "test-selected native owner: unknown CI operation")
		os.Exit(2)
	}
	os.Exit(run(context.Background(), os.Args[1:]))
}`
			writeFixture(path, []byte(strings.Replace(string(data), old, body, 1)), 0644)
		})
		f, driver, root = unsupported.checkout()
		writeFixture(driver, candidateBinary, 0700)
		unchanged := installationPreservationWitness(root)
		out = handoffNativeProcess(root, f.environment, "", driver, ciPrepareArguments(runtime.GOOS+"/"+runtime.GOARCH)...)
		Expect(out.status).NotTo(BeZero(), "%+v", out)
		Expect(out.stderr).To(ContainSubstring("test-selected native owner: unknown CI operation"))
		unchanged()
	})

	// per docs/adr/0143-ci-checkout-preparation.md:89
	// per docs/adr/0143-ci-checkout-preparation.md:115
	// per docs/adr/0143-ci-checkout-preparation.md:143
	It("settles acquired children on cancellation and retains a real published barrier after acknowledged interruption", func() {
		for _, phase := range []string{"acquisition", "publication"} {
			f, driver, root := project.checkout()
			ready := filepath.Join(f.records, "cancel-ready")
			if phase == "acquisition" {
				path := filepath.Join(f.root, "host tools", "gh")
				data, err := os.ReadFile(path)
				Expect(err).NotTo(HaveOccurred())
				old := "  verify)\n"
				Expect(strings.Count(string(data), old)).To(Equal(1))
				body := `  verify)
    if [ "${3##*/}" = archive ]; then
      /bin/sleep 120 &
      printf '%s %s\n' "$$" "$!" > "$CI_PREPARE_CANCEL_READY"
      wait
    fi
`
				writeFixture(path, []byte(strings.Replace(string(data), old, body, 1)), 0700)
				f.environment["CI_PREPARE_CANCEL_READY"] = ready
			} else {
				f.environment["CI_PREPARE_PENDING_RECEIPT"] = ready
			}
			unchanged := installationPreservationWitness(root)
			beforePaths := ciPreparePublicPaths(root)
			head, index := imageGit(root, "rev-parse", "HEAD"), imageGit(root, "ls-files", "--stage", "-z")
			preserved := map[string][]byte{}
			for _, path := range []string{"factory.yaml", "application.txt", "notes/operator-note", ".factory/operator-note", ".gitignore"} {
				data, err := os.ReadFile(filepath.Join(root, path))
				Expect(err).NotTo(HaveOccurred())
				preserved[path] = data
			}
			ciPrepareCanceledProcess(f, driver, root, ready, phase)
			if phase == "acquisition" {
				unchanged()
			} else {
				pending := filepath.Join(root, ".factory/runtime-publication.pending")
				info, err := os.Lstat(pending)
				Expect(err).NotTo(HaveOccurred(), "actual pending publication must survive interrupted preparation")
				Expect(info.Mode().Perm()).To(Equal(os.FileMode(0600)))
				Expect(ciPreparePublicPaths(root)).To(Equal(beforePaths), "failed publication may not create adjacent backups")
				for path, data := range preserved {
					Expect(os.ReadFile(filepath.Join(root, path))).To(Equal(data), path)
				}
				Expect(imageGit(root, "rev-parse", "HEAD")).To(Equal(head))
				Expect(imageGit(root, "ls-files", "--stage", "-z")).To(Equal(index))
				delete(f.environment, "CI_PREPARE_PENDING_RECEIPT")
				ciPrepareRefusal(root, driver, f, ciPrepareArguments(runtime.GOOS+"/"+runtime.GOARCH))
				result := handoffNativeProcess(root, f.environment, "", filepath.Join(root, "factory"), "help")
				Expect(result.status).NotTo(BeZero(), "a failed publication must never admit the installed launcher")
			}
		}
	})
})

// Retain actual observed process groups, never derive child authority from a
// launcher PID. Cleanup attempts TERM/join before reporting any failed assertion.
// per docs/adr/0143-ci-checkout-preparation.md:95
// per docs/adr/0143-ci-checkout-preparation.md:143
// per docs/adr/0144-ci-installer-bootstrap.md:106
func ciPrepareCanceledProcess(f handoffBootstrapFixture, driver, root, ready, phase string, prefix ...string) {
	GinkgoHelper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	args := append(append([]string{}, prefix...), ciPrepareArguments(runtime.GOOS+"/"+runtime.GOARCH)...)
	cmd := handoffCommand(ctx, root, f.environment, "", driver, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	Expect(cmd.Start()).To(Succeed())
	finished := make(chan struct{})
	var processError error
	go func() { processError = cmd.Wait(); close(finished) }()
	settled := false
	child, childGroup := 0, 0
	parentGroup, parentErr := syscall.Getpgid(cmd.Process.Pid)
	defer func() {
		if settled {
			return
		}
		var cleanupErr error
		if phase == "acquisition" && childGroup == 0 {
			child, childGroup, cleanupErr = ciPrepareObserveVerifier(ready, parentGroup)
		}
		_ = cmd.Process.Signal(syscall.SIGTERM)
		select {
		case <-finished:
		case <-time.After(3 * time.Second):
			if parentErr == nil && parentGroup == cmd.Process.Pid {
				_, err := sourceRunnerStopObservedGroup(cmd.Process.Pid, parentGroup)
				cleanupErr = errors.Join(cleanupErr, err)
			} else {
				cleanupErr = errors.Join(cleanupErr, errors.New("CI parent group was not observed"))
			}
			select {
			case <-finished:
			case <-time.After(3 * time.Second):
				cleanupErr = errors.Join(cleanupErr, errors.New("CI parent did not join"))
			}
		}
		if child > 1 && childGroup > 1 {
			_, err := sourceRunnerStopObservedGroup(child, childGroup)
			cleanupErr = errors.Join(cleanupErr, err)
			deadline := time.Now().Add(time.Second)
			for time.Now().Before(deadline) && (!errors.Is(syscall.Kill(child, 0), syscall.ESRCH) || !errors.Is(syscall.Kill(-childGroup, 0), syscall.ESRCH)) {
				time.Sleep(10 * time.Millisecond)
			}
			if !errors.Is(syscall.Kill(child, 0), syscall.ESRCH) || !errors.Is(syscall.Kill(-childGroup, 0), syscall.ESRCH) {
				cleanupErr = errors.Join(cleanupErr, errors.New("CI verifier child/group absence unconfirmed"))
			}
		} else if phase == "acquisition" {
			cleanupErr = errors.Join(cleanupErr, errors.New("CI verifier receipt was not confirmed"))
		}
		if parentErr != nil || !errors.Is(syscall.Kill(-parentGroup, 0), syscall.ESRCH) {
			cleanupErr = errors.Join(cleanupErr, errors.New("CI parent group absence unconfirmed"))
		}
		if cleanupErr != nil {
			holdFixtureCleanup(f.root, cleanupErr.Error())
		}
		Expect(cleanupErr).NotTo(HaveOccurred())
	}()
	Expect(parentErr).NotTo(HaveOccurred())
	Expect(parentGroup).To(Equal(cmd.Process.Pid))
	Eventually(func() bool {
		data, err := os.ReadFile(ready)
		return err == nil && len(strings.Fields(string(data))) == map[string]int{"acquisition": 2, "publication": 1}[phase]
	}).WithTimeout(8*time.Second).Should(BeTrue(), "must acknowledge the actual %s phase before signaling", phase)
	data, err := os.ReadFile(ready)
	Expect(err).NotTo(HaveOccurred())
	fields := strings.Fields(string(data))
	owner, err := strconv.Atoi(fields[0])
	Expect(err).NotTo(HaveOccurred())
	if phase == "acquisition" {
		child, childGroup, err = ciPrepareObserveVerifier(ready, parentGroup)
		Expect(err).NotTo(HaveOccurred())
	} else {
		Expect(owner).To(Equal(cmd.Process.Pid), "publication acknowledgement must come from the actual matching host owner")
		_, err = os.Stat(filepath.Join(root, ".factory/runtime-publication.pending"))
		Expect(err).NotTo(HaveOccurred())
	}
	Expect(cmd.Process.Signal(syscall.SIGTERM)).To(Succeed())
	select {
	case <-finished:
	case <-time.After(8 * time.Second):
		Fail("CI preparation did not join acknowledged cancellation")
	}
	Expect(ctx.Err()).NotTo(HaveOccurred())
	Expect(processError).To(HaveOccurred())
	Expect(cmd.ProcessState.ExitCode()).NotTo(Equal(-1), "the native owner must settle cancellation itself: %s", stderr.String())
	Eventually(func() error { return syscall.Kill(-parentGroup, 0) }).WithTimeout(time.Second).Should(MatchError(syscall.ESRCH))
	if child > 1 {
		Eventually(func() error { return syscall.Kill(child, 0) }).WithTimeout(time.Second).Should(MatchError(syscall.ESRCH))
		Eventually(func() error { return syscall.Kill(-childGroup, 0) }).WithTimeout(time.Second).Should(MatchError(syscall.ESRCH))
	}
	settled = true
	if phase == "publication" {
		Expect(stderr.String()).To(ContainSubstring("fresh disposable checkout"), "a retained published barrier must include actionable recovery guidance")
	}
	AddReportEntry("ci-prepare.instrumented-cancellation", map[string]any{"phase": phase, "parent": cmd.Process.Pid, "child": child, "observed_child_group": childGroup, "status": cmd.ProcessState.ExitCode(), "stderr": stderr.String()}, ReportEntryVisibilityNever)
}

// The receipt belongs to the private verifier script. Confirm both live process
// groups before granting cleanup authority; an incomplete receipt grants none.
// per docs/adr/0143-ci-checkout-preparation.md:95
func ciPrepareObserveVerifier(receipt string, parentGroup int) (int, int, error) {
	data, err := os.ReadFile(receipt)
	if err != nil {
		return 0, 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) != 2 {
		return 0, 0, errors.New("incomplete CI verifier receipt")
	}
	owner, ownerErr := strconv.Atoi(fields[0])
	child, childErr := strconv.Atoi(fields[1])
	if ownerErr != nil || childErr != nil || owner <= 1 || child <= 1 {
		return 0, 0, errors.New("invalid CI verifier receipt")
	}
	ownerGroup, ownerErr := syscall.Getpgid(owner)
	childGroup, childErr := syscall.Getpgid(child)
	if ownerErr != nil || childErr != nil || ownerGroup != owner || childGroup != ownerGroup || ownerGroup == parentGroup || ownerGroup == syscall.Getpgrp() {
		return 0, 0, errors.New("CI verifier receipt does not identify an independent live owned group")
	}
	return child, childGroup, nil
}
