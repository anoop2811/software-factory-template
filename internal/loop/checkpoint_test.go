package loop

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/anoop2811/software-factory-template/internal/jsonvalue"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func checkpointStoreFixture() (*Store, string, History) {
	GinkgoHelper()
	root := GinkgoT().TempDir()
	Expect(os.Mkdir(filepath.Join(root, ".factory"), 0700)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(root, ".factory/loops.json"), []byte(`{"schema":1,"runs":[],"note":"before"}`), 0600)).To(Succeed())
	history, err := ParseHistory(context.Background(), strings.NewReader(`{"schema":1,"runs":[],"note":"after"}`))
	Expect(err).NotTo(HaveOccurred())
	return NewStore(root), root, history
}
func checkpointLocked(store *Store) *Transaction {
	GinkgoHelper()
	transaction, err := store.Lock(context.Background())
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() { Expect(transaction.Close()).To(Succeed()) })
	_, err = transaction.Read(context.Background())
	Expect(err).NotTo(HaveOccurred())
	return transaction
}

var _ = Describe("Loop checkpoint publication boundaries", func() {
	// per docs/adr/0074-go-loop-checkpoint-storage.md:78
	DescribeTable("preserves old versus committed bytes and poisons publication retries", func(boundary string, committed bool) {
		store, root, history := checkpointStoreFixture()
		path := filepath.Join(root, ".factory/loops.json")
		before, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		calls := 0
		fail := func(*os.File) error { calls++; return errors.New("PRIVATE_IO") }
		switch boundary {
		case "file":
			store.ops.syncFile = fail
		case "directory":
			store.ops.syncDirectory = fail
		case "rename":
			store.ops.rename = func(*os.Root, string, string) error { calls++; return errors.New("PRIVATE_RENAME") }
		}
		transaction := checkpointLocked(store)
		err = transaction.Write(context.Background(), history)
		var publication *PublicationError
		Expect(errors.As(err, &publication)).To(BeTrue())
		Expect(publication.MayHaveCommitted).To(Equal(committed))
		Expect(err.Error()).NotTo(ContainSubstring("PRIVATE"))
		Expect(calls).To(Equal(1))
		after, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		if committed {
			Expect(string(after)).To(ContainSubstring(`"after"`))
		} else {
			Expect(after).To(Equal(before))
		}
		_, _ = transaction.Read(context.Background()) // Inspection cannot restore publication permission.
		Expect(transaction.Write(context.Background(), history)).NotTo(Succeed())
		Expect(calls).To(Equal(1))
		files, err := os.ReadDir(filepath.Dir(path))
		Expect(err).NotTo(HaveOccurred())
		for _, file := range files {
			Expect(file.Name()).NotTo(HaveSuffix(".tmp"))
		}
	}, Entry("file sync before rename", "file", false), Entry("rename failure", "rename", false), Entry("directory sync after rename", "directory", true))
	// per docs/adr/0074-go-loop-checkpoint-storage.md:80
	It("preserves a replacement at the temporary pathname on cleanup failure", func() {
		store, root, history := checkpointStoreFixture()
		replacement := ""
		held := filepath.Join(root, "held")
		store.ops.syncFile = func(*os.File) error {
			entries, err := os.ReadDir(filepath.Join(root, ".factory"))
			if err != nil {
				return err
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), "loop-") && strings.HasSuffix(entry.Name(), ".tmp") {
					replacement = filepath.Join(root, ".factory", entry.Name())
					if err := os.Rename(replacement, held); err != nil {
						return err
					}
					if err := os.WriteFile(replacement, []byte("replacement-owned-by-test"), 0600); err != nil {
						return err
					}
					return errors.New("PRIVATE_FAILURE")
				}
			}
			return errors.New("missing temporary fixture")
		}
		transaction := checkpointLocked(store)
		Expect(transaction.Write(context.Background(), history)).NotTo(Succeed())
		Expect(replacement).NotTo(BeEmpty())
		data, err := os.ReadFile(replacement)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(Equal("replacement-owned-by-test"))
		_, err = os.Stat(held)
		Expect(err).NotTo(HaveOccurred())
	})
	// per docs/adr/0074-go-loop-checkpoint-storage.md:80
	DescribeTable("refuses publication after a pinned storage identity is replaced", func(kind string) {
		store, root, history := checkpointStoreFixture()
		transaction := checkpointLocked(store)
		directory := filepath.Join(root, ".factory")
		switch kind {
		case "history":
			Expect(os.Rename(filepath.Join(directory, "loops.json"), filepath.Join(root, "old-history"))).To(Succeed())
			Expect(os.WriteFile(filepath.Join(directory, "loops.json"), []byte(`{"schema":1,"runs":[],"note":"replacement"}`), 0600)).To(Succeed())
		case "lock":
			Expect(os.Rename(filepath.Join(directory, "loops.lock"), filepath.Join(root, "old-lock"))).To(Succeed())
			Expect(os.WriteFile(filepath.Join(directory, "loops.lock"), nil, 0600)).To(Succeed())
		case "directory":
			Expect(os.Rename(directory, filepath.Join(root, "old-directory"))).To(Succeed())
			Expect(os.Mkdir(directory, 0700)).To(Succeed())
		}
		Expect(transaction.Write(context.Background(), history)).NotTo(Succeed())
		if kind == "history" {
			data, err := os.ReadFile(filepath.Join(directory, "loops.json"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).To(ContainSubstring("replacement"))
		}
	}, Entry("history", "history"), Entry("lock", "lock"), Entry("directory", "directory"))
	// per docs/adr/0074-go-loop-checkpoint-storage.md:85
	It("cancels after temporary preparation without replacing history", func() {
		store, root, history := checkpointStoreFixture()
		path := filepath.Join(root, ".factory/loops.json")
		before, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		store.ops.syncFile = func(file *os.File) error { cancel(); return file.Sync() }
		transaction := checkpointLocked(store)
		Expect(transaction.Write(ctx, history)).NotTo(Succeed())
		after, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(before))
		files, err := os.ReadDir(filepath.Dir(path))
		Expect(err).NotTo(HaveOccurred())
		for _, file := range files {
			Expect(file.Name()).NotTo(HaveSuffix(".tmp"))
		}
	})
})

var _ = Describe("Loop checkpoint transaction lifetime", func() {
	// per docs/adr/0074-go-loop-checkpoint-storage.md:67
	It("holds a persistent lock through successive successful publications", func() {
		store, root, history := checkpointStoreFixture()
		transaction := checkpointLocked(store)
		path := filepath.Join(root, ".factory/loops.lock")
		lock, err := os.Stat(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(transaction.Write(context.Background(), history)).To(Succeed())
		first, err := os.Stat(filepath.Join(root, ".factory/loops.json"))
		Expect(err).NotTo(HaveOccurred())
		Expect(transaction.Write(context.Background(), history)).To(Succeed())
		second, err := os.Stat(filepath.Join(root, ".factory/loops.json"))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(first, second)).To(BeFalse())
		Expect(transaction.Close()).To(Succeed())
		next, err := store.Lock(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(next.Close()).To(Succeed())
		after, err := os.Stat(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(lock, after)).To(BeTrue())
	})
	// per docs/adr/0074-go-loop-checkpoint-storage.md:68
	It("requires a successful locked read before publication", func() {
		store, root, history := checkpointStoreFixture()
		transaction, err := store.Lock(context.Background())
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(transaction.Close()).To(Succeed()) })
		path := filepath.Join(root, ".factory/loops.json")
		before, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(transaction.Write(context.Background(), history)).NotTo(Succeed())
		after, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(before))
	})
	// per docs/adr/0074-go-loop-checkpoint-storage.md:67
	It("closes idempotently and refuses subsequent operations", func() {
		store, _, history := checkpointStoreFixture()
		transaction := checkpointLocked(store)
		Expect(transaction.Close()).To(Succeed())
		Expect(transaction.Close()).To(Succeed())
		_, err := transaction.Read(context.Background())
		Expect(err).To(HaveOccurred())
		Expect(transaction.Write(context.Background(), history)).NotTo(Succeed())
	})
	// per docs/adr/0074-go-loop-checkpoint-storage.md:68
	It("reads fresh checkpoint data after acquiring the lock", func() {
		store, root, _ := checkpointStoreFixture()
		_, err := store.Read(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(os.WriteFile(filepath.Join(root, ".factory/loops.json"), []byte(`{"schema":1,"runs":[],"note":"fresh"}`), 0600)).To(Succeed())
		transaction := checkpointLocked(store)
		history, err := transaction.Read(context.Background())
		Expect(err).NotTo(HaveOccurred())
		data, err := history.MarshalJSON()
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(ContainSubstring("fresh"))
	})
	// per docs/adr/0074-go-loop-checkpoint-storage.md:67
	It("excludes a real Python participant while Go owns the flock", func() {
		store, root, _ := checkpointStoreFixture()
		transaction := checkpointLocked(store)
		script := `import fcntl,sys
f=open(sys.argv[1],'r+')
try: fcntl.flock(f,fcntl.LOCK_EX|fcntl.LOCK_NB);print('unlocked')
except BlockingIOError: print('locked')`
		command := exec.Command("python3", "-B", "-c", script, filepath.Join(root, ".factory/loops.lock")) // #nosec G204 G702 -- fixed Python lock oracle and test-owned lock path.
		output, err := command.CombinedOutput()
		Expect(err).NotTo(HaveOccurred())
		Expect(string(output)).To(Equal("locked\n"))
		Expect(transaction.Close()).To(Succeed())
	})
	// per docs/adr/0074-go-loop-checkpoint-storage.md:67
	It("immediately refuses Go acquisition while a real Python participant owns the flock", func() {
		store, root, _ := checkpointStoreFixture()
		path := filepath.Join(root, ".factory/loops.lock")
		Expect(os.WriteFile(path, nil, 0600)).To(Succeed())
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, "python3", "-B", "-c", `import fcntl,sys
f=open(sys.argv[1],'r+')
fcntl.flock(f,fcntl.LOCK_EX|fcntl.LOCK_NB)
print('ready',flush=True)
sys.stdin.read()`, path) // #nosec G204 G702 -- fixed lock-holding oracle with a test-owned file.
		stdout, err := command.StdoutPipe()
		Expect(err).NotTo(HaveOccurred())
		stdin, err := command.StdinPipe()
		Expect(err).NotTo(HaveOccurred())
		Expect(command.Start()).To(Succeed())
		done := make(chan error, 1)
		go func() { defer close(done); done <- command.Wait() }()
		DeferCleanup(func() {
			Expect(stdin.Close()).To(Succeed())
			cancel()
			Eventually(done, 2*time.Second).Should(BeClosed())
		})
		ready, err := bufio.NewReader(stdout).ReadString('\n')
		Expect(err).NotTo(HaveOccurred())
		Expect(ready).To(Equal("ready\n"))
		started := time.Now()
		transaction, err := store.Lock(context.Background())
		Expect(err).To(HaveOccurred())
		Expect(transaction).To(BeNil())
		Expect(time.Since(started)).To(BeNumerically("<", time.Second))
	})
})

var _ = Describe("Loop checkpoint encoded capacity", func() {
	// per docs/adr/0074-go-loop-checkpoint-storage.md:42
	It("bounds direct string writes from the canonical encoder before allocation grows past32MiB", func() {
		var buffer limitedBuffer
		count, err := buffer.Write(bytes.Repeat([]byte(" "), 32*1024*1024-100))
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(32*1024*1024 - 100))
		err = jsonvalue.EncodePython(context.Background(), &buffer, json.Number(strings.Repeat("1", 4300)))
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("Loop checkpoint read replacement qualification", func() {
	// per docs/adr/0074-go-loop-checkpoint-storage.md:74
	DescribeTable("retries only bounded safe readonly replacements", func(kind string, wantCalls int, success bool) {
		store, root, _ := checkpointStoreFixture()
		path := filepath.Join(root, ".factory/loops.json")
		if kind == "parse" {
			Expect(os.WriteFile(path, []byte("PRIVATE_BROKEN"), 0600)).To(Succeed())
		}
		calls := 0
		store.ops.openHistory = func(directory *os.Root) (*os.File, error) {
			calls++
			if kind != "parse" && (kind != "once" || calls == 1) {
				replacement := filepath.Join(root, ".factory", fmt.Sprintf("replacement-%d", calls))
				if err := os.WriteFile(replacement, []byte(`{"schema":1,"runs":[],"fresh":true}`), 0600); err != nil {
					return nil, err
				}
				if err := os.Rename(replacement, path); err != nil {
					return nil, err
				}
			}
			return directory.OpenFile("loops.json", os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		}
		var err error
		if kind == "locked" {
			transaction, lockErr := store.Lock(context.Background())
			Expect(lockErr).NotTo(HaveOccurred())
			DeferCleanup(func() { Expect(transaction.Close()).To(Succeed()) })
			_, err = transaction.Read(context.Background())
		} else {
			_, err = store.Read(context.Background())
		}
		if success {
			Expect(err).NotTo(HaveOccurred())
		} else {
			Expect(err).To(HaveOccurred())
		}
		Expect(calls).To(Equal(wantCalls))
	}, Entry("one safe replacement then success", "once", 2, true), Entry("eight replacement attempts exhausted", "repeat", 8, false), Entry("parse error has no retry", "parse", 1, false), Entry("locked read has no retry", "locked", 1, false))
})
