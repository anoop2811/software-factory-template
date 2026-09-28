package acceptance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Adversarial review large payload boundary", func() {
	// per docs/DECISION_LOG.md:2478
	DescribeTable("carries the complete diff without payload-sized argv and removes private temporaries", func(provider string, goClient bool, transportFailure bool) {
		base, err := os.MkdirTemp("", "factory-review-payload-")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(os.RemoveAll(base)).To(Succeed()) })
		script, err := os.ReadFile("../scripts/adversarial-review.sh")
		Expect(err).NotTo(HaveOccurred())
		configuration, err := os.ReadFile("../scripts/lib/config.sh")
		Expect(err).NotTo(HaveOccurred())
		writeFixture(filepath.Join(base, "template/scripts/adversarial-review.sh"), script, 0700)
		writeFixture(filepath.Join(base, "template/scripts/lib/config.sh"), configuration, 0600)
		writeFixture(filepath.Join(base, "factory.yaml"), []byte("model_provider: openrouter\n"), 0600)
		temporary := filepath.Join(base, "private-temp")
		Expect(os.Mkdir(temporary, 0700)).To(Succeed())
		writeFixture(filepath.Join(temporary, "unrelated"), []byte("preserve this sibling"), 0600)
		diff := strings.Repeat("+ payload \"quoted\" \\slash\tline\n", 6200) + "+ FINAL_FULL_DIFF_SENTINEL"
		Expect(len(diff)).To(BeNumerically(">", 128<<10))
		Expect(len(diff)).To(BeNumerically("<", 200000))
		diffPath := filepath.Join(base, "diff.patch")
		writeFixture(diffPath, []byte(diff), 0600)
		jq, err := exec.LookPath("jq")
		Expect(err).NotTo(HaveOccurred())
		guard := `#!/bin/bash
set -euo pipefail
for arg in "$@"; do
 if [ "${#arg}" -gt 131071 ]; then
  printf '%s\n' 'fixture detected payload-sized argv' >&2
  exit 90
 fi
done
exec "$FIXTURE_REAL_JQ" "$@"
`
		writeFixture(filepath.Join(base, "bin/jq"), []byte(guard), 0700)
		python, pythonErr := exec.Command("python3", "-B", "-c", "import sys; print(sys.executable)").Output()
		Expect(pythonErr).NotTo(HaveOccurred())
		transport := "#!" + strings.TrimSpace(string(python)) + `
import json,os,stat,sys
args=sys.argv[1:]
if any(len(arg.encode())>131071 for arg in args):
 sys.stderr.write('fixture detected payload-sized argv\n');sys.exit(90)
with open(os.environ['FIXTURE_ARGUMENTS'],'w') as f: json.dump(args,f)
modes={}
for root,dirs,files in os.walk(os.environ['TMPDIR']):
 for name in dirs+files:
  path=os.path.join(root,name)
  relative=os.path.relpath(path,os.environ['TMPDIR'])
  if relative=='unrelated': continue
  info=os.lstat(path)
  modes[relative]={'mode':stat.S_IMODE(info.st_mode),'directory':stat.S_ISDIR(info.st_mode)}
with open(os.environ['FIXTURE_MODES'],'w') as f: json.dump(modes,f)
selected=os.path.basename(sys.argv[0])=='review-client'
with open(os.environ['FIXTURE_CALLS'],'a') as f: f.write('go\n' if selected else 'curl\n')
body=None
output=None
if selected:
 body=sys.stdin.buffer.read()
else:
 for i,arg in enumerate(args):
  if arg in ['-d','--data','--data-binary']:
   value=args[i+1]
   if value.startswith('@'):
    with open(value[1:],'rb') as f: body=f.read()
   else: body=value.encode()
  if arg in ['-o','--output']: output=args[i+1]
if body is None: sys.exit(91)
with open(os.environ['FIXTURE_BODY'],'wb') as f: f.write(body)
if os.environ.get('FIXTURE_FAIL')=='1': sys.exit(73)
if selected:
 print('No findings.')
else:
 response={'content':[{'text':'No findings.'}]} if os.environ['MODEL_PROVIDER']=='anthropic' else {'choices':[{'finish_reason':'stop','message':{'content':'No findings.'}}]}
 with open(output,'w') as f: json.dump(response,f)
 sys.stdout.write('200\t0.1\t0.05\t100')
`
		writeFixture(filepath.Join(base, "bin/curl"), []byte(transport), 0700)
		writeFixture(filepath.Join(base, "bin/review-client"), []byte(transport), 0700)
		environment := []string{"PATH=" + filepath.Join(base, "bin") + string(os.PathListSeparator) + os.Getenv("PATH"), "FACTORY_CONFIG=" + filepath.Join(base, "factory.yaml"), "MODEL_PROVIDER=" + provider, "REVIEW_API_KEY=fixture-not-a-secret", "REVIEW_MODEL=fixture-model", "REVIEW_REASONING_EFFORT=low", "REVIEW_OPENROUTER_PROVIDER=deepinfra", "REVIEW_TIMEOUT_SECONDS=240", "FIXTURE_REAL_JQ=" + jq, "FIXTURE_ARGUMENTS=" + filepath.Join(base, "arguments.json"), "FIXTURE_MODES=" + filepath.Join(base, "modes.json"), "FIXTURE_BODY=" + filepath.Join(base, "body.json"), "FIXTURE_CALLS=" + filepath.Join(base, "calls"), "TMPDIR=" + temporary, "PYTHONDONTWRITEBYTECODE=1"}
		if goClient {
			environment = append(environment, "REVIEW_GO_CLIENT="+filepath.Join(base, "bin/review-client"))
		}
		if transportFailure {
			environment = append(environment, "FIXTURE_FAIL=1")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, "bash", filepath.Join(base, "template/scripts/adversarial-review.sh"), diffPath) // #nosec G204 -- actual repository script in isolated fixture, literal diff path, fake HTTP only.
		command.Env = environment
		command.Dir = base
		var stdout, stderr bytes.Buffer
		command.Stdout = &stdout
		command.Stderr = &stderr
		runErr := command.Run()
		Expect(ctx.Err()).NotTo(HaveOccurred())
		if transportFailure {
			Expect(runErr).To(HaveOccurred())
			var exit *exec.ExitError
			Expect(errors.As(runErr, &exit)).To(BeTrue())
			Expect(stdout.String()).To(BeEmpty())
		} else {
			Expect(runErr).NotTo(HaveOccurred(), "%s", stderr.String())
			Expect(stdout.String()).To(Equal("No findings.\n"))
			Expect(stderr.String()).To(BeEmpty())
		}
		calls, callErr := os.ReadFile(filepath.Join(base, "calls"))
		Expect(callErr).NotTo(HaveOccurred())
		expectedCall := "curl\n"
		if goClient {
			expectedCall = "go\n"
		}
		Expect(string(calls)).To(Equal(expectedCall), "one selected transport; no retry or fallback")
		body, err := os.ReadFile(filepath.Join(base, "body.json"))
		Expect(err).NotTo(HaveOccurred())
		var request map[string]any
		Expect(json.Unmarshal(body, &request)).To(Succeed())
		Expect(request["model"]).To(Equal("fixture-model"))
		messages := request["messages"].([]any)
		userIndex := 1
		switch provider {
		case "anthropic":
			userIndex = 0
			Expect(request["max_tokens"]).To(Equal(float64(4096)))
		case "openrouter":
			Expect(request["max_tokens"]).To(Equal(float64(8192)))
			Expect(request["reasoning"]).To(Equal(map[string]any{"effort": "low"}))
			Expect(request["provider"]).To(Equal(map[string]any{"order": []any{"deepinfra"}, "allow_fallbacks": false, "require_parameters": true}))
		default:
			Expect(request).NotTo(HaveKey("max_tokens"))
		}
		Expect(messages[userIndex].(map[string]any)["content"]).To(Equal("Review this diff.\n\n```diff\n" + diff + "\n```"))
		rawModes, err := os.ReadFile(filepath.Join(base, "modes.json"))
		Expect(err).NotTo(HaveOccurred())
		var modes map[string]struct {
			Mode      uint32 `json:"mode"`
			Directory bool   `json:"directory"`
		}
		Expect(json.Unmarshal(rawModes, &modes)).To(Succeed())
		Expect(modes).NotTo(BeEmpty())
		for name, info := range modes {
			expected := uint32(0600)
			if info.Directory {
				expected = 0700
			}
			Expect(info.Mode).To(Equal(expected), "private temporary %s", name)
		}
		entries, err := os.ReadDir(temporary)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(1))
		Expect(entries[0].Name()).To(Equal("unrelated"))
		preserved, err := os.ReadFile(filepath.Join(temporary, "unrelated"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(preserved)).To(Equal("preserve this sibling"))
	}, Entry("Anthropic curl", "anthropic", false, false), Entry("OpenAI curl", "openai", false, false), Entry("OpenRouter curl", "openrouter", false, false), Entry("selected Go client", "openrouter", true, false), Entry("curl failure cleanup", "openrouter", false, true), Entry("Go client failure cleanup", "openrouter", true, true))
})
