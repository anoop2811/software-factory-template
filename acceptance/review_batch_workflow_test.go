package acceptance_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.yaml.in/yaml/v3"
)

type ciReviewWorkflow struct {
	On          map[string]any    `yaml:"on"`
	Permissions map[string]string `yaml:"permissions"`
	Env         map[string]string `yaml:"env"`
	Jobs        map[string]struct {
		If    string            `yaml:"if"`
		Env   map[string]string `yaml:"env"`
		Steps []ciReviewStep    `yaml:"steps"`
	} `yaml:"jobs"`
}

type ciReviewStep struct {
	Name string            `yaml:"name"`
	ID   string            `yaml:"id"`
	Uses string            `yaml:"uses"`
	If   string            `yaml:"if"`
	Run  string            `yaml:"run"`
	Env  map[string]string `yaml:"env"`
	With map[string]any    `yaml:"with"`
}

func ciReviewLoadWorkflow(path string) ciReviewWorkflow {
	GinkgoHelper()
	data, err := os.ReadFile(filepath.Join("..", path))
	Expect(err).NotTo(HaveOccurred())
	var workflow ciReviewWorkflow
	Expect(yaml.Unmarshal(data, &workflow)).To(Succeed())
	return workflow
}

func ciReviewNativeStep(workflow ciReviewWorkflow) ciReviewStep {
	GinkgoHelper()
	for _, job := range workflow.Jobs {
		for _, step := range job.Steps {
			if step.ID == "review" {
				return step
			}
		}
	}
	Fail("missing independently identifiable native Review step")
	return ciReviewStep{}
}

func ciReviewWorkflowEntries() []TableEntry {
	return []TableEntry{Entry("repository", ".github/workflows/adversarial-review.yml"), Entry("generated adopter", "packs/review-lane/review-pr.yml")}
}

var _ = Describe("Budgeted review workflow security and publication", func() {
	// per docs/adr/0100-budgeted-batched-adversarial-review.md:83
	// per docs/adr/0100-budgeted-batched-adversarial-review.md:117
	DescribeTable("keeps executable checkout trusted and scopes credentials to review steps", func(path string) {
		workflow := ciReviewLoadWorkflow(path)
		Expect(workflow.On).To(HaveKey("pull_request_target"))
		Expect(workflow.On).To(HaveKey("workflow_dispatch"))
		Expect(workflow.Permissions).To(HaveKeyWithValue("contents", "read"))
		Expect(workflow.Permissions).To(HaveKeyWithValue("pull-requests", "write"))
		Expect(workflow.Permissions).To(HaveKeyWithValue("checks", "write"))
		for _, value := range workflow.Env {
			Expect(value).NotTo(ContainSubstring("secrets."))
		}
		checkouts := 0
		for _, job := range workflow.Jobs {
			Expect(job.If).To(And(ContainSubstring("head.repo.full_name == github.repository"), ContainSubstring("base.ref == github.event.repository.default_branch"), ContainSubstring("workflow_dispatch"), ContainSubstring("github.ref")))
			for _, value := range job.Env {
				Expect(value).NotTo(ContainSubstring("secrets."))
			}
			for _, step := range job.Steps {
				if strings.HasPrefix(step.Uses, "actions/checkout@") {
					checkouts++
					ref, ok := step.With["ref"].(string)
					Expect(ok).To(BeTrue())
					Expect(ref).To(And(ContainSubstring("github.event.pull_request.base.sha"), ContainSubstring("github.sha")))
					Expect(ref).NotTo(ContainSubstring("head"))
					Expect(step.With["persist-credentials"]).To(BeFalse())
				}
				if step.ID != "review" && !strings.Contains(strings.ToLower(step.Name), "review legacy") {
					for _, value := range step.Env {
						Expect(value).NotTo(ContainSubstring("secrets."), "secret unexpectedly exposed to %s", step.Name)
					}
				}
			}
		}
		Expect(checkouts).To(Equal(1))
	}, ciReviewWorkflowEntries())
	// per docs/adr/0100-budgeted-batched-adversarial-review.md:103
	// per docs/adr/0100-budgeted-batched-adversarial-review.md:112
	DescribeTable("passes revision and artifact arguments literally to the shared native adapter", func(path string) {
		step := ciReviewNativeStep(ciReviewLoadWorkflow(path))
		for _, exitCode := range []string{"0", "37"} {
			temp, err := os.MkdirTemp("", "review workflow with spaces ")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(os.RemoveAll, temp)
			client := filepath.Join(temp, "literal client")
			argsFile := filepath.Join(temp, "literal-args")
			bridgeFile := filepath.Join(temp, "bridge-env")
			writeFixture(client, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$RECORD_ARGS\"\nprintf '%s' \"${FACTORY_BRIDGE_PROTOCOL-unset}\" > \"$RECORD_BRIDGE\"\nexit \"$RECORD_EXIT\"\n"), 0700)
			cmd := exec.Command("bash", "-e", "-c", step.Run) // #nosec G204 G702 -- reviewed workflow step under an inert test-owned recording client; no model or PR content executes.
			cmd.Dir = temp
			cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "PR=7", "REPO=fixture/project", "EXPECTED_BASE=" + ciReviewBase, "EXPECTED_HEAD=" + ciReviewHead, "RUNNER_TEMP=" + temp, "REVIEW_GO_CLIENT=" + client, "RECORD_ARGS=" + argsFile, "RECORD_BRIDGE=" + bridgeFile, "RECORD_EXIT=" + exitCode, "FACTORY_BRIDGE_PROTOCOL=1"}
			Expect(cmd.Start()).To(Succeed())
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			var runErr error
			select {
			case runErr = <-done:
			case <-time.After(3 * time.Second):
				_ = cmd.Process.Kill()
				<-done
				Fail("workflow adapter invocation exceeded fixture watchdog")
			}
			if exitCode == "0" {
				Expect(runErr).NotTo(HaveOccurred())
			} else {
				var exit *exec.ExitError
				Expect(errors.As(runErr, &exit)).To(BeTrue())
				Expect(exit.ExitCode()).To(Equal(37))
			}
			data, err := os.ReadFile(argsFile)
			Expect(err).NotTo(HaveOccurred())
			args := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
			Expect(args[:2]).To(Equal([]string{"review-batch", "github"}))
			values := map[string]string{}
			Expect(len(args) % 2).To(BeZero())
			for i := 2; i < len(args); i += 2 {
				values[args[i]] = args[i+1]
			}
			Expect(values).To(Equal(map[string]string{"--pr": "7", "--repository": "fixture/project", "--expected-base": ciReviewBase, "--expected-head": ciReviewHead, "--output": filepath.Join(temp, "factory-review-output"), "--cache": filepath.Join(temp, "factory-review-cache")}))
			bridge, err := os.ReadFile(bridgeFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(bridge)).To(BeEmpty(), "public batch adapter must not inherit the legacy bridge protocol")
		}
	}, ciReviewWorkflowEntries())
	// per docs/adr/0100-budgeted-batched-adversarial-review.md:85
	DescribeTable("retains findings on failure and restores only dedicated completed-batch cache artifacts", func(path string) {
		workflow := ciReviewLoadWorkflow(path)
		artifacts, restores, saves := 0, 0, 0
		for _, job := range workflow.Jobs {
			for _, step := range job.Steps {
				if strings.HasPrefix(step.Uses, "actions/upload-artifact@") {
					artifacts++
					Expect(step.If).To(ContainSubstring("always()"))
					paths, ok := step.With["path"].(string)
					Expect(ok).To(BeTrue())
					Expect(paths).To(And(ContainSubstring("report.json"), ContainSubstring("review.md")))
				}
				if strings.HasPrefix(step.Uses, "actions/cache/restore@") {
					restores++
					Expect(step.With["path"]).To(ContainSubstring("factory-review-cache"))
					Expect(step.With["key"]).To(ContainSubstring("factory-review-v1"))
				}
				if strings.HasPrefix(step.Uses, "actions/cache/save@") {
					saves++
					Expect(step.If).To(ContainSubstring("always()"))
					Expect(step.With["path"]).To(ContainSubstring("factory-review-cache"))
				}
			}
		}
		Expect(artifacts).To(Equal(1))
		Expect(restores).To(Equal(1))
		Expect(saves).To(Equal(1))
	}, ciReviewWorkflowEntries())
})
