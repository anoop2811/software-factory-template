package acceptance_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Isolate compiler-cancellation setup from the size of the factory repository.
// The public caller still collects both real fixed predecessor catalogs.
// per docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:305
func localCompilerCancellationSource(root string, image handoffReleaseImage) string {
	GinkgoHelper()
	source := filepath.Join(root, "small committed compiler cancellation source")
	writeFixture(filepath.Join(source, "go.mod"), []byte("module example.com/local-compiler-cancellation\n\ngo 1.27.3\n"), 0600)
	writeFixture(filepath.Join(source, "cmd/factory/main.go"), []byte("package main\nfunc main() {}\n"), 0600)
	imageGit(source, "init", "--quiet", "--template=")
	imageGit(source, "config", "gc.auto", "0")
	imageGit(source, "config", "maintenance.auto", "false")
	imageGit(source, "add", ".")
	imageGit(source, "-c", "user.name=Cancellation fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "compiler cancellation source")
	objects := imageProcess("..", nil, "git", "rev-parse", "--path-format=absolute", "--git-path", "objects")
	Expect(objects.status).To(Equal(0), "%+v", objects)
	writeFixture(filepath.Join(source, ".git/objects/info/alternates"), []byte(strings.TrimSpace(objects.stdout)+"\n"), 0600)
	references := map[string]any{}
	total := 0
	for _, label := range []string{"v0.1.6", "bash-baseline"} {
		reference, present := image.references[label]
		Expect(present).To(BeTrue())
		Expect(strings.TrimSpace(imageGit(source, "cat-file", "-t", reference.revision))).To(Equal("commit"))
		tree := imageGit(source, "ls-tree", "-r", "-z", "--full-tree", reference.revision)
		count := strings.Count(tree, "\x00")
		Expect(count).To(Equal(len(reference.assets)))
		total += count
		references[label] = map[string]any{"revision": reference.revision, "actual_blob_count": count}
	}
	Expect(total).To(Equal(297))
	tree := imageGit(source, "ls-tree", "-r", "-z", "--full-tree", "HEAD")
	Expect(strings.Count(tree, "\x00")).To(Equal(2))
	Expect(imageGit(source, "status", "--porcelain", "--untracked-files=all")).To(BeEmpty())
	AddReportEntry("local-compiler.cancellation-source", map[string]any{"source": source, "source_blob_count": 2, "reference_blob_count": total, "reference_checks": references, "revision": strings.TrimSpace(imageGit(source, "rev-parse", "HEAD"))}, ReportEntryVisibilityNever)
	return source
}

type localCompilerCancellationCustody struct {
	command *exec.Cmd
	done    chan struct{}
	waitErr error
	ready   string
	roots   []string
	receipt *localCompilerReceipt
}

// One waiter publishes every capture/error read through a closed-channel barrier.
// per docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:315
func localCompilerCancellationWait(command *exec.Cmd, ready string, roots ...string) *localCompilerCancellationCustody {
	custody := &localCompilerCancellationCustody{command: command, done: make(chan struct{}), ready: ready, roots: roots}
	go func() {
		custody.waitErr = command.Wait()
		close(custody.done)
	}()
	DeferCleanup(custody.cleanup)
	return custody
}

func (custody *localCompilerCancellationCustody) joined() bool {
	select {
	case <-custody.done:
		return true
	default:
		return false
	}
}

// Register before READY. Only an admitted descendant receipt authorizes its PIDs;
// an owned parent handle and a private release gate remain usable without one.
// per docs/adr/0126-go-source-launcher-and-direct-init-upgrade.md:315
func (custody *localCompilerCancellationCustody) cleanup() error {
	settled := custody.joined() && custody.receipt != nil && !localCompilerAlive(*custody.receipt)
	if settled {
		return nil
	}
	defer func() {
		if !settled {
			for _, root := range custody.roots {
				holdFixtureCleanup(root, "compiler cancellation fixture lacks confirmed caller and descendant closure")
			}
		}
	}()
	// Cooperative fixture release never assumes ownership of an unadmitted group.
	if err := os.WriteFile(custody.ready+".release", nil, 0600); err != nil {
		return err
	}
	join := func() bool {
		select {
		case <-custody.done:
			return true
		case <-time.After(6 * time.Second):
			return false
		}
	}
	if !custody.joined() {
		_ = custody.command.Process.Signal(syscall.SIGTERM)
		if !join() {
			if custody.receipt != nil {
				localCompilerCleanup(*custody.receipt)
			}
			_ = custody.command.Process.Kill()
			if !join() {
				return errors.New("owned local compiler caller did not join; retain fixture evidence")
			}
		}
	}
	if custody.receipt == nil {
		AddReportEntry("local-compiler.unadmitted-cleanup", map[string]any{"caller_joined": custody.joined(), "retained": true, "ready": custody.ready}, ReportEntryVisibilityNever)
		return nil
	}
	if localCompilerAlive(*custody.receipt) {
		localCompilerCleanup(*custody.receipt)
		deadline := time.Now().Add(6 * time.Second)
		for localCompilerAlive(*custody.receipt) && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
	}
	settled = custody.joined() && !localCompilerAlive(*custody.receipt)
	AddReportEntry("local-compiler.cancellation-cleanup", map[string]any{"caller_joined": custody.joined(), "descendants_absent": !localCompilerAlive(*custody.receipt), "retained": !settled}, ReportEntryVisibilityNever)
	if !settled {
		return errors.New("admitted local compiler descendants did not close; retain fixture evidence")
	}
	return nil
}
