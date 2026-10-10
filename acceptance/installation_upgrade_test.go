package acceptance_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
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

// The acceptance driver is built independently of the target archive and calls
// only the public CLI. per docs/adr/0098-whole-installation-upgrade-and-rollback.md:117
func installationBuildDriver() []byte {
	GinkgoHelper()
	root, err := filepath.Abs("..")
	Expect(err).NotTo(HaveOccurred())
	directory, err := os.MkdirTemp("", "factory-installation-driver-")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(os.RemoveAll, directory)
	args := []string{"build", "-o", filepath.Join(directory, "factory"), "./cmd/factory"}
	if os.Getenv("FACTORY_CLI_TEST_RACE") == "1" {
		args = append([]string{"build", "-race"}, args[1:]...)
	}
	command := exec.Command("go", args...) // #nosec G204 -- fixed package and evaluator-owned executable destination.
	command.Dir = root
	output, err := command.CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "independent CLI build failed: %s", output)
	binary, err := os.ReadFile(filepath.Join(directory, "factory"))
	Expect(err).NotTo(HaveOccurred())
	return binary
}

func installationSurfaceFixture(binary []byte) (driver, root, sentinel string) {
	GinkgoHelper()
	temporary, err := os.MkdirTemp("", "factory installation surface ")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(os.RemoveAll, temporary)
	temporary, err = filepath.EvalSymlinks(temporary)
	Expect(err).NotTo(HaveOccurred())
	driver = filepath.Join(temporary, "independent driver")
	root = filepath.Join(temporary, "installed project")
	Expect(os.MkdirAll(root, 0700)).To(Succeed())
	writeFixture(filepath.Join(driver, "factory"), binary, 0700)
	sentinel = filepath.Join(root, "LEGACY_UPGRADE_EXECUTED")
	// No interpolated shell path: the script receives the marker as an environment
	// value and records whether the reserved public route reached legacy code.
	writeFixture(filepath.Join(driver, "scripts/factory-upgrade.sh"), []byte("#!/bin/sh\nprintf called > \"$INSTALLATION_SURFACE_SENTINEL\"\nprintf 'legacy upgrade fallback executed\\n' >&2\nexit 99\n"), 0700)
	return driver, root, sentinel
}

func installationCommand(driver, root, sentinel string, args ...string) cliResult {
	return installationCommandWithAllowance(driver, root, sentinel, 10*time.Second, args...)
}

// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:448
func installationMutationCommand(driver, root, sentinel string, args ...string) cliResult {
	return installationCommandWithAllowance(driver, root, sentinel, 60*time.Second, args...)
}

func installationCommandWithAllowance(driver, root, sentinel string, allowance time.Duration, args ...string) cliResult {
	GinkgoHelper()
	ctx, cancel := context.WithTimeout(context.Background(), allowance)
	defer cancel()
	command := exec.CommandContext(ctx, filepath.Join(driver, "factory"), append([]string{"upgrade"}, args...)...) // #nosec G204 -- independent compiled acceptance CLI, literal public operands.
	command.Dir = root
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if key != "FACTORY_BRIDGE_PROTOCOL" && key != "INSTALLATION_SURFACE_SENTINEL" {
			command.Env = append(command.Env, entry)
		}
	}
	command.Env = append(command.Env, "INSTALLATION_SURFACE_SENTINEL="+sentinel)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	Expect(ctx.Err()).NotTo(HaveOccurred(), "installation command timed out: %s", stderr.String())
	status := 0
	if err != nil {
		var exitError *exec.ExitError
		Expect(errors.As(err, &exitError)).To(BeTrue(), "installation command could not start: %v", err)
		status = exitError.ExitCode()
	}
	result := cliResult{stdout.String(), stderr.String(), status}
	_, _ = fmt.Fprintf(GinkgoWriter, "installation surface command: %q\ncwd: %q\nstdout: %q\nstderr: %q\nstatus: %d\n", command.Args, root, result.stdout, result.stderr, result.status)
	return result
}

var _ = Describe("Whole installation upgrade public interface", func() {
	var driverBinary []byte
	BeforeEach(func() {
		if driverBinary == nil {
			driverBinary = installationBuildDriver()
		}
	})

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:33
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:56
	It("serves installation help through the native owner without legacy dispatch", func() {
		driver, root, sentinel := installationSurfaceFixture(driverBinary)
		out := installationCommand(driver, root, sentinel, "--installation", "--help")
		Expect(out.status).To(Equal(0), "%+v", out)
		Expect(out.stderr).To(BeEmpty())
		for _, operand := range []string{"--installation", "--source", "--profile", "--version", "--revision", "--target", "--dry-run", "--rollback", "--recover"} {
			Expect(out.stdout).To(ContainSubstring(operand))
		}
		_, err := os.Lstat(sentinel)
		Expect(os.IsNotExist(err)).To(BeTrue(), "legacy upgrade script executed")
	})

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:33
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:56
	It("owns malformed installation markers before preview recovery and script fallback", func() {
		cases := [][]string{
			{"--installation"},
			{"--installation=true"},
			{"--installation=false"},
			{"--installation="},
			{"--installation=PRIVATE_INVALID"},
			{"--installation", "--installation"},
			{"--installation", "--dry-run", "--source=."},
			{"--installation=false", "--dry-run", "--source=."},
			{"--installation", "--plan-restore", "--migration-id=private-migration"},
			{"--installation=false", "--plan-restore", "--migration-id=private-migration"},
			{"--source", "--installation"},
		}
		type observation struct {
			args     []string
			result   cliResult
			fallback bool
		}
		observations := make([]observation, 0, len(cases))
		// Capture every surface outcome before assertions, so the initial RED does
		// not hide attached forms or precedence behind the first failing case.
		for _, args := range cases {
			driver, root, sentinel := installationSurfaceFixture(driverBinary)
			out := installationCommand(driver, root, sentinel, args...)
			_, err := os.Lstat(sentinel)
			Expect(err == nil || os.IsNotExist(err)).To(BeTrue())
			observations = append(observations, observation{args, out, err == nil})
		}
		for _, observed := range observations {
			Expect(observed.result.status).To(Equal(2), "args %q: %+v", observed.args, observed.result)
			Expect(observed.result.stdout).To(BeEmpty(), "args %q", observed.args)
			Expect(observed.result.stderr).To(HavePrefix("factory upgrade:"), "args %q", observed.args)
			Expect(observed.result.stderr).To(ContainSubstring("installation"), "args %q must be diagnosed by the installation owner", observed.args)
			Expect(strings.Count(observed.result.stderr, "\n")).To(Equal(1), "args %q", observed.args)
			Expect(observed.result.stderr).NotTo(ContainSubstring("PRIVATE_INVALID"))
			Expect(observed.fallback).To(BeFalse(), "args %q reached legacy upgrade", observed.args)
		}
	})
})

var _ = Describe("Whole installation upgrade empty mode operands", func() {
	var binary []byte
	BeforeEach(func() {
		if binary == nil {
			binary = installationBuildDriver()
		}
	})
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:45
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:50
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:56
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:356
	DescribeTable("distinguishes an absent mode operand from an explicitly empty operand", func(mode string, apply bool) {
		driver, root, sentinel := installationSurfaceFixture(binary)
		args := []string{"--installation", "--source", filepath.Join(root, "not-yet-opened.tar.gz"), "--profile", "bash-baseline", "--version", "v9.8.0", "--revision", strings.Repeat("a", 40), "--target", runtime.GOOS + "/" + runtime.GOARCH, "--local"}
		if apply {
			args = append(args, "--migration-id", "empty-mode-control", "--confirm", strings.Repeat("a", 64), "--quiescent")
		} else {
			args = append(args, "--dry-run", "--json")
		}
		healthySyntax := installationCommand(driver, root, sentinel, args...)
		Expect(healthySyntax.status).To(Equal(1), "valid syntax reaches the operation (unsupported before behavior, missing archive afterward): %+v", healthySyntax)
		malformed := installationCommand(driver, root, sentinel, append(args, mode+"=")...)
		Expect(malformed.status).To(Equal(2), "explicit empty mode is malformed: %+v", malformed)
		Expect(malformed.stdout).To(BeEmpty())
		Expect(malformed.stderr).To(HavePrefix("factory upgrade:"))
		Expect(malformed.stderr).To(ContainSubstring("installation"))
		_, err := os.Lstat(sentinel)
		Expect(os.IsNotExist(err)).To(BeTrue())
	}, Entry("empty rollback on preview", "--rollback", false), Entry("empty recovery on preview", "--recover", false), Entry("empty direction on preview", "--direction", false), Entry("empty confirmation on preview", "--confirm", false), Entry("empty rollback on forward apply", "--rollback", true))

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:41
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:362
	It("preserves a separate flag-looking source operand equivalently to the attached literal", func() {
		driver, root, sentinel := installationSurfaceFixture(binary)
		common := []string{"--installation", "--profile", "bash-baseline", "--version", "v9.8.0", "--revision", strings.Repeat("a", 40), "--target", runtime.GOOS + "/" + runtime.GOARCH, "--local", "--dry-run", "--json"}
		attached := installationCommand(driver, root, sentinel, append(append([]string{}, common...), "--source=--local")...)
		Expect(attached.status).To(Equal(1), "healthy syntax reaches unsupported or missing-archive operation: %+v", attached)
		separate := installationCommand(driver, root, sentinel, append(append([]string{}, common...), "--source", "--local")...)
		Expect(separate).To(Equal(attached), "literal operand is not a second local-mode flag")
	})
})

type installationReference struct {
	profile, revision string
	assets            []imageAssetOracle
	payload           map[string][]byte
}

type installationFixtureData struct {
	driver, root, source, archive, revision, target, profile, sentinel string
	before                                                             map[string]string
}

type installationRoleOracle struct {
	Path       string  `json:"path"`
	SourcePath *string `json:"source_path"`
	Operation  string  `json:"operation"`
	Generator  *string `json:"generator"`
	Mode       *string `json:"mode"`
	Required   bool    `json:"required"`
	Reason     string  `json:"reason"`
}

type installationRoleMapOracle struct {
	Roles                 []installationRoleOracle `json:"roles"`
	CanonicalSourceAssets []installationRoleOracle `json:"canonical_source_assets"`
	PreservePaths         []string                 `json:"preserve_paths"`
	PreservePrefixes      []string                 `json:"preserve_prefixes"`
}

// Build the exact owned fixture commit rather than attaching HEAD identity to
// an uncommitted binary. User settings and test-writer changes stay excluded.
// per docs/adr/0097-installation-source-image-bundles.md:123
func installationTargetSnapshot(repository, destination string) (string, []byte, []imageAssetOracle, map[string][]byte) {
	GinkgoHelper()
	assets, payload := installationGitCatalog(repository, "HEAD")
	installationMaterializeReference(destination, installationReference{assets: assets, payload: payload})
	changed := imageProcess(repository, nil, "git", "diff", "--name-only", "-z", "HEAD")
	Expect(changed.status).To(Equal(0), changed.stderr)
	untracked := imageProcess(repository, nil, "git", "ls-files", "--others", "--exclude-standard", "-z")
	Expect(untracked.status).To(Equal(0), untracked.stderr)
	for _, path := range strings.Split(changed.stdout+untracked.stdout, "\x00") {
		production := strings.HasPrefix(path, "internal/") || strings.HasPrefix(path, "cmd/") || strings.HasPrefix(path, "runtime/")
		// The retained gate's reviewed mode correction belongs to the exact source
		// fixture as well as the compiled consumer; user scripts stay excluded.
		// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:392
		production = production || path == "scripts/hooks/hook-existence-check.sh"
		production = production || path == "Makefile" || path == ".github/workflows/go-runtime.yml"
		if path == "" || !production || strings.HasSuffix(path, "_test.go") {
			continue
		}
		info, err := os.Lstat(filepath.Join(repository, filepath.FromSlash(path)))
		if os.IsNotExist(err) {
			Expect(os.Remove(filepath.Join(destination, filepath.FromSlash(path)))).To(Succeed())
			continue
		}
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode().IsRegular()).To(BeTrue(), "explicit production overlay must be regular source data: %s", path)
		body, err := os.ReadFile(filepath.Join(repository, filepath.FromSlash(path)))
		Expect(err).NotTo(HaveOccurred())
		writeFixture(filepath.Join(destination, filepath.FromSlash(path)), body, info.Mode().Perm())
	}
	imageGit(destination, "init", "--quiet")
	imageGit(destination, "add", "--force", "--all")
	imageGit(destination, "commit", "--quiet", "-m", "installation acceptance target source")
	revision := strings.TrimSpace(imageGit(destination, "rev-parse", "HEAD"))
	binaryPath := filepath.Join(filepath.Dir(destination), "compiled-target")
	args := []string{"build", "-o", binaryPath, "./cmd/factory"}
	if os.Getenv("FACTORY_CLI_TEST_RACE") == "1" {
		args = append([]string{"build", "-race"}, args[1:]...)
	}
	out := imageProcess(destination, nil, "go", args...)
	Expect(out.status).To(Equal(0), "exact fixture commit must compile: %+v", out)
	binary, err := os.ReadFile(binaryPath)
	Expect(err).NotTo(HaveOccurred())
	assets, payload = installationGitCatalog(destination, revision)
	_, _ = fmt.Fprintf(GinkgoWriter, "exact target fixture revision: %s; source rows: %d; binary sha256: %x\n", revision, len(assets), sha256.Sum256(binary))
	return revision, binary, assets, payload
}

var installationInputs = []string{
	"PROJECT_NAME=Installation Project",
	"PROJECT_SLUG=installation-project",
	"GITHUB_OWNER=@factory-fixture",
	"OPENCODE_USERNAME=fixture-user",
	"PROTECTED_PATH=internal",
	"DOCS_ROOT=docs",
	"CITATION_PREFIX=",
}

// Read immutable Git objects in one batch, independently of installationimage,
// packaging, staging and the consumer. per docs/adr/0098-whole-installation-upgrade-and-rollback.md:289
func installationGitCatalog(repository, revision string) ([]imageAssetOracle, map[string][]byte) {
	GinkgoHelper()
	listing := imageProcess(repository, nil, "git", "--no-replace-objects", "ls-tree", "-r", "-z", "--full-tree", revision)
	Expect(listing.status).To(Equal(0), listing.stderr)
	type object struct{ mode, id, path string }
	objects := []object{}
	var requests strings.Builder
	for _, record := range strings.Split(listing.stdout, "\x00") {
		if record == "" {
			continue
		}
		metadata, path, ok := strings.Cut(record, "\t")
		Expect(ok).To(BeTrue())
		fields := strings.Fields(metadata)
		Expect(fields).To(HaveLen(3))
		Expect(fields[1]).To(Equal("blob"))
		objects = append(objects, object{fields[0], fields[2], path})
		requests.WriteString(fields[2] + "\n")
	}
	command := exec.Command("git", "--no-replace-objects", "cat-file", "--batch") // #nosec G204 -- read-only immutable object requests, not shell source.
	command.Dir = repository
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(key, "GIT_") {
			command.Env = append(command.Env, entry)
		}
	}
	command.Stdin = strings.NewReader(requests.String())
	output, err := command.Output()
	Expect(err).NotTo(HaveOccurred())
	reader := bufio.NewReader(bytes.NewReader(output))
	assets := make([]imageAssetOracle, 0, len(objects))
	payload := make(map[string][]byte, len(objects))
	for _, object := range objects {
		header, err := reader.ReadString('\n')
		Expect(err).NotTo(HaveOccurred())
		fields := strings.Fields(header)
		Expect(fields).To(HaveLen(3))
		Expect(fields[0]).To(Equal(object.id))
		Expect(fields[1]).To(Equal("blob"))
		size, err := strconv.Atoi(fields[2])
		Expect(err).NotTo(HaveOccurred())
		body := make([]byte, size)
		_, err = io.ReadFull(reader, body)
		Expect(err).NotTo(HaveOccurred())
		separator, err := reader.ReadByte()
		Expect(err).NotTo(HaveOccurred())
		Expect(separator).To(Equal(byte('\n')))
		assets = append(assets, imageAssetOracle{object.path, object.mode, fmt.Sprintf("%x", sha256.Sum256(body)), int64(len(body))})
		payload[object.path] = body
	}
	_, err = reader.ReadByte()
	Expect(err).To(MatchError(io.EOF))
	sort.Slice(assets, func(i, j int) bool { return assets[i].Path < assets[j].Path })
	return assets, payload
}

func installationMaterializeReference(root string, reference installationReference) {
	GinkgoHelper()
	for _, asset := range reference.assets {
		path := filepath.Join(root, filepath.FromSlash(asset.Path))
		if asset.Mode == "120000" {
			Expect(os.MkdirAll(filepath.Dir(path), 0700)).To(Succeed())
			Expect(os.Symlink(string(reference.payload[asset.Path]), path)).To(Succeed())
			continue
		}
		mode := os.FileMode(0644)
		if asset.Mode == "100755" {
			mode = 0755
		}
		writeFixture(path, reference.payload[asset.Path], mode)
	}
}

func installationSnapshot(root string) map[string]string {
	GinkgoHelper()
	result := map[string]string{}
	Expect(filepath.WalkDir(root, func(path string, _ fs.DirEntry, walkError error) error {
		if walkError != nil {
			return walkError
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		value := fmt.Sprintf("%v:", info.Mode())
		switch {
		case info.Mode().IsRegular():
			body, err := os.ReadFile(path) // #nosec G122 -- quiescent private test fixture; symlink entries are recorded separately rather than read.
			if err != nil {
				return err
			}
			value += fmt.Sprintf("%x", sha256.Sum256(body))
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			value += link
		}
		result[filepath.ToSlash(relative)] = value
		return nil
	})).To(Succeed())
	return result
}

// This executes the actual pinned initializer, including its substitutions,
// sync and installed source selftest. Only external npm/GitHub network entry
// points are stubbed. per docs/adr/0098-whole-installation-upgrade-and-rollback.md:90
func installationLegacyFixture(binary []byte, reference installationReference, archive, revision string) installationFixtureData {
	GinkgoHelper()
	driver, root, sentinel := installationSurfaceFixture(binary)
	source := filepath.Join(filepath.Dir(root), "immutable template")
	installationMaterializeReference(source, reference)
	tools := filepath.Join(filepath.Dir(root), "external tools")
	writeFixture(filepath.Join(tools, "npm"), []byte("#!/bin/sh\nexit 0\n"), 0700)
	writeFixture(filepath.Join(tools, "gh"), []byte("#!/bin/sh\nexit 1\n"), 0700)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "bash", filepath.Join(source, "scripts/factory-init.sh"), root, "--pack", "none") // #nosec G204 -- immutable legacy initializer and fixture-owned output, literal operands.
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.WaitDelay = time.Second
	command.Cancel = func() error {
		if command.Process == nil {
			return nil
		}
		return syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	}
	command.Dir = filepath.Dir(root)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(key, "FACTORY_") && !strings.HasPrefix(key, "GIT_") && key != "PATH" {
			command.Env = append(command.Env, entry)
		}
	}
	command.Env = append(command.Env, "PATH="+tools+string(os.PathListSeparator)+os.Getenv("PATH"), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
	command.Stdin = strings.NewReader(strings.Join([]string{"Installation Project", "installation-project", "@factory-fixture", "fixture-user", "internal", "docs", "", "inherit", "standard", "n", "y", ""}, "\n"))
	output, err := command.CombinedOutput()
	_, _ = fmt.Fprintf(GinkgoWriter, "actual legacy initializer: %s (%s)\n%s\n", reference.profile, reference.revision, output)
	Expect(ctx.Err()).NotTo(HaveOccurred())
	Expect(err).NotTo(HaveOccurred(), "actual %s initializer failed: %s", reference.profile, output)
	Expect(string(output)).To(ContainSubstring("gates proven"), "legacy fixture must actually qualify its installed gates")
	for _, path := range []string{"factory", "scripts/factory-doctor.sh", "scripts/selftest/run.sh", "scripts/hooks/test-edit-denial.sh", "templates/metrics.html", "packs/review-lane/review-pr.yml"} {
		_, err := os.Lstat(filepath.Join(root, path))
		Expect(err).NotTo(HaveOccurred(), "actual legacy fixture omitted %s", path)
	}
	for _, path := range []string{"scripts/lib/budget.py", "scripts/lib/budget_adapters.py", "scripts/lib/loop.py"} {
		_, err := os.Lstat(filepath.Join(root, path))
		if reference.profile == "v0.1.6" {
			Expect(os.IsNotExist(err)).To(BeTrue(), "v0.1.6 has no controller: %s", path)
		} else {
			Expect(err).NotTo(HaveOccurred(), "bash baseline must install controller: %s", path)
		}
	}
	git := imageProcess(root, nil, "git", "-c", "core.hooksPath="+os.DevNull, "init", "--quiet")
	Expect(git.status).To(Equal(0), git.stderr)
	return installationFixtureData{driver, root, source, archive, revision, runtime.GOOS + "/" + runtime.GOARCH, reference.profile, sentinel, installationSnapshot(root)}
}

func installationCloneLegacyFixture(binary []byte, prepared installationFixtureData) installationFixtureData {
	GinkgoHelper()
	driver, root, sentinel := installationSurfaceFixture(binary)
	Expect(filepath.WalkDir(prepared.root, func(path string, _ fs.DirEntry, walkError error) error {
		if walkError != nil {
			return walkError
		}
		relative, err := filepath.Rel(prepared.root, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		destination := filepath.Join(root, relative)
		if info.IsDir() {
			return os.MkdirAll(destination, info.Mode().Perm())
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, destination) // #nosec G122 -- inert link data from the immutable baseline fixture, copied into a new private test root.
		}
		body, err := os.ReadFile(path) // #nosec G122 -- prepared baseline fixture is immutable after its actual initializer and owned proof exit.
		if err != nil {
			return err
		}
		return os.WriteFile(destination, body, info.Mode().Perm()) // #nosec G703 -- WalkDir-relative path inside a new private fixture; the source is an immutable baseline.
	})).To(Succeed())
	prepared.driver, prepared.root, prepared.sentinel = driver, root, sentinel
	prepared.before = installationSnapshot(root)
	return prepared
}

func (fixture installationFixtureData) arguments() []string {
	args := []string{"--installation", "--source", fixture.archive, "--profile", fixture.profile, "--version", "v9.8.0", "--revision", fixture.revision, "--target", fixture.target, "--local"}
	for _, value := range installationInputs {
		args = append(args, "--project-input", value)
	}
	return args
}

func installationProposal(fixture installationFixtureData, extra ...string) map[string]any {
	GinkgoHelper()
	args := append(fixture.arguments(), "--dry-run", "--json")
	args = append(args, extra...)
	out := installationCommand(fixture.driver, fixture.root, fixture.sentinel, args...)
	Expect(out.status).To(Equal(0), "healthy installation preview must be available: %+v", out)
	return installationDecodeProposal(out)
}

func installationDecodeProposal(out cliResult) map[string]any {
	GinkgoHelper()
	Expect(out.stderr).To(BeEmpty())
	var proposal map[string]any
	decoder := json.NewDecoder(strings.NewReader(out.stdout))
	decoder.UseNumber()
	Expect(decoder.Decode(&proposal)).To(Succeed())
	var extraJSON any
	Expect(decoder.Decode(&extraJSON)).To(MatchError(io.EOF))
	Expect(proposal["proposal_digest"]).To(MatchRegexp("^[0-9a-f]{64}$"))
	return proposal
}

func installationRows(proposal map[string]any) map[string]map[string]any {
	GinkgoHelper()
	rows := map[string]map[string]any{}
	Expect(proposal["actions"]).To(BeAssignableToTypeOf([]any{}))
	for _, item := range proposal["actions"].([]any) {
		row := item.(map[string]any)
		path := row["path"].(string)
		Expect(rows).NotTo(HaveKey(path))
		rows[path] = row
	}
	return rows
}

func installationAssertExistingUnchanged(root string, before map[string]string) {
	GinkgoHelper()
	after := installationSnapshot(root)
	for path, value := range before {
		Expect(after[path]).To(Equal(value), "refused operation changed existing entry %s", path)
	}
	for _, path := range []string{".factory/installation.current", ".factory/bin/factory-runtime"} {
		if _, existed := before[path]; !existed {
			Expect(after).NotTo(HaveKey(path), "refused operation activated %s", path)
		}
	}
}

func installationForwardArguments(fixture installationFixtureData, migration, digest string) []string {
	return append(fixture.arguments(), "--migration-id", migration, "--confirm", digest, "--quiescent")
}

func installationRewrittenArchive(fixture installationFixtureData, mutate func([]stageEntry) []stageEntry) string {
	GinkgoHelper()
	data, err := os.ReadFile(fixture.archive)
	Expect(err).NotTo(HaveOccurred())
	entries, _ := imageDecodeArchive(data)
	path := filepath.Join(filepath.Dir(fixture.root), "changed-image.tar.gz")
	writeFixture(path, stageArchive(mutate(entries)), 0600)
	return path
}

func installationApply(fixture installationFixtureData, migration string) {
	GinkgoHelper()
	proposal := installationProposal(fixture)
	args := append(fixture.arguments(), "--migration-id", migration, "--confirm", proposal["proposal_digest"].(string), "--quiescent")
	out := installationMutationCommand(fixture.driver, fixture.root, fixture.sentinel, args...)
	Expect(out.status).To(Equal(0), "healthy complete installation activation failed: %+v", out)
	Expect(out.stderr).To(BeEmpty())
	_, err := os.Lstat(fixture.sentinel)
	Expect(os.IsNotExist(err)).To(BeTrue(), "whole consumer executed legacy upgrade")
}

// This watcher counts only a stopped live process with the committed journal
// and pending marker still present, followed by an actual SIGKILL exit.
// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:425
func installationInterruptTerminal(fixture installationFixtureData, migration, digest string) bool {
	_, witnessed := installationInterruptPhase(fixture, migration, digest, "committed", false)
	return witnessed
}

// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:187
func installationInterruptPhase(fixture installationFixtureData, migration, digest, phase string, createdRow bool) (string, bool) {
	GinkgoHelper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, filepath.Join(fixture.driver, "factory"), append([]string{"upgrade"}, installationForwardArguments(fixture, migration, digest)...)...) // #nosec G204 -- evaluator-owned driver and literal qualified operands.
	command.Dir = fixture.root
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.WaitDelay = time.Second
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "FACTORY_BRIDGE_PROTOCOL=") {
			command.Env = append(command.Env, entry)
		}
	}
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	Expect(command.Start()).To(Succeed())
	finished := make(chan error, 1)
	go func() { finished <- command.Wait() }()
	waited := false
	defer func() {
		if !waited {
			_ = syscall.Kill(command.Process.Pid, syscall.SIGCONT)
			_ = syscall.Kill(command.Process.Pid, syscall.SIGINT)
			select {
			case <-finished:
			case <-time.After(5 * time.Second):
				_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
				<-finished
			}
		}
	}()
	journalPath := filepath.Join(fixture.root, ".factory/installation-transactions", migration+".json")
	pendingPath := filepath.Join(fixture.root, ".factory/runtime-publication.pending")
	selectCreated := func(journal map[string]any) string {
		entries, ok := journal["entries"].([]any)
		if !ok {
			return ""
		}
		applied := 0
		selected := ""
		for _, raw := range entries {
			entry, ok := raw.(map[string]any)
			if !ok || entry["phase"] != "applied" {
				continue
			}
			applied++
			path, _ := entry["path"].(string)
			after, hasAfter := entry["observed_after"].(map[string]any)
			if selected == "" && entry["action"] == "create" && strings.HasPrefix(path, ".factory/assets/scaffold/") && hasAfter && after["identity"] != nil {
				selected = path
			}
		}
		if applied < 2 {
			return ""
		}
		return selected
	}
	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-finished:
			waited = true
			Expect(err).NotTo(HaveOccurred(), "healthy terminal sibling failed before a crash witness: %s", stderr.String())
			_, _ = fmt.Fprintf(GinkgoWriter, "terminal cleanup completed before SIGSTOP witness: %s\n", migration)
			return "", false
		default:
		}
		var journal map[string]any
		ready := false
		if createdRow {
			body, err := os.ReadFile(journalPath)
			ready = err == nil && json.Unmarshal(body, &journal) == nil && journal["phase"] == phase
		} else {
			// This bounded prefix is only a fast wake-up probe. The actual witness
			// still requires the complete stopped journal and exact pending entry.
			file, err := os.Open(journalPath)
			if err == nil {
				var prefix [1024]byte
				n, readErr := file.Read(prefix[:])
				closeErr := file.Close()
				if closeErr == nil && (readErr == nil || errors.Is(readErr, io.EOF)) {
					ready = bytes.Contains(prefix[:n], []byte(`"phase":"`+phase+`"`)) || bytes.Contains(prefix[:n], []byte(`"phase": "`+phase+`"`))
				}
			}
		}
		if ready {
			selected := ""
			if createdRow {
				selected = selectCreated(journal)
				if selected == "" {
					continue
				}
			}
			if err := syscall.Kill(command.Process.Pid, syscall.SIGSTOP); err != nil {
				continue
			}
			stoppedBody, journalErr := os.ReadFile(journalPath)
			pending, pendingErr := os.Lstat(pendingPath)
			var stoppedJournal map[string]any
			witness := journalErr == nil && json.Unmarshal(stoppedBody, &stoppedJournal) == nil && stoppedJournal["phase"] == phase && pendingErr == nil && pending.Mode().IsRegular()
			if createdRow {
				witness = witness && selectCreated(stoppedJournal) == selected
			}
			if !witness && createdRow {
				_ = syscall.Kill(command.Process.Pid, syscall.SIGCONT)
				_ = syscall.Kill(command.Process.Pid, syscall.SIGINT)
				select {
				case <-finished:
					waited = true
					return "", false
				case <-time.After(5 * time.Second):
					Fail("missed applying witness and owned driver did not finish cancellation cleanup")
				}
			}
			Expect(syscall.Kill(-command.Process.Pid, syscall.SIGKILL)).To(Succeed())
			err := <-finished
			waited = true
			if !witness {
				_, _ = fmt.Fprintf(GinkgoWriter, "terminal phase was already cleaned before stop: %s\n", migration)
				return "", false
			}
			var exit *exec.ExitError
			Expect(errors.As(err, &exit)).To(BeTrue(), "terminal witness must end in an actual signal exit")
			status, ok := exit.Sys().(syscall.WaitStatus)
			Expect(ok).To(BeTrue())
			Expect(status.Signal()).To(Equal(syscall.SIGKILL))
			_, err = os.Lstat(pendingPath)
			Expect(err).NotTo(HaveOccurred(), "SIGKILL cannot erase witnessed unresolved pending")
			_, _ = fmt.Fprintf(GinkgoWriter, "actual %s/pending SIGSTOP then SIGKILL witness: pid=%d id=%s row=%s\n", phase, command.Process.Pid, migration, selected)
			return selected, true
		}
		if createdRow {
			time.Sleep(100 * time.Microsecond)
		} else {
			runtime.Gosched()
		}
	}
	Fail("healthy installer did not reach a committed terminal witness within the bounded observation window")
	return "", false
}

var _ = Describe("Whole installation upgrade semantic consumer", Ordered, ContinueOnFailure, func() {
	var binary []byte
	var references map[string]installationReference
	var prepared map[string]installationFixtureData
	var archive, revision, stager string
	var roleMap installationRoleMapOracle
	BeforeAll(func() {
		binary = installationBuildDriver()
		repository, err := filepath.Abs("..")
		Expect(err).NotTo(HaveOccurred())
		references = map[string]installationReference{}
		baselines := []imageBaselineOracle{}
		for _, reference := range []installationReference{{profile: "v0.1.6", revision: "b71ecc32e07ecd87eb330ba8e497c86612f92acd"}, {profile: "bash-baseline", revision: "76952eaa63aebd1ecd282f5ab51dd7c3627cb497"}} {
			reference.assets, reference.payload = installationGitCatalog(repository, reference.revision)
			references[reference.profile] = reference
			baselines = append(baselines, imageBaselineOracle{reference.profile, reference.revision, reference.assets})
		}
		Expect(references["v0.1.6"].assets).To(HaveLen(137))
		Expect(references["bash-baseline"].assets).To(HaveLen(160))
		Expect(imageCatalogDigest(references["v0.1.6"].assets)).To(Equal("595f75dddcda285ba3b43a0936970ef96af3b9eae66f4beb9fd3a37df5d7e16b"))
		Expect(imageCatalogDigest(references["bash-baseline"].assets)).To(Equal("8514055a8ee6f79a8fd891a6107e46bd9d0f1a2096e349825db83aafc7b91bbe"))
		work, err := os.MkdirTemp("", "factory-whole-installation-archive-")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(os.RemoveAll, work)
		var targetBinary []byte
		var assets []imageAssetOracle
		var payload map[string][]byte
		revision, targetBinary, assets, payload = installationTargetSnapshot(repository, filepath.Join(work, "committed-target-source"))
		for _, asset := range assets {
			Expect(asset.SHA256).To(MatchRegexp("^[0-9a-f]{64}$"), asset.Path)
			Expect(asset.Bytes).To(BeNumerically("<=", 1<<20), "exact production source asset exceeds installation image limit: %s", asset.Path)
		}
		manifest := imageManifestOracle{1, "installation_source_image", "committed_source_and_baseline_references", "v9.8.0", runtime.GOOS + "/" + runtime.GOARCH, revision, false, assets, baselines}
		archive = filepath.Join(work, "factory-runtime.tar.gz")
		writeFixture(archive, stageArchive(imageFixtureEntries(manifest, payload, targetBinary)), 0600)
		canon, err := os.ReadFile(filepath.Join(repository, "docs/migration/INSTALLATION_ROLE_MAP.json"))
		Expect(err).NotTo(HaveOccurred())
		Expect(json.Unmarshal(canon, &roleMap)).To(Succeed())
		Expect(roleMap.Roles).To(HaveLen(51))
		// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:440
		Expect(roleMap.CanonicalSourceAssets).To(HaveLen(85))
		stager = filepath.Join(work, "factory-stage")
		out := imageProcess(repository, nil, "go", "build", "-o", stager, "./cmd/factory-stage")
		Expect(out.status).To(Equal(0), out.stderr)
		out = imageProcess(repository, nil, stager, "--installation", "--archive", archive, "--output", filepath.Join(work, "independently-staged"), "--version", manifest.Version, "--revision", revision, "--target", manifest.Target, "--local")
		Expect(out.status).To(Equal(0), "healthy complete archive must pass the existing stager: %+v", out)
		prepared = map[string]installationFixtureData{}
		for _, profile := range []string{"v0.1.6", "bash-baseline"} {
			prepared[profile] = installationLegacyFixture(binary, references[profile], archive, revision)
		}
	})
	fixture := func(profile string) installationFixtureData {
		return installationCloneLegacyFixture(binary, prepared[profile])
	}

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:90
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:117
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:130
	DescribeTable("proposes a complete read-only installation from actual legacy init", func(profile string) {
		installed := fixture(profile)
		proposal := installationProposal(installed)
		Expect(proposal["authentication"]).To(Equal("local-source"))
		for _, field := range []string{"checks", "conflicts", "blockers", "rollback_blockers"} {
			Expect(proposal[field]).To(BeAssignableToTypeOf([]any{}), field)
		}
		for _, field := range []string{"applicable", "checks_ready", "success", "rollback_ready"} {
			Expect(proposal[field]).To(BeFalse(), "read-only preview cannot claim completed %s", field)
		}
		Expect(proposal["actions"]).To(BeAssignableToTypeOf([]any{}))
		actions := proposal["actions"].([]any)
		Expect(len(actions)).To(BeNumerically(">", 6), "a whole installation cannot be the old six-file scope")
		seen := map[string]string{}
		for _, raw := range actions {
			row := raw.(map[string]any)
			for _, field := range []string{"path", "source_path", "action", "reason", "required", "before", "after"} {
				Expect(row).To(HaveKey(field))
			}
			path := row["path"].(string)
			Expect(seen).NotTo(HaveKey(path), "duplicate proposal row")
			seen[path] = row["action"].(string)
		}
		// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:257
		for _, role := range append(append([]installationRoleOracle{}, roleMap.Roles...), roleMap.CanonicalSourceAssets...) {
			Expect(seen).To(HaveKey(role.Path), "omitted reviewed installation role")
		}
		allowed := map[string]bool{}
		for _, role := range append(append([]installationRoleOracle{}, roleMap.Roles...), roleMap.CanonicalSourceAssets...) {
			allowed[role.Path] = true
		}
		for path, action := range seen {
			if action != "retain" && !allowed[path] {
				Fail("unreviewed installation action: " + path)
			}
		}
		for _, path := range []string{"go.mod", "go.sum", "cmd/factory/main.go", "acceptance/installation_image_test.go", "AGENTS.md", "factory.yaml", ".opencode/package.json", "Makefile", "README.md"} {
			if action, exists := seen[path]; exists {
				Expect(action).To(Equal("retain"), "source/product/user file selected for mutation: %s", path)
			}
		}
		Expect(installationSnapshot(installed.root)).To(Equal(installed.before))
	}, Entry("v0.1.6 genuine installation", "v0.1.6"), Entry("bash baseline genuine installation", "bash-baseline"))

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:19
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:67
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:102
	DescribeTable("activates the complete target and runs the installed native proof", func(profile string) {
		installed := fixture(profile)
		installationApply(installed, "whole-forward")
		for _, path := range []string{".factory/bin/factory-runtime", ".factory/bin/runtime.manifest", ".factory/bin/source.json", ".factory/installation.current"} {
			info, err := os.Lstat(filepath.Join(installed.root, path))
			Expect(err).NotTo(HaveOccurred(), "missing declared installed control: %s", path)
			Expect(info.Mode().IsRegular()).To(BeTrue())
			want := os.FileMode(0600)
			if path == ".factory/bin/factory-runtime" {
				want = 0700
			}
			Expect(info.Mode().Perm()).To(Equal(want), path)
		}
		out := imageProcess(installed.root, map[string]string{"FACTORY_BRIDGE_PROTOCOL": ""}, filepath.Join(installed.root, "factory"), "selftest")
		Expect(out.status).To(Equal(0), "installed native proof failed: %+v", out)
		Expect(out.stdout).NotTo(BeEmpty(), "installed proof cannot succeed from status alone")
		// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:207
		forbiddenMarker := filepath.Join(filepath.Dir(installed.root), "FORBIDDEN_PROOF_TOOL_EXECUTED")
		forbiddenTools := filepath.Join(filepath.Dir(installed.root), "forbidden proof tools")
		for _, tool := range []string{"go", "python", "python3", "claude", "codex", "opencode", "curl", "gh", "npm"} {
			writeFixture(filepath.Join(forbiddenTools, tool), []byte("#!/bin/sh\nprintf called > \"$INSTALLATION_PROOF_TOOL_SENTINEL\"\nexit 98\n"), 0700)
		}
		out = imageProcess(installed.root, map[string]string{"FACTORY_BRIDGE_PROTOCOL": "", "PATH": forbiddenTools + string(os.PathListSeparator) + os.Getenv("PATH"), "INSTALLATION_PROOF_TOOL_SENTINEL": forbiddenMarker}, filepath.Join(installed.root, "factory"), "selftest")
		Expect(out.status).To(Equal(0), "native installed proof depends on a forbidden runtime tool: %+v", out)
		_, err := os.Lstat(forbiddenMarker)
		Expect(os.IsNotExist(err)).To(BeTrue(), "installed proof called a compiler, Python, model/client or download tool")
		_, err = os.Lstat(filepath.Join(installed.root, ".factory/runtime-publication.pending"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		for _, path := range []string{"scripts/lib/budget.py", "scripts/lib/budget_adapters.py", "scripts/lib/loop.py"} {
			_, err := os.Lstat(filepath.Join(installed.root, path))
			Expect(os.IsNotExist(err)).To(BeTrue(), "legacy controller retained after complete activation: %s", path)
		}
		for _, path := range []string{"factory.yaml", "AGENTS.md", "opencode.json", ".opencode/package.json", "Makefile", "README.md", ".github/CODEOWNERS", ".github/workflows/ci.yml"} {
			Expect(installationSnapshot(installed.root)[path]).To(Equal(installed.before[path]), "user file changed: %s", path)
		}
	}, Entry("v0.1.6 complete activation", "v0.1.6"), Entry("bash baseline complete activation", "bash-baseline"))

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:50
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:229
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:297
	DescribeTable("uses fresh consent to restore a completed legacy installation", func(profile string) {
		installed := fixture(profile)
		installationApply(installed, "whole-rollback")
		proposal := installationProposal(installed, "--rollback", "whole-rollback")
		args := append(installed.arguments(), "--rollback", "whole-rollback", "--confirm", proposal["proposal_digest"].(string), "--quiescent")
		out := installationMutationCommand(installed.driver, installed.root, installed.sentinel, args...)
		Expect(out.status).To(Equal(0), "qualified completed rollback failed: %+v", out)
		Expect(out.stderr).To(BeEmpty())
		after := installationSnapshot(installed.root)
		for path, value := range installed.before {
			if !strings.HasPrefix(path, ".git/") && path != ".gitignore" {
				Expect(after[path]).To(Equal(value), "legacy before-image not restored: %s", path)
			}
		}
		for _, path := range []string{".factory/installation.current", ".factory/bin/factory-runtime", ".factory/runtime-publication.pending"} {
			_, err := os.Lstat(filepath.Join(installed.root, path))
			Expect(os.IsNotExist(err)).To(BeTrue(), "legacy rollback left active target custody: %s", path)
		}
	}, Entry("v0.1.6 fresh completed rollback", "v0.1.6"), Entry("bash baseline fresh completed rollback", "bash-baseline"))

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:130
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:184
	DescribeTable("refuses stale consent after a fresh healthy proposal", func(change string) {
		installed := fixture("bash-baseline")
		proposal := installationProposal(installed)
		path := filepath.Join(installed.root, "scripts/factory-doctor.sh")
		switch change {
		case "bytes":
			writeFixture(path, []byte("#!/bin/sh\nprintf customized\\n\n"), 0755)
		case "mode":
			Expect(os.Chmod(path, 0700)).To(Succeed())
		case "identity":
			body, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(os.Rename(path, path+".foreign-original")).To(Succeed())
			writeFixture(path, body, 0755)
		case "absence":
			writeFixture(filepath.Join(installed.root, "scripts/factory-init.sh"), []byte("foreign creation\n"), 0600)
		case "root":
			installed = fixture("bash-baseline")
		}
		before := installationSnapshot(installed.root)
		out := installationMutationCommand(installed.driver, installed.root, installed.sentinel, installationForwardArguments(installed, "stale-consent", proposal["proposal_digest"].(string))...)
		Expect(out.status).To(Equal(2), "%+v", out)
		Expect(out.stderr).NotTo(ContainSubstring("unsupported"))
		installationAssertExistingUnchanged(installed.root, before)
	}, Entry("changed selected bytes", "bytes"), Entry("changed full mode", "mode"), Entry("equal bytes new leaf identity", "identity"), Entry("late foreign checked absence", "absence"), Entry("copied confirmation at another physical root", "root"))

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:84
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:184
	DescribeTable("names and preserves mandatory conflicts after a healthy sibling", func(change string) {
		installed := fixture("bash-baseline")
		installationProposal(installed)
		path := "scripts/factory-doctor.sh"
		absolute := filepath.Join(installed.root, path)
		switch change {
		case "bytes":
			writeFixture(absolute, []byte("#!/bin/sh\nprintf customized\\n\n"), 0755)
		case "mode":
			Expect(os.Chmod(absolute, 0700)).To(Succeed())
		case "symlink":
			Expect(os.Remove(absolute)).To(Succeed())
			Expect(os.Symlink(filepath.Join(installed.root, "README.md"), absolute)).To(Succeed())
		case "fifo":
			Expect(os.Remove(absolute)).To(Succeed())
			Expect(syscall.Mkfifo(absolute, 0600)).To(Succeed())
		case "parent":
			path = "templates/metrics.html"
			Expect(os.Rename(filepath.Join(installed.root, "templates"), filepath.Join(installed.root, "foreign-template-parent"))).To(Succeed())
			Expect(os.Symlink("foreign-template-parent", filepath.Join(installed.root, "templates"))).To(Succeed())
		case "transformation":
			path = "scripts/hooks/test-edit-denial.sh"
			absolute = filepath.Join(installed.root, path)
			body, err := os.ReadFile(absolute)
			Expect(err).NotTo(HaveOccurred())
			writeFixture(absolute, append(body, []byte("\n# independently customized transformed framework hook\n")...), 0755)
		}
		before := installationSnapshot(installed.root)
		out := installationCommand(installed.driver, installed.root, installed.sentinel, append(installed.arguments(), "--dry-run", "--json")...)
		Expect(out.status).To(Equal(2), "%+v", out)
		proposal := installationDecodeProposal(out)
		Expect(proposal["conflicts"]).NotTo(BeEmpty())
		Expect(out.stdout).To(ContainSubstring(path))
		row := installationRows(proposal)[path]
		Expect(row["required"]).To(BeTrue())
		Expect(row["action"]).NotTo(BeElementOf("replace", "create", "retire"))
		Expect(installationSnapshot(installed.root)).To(Equal(before))
	}, Entry("custom required bytes", "bytes"), Entry("unexpected required mode", "mode"), Entry("required leaf symlink", "symlink"), Entry("required leaf FIFO", "fifo"), Entry("unsafe required parent", "parent"), Entry("unknown baseline transformation", "transformation"))

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:312
	DescribeTable("rejects unbound or ambiguous project inputs beside a valid request", func(change string) {
		installed := fixture("bash-baseline")
		if change == "missing" {
			// A required input must occur in an actual selected transformation;
			// this complete local image makes the declared target row use it.
			// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:315
			installed.archive = installationRewrittenArchive(installed, func(entries []stageEntry) []stageEntry {
				var manifest imageManifestOracle
				Expect(json.Unmarshal(entries[3].body, &manifest)).To(Succeed())
				selected := -1
				for index, asset := range manifest.Assets {
					if asset.Path == "scripts/hooks/test-edit-denial.sh" {
						selected = index
						break
					}
				}
				Expect(selected).To(BeNumerically(">=", 0))
				body := append(entries[4+selected].body, []byte("\n# Project __PROJECT_NAME__\n")...)
				manifest.Assets[selected].Bytes = int64(len(body))
				manifest.Assets[selected].SHA256 = fmt.Sprintf("%x", sha256.Sum256(body))
				entries[4+selected].body = body
				entries[3].body = imageMarshal(manifest)
				return entries
			})
			qualified := imageProcess(installed.root, nil, stager, "--installation", "--archive", installed.archive, "--output", filepath.Join(filepath.Dir(installed.root), "qualified-transformed-source"), "--version", "v9.8.0", "--revision", installed.revision, "--target", installed.target, "--local")
			Expect(qualified.status).To(Equal(0), "complete transformed-input carrier must pass the public stager: %+v", qualified)
		}
		installationProposal(installed)
		args := append(installed.arguments(), "--dry-run", "--json")
		switch change {
		case "duplicate":
			args = append(args, "--project-input", "PROJECT_NAME=Another Project")
		case "unknown":
			args = append(args, "--project-input", "PRIVATE_INPUT=secret literal")
		case "malformed":
			args = append(args, "--project-input", "PROJECT_NAME")
		case "missing":
			for index := 0; index < len(args)-1; index++ {
				if args[index] == "--project-input" && args[index+1] == installationInputs[0] {
					args = append(args[:index], args[index+2:]...)
					break
				}
			}
		}
		before := installationSnapshot(installed.root)
		out := installationCommand(installed.driver, installed.root, installed.sentinel, args...)
		Expect(out.status).To(Equal(2), "%+v", out)
		Expect(out.stderr).NotTo(ContainSubstring("unsupported"))
		Expect(out.stderr).NotTo(ContainSubstring("secret literal"))
		Expect(installationSnapshot(installed.root)).To(Equal(before))
	}, Entry("duplicate allowlisted key", "duplicate"), Entry("unknown key", "unknown"), Entry("no literal value separator", "malformed"), Entry("missing selected transformation input", "missing"))

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:73
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:84
	DescribeTable("preserves custom optional and application files during complete activation", func(path string) {
		installed := fixture("bash-baseline")
		custom := []byte("private application-owned bytes\n")
		writeFixture(filepath.Join(installed.root, path), custom, 0600)
		proposal := installationProposal(installed)
		if row, selected := installationRows(proposal)[path]; selected {
			Expect(row["action"]).To(Equal("retain"))
		}
		installationApply(installed, "preserve-custom")
		actual, err := os.ReadFile(filepath.Join(installed.root, path))
		Expect(err).NotTo(HaveOccurred())
		Expect(actual).To(Equal(custom))
	}, Entry("unrecognized later transition implementation", "scripts/lib/runtime_transition.py"), Entry("custom optional budget controller", "scripts/lib/budget.py"), Entry("application Makefile", "Makefile"))

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:109
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:255
	DescribeTable("uses installed native commands and canonical data from a different application cwd", func(command string) {
		installed := fixture("bash-baseline")
		installationApply(installed, "native-assets")
		application := filepath.Join(filepath.Dir(installed.root), "separate application")
		Expect(os.MkdirAll(application, 0700)).To(Succeed())
		git := imageProcess(application, nil, "git", "init", "--quiet")
		Expect(git.status).To(Equal(0), git.stderr)
		var out cliResult
		switch command {
		case "metrics":
			out = imageProcess(application, nil, filepath.Join(installed.root, "factory"), "metrics", "--html", "--no-open")
			Expect(out.status).To(Equal(0), "%+v", out)
			html, err := os.ReadFile(filepath.Join(application, ".factory/metrics.html"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(html)).To(ContainSubstring("<html"))
		case "review-lane":
			writeFixture(filepath.Join(application, "factory.yaml"), []byte("review_lane: off\n"), 0600)
			out = imageProcess(application, nil, filepath.Join(installed.root, "factory"), "review-lane", "enable")
			Expect(out.status).To(Equal(0), "%+v", out)
			workflow, err := os.ReadFile(filepath.Join(application, ".github/workflows/adversarial-review.yml"))
			Expect(err).NotTo(HaveOccurred())
			Expect(workflow).NotTo(BeEmpty())
		case "direct-help":
			out = imageProcess(application, nil, filepath.Join(installed.root, "scripts/factory-budget.sh"), "--help")
			Expect(out.status).To(Equal(0), "%+v", out)
			// per docs/adr/0072-go-budget-argument-compatibility.md:37
			Expect(out.stdout).To(ContainSubstring("Available Commands:"))
			for _, description := range []string{"Budget plan", "Budget report", "Budget run"} {
				Expect(out.stdout).To(ContainSubstring(description))
			}
		}
		Expect(out.stderr).To(BeEmpty())
	}, Entry("native metrics template lookup", "metrics"), Entry("native review template lookup", "review-lane"), Entry("generated direct native adapter", "direct-help"))

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:109
	DescribeTable("refuses corrupted installed custody after a healthy installed invocation", func(path string) {
		installed := fixture("bash-baseline")
		installationApply(installed, "custody-controls")
		launcher := filepath.Join(installed.root, "factory")
		healthy := imageProcess(installed.root, nil, launcher, "--help")
		Expect(healthy.status).To(Equal(0), "%+v", healthy)
		absolute := filepath.Join(installed.root, path)
		if path == ".factory/bin/runtime.manifest" {
			Expect(os.Remove(absolute)).To(Succeed())
		} else {
			info, err := os.Stat(absolute)
			Expect(err).NotTo(HaveOccurred())
			writeFixture(absolute, []byte("corrupted installed selection\n"), info.Mode().Perm())
		}
		out := imageProcess(installed.root, nil, launcher, "--help")
		Expect(out.status).NotTo(Equal(0), "%+v", out)
		Expect(out.stdout).To(BeEmpty())
		Expect(out.stderr).NotTo(BeEmpty())
	}, Entry("runtime binary bytes", ".factory/bin/factory-runtime"), Entry("missing binary manifest", ".factory/bin/runtime.manifest"), Entry("descriptor bytes", ".factory/installation.current"), Entry("canonical native data", ".factory/assets/scaffold/templates/metrics.html"))

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:215
	DescribeTable("blocks v0.1.6 downgrade when modern configuration requires unavailable controls", func(key string) {
		installed := fixture("v0.1.6")
		installationApply(installed, "legacy-downgrade")
		path := filepath.Join(installed.root, "factory.yaml")
		body, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		writeFixture(path, append(body, []byte("\n"+key+": true\n")...), 0644)
		before := installationSnapshot(installed.root)
		out := installationCommand(installed.driver, installed.root, installed.sentinel, append(installed.arguments(), "--rollback", "legacy-downgrade", "--dry-run", "--json")...)
		Expect(out.status).To(Equal(2), "%+v", out)
		proposal := installationDecodeProposal(out)
		Expect(proposal["rollback_blockers"]).NotTo(BeEmpty())
		Expect(installationSnapshot(installed.root)).To(Equal(before))
	}, Entry("enabled budget controller", "budget_enabled"), Entry("enabled loop controller", "loop_enabled"))

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:229
	DescribeTable("keeps later user edits when completed rollback cannot restore a selected entry", func(path string) {
		installed := fixture("bash-baseline")
		installationApply(installed, "late-rollback-edit")
		writeFixture(filepath.Join(installed.root, path), []byte("later user edit\n"), 0644)
		before := installationSnapshot(installed.root)
		out := installationCommand(installed.driver, installed.root, installed.sentinel, append(installed.arguments(), "--rollback", "late-rollback-edit", "--dry-run", "--json")...)
		Expect(out.status).To(Equal(2), "%+v", out)
		proposal := installationDecodeProposal(out)
		Expect(proposal["conflicts"]).NotTo(BeEmpty())
		Expect(out.stdout).To(ContainSubstring(path))
		Expect(installationSnapshot(installed.root)).To(Equal(before))
	}, Entry("later adapter bytes", "scripts/factory-doctor.sh"), Entry("later copied native data", "templates/metrics.html"))

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:233
	It("does not create another before-image set when the identical completed operation is repeated", func() {
		installed := fixture("bash-baseline")
		proposal := installationProposal(installed)
		args := installationForwardArguments(installed, "completed-idempotence", proposal["proposal_digest"].(string))
		first := installationMutationCommand(installed.driver, installed.root, installed.sentinel, args...)
		Expect(first.status).To(Equal(0), "%+v", first)
		before := installationSnapshot(installed.root)
		second := installationMutationCommand(installed.driver, installed.root, installed.sentinel, args...)
		Expect(second.status).To(Equal(0), "%+v", second)
		Expect(installationSnapshot(installed.root)).To(Equal(before))
	})

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:150
	It("refuses installation under an independently retained shared physical-root lease", func() {
		installed := fixture("bash-baseline")
		proposal := installationProposal(installed)
		root, err := os.Open(installed.root)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(root.Close)
		Expect(syscall.Flock(int(root.Fd()), syscall.LOCK_SH|syscall.LOCK_NB)).To(Succeed())
		DeferCleanup(syscall.Flock, int(root.Fd()), syscall.LOCK_UN)
		before := installationSnapshot(installed.root)
		out := installationMutationCommand(installed.driver, installed.root, installed.sentinel, installationForwardArguments(installed, "retained-reader", proposal["proposal_digest"].(string))...)
		Expect(out.status).To(Equal(2), "%+v", out)
		Expect(out.stderr).NotTo(ContainSubstring("unsupported"))
		Expect(installationSnapshot(installed.root)).To(Equal(before))
	})

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:150
	It("refuses an installed read-only command under an independent exclusive root lease", func() {
		installed := fixture("bash-baseline")
		installationApply(installed, "retained-installer")
		launcher := filepath.Join(installed.root, "factory")
		healthy := imageProcess(installed.root, nil, launcher, "budget", "report", "--json")
		Expect(healthy.status).To(Equal(0), "%+v", healthy)
		root, err := os.Open(installed.root)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(root.Close)
		Expect(syscall.Flock(int(root.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)).To(Succeed())
		DeferCleanup(syscall.Flock, int(root.Fd()), syscall.LOCK_UN)
		out := imageProcess(installed.root, nil, launcher, "budget", "report", "--json")
		Expect(out.status).To(Equal(2), "%+v", out)
		Expect(out.stdout).To(BeEmpty())
	})

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:145
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:275
	DescribeTable("does not bypass pending admission through public or sourceable bridge dispatch", func(bridge bool) {
		installed := fixture("bash-baseline")
		installationApply(installed, "ordinary-pending")
		launcher := filepath.Join(installed.root, "factory")
		healthy := imageProcess(installed.root, nil, launcher, "budget", "report", "--json")
		Expect(healthy.status).To(Equal(0), "%+v", healthy)
		pending := filepath.Join(installed.root, ".factory/runtime-publication.pending")
		writeFixture(pending, []byte("unresolved installation evidence\n"), 0600)
		var out cliResult
		if bridge {
			out = imageProcess(installed.root, map[string]string{"FACTORY_ROOT": installed.root}, "bash", "-c", `. "$1"; factory_config_get project_name`, "bridge-probe", filepath.Join(installed.root, "scripts/lib/config.sh"))
		} else {
			out = imageProcess(installed.root, map[string]string{"FACTORY_INSTALLATION_BYPASS": "1", "FACTORY_QUIESCENT": "true"}, launcher, "budget", "report", "--json")
		}
		Expect(out.status).NotTo(Equal(0), "%+v", out)
		Expect(out.stdout).To(BeEmpty())
		body, err := os.ReadFile(pending)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(body)).To(Equal("unresolved installation evidence\n"))
	}, Entry("public command and forged environment values", false), Entry("sourceable Go bridge", true))

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:40
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:122
	DescribeTable("rejects unqualified sources beside a genuinely qualified archive", func(change string) {
		installed := fixture("bash-baseline")
		proposal := installationProposal(installed)
		switch change {
		case "directory":
			installed.archive = installed.source
		case "damaged":
			path := filepath.Join(filepath.Dir(installed.root), "damaged-archive.tar.gz")
			writeFixture(path, []byte("not an archive\n"), 0600)
			installed.archive = path
		case "fresh-archive":
			installed.archive = installationRewrittenArchive(installed, func(entries []stageEntry) []stageEntry {
				entries[0].body = append(entries[0].body, '\n')
				entries[1].body = []byte(strings.Replace(string(entries[1].body), fmt.Sprintf("%x", sha256.Sum256(entries[0].body[:len(entries[0].body)-1])), fmt.Sprintf("%x", sha256.Sum256(entries[0].body)), 1))
				return entries
			})
		}
		before := installationSnapshot(installed.root)
		out := installationMutationCommand(installed.driver, installed.root, installed.sentinel, installationForwardArguments(installed, "changed-source", proposal["proposal_digest"].(string))...)
		Expect(out.status).To(BeElementOf(1, 2), "%+v", out)
		Expect(out.stderr).NotTo(ContainSubstring("unsupported"))
		installationAssertExistingUnchanged(installed.root, before)
	}, Entry("Git checkout is not an archive", "directory"), Entry("invalid archive bytes", "damaged"), Entry("fresh changed archive after consent", "fresh-archive"))

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:175
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:326
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:367
	It("keeps one inert complete before-image set and actual terminal journal without changing the Git index", func() {
		installed := fixture("bash-baseline")
		imageGit(installed.root, "add", "--force", "README.md")
		index, err := os.ReadFile(filepath.Join(installed.root, ".git/index"))
		Expect(err).NotTo(HaveOccurred())
		proposal := installationProposal(installed)
		originals := map[string][]byte{}
		for path, row := range installationRows(proposal) {
			if row["action"] == "replace" || row["action"] == "retire" {
				body, err := os.ReadFile(filepath.Join(installed.root, filepath.FromSlash(path)))
				Expect(err).NotTo(HaveOccurred())
				originals[path] = body
			}
		}
		Expect(originals).NotTo(BeEmpty(), "real baseline must have changed before-images")
		out := installationMutationCommand(installed.driver, installed.root, installed.sentinel, installationForwardArguments(installed, "one-before-set", proposal["proposal_digest"].(string))...)
		Expect(out.status).To(Equal(0), "%+v", out)
		for path, before := range originals {
			saved := filepath.Join(installed.root, ".factory/backups/one-before-set/assets", filepath.FromSlash(path))
			body, err := os.ReadFile(saved)
			Expect(err).NotTo(HaveOccurred(), "missing complete saved before-image: %s", path)
			Expect(body).To(Equal(before), path)
			info, err := os.Lstat(saved)
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Mode().Perm()).To(Equal(os.FileMode(0600)), "backup cannot execute: %s", path)
		}
		for _, path := range []string{".factory/backups/one-before-set/installation.manifest", ".factory/installation-transactions/one-before-set.json"} {
			info, err := os.Lstat(filepath.Join(installed.root, path))
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Mode().Perm()).To(Equal(os.FileMode(0600)), path)
		}
		journalBytes, err := os.ReadFile(filepath.Join(installed.root, ".factory/installation-transactions/one-before-set.json"))
		Expect(err).NotTo(HaveOccurred())
		var journal map[string]any
		Expect(json.Unmarshal(journalBytes, &journal)).To(Succeed())
		Expect(journal["schema_version"]).To(Equal(float64(1)))
		Expect(journal["kind"]).To(Equal("installation-transaction"))
		Expect(journal["migration_id"]).To(Equal("one-before-set"))
		Expect(journal["phase"]).To(Equal("committed"))
		Expect(journal["entries"]).NotTo(BeEmpty())
		Expect(journal["checks"]).NotTo(BeEmpty())
		actualPID := false
		for _, check := range journal["checks"].([]any) {
			if pid, ok := check.(map[string]any)["pid"].(float64); ok && pid > 0 {
				actualPID = true
			}
		}
		Expect(actualPID).To(BeTrue(), "candidate process checks must retain actual onSpawn PID evidence")
		afterIndex, err := os.ReadFile(filepath.Join(installed.root, ".git/index"))
		Expect(err).NotTo(HaveOccurred())
		Expect(afterIndex).To(Equal(index))
		for _, path := range []string{".factory/bin/factory-runtime", ".factory/backups/one-before-set/installation.manifest"} {
			ignored := imageProcess(installed.root, nil, "git", "check-ignore", "--no-index", "-q", "--", path)
			Expect(ignored.status).To(Equal(0), "private operation is not effectively ignored: %s", path)
		}
	})

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:200
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:207
	DescribeTable("rejects an unqualified candidate before activation beside a successful complete sibling", func(change string) {
		healthy := fixture("bash-baseline")
		installationApply(healthy, "healthy-candidate-sibling")
		installed := fixture("bash-baseline")
		installed.archive = installationRewrittenArchive(installed, func(entries []stageEntry) []stageEntry {
			var manifest imageManifestOracle
			Expect(json.Unmarshal(entries[3].body, &manifest)).To(Succeed())
			path := "scripts/hooks/test-edit-denial.sh"
			if change == "missing-canonical" {
				path = "templates/metrics.html"
			}
			selected := -1
			for index, asset := range manifest.Assets {
				if asset.Path == path {
					selected = index
					break
				}
			}
			Expect(selected).To(BeNumerically(">=", 0))
			if change == "missing-canonical" {
				manifest.Assets = append(manifest.Assets[:selected], manifest.Assets[selected+1:]...)
				entries = append(entries[:4+selected], entries[5+selected:]...)
			} else {
				body := []byte("#!/bin/sh\nexit 0\n")
				manifest.Assets[selected].Bytes = int64(len(body))
				manifest.Assets[selected].SHA256 = fmt.Sprintf("%x", sha256.Sum256(body))
				entries[4+selected].body = body
			}
			entries[3].body = imageMarshal(manifest)
			return entries
		})
		before := installationSnapshot(installed.root)
		preview := installationCommand(installed.driver, installed.root, installed.sentinel, append(installed.arguments(), "--dry-run", "--json")...)
		Expect(preview.status).To(BeElementOf(0, 2), "%+v", preview)
		proposal := installationDecodeProposal(preview)
		out := installationMutationCommand(installed.driver, installed.root, installed.sentinel, installationForwardArguments(installed, "unqualified-candidate", proposal["proposal_digest"].(string))...)
		Expect(out.status).To(BeElementOf(1, 2), "%+v", out)
		Expect(out.stderr).NotTo(ContainSubstring("unsupported"))
		installationAssertExistingUnchanged(installed.root, before)
	}, Entry("required canonical data missing", "missing-canonical"), Entry("required gate falsely returns success for its negative control", "vacuous-gate"))

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:215
	It("qualifies unchanged baseline budget state through the independent legacy reader before forward and reverse", func() {
		installed := fixture("bash-baseline")
		row := map[string]any{"id": "known-run", "session": "known-session", "task": "known-task", "harness": "claude", "role": "reviewer", "status": "completed", "outcome": "completed", "elapsed_seconds": 1.0, "reserved_seconds": 2.0, "attempt": 1, "owner_pid": 1, "complete": true, "warnings": []any{}, "model": "", "started_at": "2026-01-01T00:00:00Z", "ended_at": "2026-01-01T00:00:01Z", "source": "client estimate", "tokens": map[string]any{"input_tokens": 2}, "estimated_usd": 0.1, "process_pid": nil, "exit_code": 0}
		state := imageMarshal(map[string]any{"schema": 1, "runs": []any{row}})
		statePath := filepath.Join(installed.root, ".factory/budget.json")
		writeFixture(statePath, state, 0600)
		old := imageProcess(installed.root, nil, filepath.Join(installed.root, "factory"), "budget", "report", "--json")
		Expect(old.status).To(Equal(0), "independent actual 76952 reader rejected fixture: %+v", old)
		Expect(old.stdout).To(ContainSubstring("known-session"))
		installationApply(installed, "compatible-history")
		current := imageProcess(installed.root, nil, filepath.Join(installed.root, "factory"), "budget", "report", "--json")
		Expect(current.status).To(Equal(0), "%+v", current)
		Expect(current.stdout).To(ContainSubstring("known-session"))
		proposal := installationProposal(installed, "--rollback", "compatible-history")
		out := installationMutationCommand(installed.driver, installed.root, installed.sentinel, append(installed.arguments(), "--rollback", "compatible-history", "--confirm", proposal["proposal_digest"].(string), "--quiescent")...)
		Expect(out.status).To(Equal(0), "%+v", out)
		restored, err := os.ReadFile(statePath)
		Expect(err).NotTo(HaveOccurred())
		Expect(restored).To(Equal(state), "rollback changed retained budget history")
	})

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:145
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:223
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:326
	DescribeTable("retains one pending installation after native interruption and recovers with fresh consent", func(direction string) {
		installed := fixture("bash-baseline")
		proposal := installationProposal(installed)
		migration := "interrupted-" + direction
		args := append([]string{"upgrade"}, installationForwardArguments(installed, migration, proposal["proposal_digest"].(string))...)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, filepath.Join(installed.driver, "factory"), args...) // #nosec G204 -- actual independently compiled driver and fixture-owned literal operation.
		command.Dir = installed.root
		command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		command.WaitDelay = time.Second
		for _, entry := range os.Environ() {
			if !strings.HasPrefix(entry, "FACTORY_BRIDGE_PROTOCOL=") {
				command.Env = append(command.Env, entry)
			}
		}
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
		Expect(command.Start()).To(Succeed())
		waited := false
		DeferCleanup(func() {
			if !waited {
				_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
				_ = command.Wait()
			}
		})
		journalPath := filepath.Join(installed.root, ".factory/installation-transactions", migration+".json")
		pendingPath := filepath.Join(installed.root, ".factory/runtime-publication.pending")
		Eventually(func() bool {
			body, err := os.ReadFile(journalPath)
			if err != nil {
				return false
			}
			var journal map[string]any
			if json.Unmarshal(body, &journal) != nil || journal["phase"] != "applying" {
				return false
			}
			entries, ok := journal["entries"].([]any)
			if !ok {
				return false
			}
			applied := 0
			for _, raw := range entries {
				if raw.(map[string]any)["phase"] == "applied" {
					applied++
				}
			}
			_, err = os.Lstat(pendingPath)
			return applied >= 2 && err == nil
		}, 20*time.Second, time.Millisecond).Should(BeTrue(), "must observe multiple rows under one live pending marker")
		Expect(syscall.Kill(command.Process.Pid, syscall.SIGSTOP)).To(Succeed())
		Expect(syscall.Kill(-command.Process.Pid, syscall.SIGKILL)).To(Succeed())
		err := command.Wait()
		waited = true
		var exit *exec.ExitError
		Expect(errors.As(err, &exit)).To(BeTrue(), "actual interrupted driver did not exit by signal")
		_, err = os.Lstat(pendingPath)
		Expect(err).NotTo(HaveOccurred(), "crash cleared unresolved pending evidence")
		recovery := installationProposal(installed, "--recover", migration, "--direction", direction)
		Expect(recovery).To(HaveKey("recovery_command"))
		Expect(recovery).To(HaveKey("recovery_arguments"))
		freshArgs := append(installed.arguments(), "--recover", migration, "--direction", direction, "--confirm", recovery["proposal_digest"].(string), "--quiescent")
		out := installationMutationCommand(installed.driver, installed.root, installed.sentinel, freshArgs...)
		Expect(out.status).To(Equal(0), "fresh interrupted recovery failed: %+v", out)
		_, err = os.Lstat(pendingPath)
		Expect(os.IsNotExist(err)).To(BeTrue(), "qualified terminal recovery retained pending")
		if direction == "forward" {
			proof := imageProcess(installed.root, nil, filepath.Join(installed.root, "factory"), "selftest")
			Expect(proof.status).To(Equal(0), "%+v", proof)
		} else {
			after := installationSnapshot(installed.root)
			for path, value := range installed.before {
				if path != ".gitignore" && !strings.HasPrefix(path, ".git/") {
					Expect(after[path]).To(Equal(value), "reverse recovery omitted baseline entry %s", path)
				}
			}
		}
	}, Entry("forward after actual SIGKILL", "forward"), Entry("reverse after actual SIGKILL", "reverse"))

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:411
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:416
	It("permits fresh same-ID work after a known exited candidate failure without consuming the baseline", func() {
		healthy := fixture("bash-baseline")
		installationApply(healthy, "healthy-preparation-sibling")
		installed := fixture("bash-baseline")
		imageGit(installed.root, "add", "--force", "README.md")
		before := installationSnapshot(installed.root)
		archive := installed.archive
		installed.archive = installationRewrittenArchive(installed, func(entries []stageEntry) []stageEntry {
			var manifest imageManifestOracle
			Expect(json.Unmarshal(entries[3].body, &manifest)).To(Succeed())
			selected := -1
			for index, asset := range manifest.Assets {
				if asset.Path == "scripts/hooks/test-edit-denial.sh" {
					selected = index
					break
				}
			}
			Expect(selected).To(BeNumerically(">=", 0))
			body := []byte("#!/bin/sh\nexit 0\n")
			manifest.Assets[selected].Bytes = int64(len(body))
			manifest.Assets[selected].SHA256 = fmt.Sprintf("%x", sha256.Sum256(body))
			entries[4+selected].body = body
			entries[3].body = imageMarshal(manifest)
			return entries
		})
		proposal := installationProposal(installed)
		failed := installationMutationCommand(installed.driver, installed.root, installed.sentinel, installationForwardArguments(installed, "preparation-retry", proposal["proposal_digest"].(string))...)
		Expect(failed.status).To(Equal(1), "%+v", failed)
		Expect(failed.stderr).To(ContainSubstring("native-gate-proof"))
		afterFailure := installationSnapshot(installed.root)
		_, journalErr := os.Lstat(filepath.Join(installed.root, ".factory/installation-transactions/preparation-retry.json"))
		for _, path := range []string{".factory/backups/preparation-retry", ".factory/runtime-publication.pending", ".factory/installation.current"} {
			_, err := os.Lstat(filepath.Join(installed.root, path))
			Expect(os.IsNotExist(err)).To(BeTrue(), "candidate failure cannot create active or backup authority: %s", path)
		}
		for path, value := range before {
			Expect(afterFailure[path]).To(Equal(value), "candidate failure changed existing entry %s", path)
		}
		installed.archive = archive
		fresh := installationProposal(installed)
		retry := installationMutationCommand(installed.driver, installed.root, installed.sentinel, installationForwardArguments(installed, "preparation-retry", fresh["proposal_digest"].(string))...)
		Expect(os.IsNotExist(journalErr)).To(BeTrue(), "known exited preflight left its newly owned journal occupying the migration ID")
		Expect(retry.status).To(Equal(0), "fresh same-ID request was consumed by a failed preparation: %+v", retry)
	})

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:425
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:427
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:429
	It("recovers actual committed pending cleanup without replaying assets or making another backup", func() {
		var installed installationFixtureData
		var migration string
		witnessed := false
		for attempt := 1; attempt <= 3; attempt++ {
			installed = fixture("bash-baseline")
			imageGit(installed.root, "add", "--force", "README.md")
			migration = fmt.Sprintf("terminal-cleanup-%d", attempt)
			proposal := installationProposal(installed)
			if installationInterruptTerminal(installed, migration, proposal["proposal_digest"].(string)) {
				witnessed = true
				break
			}
		}
		Expect(witnessed).To(BeTrue(), "three healthy attempts produced no actual committed/pending process witness")
		before := installationSnapshot(installed.root)
		identities := map[string]os.FileInfo{}
		for _, role := range append(append([]installationRoleOracle{}, roleMap.Roles...), roleMap.CanonicalSourceAssets...) {
			info, err := os.Lstat(filepath.Join(installed.root, filepath.FromSlash(role.Path)))
			if os.IsNotExist(err) && role.Operation == "retire" {
				continue
			}
			Expect(err).NotTo(HaveOccurred(), role.Path)
			identities[role.Path] = info
		}
		recovery := installationProposal(installed, "--recover", migration, "--direction", "forward")
		out := installationMutationCommand(installed.driver, installed.root, installed.sentinel, append(installed.arguments(), "--recover", migration, "--direction", "forward", "--confirm", recovery["proposal_digest"].(string), "--quiescent")...)
		Expect(out.status).To(Equal(0), "qualified terminal cleanup was blocked by committed evidence: %+v", out)
		after := installationSnapshot(installed.root)
		for path, value := range before {
			if path != ".factory/runtime-publication.pending" && path != ".factory/installation-transactions/"+migration+".json" {
				Expect(after[path]).To(Equal(value), "terminal cleanup changed existing asset, backup or index: %s", path)
			}
		}
		for path := range after {
			Expect(before).To(HaveKey(path), "terminal cleanup created another artifact: %s", path)
		}
		for path, beforeInfo := range identities {
			afterInfo, err := os.Lstat(filepath.Join(installed.root, filepath.FromSlash(path)))
			Expect(err).NotTo(HaveOccurred(), path)
			Expect(os.SameFile(beforeInfo, afterInfo)).To(BeTrue(), "terminal cleanup replayed selected publication: %s", path)
		}
		_, err := os.Lstat(filepath.Join(installed.root, ".factory/runtime-publication.pending"))
		Expect(os.IsNotExist(err)).To(BeTrue(), "fresh terminal cleanup retained its exact pending marker")
	})

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:184
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:187
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:226
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:229
	It("preserves a foreign equal-byte created leaf after an actual applied-row interruption", func() {
		var installed installationFixtureData
		var migration, path string
		witnessed := false
		for attempt := 1; attempt <= 3; attempt++ {
			installed = fixture("bash-baseline")
			migration = fmt.Sprintf("foreign-applied-leaf-%d", attempt)
			proposal := installationProposal(installed)
			path, witnessed = installationInterruptPhase(installed, migration, proposal["proposal_digest"].(string), "applying", true)
			if witnessed {
				break
			}
		}
		Expect(witnessed).To(BeTrue(), "no actual applied creation with observed_after identity was interrupted")
		absolute := filepath.Join(installed.root, filepath.FromSlash(path))
		original, err := os.Lstat(absolute)
		Expect(err).NotTo(HaveOccurred())
		body, err := os.ReadFile(absolute)
		Expect(err).NotTo(HaveOccurred())
		journalBytes, err := os.ReadFile(filepath.Join(installed.root, ".factory/installation-transactions", migration+".json"))
		Expect(err).NotTo(HaveOccurred())
		var journal map[string]any
		Expect(json.Unmarshal(journalBytes, &journal)).To(Succeed())
		observed := false
		for _, raw := range journal["entries"].([]any) {
			entry := raw.(map[string]any)
			if entry["path"] == path {
				Expect(entry["phase"]).To(Equal("applied"))
				after := entry["observed_after"].(map[string]any)
				Expect(after["identity"]).NotTo(BeNil())
				Expect(after["sha256"]).To(Equal(fmt.Sprintf("%x", sha256.Sum256(body))))
				observed = true
			}
		}
		Expect(observed).To(BeTrue(), "the live journal must contain the actual selected publication")
		healthy := installationProposal(installed, "--recover", migration, "--direction", "reverse")
		Expect(installationRows(healthy)[path]["action"]).To(Equal("retire"), "unchanged owned creation must have its healthy reverse sibling")
		Expect(os.Rename(absolute, absolute+".foreign-original")).To(Succeed())
		writeFixture(absolute, body, original.Mode().Perm())
		foreign, err := os.Lstat(absolute)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(original, foreign)).To(BeFalse(), "equal bytes must be a genuinely distinct native leaf identity")
		before := installationSnapshot(installed.root)
		out := installationCommand(installed.driver, installed.root, installed.sentinel, append(installed.arguments(), "--recover", migration, "--direction", "reverse", "--dry-run", "--json")...)
		Expect(out.status).To(Equal(2), "equal bytes cannot restore publication ownership: %+v", out)
		proposal := installationDecodeProposal(out)
		Expect(proposal["conflicts"]).NotTo(BeEmpty())
		Expect(out.stdout).To(ContainSubstring(path))
		Expect(installationRows(proposal)[path]["action"]).To(Equal("retain"))
		Expect(installationSnapshot(installed.root)).To(Equal(before), "foreign replacement and unresolved evidence must remain unchanged")
	})
})
