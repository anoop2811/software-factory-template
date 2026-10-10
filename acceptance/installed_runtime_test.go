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
	"sort"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type installedRuntimeFixture struct {
	root, assets, caller, launcher string
	environment                    []string
}

// This is an independent local custody fixture, not evidence of archive trust or
// whole-transaction activation. It uses the specified descriptor and reads the
// reviewed canonical selection directly, without importing installation helpers.
// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:374
// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:255
func installedRuntimeCustodyFixture() installedRuntimeFixture {
	GinkgoHelper()
	temporary, err := os.MkdirTemp("", "factory installed runtime ")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(os.RemoveAll, temporary)
	temporary, err = filepath.EvalSymlinks(temporary)
	Expect(err).NotTo(HaveOccurred())
	fixture := installedRuntimeFixture{
		root:   filepath.Join(temporary, "installation root"),
		caller: filepath.Join(temporary, "application cwd"),
	}
	fixture.assets = filepath.Join(fixture.root, ".factory/assets/scaffold")
	fixture.launcher = filepath.Join(fixture.root, "factory")
	Expect(os.MkdirAll(fixture.caller, 0700)).To(Succeed())
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(key, "FACTORY_") && !strings.HasPrefix(key, "GIT_") && !strings.HasPrefix(key, "OPENCODE_") && key != "COST_PROFILE" {
			fixture.environment = append(fixture.environment, entry)
		}
	}
	fixture.environment = append(fixture.environment, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GORACE=atexit_sleep_ms=0")

	const version = "v9.8.0"
	revision := strings.Repeat("a", 40)
	target := runtime.GOOS + "/" + runtime.GOARCH
	digest := fmt.Sprintf("%x", sha256.Sum256(candidateBinary))
	selected := []map[string]any{}
	add := func(path string, data []byte, mode os.FileMode) {
		writeFixture(filepath.Join(fixture.root, filepath.FromSlash(path)), data, mode)
		Expect(os.Chmod(filepath.Join(fixture.root, filepath.FromSlash(path)), mode)).To(Succeed())
		selected = append(selected, map[string]any{"path": path, "mode": uint32(0100000 | mode.Perm()), "sha256": fmt.Sprintf("%x", sha256.Sum256(data)), "bytes": len(data)})
	}
	add(".factory/bin/factory-runtime", candidateBinary, 0700)
	manifest := fmt.Sprintf("FACTORY_RUNTIME_ARTIFACT_V1\nversion\t%s\ntarget\t%s\nbinary\tbin/factory-runtime\nsha256\t%s\nEND\n", version, target, digest)
	add(".factory/bin/runtime.manifest", []byte(manifest), 0600)
	add(".factory/bin/source.json", imageMarshal(map[string]string{"revision": revision, "version": version, "target": target, "go_version": "go1.27.1", "kind": "installation-source-build"}), 0600)

	canonical, err := os.ReadFile("../docs/migration/INSTALLATION_ROLE_MAP.json")
	Expect(err).NotTo(HaveOccurred())
	var roles installationRoleMapOracle
	Expect(json.Unmarshal(canonical, &roles)).To(Succeed())
	Expect(roles.CanonicalSourceAssets).NotTo(BeEmpty())
	for _, role := range roles.CanonicalSourceAssets {
		Expect(role.SourcePath).NotTo(BeNil(), role.Path)
		Expect(role.Mode).NotTo(BeNil(), role.Path)
		data, err := os.ReadFile(filepath.Join("..", filepath.FromSlash(*role.SourcePath)))
		Expect(err).NotTo(HaveOccurred(), role.Path)
		mode, err := strconv.ParseUint(*role.Mode, 8, 32)
		Expect(err).NotTo(HaveOccurred(), role.Path)
		add(role.Path, data, os.FileMode(mode))
	}
	// One actual active gate distinguishes the installation from the many inert
	// canonical hook files. This fixture never executes either hook selection.
	gate, err := os.ReadFile("../scripts/hooks/hook-existence-check.sh")
	Expect(err).NotTo(HaveOccurred())
	add("scripts/hooks/hook-existence-check.sh", gate, 0755)
	launcher := fmt.Sprintf("#!/bin/bash\nset -e\nroot=\"$(cd \"$(dirname \"${BASH_SOURCE[0]}\")\" && pwd -P)\"\nexec \"$root/.factory/bin/factory-runtime\" --installed %s %s %s -- \"$@\"\n", version, revision, digest)
	add("factory", []byte(launcher), 0755)
	sort.Slice(selected, func(i, j int) bool { return selected[i]["path"].(string) < selected[j]["path"].(string) })
	descriptor := map[string]any{"schema_version": 1, "kind": "go-hybrid-v1", "version": version, "revision": revision, "target": target, "runtime_sha256": digest, "assets": selected}
	writeFixture(filepath.Join(fixture.root, ".factory/installation.current"), imageMarshal(descriptor), 0600)
	writeFixture(filepath.Join(fixture.root, ".factory-version"), []byte("ref="+version+"\ncommit="+revision+"\n"), 0600)
	return fixture
}

func installedRuntimeInvoke(fixture installedRuntimeFixture, input string, extraEnvironment []string, args ...string) cliResult {
	GinkgoHelper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, fixture.launcher, args...) // #nosec G204 -- evaluator-owned public launcher and literal operands.
	command.Dir = fixture.caller
	command.Env = append(append([]string{}, fixture.environment...), extraEnvironment...)
	command.Stdin = strings.NewReader(input)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	Expect(ctx.Err()).NotTo(HaveOccurred(), "installed command timed out: %s", stderr.String())
	status := 0
	if err != nil {
		var exited *exec.ExitError
		Expect(errors.As(err, &exited)).To(BeTrue(), "installed command could not start: %v", err)
		status = exited.ExitCode()
	}
	out := cliResult{stdout.String(), stderr.String(), status}
	_, _ = fmt.Fprintf(GinkgoWriter, "installed command: %q\ncwd: %q\nstdout: %q\nstderr: %q\nstatus: %d\n", command.Args, command.Dir, out.stdout, out.stderr, out.status)
	return out
}

// Stop a second public-launcher invocation before it can execute another runtime.
// The instrumented launcher and literal thin adapter are both included by their
// exact bytes in the independently constructed installed descriptor. This is a
// bounded command-routing fixture, not whole-installation authentication proof.
// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:457
// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:374
func installedRuntimeBoundedUpgradeFixture() (installedRuntimeFixture, string) {
	GinkgoHelper()
	fixture := installedRuntimeCustodyFixture()
	descriptorPath := filepath.Join(fixture.root, ".factory/installation.current")
	body, err := os.ReadFile(descriptorPath)
	Expect(err).NotTo(HaveOccurred())
	var descriptor map[string]any
	Expect(json.Unmarshal(body, &descriptor)).To(Succeed())
	selected := descriptor["assets"].([]any)
	record := func(path string, data []byte) {
		writeFixture(filepath.Join(fixture.root, filepath.FromSlash(path)), data, 0755)
		Expect(os.Chmod(filepath.Join(fixture.root, filepath.FromSlash(path)), 0755)).To(Succeed())
		row := map[string]any{"path": path, "mode": uint32(0100755), "sha256": fmt.Sprintf("%x", sha256.Sum256(data)), "bytes": len(data)}
		for index, current := range selected {
			if current.(map[string]any)["path"] == path {
				selected[index] = row
				return
			}
		}
		selected = append(selected, row)
	}
	launcher, err := os.ReadFile(fixture.launcher)
	Expect(err).NotTo(HaveOccurred())
	bound := `set -e
depth="${FACTORY_ACCEPTANCE_UPGRADE_DEPTH:-0}"
printf '%s\n' "$depth" >> "$FACTORY_ACCEPTANCE_UPGRADE_LAUNCH_LOG"
if [ "$depth" != 0 ]; then
  printf 'BOUNDED_UPGRADE_RECURSION\n' >&2
  exit 99
fi
export FACTORY_ACCEPTANCE_UPGRADE_DEPTH=1
`
	Expect(strings.Count(string(launcher), "set -e\n")).To(Equal(1))
	record("factory", []byte(strings.Replace(string(launcher), "set -e\n", bound, 1)))
	adapter := fmt.Sprintf("#!/bin/bash\n# Factory installation %s %s %s\nset -e\n_factory_root=\"$(cd \"$(dirname \"${BASH_SOURCE[0]}\")/..\" && pwd -P)\"\nexec \"$_factory_root/factory\" upgrade \"$@\"\n", descriptor["version"], descriptor["revision"], descriptor["runtime_sha256"])
	record("scripts/factory-upgrade.sh", []byte(adapter))
	sort.Slice(selected, func(i, j int) bool {
		return selected[i].(map[string]any)["path"].(string) < selected[j].(map[string]any)["path"].(string)
	})
	descriptor["assets"] = selected
	writeFixture(descriptorPath, imageMarshal(descriptor), 0600)
	log := filepath.Join(fixture.root, ".factory/upgrade-launches.log")
	fixture.environment = append(fixture.environment, "FACTORY_ACCEPTANCE_UPGRADE_LAUNCH_LOG="+log)
	return fixture, log
}

var _ = Describe("Installed runtime root separation", func() {
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:109
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:112
	// per docs/adr/0087-go-native-report.md:22
	It("reports actual installation events and active gates from a separate application cwd", func() {
		fixture := installedRuntimeCustodyFixture()
		log := filepath.Join(fixture.root, ".factory/events.log")
		writeFixture(log, []byte("today\tactual-installed-gate\tactual installed event\n"), 0600)
		writeFixture(filepath.Join(fixture.caller, ".factory/events.log"), []byte("caller\tforeign\tfirst\ncaller\tforeign\tsecond\n"), 0600)
		writeFixture(filepath.Join(fixture.assets, ".factory/events.log"), []byte("canonical\tforeign\tfirst\ncanonical\tforeign\tsecond\ncanonical\tforeign\tthird\n"), 0600)
		control := installedRuntimeInvoke(fixture, "", []string{"FACTORY_EVENT_LOG=" + log}, "report")
		Expect(control.status).To(Equal(0), "%+v", control)
		Expect(control.stdout).To(ContainSubstring("Gate blocks recorded:           1"))
		actual := installedRuntimeInvoke(fixture, "", nil, "report")
		Expect(actual.status).To(Equal(0), "%+v", actual)
		Expect(actual.stdout).To(ContainSubstring("Gate blocks recorded:           1"))
		Expect(actual.stdout).To(ContainSubstring("actual-installed-gate"))
		Expect(actual.stdout).To(ContainSubstring("Deterministic gates installed:  1  "))
		Expect(actual.stderr).To(BeEmpty())
	})

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:112
	// per docs/adr/0087-go-native-report.md:31
	It("clears only the installation event log when the default installed report is cleared", func() {
		fixture := installedRuntimeCustodyFixture()
		log := filepath.Join(fixture.root, ".factory/events.log")
		foreign := []byte("foreign event must remain\n")
		callerLog := filepath.Join(fixture.caller, ".factory/events.log")
		canonicalLog := filepath.Join(fixture.assets, ".factory/events.log")
		writeFixture(log, []byte("actual event\n"), 0600)
		writeFixture(callerLog, foreign, 0600)
		writeFixture(canonicalLog, foreign, 0600)
		control := installedRuntimeInvoke(fixture, "", []string{"FACTORY_EVENT_LOG=" + log}, "report", "--clear")
		Expect(control).To(Equal(cliResult{"factory report: event log cleared.\n", "", 0}))
		_, err := os.Lstat(log)
		Expect(os.IsNotExist(err)).To(BeTrue(), "explicit-log healthy sibling must unlink the actual event log")
		writeFixture(log, []byte("actual event\n"), 0600)
		actual := installedRuntimeInvoke(fixture, "", nil, "report", "--clear")
		Expect(actual).To(Equal(cliResult{"factory report: event log cleared.\n", "", 0}))
		_, err = os.Lstat(log)
		Expect(os.IsNotExist(err)).To(BeTrue(), "default installed report cleared a different root")
		for _, path := range []string{callerLog, canonicalLog} {
			body, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred(), "foreign log removed: %s", path)
			Expect(body).To(Equal(foreign), path)
		}
	})

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:106
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:260
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:435
	// per docs/adr/0085-go-native-init.md:20
	// per docs/adr/0085-go-native-init.md:99
	It("has the canonical README needed to review initialization of an empty target beside a retained README sibling", func() {
		fixture := installedRuntimeCustodyFixture()
		existing := filepath.Join(filepath.Dir(fixture.root), "existing project")
		readme := []byte("application README is retained verbatim\n")
		writeFixture(filepath.Join(existing, "README.md"), readme, 0644)
		before := nativeInitArtifacts(existing)
		control := installedRuntimeInvoke(fixture, nativeInitAnswers("n"), nil, "init", existing, "--pack=none")
		Expect(control.status).To(Equal(1), "%+v", control)
		Expect(control.stdout).To(ContainSubstring("=== Summary ==="))
		Expect(control.stdout).To(ContainSubstring("Aborted."))
		Expect(control.stderr).To(BeEmpty())
		Expect(nativeInitArtifacts(existing)).To(Equal(before))
		empty := filepath.Join(filepath.Dir(fixture.root), "empty project")
		actual := installedRuntimeInvoke(fixture, nativeInitAnswers("n"), nil, "init", empty, "--pack=none")
		Expect(actual.status).To(Equal(1), "%+v", actual)
		Expect(actual.stdout).To(ContainSubstring("=== Summary ==="), "empty target must reach the same explicit review boundary as the existing README sibling: %+v", actual)
		Expect(actual.stdout).To(ContainSubstring("Aborted."))
		Expect(actual.stderr).To(BeEmpty())
		_, err := os.Lstat(empty)
		Expect(os.IsNotExist(err)).To(BeTrue(), "decline must not create the target")
	})

	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:33
	// per docs/adr/0098-whole-installation-upgrade-and-rollback.md:457
	It("owns installed upgrade aliases through public and direct adapters without recursive fallback", func() {
		fixture, log := installedRuntimeBoundedUpgradeFixture()
		control := installedRuntimeInvoke(fixture, "", nil, "upgrade", "--installation", "--help")
		Expect(control.status).To(Equal(0), "%+v", control)
		Expect(control.stdout).To(ContainSubstring("--installation"))
		Expect(control.stderr).To(BeEmpty())
		calls, err := os.ReadFile(log)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(calls)).To(Equal("0\n"), "explicit whole help must execute only the first public launcher")
		type observation struct {
			route string
			args  []string
			out   cliResult
			calls string
		}
		observations := []observation{}
		for _, direct := range []bool{false, true} {
			invocation := fixture
			route := "public launcher"
			if direct {
				invocation.launcher = filepath.Join(fixture.root, "scripts/factory-upgrade.sh")
				route = "direct generated adapter"
			}
			for _, operands := range [][]string{{}, {"--help"}, {"-h"}, {"--force"}} {
				Expect(os.Remove(log)).To(Succeed())
				args := append([]string{}, operands...)
				if !direct {
					args = append([]string{"upgrade"}, args...)
				}
				out := installedRuntimeInvoke(invocation, "", nil, args...)
				calls, err := os.ReadFile(log)
				Expect(err).NotTo(HaveOccurred())
				observations = append(observations, observation{route, operands, out, string(calls)})
				_, _ = fmt.Fprintf(GinkgoWriter, "bounded upgrade route: %s; operands: %q; launcher depths: %q\n", route, operands, calls)
			}
		}
		// Collect all bounded outcomes before asserting so initial RED includes
		// both public/direct routes and all aliases, rather than stopping at bare.
		for _, observed := range observations {
			if len(observed.args) == 1 && observed.args[0] == "--force" {
				Expect(observed.out.status).To(Equal(2), "%s %q: %+v", observed.route, observed.args, observed.out)
				Expect(observed.out.stdout).To(BeEmpty())
				Expect(observed.out.stderr).To(ContainSubstring("--installation"))
			} else {
				Expect(observed.out.status).To(Equal(0), "%s %q: %+v", observed.route, observed.args, observed.out)
				Expect(observed.out.stdout).To(Equal(control.stdout), "%s %q must show native whole help", observed.route, observed.args)
				Expect(observed.out.stderr).To(BeEmpty())
			}
			Expect(observed.calls).To(Equal("0\n"), "%s %q unexpectedly launched the upgrade adapter again", observed.route, observed.args)
			Expect(observed.out.stderr).NotTo(ContainSubstring("BOUNDED_UPGRADE_RECURSION"))
		}
	})
})
