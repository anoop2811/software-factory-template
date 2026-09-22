package budget

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	ginkgo "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestBudgetStorage(t *testing.T) {
	RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Budget storage collaborator acceptance")
}
func storageFixture() (*Ledger, Config, Request) {
	ginkgo.GinkgoHelper()
	root, err := os.MkdirTemp("", "factory-budget-storage-")
	Expect(err).NotTo(HaveOccurred())
	ginkgo.DeferCleanup(os.RemoveAll, root)
	Expect(os.Mkdir(filepath.Join(root, ".factory"), 0700)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(root, ".factory/budget.json"), []byte("{\"schema\":1,\"runs\":[]}\n"), 0600)).To(Succeed())
	cfg, err := Configuration(map[string]string{"FACTORY_BUDGET_ENABLED": "true"})
	Expect(err).NotTo(HaveOccurred())
	return NewLedger(root), cfg, Request{Session: "s", Task: "t", Harness: "claude", Role: "reviewer"}
}

var _ = ginkgo.Describe("Budget publication failures", func() {
	// per docs/adr/0069-go-budget-ledger-admission.md:86
	ginkgo.DescribeTable("distinguishes before and after rename without silently retrying", func(boundary string, committed bool) {
		ledger, cfg, request := storageFixture()
		historyPath := filepath.Join(ledger.root, ".factory/budget.json")
		before, err := os.ReadFile(historyPath)
		Expect(err).NotTo(HaveOccurred())
		calls := 0
		fail := func(*os.File) error { calls++; return errors.New("PRIVATE_STORAGE_ERROR") }
		if boundary == "file sync" {
			ledger.ops.syncFile = fail
		} else {
			ledger.ops.syncDirectory = fail
		}
		_, err = ledger.Admit(context.Background(), request, cfg)
		var publication *PublicationError
		Expect(errors.As(err, &publication)).To(BeTrue())
		Expect(publication.MayHaveCommitted).To(Equal(committed))
		Expect(publication.RunID).NotTo(BeEmpty())
		Expect(err.Error()).NotTo(ContainSubstring("PRIVATE"))
		Expect(calls).To(Equal(1))
		after, readErr := os.ReadFile(historyPath)
		Expect(readErr).NotTo(HaveOccurred())
		if !committed {
			Expect(after).To(Equal(before))
		} else {
			var history map[string]any
			Expect(json.Unmarshal(after, &history)).To(Succeed())
			rows := history["runs"].([]any)
			Expect(rows).To(HaveLen(1))
			Expect(rows[0].(map[string]any)["id"]).To(Equal(publication.RunID))
		}
		files, readErr := os.ReadDir(filepath.Join(ledger.root, ".factory"))
		Expect(readErr).NotTo(HaveOccurred())
		for _, file := range files {
			Expect(file.Name()).NotTo(HaveSuffix(".tmp"))
		}
	}, ginkgo.Entry("before rename", "file sync", false), ginkgo.Entry("after rename", "directory sync", true))
	// per docs/adr/0069-go-budget-ledger-admission.md:89
	ginkgo.It("preserves a replacement at the temporary pathname during failure cleanup", func() {
		ledger, cfg, request := storageFixture()
		sentinel := ""
		held := filepath.Join(ledger.root, "held-original-temp")
		ledger.ops.syncFile = func(*os.File) error {
			entries, err := os.ReadDir(filepath.Join(ledger.root, ".factory"))
			if err != nil {
				return err
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), "budget-") && strings.HasSuffix(entry.Name(), ".tmp") {
					sentinel = filepath.Join(ledger.root, ".factory", entry.Name())
					if err := os.Rename(sentinel, held); err != nil {
						return err
					}
					if err := os.WriteFile(sentinel, []byte("replacement-owned-by-fixture"), 0600); err != nil {
						return err
					}
					return errors.New("test-owned prepublication failure")
				}
			}
			return errors.New("test fixture did not find temporary file")
		}
		_, err := ledger.Admit(context.Background(), request, cfg)
		Expect(err).To(HaveOccurred())
		Expect(sentinel).NotTo(BeEmpty())
		data, err := os.ReadFile(sentinel)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(Equal("replacement-owned-by-fixture"))
		_, err = os.Stat(held)
		Expect(err).NotTo(HaveOccurred())
	})
	// per docs/adr/0069-go-budget-ledger-admission.md:60
	ginkgo.It("holds the same flock that excludes a real Python participant", func() {
		ledger, cfg, request := storageFixture()
		python, err := exec.LookPath("python3")
		Expect(err).NotTo(HaveOccurred())
		observed := ""
		ledger.ops.syncFile = func(file *os.File) error {
			cmd := exec.Command(python, "-I", "-B", "-c", `import fcntl,sys
f=open(sys.argv[1],"r+")
try:fcntl.flock(f,fcntl.LOCK_EX|fcntl.LOCK_NB);print("UNLOCKED")
except BlockingIOError:print("LOCKED")`, filepath.Join(ledger.root, ".factory/budget.lock")) // #nosec G204 G702 -- test-owned Python oracle and fixed script, only fixture pathname is passed.
			cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
			out, err := cmd.CombinedOutput()
			observed = string(out)
			if err != nil {
				return err
			}
			return file.Sync()
		}
		admitted, err := ledger.Admit(context.Background(), request, cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(admitted.Record).NotTo(BeNil())
		Expect(observed).To(Equal("LOCKED\n"))
	})
})

var _ = ginkgo.Describe("Budget ambiguous process publication", func() {
	// per docs/adr/0069-go-budget-ledger-admission.md:90
	ginkgo.It("retains run and process identity when PID publication may have committed", func() {
		ledger, cfg, request := storageFixture()
		admitted, err := ledger.Admit(context.Background(), request, cfg)
		Expect(err).NotTo(HaveOccurred())
		raw, err := json.Marshal(admitted.Record)
		Expect(err).NotTo(HaveOccurred())
		var row map[string]any
		Expect(json.Unmarshal(raw, &row)).To(Succeed())
		id := row["id"].(string)
		ledger.ops.syncDirectory = func(*os.File) error { return errors.New("PRIVATE_DIRECTORY_SYNC") }
		_, err = ledger.PublishPID(context.Background(), id, 1234)
		var publication *PublicationError
		Expect(errors.As(err, &publication)).To(BeTrue())
		Expect(publication.RunID).To(Equal(id))
		Expect(publication.ProcessPID).NotTo(BeNil())
		Expect(*publication.ProcessPID).To(Equal(1234))
		Expect(publication.MayHaveCommitted).To(BeTrue())
		history, err := ledger.Read(context.Background())
		Expect(err).NotTo(HaveOccurred())
		raw, err = json.Marshal(history)
		Expect(err).NotTo(HaveOccurred())
		var stored map[string]any
		Expect(json.Unmarshal(raw, &stored)).To(Succeed())
		Expect(stored["runs"].([]any)[0].(map[string]any)["process_pid"]).To(Equal(float64(1234)))
	})
})
