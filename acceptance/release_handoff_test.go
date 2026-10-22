package acceptance_test

import (
	"archive/tar"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const handoffVersion = "v9.8.0-rc.1"

// per docs/adr/0101-go-release-handoff.md:32
func handoffSurfaceFixture() (string, string, string) {
	GinkgoHelper()
	root, cwd := fixture()
	marker := filepath.Join(cwd, "legacy-or-candidate-executed")
	for _, name := range []string{"factory-init.sh", "factory-upgrade.sh"} {
		writeFixture(filepath.Join(root, "scripts", name), []byte("#!/bin/sh\nprintf called > \"$HANDOFF_EXECUTION_MARKER\"\nexit 97\n"), 0700)
	}
	writeFixture(filepath.Join(root, "tools", "gh"), []byte("#!/bin/sh\nprintf called > \"$HANDOFF_EXECUTION_MARKER\"\nexit 97\n"), 0700)
	return root, cwd, marker
}

var _ = Describe("Authenticated release candidate public handoff", func() {
	// per docs/adr/0101-go-release-handoff.md:32
	DescribeTable("serves candidate help before source selection or candidate execution", func(verb string) {
		root, cwd, marker := handoffSurfaceFixture()
		for _, selector := range [][]string{{"--go-candidate", handoffVersion}, {"--go-candidate=" + handoffVersion}} {
			args := append([]string{verb}, selector...)
			args = append(args, "--help")
			result := imageProcess(cwd, map[string]string{"PATH": filepath.Join(root, "tools") + string(os.PathListSeparator) + os.Getenv("PATH"), "HANDOFF_EXECUTION_MARKER": marker, "FACTORY_BRIDGE_PROTOCOL": ""}, filepath.Join(root, "factory"), args...)
			Expect(result.status).To(Equal(0), "%+v", result)
			Expect(result.stdout).To(ContainSubstring("--go-candidate"))
			Expect(result.stderr).To(BeEmpty())
			_, err := os.Lstat(marker)
			Expect(os.IsNotExist(err)).To(BeTrue())
		}
	}, Entry("init", "init"), Entry("upgrade", "upgrade"))

	// per docs/adr/0101-go-release-handoff.md:40
	DescribeTable("owns malformed candidate selectors before legacy dispatch and filesystem changes", func(verb string) {
		root, cwd, marker := handoffSurfaceFixture()
		before := nativeInitArtifacts(cwd)
		cases := [][]string{
			{"--go-candidate"}, {"--go-candidate="}, {"--go-candidate", "main"}, {"--go-candidate", strings.Repeat("v", 129)},
			{"--go-candidate", handoffVersion, "--go-candidate", handoffVersion},
			{"--go-candidate", handoffVersion, "--source", root},
			{"--go-candidate", handoffVersion, "--ref", "main"},
			{"--go-candidate", handoffVersion, "--local"},
		}
		for _, operands := range cases {
			result := imageProcess(cwd, map[string]string{"PATH": filepath.Join(root, "tools") + string(os.PathListSeparator) + os.Getenv("PATH"), "HANDOFF_EXECUTION_MARKER": marker, "FACTORY_BRIDGE_PROTOCOL": ""}, filepath.Join(root, "factory"), append([]string{verb}, operands...)...)
			Expect(result.status).To(Equal(2), "args %q: %+v", operands, result)
			Expect(result.stderr).To(ContainSubstring("candidate"))
			Expect(result.stderr).NotTo(ContainSubstring("unknown flag"), "the candidate owner must parse its reserved selector")
			Expect(nativeInitArtifacts(cwd)).To(Equal(before))
			_, err := os.Lstat(marker)
			Expect(os.IsNotExist(err)).To(BeTrue())
		}
	}, Entry("init", "init"), Entry("upgrade", "upgrade"))
})

var _ = Describe("Authenticated release candidate archive subject", func() {
	// per docs/adr/0101-go-release-handoff.md:102
	// per docs/adr/0101-go-release-handoff.md:126
	It("refuses a different verified archive subject even when raw bootstrap authentication succeeds", func() {
		image := handoffBuildImage()
		for _, scenario := range []string{"", "wrong-archive-subject"} {
			f, driver := handoffNativeFixture(image)
			f.environment["HANDOFF_SCENARIO"] = scenario
			target := filepath.Join(f.cwd, "archive-subject-project")
			result := handoffNativeProcess(f.cwd, f.environment, nativeInitAnswers("y"), driver, "init", target, "--go-candidate", handoffVersion, "--pack", "none")
			if scenario == "" {
				Expect(result.status).To(Equal(0), "healthy complete image control: %+v", result)
				continue
			}
			Expect(result.status).NotTo(Equal(0), "a valid raw signature cannot authenticate a differently named archive subject: %+v", result)
			Expect(result.stderr).To(ContainSubstring("authentication"))
			query := handoffFlag(handoffRecorded(f, "verify"), "--jq")
			Expect(query).To(ContainSubstring("factory-installation-" + handoffVersion + "-" + f.target + ".tar.gz"))
			Expect(query).To(ContainSubstring(fmt.Sprintf("%x", sha256.Sum256(image.archive))))
			_, err := os.Lstat(target)
			Expect(os.IsNotExist(err)).To(BeTrue(), "archive subject refusal must precede initializer publication")
		}
	})
})

var _ = Describe("Authenticated release candidate publishing", func() {
	// per docs/adr/0101-go-release-handoff.md:72
	It("prepares all four runtime, complete installation and raw bootstrap assets without executing them", func() {
		workflow := ciReviewLoadWorkflow(".github/workflows/runtime-release.yml")
		root := GinkgoT().TempDir()
		tools := filepath.Join(root, "publisher-tools")
		Expect(os.Mkdir(tools, 0700)).To(Succeed())
		if _, err := exec.LookPath("sha256sum"); err != nil {
			writeFixture(filepath.Join(tools, "sha256sum"), []byte("#!/bin/sh\nexec /usr/bin/shasum -a 256 \"$@\"\n"), 0700)
		}
		payload := []byte("#!/bin/sh\nprintf called > \"$HANDOFF_EXECUTION_MARKER\"\n")
		archives := map[string][]byte{}
		for _, target := range []string{"linux-amd64", "linux-arm64", "darwin-amd64", "darwin-arm64"} {
			identity := strings.Replace(target, "-", "/", 1)
			prefix := handoffVersion + "/" + identity + "/"
			entries := []stageEntry{
				{prefix + "bin/factory-runtime", payload, 0755, tar.TypeReg},
				{prefix + "runtime.manifest", []byte(fmt.Sprintf("FACTORY_RUNTIME_ARTIFACT_V1\nversion\t%s\ntarget\t%s\nbinary\tbin/factory-runtime\nsha256\t%x\nEND\n", handoffVersion, identity, sha256.Sum256(payload))), 0644, tar.TypeReg},
				{prefix + "source.json", []byte(fmt.Sprintf(`{"revision":%q,"version":%q,"target":%q,"go_version":"go1.27.3","kind":"source-build"}`, batchBase, handoffVersion, identity)), 0644, tar.TypeReg},
			}
			for _, kind := range []string{"runtime", "installation-image"} {
				selected := append([]stageEntry(nil), entries...)
				if kind == "installation-image" {
					selected = append(selected, stageEntry{prefix + "installation.manifest", []byte("test-owned installation payload marker"), 0644, tar.TypeReg})
				}
				data := stageArchive(selected)
				directory := filepath.Join(root, "bundles", kind+"-"+target)
				writeFixture(filepath.Join(directory, "factory-runtime.tar.gz"), data, 0600)
				writeFixture(filepath.Join(directory, "SHA256SUMS"), []byte(fmt.Sprintf("%x  factory-runtime.tar.gz\n", sha256.Sum256(data))), 0600)
				if kind == "installation-image" {
					writeFixture(filepath.Join(directory, "factory-bootstrap"), payload, 0600)
				}
				archives[kind+"-"+target] = data
			}
		}
		prepared := 0
		for _, step := range workflow.Jobs["attest"].Steps {
			if strings.HasPrefix(step.Uses, "actions/attest@") {
				break
			}
			if step.Run == "" {
				continue
			}
			result := imageProcess(root, map[string]string{"PATH": tools + string(os.PathListSeparator) + os.Getenv("PATH"), "VERSION": handoffVersion, "RUNNER_TEMP": root, "HANDOFF_EXECUTION_MARKER": filepath.Join(root, "candidate-executed")}, "bash", "-e", "-c", step.Run)
			Expect(result.status).To(Equal(0), "%s: %+v", step.Name, result)
			prepared++
		}
		Expect(prepared).To(BeNumerically(">", 0))
		for _, target := range []string{"linux-amd64", "linux-arm64", "darwin-amd64", "darwin-arm64"} {
			for kind, name := range map[string]string{"runtime": "factory-runtime-", "installation-image": "factory-installation-"} {
				data, err := os.ReadFile(filepath.Join(root, "release", name+handoffVersion+"-"+target+".tar.gz"))
				Expect(err).NotTo(HaveOccurred(), "%s %s", kind, target)
				Expect(data).To(Equal(archives[kind+"-"+target]))
			}
			bootstrap, err := os.ReadFile(filepath.Join(root, "release", "factory-bootstrap-"+handoffVersion+"-"+target))
			Expect(err).NotTo(HaveOccurred(), target)
			Expect(bootstrap).To(Equal(payload))
			attested := false
			for _, step := range workflow.Jobs["attest"].Steps {
				if !strings.HasPrefix(step.Uses, "actions/attest@") {
					continue
				}
				patterns, _ := step.With["subject-path"].(string)
				for _, pattern := range strings.Fields(patterns) {
					matched, err := filepath.Match(pattern, "release/factory-bootstrap-"+handoffVersion+"-"+target)
					Expect(err).NotTo(HaveOccurred())
					attested = attested || matched
				}
			}
			Expect(attested).To(BeTrue(), "the raw %s bootstrap must be an attested release subject", target)
		}
		_, err := os.Lstat(filepath.Join(root, "candidate-executed"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
})
