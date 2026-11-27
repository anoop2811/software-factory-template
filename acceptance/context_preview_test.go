package acceptance_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf8"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type fileContextEnvelope struct {
	Version      int
	Digest       string
	PayloadBytes int `json:"payload_bytes"`
	Payload      json.RawMessage
	Included     []struct {
		Reason    string
		ItemIndex int `json:"item_index"`
		Citations []json.RawMessage
	}
	Omitted []json.RawMessage
}

const refundContextDigest = "328b03ac859bc1c82a7ced94101057820f98ffa00b695520c12986eee954e988"

func fileContextJSON(result cliResult) fileContextEnvelope {
	GinkgoHelper()
	Expect(result.status).To(BeZero(), "%s", result.stderr)
	Expect(result.stderr).To(BeEmpty())
	var preview fileContextEnvelope
	Expect(json.Unmarshal([]byte(result.stdout), &preview)).To(Succeed())
	return preview
}

func fileContextRefusal(result cliResult) {
	GinkgoHelper()
	Expect(result.status).To(Equal(2))
	Expect(result.stdout).To(BeEmpty())
	Expect(result.stderr).NotTo(BeEmpty())
	Expect(len(result.stderr)).To(BeNumerically("<=", 1024))
}

var _ = Describe("File context preview", func() {
	var bin, repo string

	BeforeEach(func() {
		bin, repo = fixtureDirectories()
		writeFixture(filepath.Join(bin, "factory"), candidateBinary, 0700)
		imageGit(repo, "init", "--quiet")
	})

	// per specs/003-task-context-and-skills.md:43
	// per specs/003-task-context-and-skills.md:316
	It("reproduces the public refund vector from a subdirectory and enforces its exact byte cap", func() {
		writeFixture(filepath.Join(repo, "docs", "refund.txt"), []byte("Refund\n"), 0600)
		imageGit(repo, "add", "docs/refund.txt")
		cwd := filepath.Join(repo, "docs")
		args := []string{"context", "preview", "--file", "docs/refund.txt", "--json"}
		before := installationSnapshot(repo)
		first := invoke(bin, cwd, "factory", "", args...)
		preview := fileContextJSON(first)
		Expect(invoke(bin, repo, "factory", "", args...)).To(Equal(first))
		git, err := exec.LookPath("git")
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Symlink(git, filepath.Join(bin, "git"))).To(Succeed())
		calls := filepath.Join(bin, "external-calls")
		for _, name := range []string{"codex", "claude", "opencode", "curl", "gh"} {
			writeFixture(filepath.Join(bin, name), []byte("#!/bin/sh\nprintf called >> \"$EXTERNAL_CALLS\"\nexit 1\n"), 0700)
		}
		env := map[string]string{"PATH": bin, "HOME": bin, "GIT_CONFIG_NOSYSTEM": "1", "EXTERNAL_CALLS": calls, "GORACE": "atexit_sleep_ms=0"}
		Expect(handoffNativeExactProcess(cwd, env, "", filepath.Join(bin, "factory"), args...)).To(Equal(first))
		_, err = os.Lstat(calls)
		Expect(os.IsNotExist(err)).To(BeTrue())
		Expect(preview.Version).To(Equal(1))
		Expect(preview.Digest).To(Equal(refundContextDigest))
		Expect(preview.PayloadBytes).To(Equal(206))
		Expect(string(preview.Payload)).To(Equal(`{"items":[{"citations":[{"lines":[1,1],"path":"docs/refund.txt","sha256":"53cac814f776c9c4b85666d5569ecc2c0439ff9474a48a0d17e5f01728dee921"}],"kind":"source","representation":"original","text":"Refund\n"}]}`))
		Expect(preview.Included).To(HaveLen(1))
		Expect(preview.Included[0].Reason).To(Equal("explicit"))
		Expect(preview.Included[0].ItemIndex).To(BeZero())
		Expect(preview.Included[0].Citations).To(HaveLen(1))
		Expect(preview.Omitted).To(Equal([]json.RawMessage{}))
		exact := fileContextJSON(invoke(bin, cwd, "factory", "", append(args, "--max-bytes", "206")...))
		Expect(exact.Payload).To(Equal(preview.Payload))
		Expect(exact.Digest).NotTo(Equal(preview.Digest))
		over := invoke(bin, cwd, "factory", "", append(args, "--max-bytes", "205")...)
		fileContextRefusal(over)
		Expect(over.stderr).To(ContainSubstring("206"))
		Expect(over.stderr).NotTo(ContainSubstring("Refund"))
		Expect(installationSnapshot(repo)).To(Equal(before))
	})

	// per specs/003-task-context-and-skills.md:99
	// per specs/003-task-context-and-skills.md:295
	It("preserves request order and source bytes while displaying terminal text safely", func() {
		const path = "scope,notes\x1b\u202e.txt"
		const content = "customer <ok> & café\u2028\r\nvisible\x1b\u202e\r\n"
		writeFixture(filepath.Join(repo, path), []byte(content), 0600)
		writeFixture(filepath.Join(repo, "empty"), nil, 0600)
		args := []string{"context", "preview", "--file", path, "--file", "empty"}
		two := fileContextJSON(invoke(bin, repo, "factory", "", append(args, "--json")...))
		// JCS uses literal UTF-8 for U+2028 and HTML characters. CR, LF and ESC are escaped.
		Expect(two.PayloadBytes).To(Equal(431))
		Expect(two.Payload).To(HaveLen(two.PayloadBytes))
		Expect(string(two.Payload)).To(ContainSubstring("customer <ok> & café\u2028"))
		Expect(two.Digest).To(Equal("f6b92c8f1c2a7a0efbd2a872c65fcaf1121919d198435f3602542eaf136d0909"))
		duplicates := fileContextJSON(invoke(bin, repo, "factory", "", append(args, "--file", path, "--json")...))
		Expect(two.Payload).To(Equal(duplicates.Payload))
		Expect(two.Digest).NotTo(Equal(duplicates.Digest))
		var payload struct {
			Items []struct {
				Text      string
				Citations []struct {
					Path, SHA256 string
					Lines        json.RawMessage
				}
			}
		}
		Expect(json.Unmarshal(two.Payload, &payload)).To(Succeed())
		Expect(payload.Items).To(HaveLen(2))
		for index, item := range payload.Items {
			Expect(item.Citations).To(HaveLen(1))
			wantPath := []string{path, "empty"}[index]
			wantText := []string{content, ""}[index]
			Expect(item.Text).To(Equal(wantText))
			Expect(item.Citations[0].Path).To(Equal(wantPath))
			Expect(item.Citations[0].SHA256).To(Equal(fmt.Sprintf("%x", sha256.Sum256([]byte(wantText)))))
			Expect(string(item.Citations[0].Lines)).To(MatchJSON([]string{"[1,2]", "null"}[index]))
		}
		reversed := fileContextJSON(invoke(bin, repo, "factory", "", "context", "preview", "--file", "empty", "--file", path, "--json"))
		Expect(reversed.Digest).NotTo(Equal(two.Digest))
		Expect(json.Unmarshal(reversed.Payload, &payload)).To(Succeed())
		Expect(payload.Items[0].Citations[0].Path).To(Equal("empty"))
		display := invoke(bin, repo, "factory", "", args...)
		Expect(display.status).To(BeZero(), "%s", display.stderr)
		Expect(display.stdout).To(ContainSubstring("visible"))
		Expect(display.stdout).To(ContainSubstring(two.Digest))
		Expect(display.stdout).NotTo(ContainSubstring("\x1b"))
		Expect(display.stdout).NotTo(ContainSubstring("\u202e"))
	})

	// per specs/003-task-context-and-skills.md:49
	// per specs/003-task-context-and-skills.md:120
	It("blocks ineligible files without printing previously read material or private contents", func() {
		const secret = "hidden ticket text"
		const prior = "first file must stay private on refusal"
		writeFixture(filepath.Join(repo, "public.txt"), []byte(prior), 0600)
		writeFixture(filepath.Join(repo, ".gitignore"), []byte("*.key\n"), 0600)
		for _, name := range []string{"ignored.key", "tracked.key", ".factory/state", "dir/private.txt", "hard-original", ".git/private"} {
			writeFixture(filepath.Join(repo, name), []byte(secret), 0600)
		}
		imageGit(repo, "add", "-f", "--", "tracked.key", ".factory/state")
		fileContextJSON(invoke(bin, repo, "factory", "", "context", "preview", "--file", "tracked.key", "--json"))
		fileContextJSON(invoke(bin, repo, "factory", "", "context", "preview", "--file", "dir/private.txt", "--json"))
		Expect(os.Symlink("dir/private.txt", filepath.Join(repo, "file-link"))).To(Succeed())
		Expect(os.Symlink("dir", filepath.Join(repo, "dir-link"))).To(Succeed())
		Expect(os.Link(filepath.Join(repo, "hard-original"), filepath.Join(repo, "hard"))).To(Succeed())
		Expect(syscall.Mkfifo(filepath.Join(repo, "fifo"), 0600)).To(Succeed())
		writeFixture(filepath.Join(repo, "invalid"), []byte{0xff}, 0600)
		writeFixture(filepath.Join(repo, "binary"), []byte{'x', 0, 'y'}, 0600)
		for _, name := range []string{"ignored.key", ".factory/state", ".git/private", "file-link", "dir-link/private.txt", "hard", "fifo", "dir", "binary", "invalid", filepath.Join(repo, "public.txt"), "dir/../public.txt", "../outside", "missing\x1b\u202e"} {
			result := invoke(bin, repo, "factory", "", "context", "preview", "--json", "--file", "public.txt", "--file", name)
			fileContextRefusal(result)
			Expect(result.stderr).NotTo(ContainSubstring(secret), "%q", name)
			Expect(result.stderr).NotTo(ContainSubstring(prior), "%q", name)
			Expect(result.stderr).NotTo(ContainSubstring("\x1b"), "%q", name)
			Expect(result.stderr).NotTo(ContainSubstring("\u202e"), "%q", name)
		}
		separate := GinkgoT().TempDir()
		fileContextRefusal(invoke(bin, separate, "factory", "", "context", "preview", "--file", "outside", "--json"))
		imageGit(separate, "init", "--quiet", "--separate-git-dir="+filepath.Join(separate, "admin"))
		writeFixture(filepath.Join(separate, "admin", "private"), []byte(secret), 0600)
		result := invoke(bin, separate, "factory", "", "context", "preview", "--file", "admin/private", "--json")
		fileContextRefusal(result)
		Expect(result.stderr).NotTo(ContainSubstring(secret))
		writeFixture(filepath.Join(separate, "private-state", "checkpoint"), []byte(secret), 0600)
		Expect(os.Symlink("private-state", filepath.Join(separate, ".factory"))).To(Succeed())
		result = invoke(bin, separate, "factory", "", "context", "preview", "--file", "private-state/checkpoint", "--json")
		fileContextRefusal(result)
		Expect(result.stderr).NotTo(ContainSubstring(secret))
		fileContextRefusal(invoke(bin, separate, "factory", "", "context", "invalid"))
		writeFixture(filepath.Join(separate, "ordinary.txt"), []byte(prior), 0600)
		writeFixture(filepath.Join(separate, "admin", "index"), []byte(secret), 0600)
		result = invoke(bin, separate, "factory", "", "context", "preview", "--file", "ordinary.txt", "--json")
		Expect(result.status).To(Equal(1))
		Expect(result.stdout).To(BeEmpty())
		Expect(result.stderr).NotTo(BeEmpty())
		Expect(len(result.stderr)).To(BeNumerically("<=", 1024))
		Expect(result.stderr).NotTo(ContainSubstring(secret))
		Expect(result.stderr).NotTo(ContainSubstring(prior))
		fileContextRefusal(invoke(bin, filepath.Join(repo, ".git"), "factory", "", "context", "preview", "--file", "public.txt", "--json"))
		sibling := GinkgoT().TempDir()
		writeFixture(filepath.Join(sibling, "public.txt"), []byte(prior), 0600)
		imageGit(repo, "config", "core.worktree", sibling)
		Expect(strings.TrimSpace(imageGit(repo, "rev-parse", "--is-inside-work-tree"))).To(Equal("false"))
		fileContextRefusal(invoke(bin, repo, "factory", "", "context", "preview", "--file", "public.txt", "--json"))
	})

	// per docs/DECISION_LOG.md:9255
	It("bounds raw reads, repeated selections and diagnostics independently of the payload cap", func() {
		large := bytes.Repeat([]byte("q"), 1<<20)
		run := func(files ...string) cliResult {
			args := []string{"context", "preview", "--json", "--max-bytes", "1048576"}
			for _, file := range files {
				args = append(args, "--file", file)
			}
			return invoke(bin, repo, "factory", "", args...)
		}
		for _, name := range []string{"a", "b", "c", "d"} {
			writeFixture(filepath.Join(repo, name), large, 0600)
		}
		writeFixture(filepath.Join(repo, "one"), []byte("1"), 0600)
		writeFixture(filepath.Join(repo, "oversize"), append(large, 'q'), 0600)
		for _, c := range []struct {
			files  []string
			reason string
		}{
			{[]string{"a"}, "payload"},
			{[]string{"oversize"}, "file byte limit"},
			{[]string{"a", "b", "c", "d"}, "payload"},
			{[]string{"a", "b", "c", "d", "one"}, "total read limit"},
		} {
			result := run(c.files...)
			fileContextRefusal(result)
			Expect(strings.ToLower(result.stderr)).To(ContainSubstring(c.reason), "%v", c.files)
		}
		writeFixture(filepath.Join(repo, "medium"), large[:40*1024], 0600)
		repeated := make([]string, 128)
		for i := range repeated {
			repeated[i] = "medium"
		}
		Expect(fileContextJSON(run(repeated...)).PayloadBytes).To(BeNumerically("<", 41*1024))
		fileContextRefusal(run(append(repeated, "medium")...))
		fileContextRefusal(run(strings.Repeat("oversized-name", 300)))
		// Derive the message overhead from a short rejected path, so its wording may evolve.
		const invalidPath = "../sentinel"
		sample := run(invalidPath)
		fileContextRefusal(sample)
		Expect(sample.stderr).To(ContainSubstring(invalidPath))
		Expect(sample.stderr).To(HaveSuffix("\n"))
		const tail = "éa"
		atLimit := invalidPath + strings.Repeat("a", 1024-len(sample.stderr)-len(tail)) + tail
		expected := strings.Replace(sample.stderr, invalidPath, atLimit, 1)
		exactDiagnostic := run(atLimit)
		fileContextRefusal(exactDiagnostic)
		Expect(exactDiagnostic.stderr).To(Equal(expected))
		Expect(exactDiagnostic.stderr).To(HaveLen(1024))
		overDiagnostic := run(atLimit + "a")
		fileContextRefusal(overDiagnostic)
		Expect(utf8.ValidString(overDiagnostic.stderr)).To(BeTrue())
		Expect(overDiagnostic.stderr).To(HaveSuffix("\n"))
		for _, cap := range []string{"0", "-1", "nonnumeric", "1048577"} {
			fileContextRefusal(invoke(bin, repo, "factory", "", "context", "preview", "--file", "one", "--max-bytes", cap))
		}
		fileContextRefusal(invoke(bin, repo, "factory", "", "context", "preview"))
	})
})
