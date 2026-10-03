package recoverycmd

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/sys/unix"
)

type recoveryPlanState struct {
	mode  os.FileMode
	bytes []byte
}

func recoveryPlanSnapshot(root string) map[string]recoveryPlanState {
	GinkgoHelper()
	result := map[string]recoveryPlanState{}
	Expect(filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		state := recoveryPlanState{mode: info.Mode()}
		if info.Mode().IsRegular() {
			state.bytes, err = os.ReadFile(path) // #nosec G122 -- this snapshot reads only the spec's private quiescent fixture; no other actor or request selects these paths.
			if err != nil {
				return err
			}
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		result[relative] = state
		return nil
	})).To(Succeed())
	return result
}

func recoveryPlanningCommandFixture() (string, []string) {
	GinkgoHelper()
	root, creationArgs, environment := recoveryCommandFixture()
	var stdout, stderr bytes.Buffer
	Expect(Run(context.Background(), creationArgs, environment, &stdout, &stderr)).To(BeZero())
	Expect(stderr.Len()).To(BeZero())
	return root, []string{"--plan-restore", "--migration-id", "command-set"}
}

var _ = Describe("Read-only restoration planning command output", func() {
	// per docs/adr/0092-go-recovery-restoration-planning.md:94
	// per docs/adr/0092-go-recovery-restoration-planning.md:100
	DescribeTable("uses operational status for report sink errors and leaves all state unchanged", func(kind string, json bool) {
		root, args := recoveryPlanningCommandFixture()
		if json {
			args = append(args, "--json")
		}
		before := recoveryPlanSnapshot(root)
		writer := &recoverySink{kind: kind}
		var stderr bytes.Buffer
		Expect(RunPlanning(context.Background(), args, writer, &stderr)).To(Equal(1))
		Expect(stderr.String()).To(Equal("factory upgrade: cannot write recovery plan\n"))
		Expect(writer.String()).NotTo(ContainSubstring("PRIVATE_"))
		Expect(writer.String()).NotTo(ContainSubstring(root))
		if kind == "write" {
			Expect(writer.Len()).To(BeZero())
		}
		if kind == "flush" {
			Expect(writer.flushes).To(Equal(1))
		}
		Expect(recoveryPlanSnapshot(root)).To(Equal(before))
	}, Entry("JSON write error", "write", true), Entry("JSON short write", "short", true), Entry("JSON flush error", "flush", true), Entry("text write error", "write", false), Entry("text short write", "short", false), Entry("text flush error", "flush", false))

	// per docs/adr/0092-go-recovery-restoration-planning.md:94
	DescribeTable("uses operational status when a refused request's diagnostic cannot complete", func(kind string) {
		var stdout bytes.Buffer
		stderr := &recoverySink{kind: kind}
		Expect(RunPlanning(context.Background(), []string{"--plan-restore"}, &stdout, stderr)).To(Equal(1))
		Expect(stdout.Len()).To(BeZero())
		Expect(stderr.String()).NotTo(ContainSubstring("PRIVATE_"))
	}, Entry("diagnostic write error", "write"), Entry("diagnostic short write", "short"), Entry("diagnostic flush error", "flush"))

	// per docs/adr/0092-go-recovery-restoration-planning.md:94
	It("preserves cancellation during report output as operational failure", func() {
		root, args := recoveryPlanningCommandFixture()
		before := recoveryPlanSnapshot(root)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		writer := &recoverySink{cancel: cancel}
		var stderr bytes.Buffer
		Expect(RunPlanning(ctx, append(args, "--json"), writer, &stderr)).To(Equal(1))
		Expect(stderr.String()).To(Equal("factory upgrade: cannot write recovery plan\n"))
		Expect(recoveryPlanSnapshot(root)).To(Equal(before))
	})

	// per docs/adr/0092-go-recovery-restoration-planning.md:94
	It("cancels a blocked diagnostic without launching fallback or waiting for a pipe reader", func() {
		reader, writer, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(reader.Close)
		DeferCleanup(writer.Close)
		descriptor := int(writer.Fd())
		Expect(unix.SetNonblock(descriptor, true)).To(Succeed())
		for {
			_, fillErr := unix.Write(descriptor, bytes.Repeat([]byte{'x'}, 4096))
			if errors.Is(fillErr, unix.EAGAIN) || errors.Is(fillErr, unix.EWOULDBLOCK) {
				break
			}
			Expect(fillErr).NotTo(HaveOccurred())
		}
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		status := make(chan int, 1)
		go func() { status <- RunPlanning(ctx, []string{"--plan-restore"}, &bytes.Buffer{}, writer) }()
		Eventually(status, 2*time.Second).Should(Receive(Equal(1)))
	})
})

var _ = Describe("Read-only restoration planning command cancellation", func() {
	// per docs/adr/0092-go-recovery-restoration-planning.md:93
	DescribeTable("reports an ended parent context with no report and no persistent changes", func(deadline bool) {
		root, args := recoveryPlanningCommandFixture()
		before := recoveryPlanSnapshot(root)
		ctx, cancel := context.WithCancel(context.Background())
		message := "factory upgrade: recovery planning canceled\n"
		if deadline {
			cancel()
			ctx, cancel = context.WithDeadline(context.Background(), time.Unix(0, 0))
			message = "factory upgrade: recovery planning deadline exceeded\n"
		} else {
			cancel()
		}
		defer cancel()
		var stdout, stderr bytes.Buffer
		Expect(RunPlanning(ctx, args, &stdout, &stderr)).To(Equal(1))
		Expect(stdout.Len()).To(BeZero())
		Expect(stderr.String()).To(Equal(message))
		Expect(recoveryPlanSnapshot(root)).To(Equal(before))
	}, Entry("parent cancellation", false), Entry("parent expired deadline", true))
})
