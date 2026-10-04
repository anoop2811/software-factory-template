package acceptance_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/anoop2811/software-factory-template/internal/transition"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/sys/unix"
)

var _ = Describe("Durable live publication process interruption", func() {
	// per docs/adr/0095-durable-live-publication.md:108
	// per docs/adr/0095-durable-live-publication.md:135
	// per docs/adr/0095-durable-live-publication.md:174
	// per docs/adr/0095-durable-live-publication.md:176
	DescribeTable("observes real SIGKILL after an acknowledged lifecycle phase without inferring restart authority", func(phase string) {
		binaryRoot, root, request := durablePublicationFixture()
		original := durableBytes(root, request.Path)
		client := durablePublicationBegin(binaryRoot, root, request)
		if phase != "prepared" {
			durablePublicationSuccess(client.call("apply"))
		}
		if phase == "forward_completed" {
			durablePublicationSuccess(client.call("finish"))
		}
		if phase == "restored" {
			durablePublicationSuccess(client.call("restore"))
		}
		pendingPresent := phase == "prepared" || phase == "forward_published"
		status := "0"
		if pendingPresent {
			status = "2"
		}
		durablePublicationInventory(client.call("inspect"), request, phase, status)
		transitionLockBlocked(root)
		record := durablePublicationRecord(root, request.MigrationID)
		recordBytes := durableBytes(root, ".factory/backups/.publications/"+request.MigrationID+".json")
		selected, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		expected := request.Replacement
		if phase == "prepared" || phase == "restored" {
			expected = original
		}
		Expect(durableBytes(root, request.Path)).To(Equal(expected))
		var pending os.FileInfo
		if pendingPresent {
			pending = publicationFaultPendingExternal(root)
		}
		queries := durableBytes(binaryRoot, "durable-git-calls")
		Expect(client.command.Process.Kill()).To(Succeed(), "the evaluator kills the actual ready owner, not a synthetic handle")
		waitErr := client.command.Wait()
		client.waited = true
		client.cancel()
		var exited *exec.ExitError
		Expect(errors.As(waitErr, &exited)).To(BeTrue())
		Expect(exited.ProcessState.Sys().(syscall.WaitStatus).Signal()).To(Equal(syscall.SIGKILL))
		Expect(client.stderr.String()).To(BeEmpty())
		lock, err := os.OpenFile(filepath.Join(root, ".factory/runtime-transition.lock"), os.O_RDWR, 0)
		Expect(err).NotTo(HaveOccurred())
		Expect(unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB)).To(Succeed(), "real process death releases its OS flock")
		Expect(unix.Flock(int(lock.Fd()), unix.LOCK_UN)).To(Succeed())
		Expect(lock.Close()).To(Succeed())
		restarted := durablePublicationStart(binaryRoot, root, "inspect", request)
		durablePublicationInventory(restarted.response("inspect"), request, phase, status)
		restarted.wait()
		actual, err := os.Lstat(filepath.Join(root, request.Path))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(selected, actual)).To(BeTrue())
		Expect(actual.Mode()).To(Equal(selected.Mode()))
		Expect(durableBytes(root, request.Path)).To(Equal(expected))
		actualRecord := durablePublicationRecord(root, request.MigrationID)
		Expect(os.SameFile(record, actualRecord)).To(BeTrue())
		Expect(durableBytes(root, ".factory/backups/.publications/"+request.MigrationID+".json")).To(Equal(recordBytes))
		Expect(durableBytes(binaryRoot, "durable-git-calls")).To(Equal(queries))
		for _, acquire := range []func(context.Context, string) (*transition.Guard, error){transition.Shared, transition.Exclusive} {
			guard, acquireErr := acquire(context.Background(), root)
			if pendingPresent {
				Expect(acquireErr).To(HaveOccurred())
				Expect(guard).To(BeNil())
			} else {
				Expect(acquireErr).NotTo(HaveOccurred())
				Expect(guard.Close(context.Background(), true)).To(Succeed())
			}
		}
		if pendingPresent {
			scripts, absErr := filepath.Abs("../scripts")
			Expect(absErr).NotTo(HaveOccurred())
			for _, legacy := range []bool{false, true} {
				for _, kind := range []string{"budget", "loop"} {
					out := transitionRun(binaryRoot, root, scripts, kind, legacy, transitionRunArgs(kind))
					Expect(out.status).To(Equal(2), "%+v", out)
					Expect(out.stdout).To(BeEmpty())
					_, markerErr := os.Lstat(filepath.Join(binaryRoot, "child-started"))
					Expect(os.IsNotExist(markerErr)).To(BeTrue(), "actual Go/Python help or checks cannot run through retained pending evidence")
				}
			}
			publicationPendingPreserved(root, pending)
		} else {
			_, pendingErr := os.Lstat(filepath.Join(root, ".factory/runtime-publication.pending"))
			Expect(os.IsNotExist(pendingErr)).To(BeTrue())
		}
		livePublicationNoExecutionState(root)
	}, Entry("before active rename", "prepared"), Entry("after active rename and before terminal completion", "forward_published"),
		Entry("after forward terminal publication and pending removal", "forward_completed"), Entry("after reverse terminal publication and pending removal", "restored"))
})
