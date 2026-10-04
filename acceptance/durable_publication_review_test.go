package acceptance_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/anoop2811/software-factory-template/internal/assessment"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/sys/unix"
)

// Keep relative-root qualification inside the actual client process. The package
// working directory is shared with other specs and must not be changed.
// per docs/adr/0095-durable-live-publication.md:257
func durablePublicationReviewStart(binaryRoot, workingRoot, operand string, request assessment.PublicationRequest) *durablePublicationClient {
	GinkgoHelper()
	program := filepath.Join(binaryRoot, "durable-publication-review-client")
	writeFixture(program, durablePublicationBuild(), 0700)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	client := &durablePublicationClient{cancel: cancel}
	client.command = exec.CommandContext(ctx, program, operand, "begin") // #nosec G204 -- fixed evaluator binary and test-owned root operands.
	client.command.Dir = workingRoot
	client.command.Env = append(durableGitEnvironment(), "PATH="+filepath.Join(binaryRoot, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"), "DURABLE_GIT_MARKER="+filepath.Join(binaryRoot, "durable-git-calls"))
	client.command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	client.command.Stderr = &client.stderr
	input, err := client.command.StdinPipe()
	Expect(err).NotTo(HaveOccurred())
	output, err := client.command.StdoutPipe()
	Expect(err).NotTo(HaveOccurred())
	client.input, client.encode, client.decode = input, json.NewEncoder(input), json.NewDecoder(output)
	client.decode.UseNumber()
	Expect(client.command.Start()).To(Succeed())
	DeferCleanup(func() {
		if !client.waited {
			_ = unix.Kill(-client.command.Process.Pid, unix.SIGKILL)
			_ = client.command.Wait()
		}
		cancel()
	})
	Expect(client.encode.Encode(request)).To(Succeed())
	return client
}

var _ = Describe("Durable publication review root spellings", func() {
	// per docs/adr/0095-durable-live-publication.md:257
	// per docs/adr/0095-durable-live-publication.md:129
	// per docs/adr/0095-durable-live-publication.md:133
	DescribeTable("completes the real compiled-client lifecycle for equivalent qualified roots", func(form string) {
		binaryRoot, root, request := durablePublicationFixture()
		packageDirectory, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		operand := root
		switch form {
		case "current directory":
			operand = "."
		case "absolute trailing separator":
			operand = root + string(os.PathSeparator)
		}
		protected := durablePublicationProtected(root, request.Path)
		original, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		client := durablePublicationReviewStart(binaryRoot, root, operand, request)
		durablePublicationSuccess(client.response("begin"))
		pending := publicationFaultPendingExternal(root)
		queries := durableBytes(binaryRoot, "durable-git-calls")
		Expect(queries).NotTo(BeEmpty(), "the constructor must qualify the same physical repository through real Git")
		durablePublicationRecord(root, request.MigrationID)
		durablePublicationSuccess(client.call("apply"))
		applied, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(original, applied)).To(BeFalse())
		Expect(applied.Mode()).To(Equal(original.Mode()))
		Expect(durableBytes(root, request.Path)).To(Equal(request.Replacement))
		publicationPendingPreserved(root, pending)
		durablePublicationSuccess(client.call("finish"))
		terminal, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(applied, terminal)).To(BeTrue())
		_, err = os.Lstat(filepath.Join(root, ".factory/runtime-publication.pending"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		durablePublicationInventory(client.call("inspect"), request, "forward_completed", "0")
		durablePublicationSuccess(client.call("close"))
		client.wait()
		Expect(durableBytes(binaryRoot, "durable-git-calls")).To(Equal(queries))
		Expect(durablePublicationProtected(root, request.Path)).To(Equal(protected))
		livePublicationNoExecutionState(root)
		actualDirectory, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		Expect(actualDirectory).To(Equal(packageDirectory), "only the actual child may change its working directory")
	}, Entry("canonical absolute control", "absolute"), Entry("literal current directory", "current directory"), Entry("absolute trailing separator", "absolute trailing separator"))

	// per docs/adr/0095-durable-live-publication.md:257
	// per docs/adr/0095-durable-live-publication.md:93
	DescribeTable("refuses traversal and symlink operands before creating durable pending or querying Git", func(form string) {
		binaryRoot, root, request := durablePublicationFixture()
		operand := root + "/../" + filepath.Base(root)
		if form == "symlink" {
			operand = filepath.Join(binaryRoot, "installation-link")
			Expect(os.Symlink(root, operand)).To(Succeed())
		}
		preserved := assessmentTree(root)
		original, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		bytes := durableBytes(root, request.Path)
		client := durablePublicationReviewStart(binaryRoot, root, operand, request)
		refused := client.response("begin")
		client.wait()
		Expect(refused).To(HaveKeyWithValue("status", json.Number("2")))
		Expect(refused["error"]).NotTo(BeEmpty())
		Expect(refused["error"]).NotTo(ContainSubstring(root))
		Expect(assessmentTree(root)).To(Equal(preserved))
		after, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(original, after)).To(BeTrue())
		Expect(durableBytes(root, request.Path)).To(Equal(bytes))
		for _, path := range []string{filepath.Join(root, ".factory/runtime-publication.pending"), durablePublicationSlot(root, request.MigrationID), filepath.Join(binaryRoot, "durable-git-calls")} {
			_, err := os.Lstat(path)
			Expect(os.IsNotExist(err)).To(BeTrue(), "%s must remain absent after rejected root qualification", path)
		}
	}, Entry("parent traversal", "traversal"), Entry("symbolic root", "symlink"))
})
