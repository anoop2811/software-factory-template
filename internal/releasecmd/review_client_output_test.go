package releasecmd

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// per docs/adr/0153-authenticated-review-client-bootstrap.md:71
var _ = Describe("Authenticated review client publication custody", func() {
	var parent, output string
	BeforeEach(func() {
		dir, err := os.MkdirTemp("", "review-client-output-")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(os.RemoveAll, dir)
		parent, err = filepath.EvalSymlinks(dir)
		Expect(err).NotTo(HaveOccurred())
		output = filepath.Join(parent, "private client")
	})

	// per docs/adr/0153-authenticated-review-client-bootstrap.md:35
	// per docs/adr/0153-authenticated-review-client-bootstrap.md:74
	It("publishes only complete checked bytes under the private fixed executable name", func() {
		owner, err := openReviewClientOutput(context.Background(), output)
		Expect(err).NotTo(HaveOccurred())
		data := "authenticated complete executable bytes"
		reader := &reviewClientActionReader{Reader: strings.NewReader(data), action: func() {
			_, err := os.Lstat(filepath.Join(output, "factory-review-client"))
			Expect(os.IsNotExist(err)).To(BeTrue(), "copying must not expose a partial final executable")
		}}
		path, err := owner.publish(context.Background(), reader, int64(len(data)), fmt.Sprintf("%x", sha256.Sum256([]byte(data))))
		Expect(err).NotTo(HaveOccurred())
		Expect(path).To(Equal(filepath.Join(output, "factory-review-client")))
		Expect(owner.close(context.Background())).To(Succeed())
		Expect(os.ReadFile(path)).To(Equal([]byte(data)))
		entries, err := os.ReadDir(output)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(1))
		for _, name := range []string{output, path} {
			info, err := os.Lstat(name)
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Mode().Perm()).To(Equal(os.FileMode(0700)))
			Expect(info.Mode() & os.ModeSymlink).To(BeZero())
		}
	})

	// per docs/adr/0153-authenticated-review-client-bootstrap.md:73
	It("rejects incorrect digest short extra and over-limit input without an executable partial copy", func() {
		data := "authenticated bytes"
		digest := fmt.Sprintf("%x", sha256.Sum256([]byte(data)))
		for index, testcase := range []struct {
			size   int64
			digest string
		}{{int64(len(data)), strings.Repeat("0", 64)}, {int64(len(data) - 1), digest}, {int64(len(data) + 1), digest}, {256<<20 + 1, digest}} {
			path := filepath.Join(parent, fmt.Sprintf("refused-%d", index))
			owner, err := openReviewClientOutput(context.Background(), path)
			Expect(err).NotTo(HaveOccurred())
			published, err := owner.publish(context.Background(), strings.NewReader(data), testcase.size, testcase.digest)
			Expect(err).To(HaveOccurred())
			Expect(published).To(BeEmpty())
			Expect(owner.close(context.Background())).To(Succeed())
			_, err = os.Lstat(path)
			Expect(os.IsNotExist(err)).To(BeTrue(), "failed owned copies leave neither an executable nor an empty reserved directory")
		}
	})

	// per docs/adr/0153-authenticated-review-client-bootstrap.md:73
	// per docs/adr/0153-authenticated-review-client-bootstrap.md:94
	It("preserves a foreign leaf created during copying and never replaces it", func() {
		owner, err := openReviewClientOutput(context.Background(), output)
		Expect(err).NotTo(HaveOccurred())
		leaf := filepath.Join(output, "factory-review-client")
		data := "authenticated bytes"
		reader := &reviewClientActionReader{Reader: strings.NewReader(data), action: func() { Expect(os.WriteFile(leaf, []byte("foreign occupant"), 0600)).To(Succeed()) }}
		published, err := owner.publish(context.Background(), reader, int64(len(data)), fmt.Sprintf("%x", sha256.Sum256([]byte(data))))
		Expect(err).To(HaveOccurred())
		Expect(published).To(BeEmpty())
		_ = owner.close(context.Background())
		Expect(os.ReadFile(leaf)).To(Equal([]byte("foreign occupant")))
		info, err := os.Lstat(leaf)
		Expect(err).NotTo(HaveOccurred())
		Expect(info.Mode().Perm()).To(Equal(os.FileMode(0600)))
	})

	// per docs/adr/0153-authenticated-review-client-bootstrap.md:72
	// per docs/adr/0153-authenticated-review-client-bootstrap.md:95
	It("refuses changed output custody during copying and preserves replacement directory evidence", func() {
		owner, err := openReviewClientOutput(context.Background(), output)
		Expect(err).NotTo(HaveOccurred())
		moved := output + ".original"
		data := "authenticated bytes"
		reader := &reviewClientActionReader{Reader: strings.NewReader(data), action: func() {
			Expect(os.Rename(output, moved)).To(Succeed())
			Expect(os.Mkdir(output, 0700)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(output, "foreign"), []byte("replacement evidence"), 0600)).To(Succeed())
		}}
		published, err := owner.publish(context.Background(), reader, int64(len(data)), fmt.Sprintf("%x", sha256.Sum256([]byte(data))))
		Expect(err).To(HaveOccurred())
		Expect(published).To(BeEmpty())
		Expect(owner.close(context.Background())).To(HaveOccurred(), "uncertain custody must not be silently reported as cleaned")
		Expect(os.ReadFile(filepath.Join(output, "foreign"))).To(Equal([]byte("replacement evidence")))
		_, err = os.Lstat(filepath.Join(output, "factory-review-client"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		_, err = os.Lstat(moved)
		Expect(err).NotTo(HaveOccurred(), "uncertain owned directory remains as evidence")
	})

	// per docs/adr/0153-authenticated-review-client-bootstrap.md:73
	It("refuses permission changes to the output or its parent before final publication", func() {
		for index, changeParent := range []bool{false, true} {
			path := filepath.Join(parent, fmt.Sprintf("mode-change-%d", index))
			owner, err := openReviewClientOutput(context.Background(), path)
			Expect(err).NotTo(HaveOccurred())
			target := path
			if changeParent {
				target = parent
			}
			data := "authenticated bytes"
			reader := &reviewClientActionReader{Reader: strings.NewReader(data), action: func() { Expect(os.Chmod(target, 0777)).To(Succeed()) }}
			published, err := owner.publish(context.Background(), reader, int64(len(data)), fmt.Sprintf("%x", sha256.Sum256([]byte(data))))
			Expect(err).To(HaveOccurred())
			Expect(published).To(BeEmpty())
			Expect(owner.close(context.Background())).To(HaveOccurred())
			_, err = os.Lstat(filepath.Join(path, "factory-review-client"))
			Expect(os.IsNotExist(err)).To(BeTrue())
			Expect(os.Chmod(target, 0700)).To(Succeed())
		}
	})

	// per docs/adr/0153-authenticated-review-client-bootstrap.md:60
	// per docs/adr/0153-authenticated-review-client-bootstrap.md:94
	It("cleans only owned incomplete copies on acknowledged cancellation and source failure", func() {
		for index, canceled := range []bool{false, true} {
			path := filepath.Join(parent, fmt.Sprintf("interrupted-%d", index))
			ctx, cancel := context.WithCancel(context.Background())
			owner, err := openReviewClientOutput(ctx, path)
			Expect(err).NotTo(HaveOccurred())
			data := "authenticated bytes"
			var reader io.Reader = reviewClientErrorReader{}
			if canceled {
				reader = &reviewClientActionReader{Reader: strings.NewReader(data), action: cancel}
			}
			published, err := owner.publish(ctx, reader, int64(len(data)), fmt.Sprintf("%x", sha256.Sum256([]byte(data))))
			cancel()
			Expect(err).To(HaveOccurred())
			Expect(published).To(BeEmpty())
			Expect(owner.close(context.Background())).To(Succeed())
			_, err = os.Lstat(path)
			Expect(os.IsNotExist(err)).To(BeTrue())
		}
	})

	// per docs/adr/0153-authenticated-review-client-bootstrap.md:73
	// per docs/adr/0153-authenticated-review-client-bootstrap.md:81
	// per docs/adr/0153-authenticated-review-client-bootstrap.md:95
	DescribeTable("refuses changed published executable custody before announcing success", func(mutation string) {
		owner, err := openReviewClientOutput(context.Background(), output)
		Expect(err).NotTo(HaveOccurred())
		data := "authenticated complete executable bytes"
		path, err := owner.publish(context.Background(), strings.NewReader(data), int64(len(data)), fmt.Sprintf("%x", sha256.Sum256([]byte(data))))
		Expect(err).NotTo(HaveOccurred())
		original, err := os.Lstat(path)
		Expect(err).NotTo(HaveOccurred())
		outside := filepath.Join(parent, "outside-canary")
		Expect(os.WriteFile(outside, []byte("foreign canary"), 0600)).To(Succeed())
		switch mutation {
		case "symlink", "replacement":
			Expect(os.Rename(path, filepath.Join(parent, "published-original"))).To(Succeed())
			if mutation == "symlink" {
				Expect(os.Symlink(outside, path)).To(Succeed())
			} else {
				Expect(os.WriteFile(path, []byte("foreign executable"), 0600)).To(Succeed())
				Expect(os.Chmod(path, 0700)).To(Succeed())
			}
		case "content":
			Expect(os.WriteFile(path, []byte("modified executable bytes"), 0600)).To(Succeed())
		case "mode":
			Expect(os.Chmod(path, 0755)).To(Succeed())
		default:
			Fail("unknown test mutation")
		}
		before, err := os.Lstat(path)
		Expect(err).NotTo(HaveOccurred())
		if mutation == "content" || mutation == "mode" {
			Expect(os.SameFile(original, before)).To(BeTrue(), "this witness must retain the published inode")
		}
		if mutation == "content" || mutation == "replacement" {
			Expect(before.Mode().Perm()).To(Equal(os.FileMode(0700)), "content and identity witnesses retain executable permissions")
		}
		bytesBefore, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(owner.close(context.Background())).To(HaveOccurred(), "changed published bytes, identity or mode cannot produce a successful receipt")
		after, err := os.Lstat(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(before, after)).To(BeTrue(), "closing custody must preserve the substituted occupant")
		Expect(after.Mode()).To(Equal(before.Mode()))
		Expect(os.ReadFile(path)).To(Equal(bytesBefore))
		Expect(os.ReadFile(outside)).To(Equal([]byte("foreign canary")))
	},
		Entry("a foreign symlink under the published name", "symlink"),
		Entry("a foreign executable under the published name", "replacement"),
		Entry("changed bytes on the original inode", "content"),
		Entry("changed mode on the original inode", "mode"),
	)
})

type reviewClientActionReader struct {
	io.Reader
	action func()
}

func (r *reviewClientActionReader) Read(p []byte) (int, error) {
	if r.action != nil {
		action := r.action
		r.action = nil
		action()
	}
	return r.Reader.Read(p)
}

type reviewClientErrorReader struct{}

func (reviewClientErrorReader) Read([]byte) (int, error) {
	return 0, errors.New("fixture source read failure")
}
