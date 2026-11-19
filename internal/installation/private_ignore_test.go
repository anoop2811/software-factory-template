package installation

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/anoop2811/software-factory-template/internal/native"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Installation private ignore policy", func() {
	const lock = ".factory/runtime-transition.lock"
	const rules = "/.factory/bin/\n/.factory/assets/\n/.factory/installation.current\n/.factory/installation-transactions/\n/.factory/backups/\n"
	git := func(root string, args ...string) string {
		GinkgoHelper()
		cmd := exec.Command("git", args...) // #nosec G204 -- only fixed Git operations in a test-owned temporary repository.
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "git %v: %s", args, out)
		return string(out)
	}
	trackSpelling := func(root, path string) {
		GinkgoHelper()
		installationComponentWrite(root, path, nil, 0600)
		blob := strings.TrimSpace(git(root, "hash-object", "-w", "--", path))
		git(root, "update-index", "--add", "--cacheinfo", "100644", blob, path)
	}
	project := func() (string, *transaction) {
		GinkgoHelper()
		root, err := filepath.EvalSymlinks(GinkgoT().TempDir())
		Expect(err).NotTo(HaveOccurred())
		git(root, "init", "--quiet")
		owner := installationComponentOwner(root)
		owner.request.MigrationID = "ignore-proof"
		return root, owner
	}

	// per docs/DECISION_LOG.md:9059
	DescribeTable("recognizes a tracked spelling only when it identifies the same transition lock", func(variant string) {
		root, owner := project()
		installationComponentWrite(root, ".gitignore", []byte(rules+"/"+lock+"\n"), 0644)
		trackSpelling(root, variant)
		if filepath.Dir(variant) == ".FACTORY" {
			parent := filepath.Join(root, ".FACTORY")
			app, err := os.Lstat(parent)
			Expect(err).NotTo(HaveOccurred())
			private, err := os.Lstat(filepath.Join(root, ".factory"))
			Expect(err).NotTo(HaveOccurred())
			if !os.SameFile(private, app) {
				Expect(os.Chmod(parent, 0775)).To(Succeed()) // #nosec G302 -- intentionally group-writable test-owned application directory.
			}
		}
		Expect(git(root, "ls-files", "-z")).To(Equal(variant + "\x00"))
		before, err := os.Lstat(filepath.Join(root, lock))
		Expect(err).NotTo(HaveOccurred())
		other, err := os.Lstat(filepath.Join(root, variant))
		Expect(err).NotTo(HaveOccurred())
		err = owner.ensurePrivateIgnore(context.Background())
		if os.SameFile(before, other) {
			Expect(err).To(HaveOccurred(), "Git's alternate spelling is the active lock inode")
			Expect(err.Error()).To(ContainSubstring("tracked"))
		} else {
			Expect(err).NotTo(HaveOccurred(), "the tracked application file has a different inode")
		}
		after, err := os.Lstat(filepath.Join(root, lock))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(before, after)).To(BeTrue())
		Expect(git(root, "ls-files", "-z")).To(Equal(variant + "\x00"))
	}, Entry("ASCII uppercase leaf", ".factory/RUNTIME-TRANSITION.LOCK"),
		Entry("Unicode long s", ".factory/runtime-tranſition.lock"),
		Entry("Unicode Kelvin sign", ".factory/runtime-transition.locK"),
		Entry("alternate parent spelling with an independently group-writable application directory", ".FACTORY/runtime-transition.lock"))

	// per docs/DECISION_LOG.md:9050
	It("refuses a transition lock hardlinked to a tracked operator file before acquiring the transition guard", func() {
		root, err := filepath.EvalSymlinks(GinkgoT().TempDir())
		Expect(err).NotTo(HaveOccurred())
		git(root, "init", "--quiet")
		installationComponentWrite(root, lock, nil, 0600)
		canonical, alias := filepath.Join(root, lock), filepath.Join(root, "operator-note")
		Expect(os.Link(canonical, alias)).To(Succeed())
		git(root, "add", "--", "operator-note")
		Expect(git(root, "ls-files", "-z")).To(Equal("operator-note\x00"))
		indexPath := filepath.Join(root, ".git/index")
		index, err := os.ReadFile(indexPath)
		Expect(err).NotTo(HaveOccurred())
		before, err := os.Lstat(canonical)
		Expect(err).NotTo(HaveOccurred())

		err = CheckTransitionLockIndex(context.Background(), root, operationEnvironment(), nil)
		var policy *Failure
		Expect(errors.As(err, &policy)).To(BeTrue(), "a tracked hard link must refuse lock admission: %v", err)
		Expect(ErrorStatus(err)).To(Equal(2))
		for _, path := range []string{canonical, alias} {
			after, err := os.Lstat(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(os.SameFile(before, after)).To(BeTrue(), path)
			Expect(after.Mode()).To(Equal(before.Mode()), path)
			Expect(after.Size()).To(BeZero(), path)
		}
		Expect(os.ReadFile(indexPath)).To(Equal(index))
	})

	// per docs/DECISION_LOG.md:9050
	// per docs/adr/0097-installation-source-image-bundles.md:223
	It("refuses an oversized transition lock index inspection and preserves process uncertainty", func() {
		root, owner := project()
		ignore := []byte(rules + "/" + lock + "\n")
		installationComponentWrite(root, ".gitignore", ignore, 0644)
		blob := strings.TrimSpace(git(root, "hash-object", "-w", "--", ".gitignore"))
		var rows strings.Builder
		for i := range 512 {
			path := fmt.Sprintf(".factory/runtime-tran%04dition.loc%s", i, strings.Repeat("x", 128))
			_, _ = fmt.Fprintf(&rows, "100644 %s\t%s\n", blob, path)
		}
		cmd := exec.Command("git", "update-index", "--index-info")
		cmd.Dir, cmd.Stdin = root, strings.NewReader(rows.String())
		out, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "load test index: %s", out)
		Expect(len(git(root, "ls-files", "-z"))).To(BeNumerically(">", 64<<10))
		indexPath := filepath.Join(root, ".git/index")
		index, err := os.ReadFile(indexPath)
		Expect(err).NotTo(HaveOccurred())
		before, err := os.Lstat(filepath.Join(root, lock))
		Expect(err).NotTo(HaveOccurred())
		err = owner.ensurePrivateIgnore(context.Background())
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("transition lock index inspection exceeded its output limit"))
		var uncertainty *native.OwnershipError
		if errors.As(err, &uncertainty) {
			Expect(uncertainty.ProcessPID).To(BeNumerically(">", 0))
			Expect(ErrorStatus(err)).To(Equal(1))
		} else {
			Expect(ErrorStatus(err)).To(Equal(2), "a settled inspection that exceeded its limit is a policy conflict")
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		canceled := CheckTransitionLockIndex(ctx, root, operationEnvironment(), nil)
		Expect(errors.Is(canceled, context.Canceled)).To(BeTrue())
		Expect(ErrorStatus(canceled)).To(Equal(1))
		Expect(os.ReadFile(indexPath)).To(Equal(index))
		Expect(os.ReadFile(filepath.Join(root, ".gitignore"))).To(Equal(ignore))
		after, err := os.Lstat(filepath.Join(root, lock))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(before, after)).To(BeTrue())
	})

	// per docs/DECISION_LOG.md:9059
	It("refuses an indexed alternate spelling whose deleted file cannot be distinguished from a future lock", func() {
		root, err := filepath.EvalSymlinks(GinkgoT().TempDir())
		Expect(err).NotTo(HaveOccurred())
		git(root, "init", "--quiet")
		const variant = ".factory/RUNTIME-TRANSITION.LOCK"
		trackSpelling(root, variant)
		ctx := context.Background()
		_, err = os.Lstat(filepath.Join(root, lock))
		if os.IsNotExist(err) {
			Expect(CheckTransitionLockIndex(ctx, root, operationEnvironment(), nil)).To(Succeed(),
				"the distinct tracked spelling exists while the canonical lock is absent on this volume")
		} else {
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(os.Remove(filepath.Join(root, variant))).To(Succeed())
		_, err = os.Lstat(filepath.Join(root, lock))
		Expect(os.IsNotExist(err)).To(BeTrue())
		Expect(git(root, "ls-files", "-z")).To(Equal(variant + "\x00"))
		Expect(CheckTransitionLockIndex(ctx, root, operationEnvironment(), nil)).NotTo(Succeed(),
			"without either file, future publication cannot prove these names identify distinct files")
		_, err = os.Lstat(filepath.Join(root, lock))
		Expect(os.IsNotExist(err)).To(BeTrue())
		Expect(git(root, "ls-files", "-z")).To(Equal(variant + "\x00"))
	})

	// per docs/DECISION_LOG.md:9061
	It("fits the merged rules within the readable installation limit and preserves an adopter file on overflow", func() {
		const limit = 1 << 20
		const missing = "/" + lock + "\n"
		for _, extra := range []int{0, 1} {
			root, owner := project()
			padding := limit - len(rules) - len(missing) - 2 + extra
			body := append([]byte("#"), bytes.Repeat([]byte("x"), padding)...)
			body = append(body, "\n"+rules...)
			path := filepath.Join(root, ".gitignore")
			installationComponentWrite(root, ".gitignore", body, 0644)
			before, err := os.Lstat(path)
			Expect(err).NotTo(HaveOccurred())
			err = owner.ensurePrivateIgnore(context.Background())
			if extra == 0 {
				Expect(err).NotTo(HaveOccurred(), "exactly %d bytes must remain supported", limit)
				want := append(bytes.Clone(body), missing...)
				Expect(os.ReadFile(path)).To(Equal(want))
			} else {
				Expect(err).To(HaveOccurred(), "adding the missing rule would exceed %d bytes", limit)
				after, err := os.Lstat(path)
				Expect(err).NotTo(HaveOccurred())
				Expect(os.SameFile(before, after)).To(BeTrue())
				Expect(os.ReadFile(path)).To(Equal(body))
			}
		}
	})
})
