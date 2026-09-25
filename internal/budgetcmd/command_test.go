package budgetcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/anoop2811/software-factory-template/internal/budget"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestBudgetCommand(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Budget command output acceptance")
}

func commandFixture(stall bool) (map[string]string, []string, string) {
	GinkgoHelper()
	root, err := os.MkdirTemp("", "factory-budgetcmd-")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(os.RemoveAll, root)
	put := func(name, contents string, mode os.FileMode) {
		Expect(os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0700)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(root, name), []byte(contents), mode)).To(Succeed())
	}
	put(".opencode/agent/reviewer.md", "instructions\n", 0600)
	put(".claude/agents/reviewer.md", "instructions\n", 0600)
	put("opencode.json", `{"agent":{"reviewer":{"permission":{"edit":"deny"}}}}`, 0600)
	put("prompt", "PRIVATE_PROMPT\n", 0600)
	script := "#!/bin/sh\nfor arg do\nif [ \"$arg\" = --help ]; then printf '%s\\n' '--print --output-format --agent --permission-mode --model'; exit 0; fi\ndone\nIFS= read -r prompt\nprintf 'ready\\n' >> \"$BUDGET_TEST_READY\"\n"
	if stall {
		script += "exec /bin/sleep 30\n"
	} else {
		script += "printf '%s\\n' '{\"type\":\"result\",\"subtype\":\"success\",\"result\":\"PRIVATE_ANSWER\",\"usage\":{\"input_tokens\":3,\"output_tokens\":2,\"cache_creation_input_tokens\":0,\"cache_read_input_tokens\":0},\"total_cost_usd\":0.1}'\n"
	}
	put("bin/claude", script, 0700)
	GinkgoT().Setenv("PATH", filepath.Join(root, "bin"))
	GinkgoT().Setenv("BUDGET_TEST_READY", filepath.Join(root, "ready"))
	env := map[string]string{"FACTORY_BUDGET_ROOT": root, "FACTORY_BUDGET_ENABLED": "true"}
	args := []string{"run", "--session=s", "--task=t", "--harness=claude", "--role=reviewer", "--prompt-file=" + filepath.Join(root, "prompt")}
	return env, args, root
}

var _ = Describe("Budget command final output", func() {
	// per docs/adr/0071-go-budget-command-candidate.md:38
	It("emits durable interrupted metadata and status130 after parent cancellation", func() {
		env, args, root := commandFixture(true)
		args = append(args, "--json")
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		done := make(chan struct{})
		ready := false
		go func() {
			defer close(done)
			for {
				if _, err := os.Stat(filepath.Join(root, "ready")); err == nil {
					ready = true
					cancel()
					return
				}
				select {
				case <-ctx.Done():
					return
				case <-time.After(5 * time.Millisecond):
				}
			}
		}()
		defer func() { cancel(); <-done }()
		var stdout, stderr bytes.Buffer
		status := Run(ctx, args, env, &stdout, &stderr)
		<-done
		Expect(ready).To(BeTrue())
		Expect(status).To(Equal(130))
		Expect(stderr.String()).To(BeEmpty())
		lines := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
		Expect(lines).To(HaveLen(2))
		var row map[string]any
		Expect(json.Unmarshal([]byte(lines[1]), &row)).To(Succeed())
		Expect(row["outcome"]).To(Equal("interrupted"))
		Expect(row["status"]).To(Equal("completed"))
		Expect(row["tokens"]).To(BeNil())
		history, err := budget.NewLedger(root).Read(context.Background())
		Expect(err).NotTo(HaveOccurred())
		raw, err := json.Marshal(history)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(raw)).To(ContainSubstring(`"outcome":"interrupted"`))
		Expect(string(raw)).NotTo(ContainSubstring("PRIVATE_"))
	})
})

type failingOutput struct {
	bytes.Buffer
	failAt, writes, flushes int
	mode                    string
}

func (w *failingOutput) Write(data []byte) (int, error) {
	w.writes++
	if w.writes == w.failAt && w.mode == "write" {
		return 0, errors.New("PRIVATE_WRITE")
	}
	if w.writes == w.failAt && w.mode == "short" {
		return len(data) - 1, nil
	}
	return w.Buffer.Write(data)
}
func (w *failingOutput) Flush() error {
	w.flushes++
	if w.flushes == w.failAt && w.mode == "flush" {
		return errors.New("PRIVATE_FLUSH")
	}
	return nil
}

var _ = Describe("Budget command writer failure", func() {
	// per docs/adr/0071-go-budget-command-candidate.md:73
	DescribeTable("never retries a model call or rolls back completed accounting", func(event int, mode string) {
		env, args, root := commandFixture(false)
		writer := &failingOutput{failAt: event, mode: mode}
		var stderr bytes.Buffer
		status := Run(context.Background(), args, env, writer, &stderr)
		Expect(status).NotTo(BeZero())
		Expect(stderr.String()).To(HavePrefix("factory budget: "))
		Expect(stderr.String()).NotTo(ContainSubstring("PRIVATE"))
		history, err := budget.NewLedger(root).Read(context.Background())
		Expect(err).NotTo(HaveOccurred())
		raw, err := json.Marshal(history)
		Expect(err).NotTo(HaveOccurred())
		var data map[string]any
		Expect(json.Unmarshal(raw, &data)).To(Succeed())
		rows := data["runs"].([]any)
		if event == 1 {
			Expect(rows).To(BeEmpty())
			_, err := os.Stat(filepath.Join(root, "ready"))
			Expect(os.IsNotExist(err)).To(BeTrue())
		} else {
			Expect(rows).To(HaveLen(1))
			calls, err := os.ReadFile(filepath.Join(root, "ready"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(calls)).To(Equal("ready\n"))
			Expect(rows[0].(map[string]any)["outcome"]).To(Equal("completed"))
			Expect(rows[0].(map[string]any)["status"]).To(Equal("completed"))
			Expect(writer.writes).To(Equal(event))
		}
	}, Entry("plan write", 1, "write"), Entry("plan short", 1, "short"), Entry("plan flush", 1, "flush"), Entry("record write", 2, "write"), Entry("record short", 2, "short"), Entry("record flush", 2, "flush"), Entry("answer write", 3, "write"), Entry("answer short", 3, "short"), Entry("answer flush", 3, "flush"))
})

var _ = Describe("Budget argument help writer failures", func() {
	// per docs/adr/0072-go-budget-argument-compatibility.md:62
	DescribeTable("checks help writes and flushes without touching execution state", func(mode string, leaf bool) {
		env, _, root := commandFixture(false)
		env["FACTORY_BUDGET_ENABLED"] = "INVALID"
		Expect(os.MkdirAll(filepath.Join(root, ".factory"), 0700)).To(Succeed())
		path := filepath.Join(root, ".factory/budget.json")
		before := []byte("PRIVATE_INVALID_HISTORY")
		Expect(os.WriteFile(path, before, 0600)).To(Succeed())
		args := []string{"--help"}
		if leaf {
			args = []string{"run", "--help"}
		}
		writer := &failingOutput{mode: mode, failAt: 1}
		var stderr bytes.Buffer
		status := Run(context.Background(), args, env, writer, &stderr)
		if mode == "ok" {
			Expect(status).To(BeZero())
			Expect(stderr.String()).To(BeEmpty())
			Expect(strings.ToLower(writer.String())).To(ContainSubstring("usage:"))
		} else {
			Expect(status).NotTo(BeZero())
			Expect(stderr.String()).NotTo(BeEmpty())
			Expect(stderr.Len()).To(BeNumerically("<", 1024))
			Expect(stderr.String()).NotTo(ContainSubstring("PRIVATE"))
		}
		Expect(writer.writes).To(Equal(1))
		if mode == "ok" || mode == "flush" {
			Expect(writer.flushes).To(Equal(1))
		} else {
			Expect(writer.flushes).To(BeZero())
		}
		after, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(after).To(Equal(before))
		for _, name := range []string{"ready", ".factory/budget.lock"} {
			_, err = os.Stat(filepath.Join(root, name))
			Expect(os.IsNotExist(err)).To(BeTrue())
		}
	}, Entry("root short write", "short", false), Entry("root write failure", "write", false), Entry("leaf flush failure", "flush", true), Entry("leaf success", "ok", true))
})
