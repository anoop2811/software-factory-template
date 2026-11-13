package acceptance_test

import (
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	. "github.com/onsi/gomega"
)

const preparedQualificationDirectory = ".factory/qualification/prepared-v1"
const preparedQualificationReceiptPath = preparedQualificationDirectory + "/receipt.json"
const preparedQualificationSuitePath = "acceptance/qualification.test"
const preparedQualificationRunnerPath = preparedQualificationDirectory + "/ginkgo"

// The evaluator creates a real committed module, not a recording compiler.
// per docs/adr/0151-reliable-ci-qualification.md:55
func preparedQualificationFixture(base, name string) (string, string) {
	GinkgoHelper()
	root := filepath.Join(base, name)
	Expect(os.MkdirAll(filepath.Join(root, "acceptance"), 0700)).To(Succeed())
	for _, name := range []string{"go.mod", "go.sum"} {
		data, err := os.ReadFile(filepath.Join("..", name))
		Expect(err).NotTo(HaveOccurred())
		writeFixture(filepath.Join(root, name), data, 0600)
	}
	writeFixture(filepath.Join(root, ".gitignore"), []byte(".factory/\n*.test\n"), 0600)
	writeFixture(filepath.Join(root, "acceptance/fixture_suite_test.go"), []byte(`package fixture_test
import (
 "os"
 "path/filepath"
 "testing"
 . "github.com/onsi/ginkgo/v2"
 . "github.com/onsi/gomega"
)
func init(){if path:=os.Getenv("PREPARED_PROGRAM_WITNESS");path!=""{if err:=os.WriteFile(path,[]byte("executed\n"),0600);err!=nil{panic(err)}}}
func TestPreparedFixture(t *testing.T){RegisterFailHandler(Fail);RunSpecs(t,"Prepared qualification fixture")}
// per docs/adr/0151-reliable-ci-qualification.md:49
var _ = Describe("Prepared fixture",func(){
 It("runs in its actual acceptance directory",func(){cwd,err:=os.Getwd();Expect(err).NotTo(HaveOccurred());Expect(filepath.Base(cwd)).To(Equal("acceptance"))})
 It("propagates a real failing criterion",func(){Expect(os.Getenv("PREPARED_INTENTIONAL_FAILURE")).To(BeEmpty())})
})
`), 0600)
	imageGit(root, "init", "--quiet", "--template=")
	imageGit(root, "config", "gc.auto", "0")
	imageGit(root, "config", "maintenance.auto", "false")
	imageGit(root, "-c", "core.hooksPath="+os.DevNull, "add", ".")
	imageGit(root, "-c", "core.hooksPath="+os.DevNull, "-c", "user.name=Qualification fixture", "-c", "user.email=qualification@example.invalid", "commit", "--quiet", "-m", "test: independent prepared suite")
	return root, strings.TrimSpace(imageGit(root, "rev-parse", "HEAD"))
}

// Hash the complete owned tree without keeping executable-sized copies in RAM.
// per docs/adr/0151-reliable-ci-qualification.md:59
func preparedQualificationSnapshot(root string) map[string]string {
	GinkgoHelper()
	result := map[string]string{}
	Expect(filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		value := fmt.Sprintf("%v/%d", info.Mode(), info.Size())
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			value += "/" + target
		case info.Mode().IsRegular():
			file, err := os.Open(path) // #nosec G122 -- owned fixture is quiescent: mutations finish before snapshots and verifier completion is awaited; links are recorded without traversal.
			if err != nil {
				return err
			}
			hash := sha256.New()
			_, copyErr := io.Copy(hash, file)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
			value += "/" + hex.EncodeToString(hash.Sum(nil))
		}
		result[relative] = value
		return nil
	})).To(Succeed())
	return result
}

// Literal private CLI arguments bind the actual commit rather than a branch name.
// per docs/adr/0151-reliable-ci-qualification.md:61
func preparedQualificationInvoke(program, root, revision, operation string, environment map[string]string) cliResult {
	GinkgoHelper()
	args := []string{operation, "--root", root, "--revision", revision, "--run-id", "12345"}
	if operation == "prepare" {
		args = append(args, "--producer-attempt", "1")
	}
	return imageProcess(root, environment, program, args...)
}

func preparedQualificationJSON(data []byte) map[string]any {
	GinkgoHelper()
	var result map[string]any
	Expect(json.Unmarshal(data, &result)).To(Succeed())
	return result
}

// Every rejection preserves its exact input, including corrupted cache evidence.
// per docs/adr/0151-reliable-ci-qualification.md:63
func preparedQualificationReject(program, root, revision, witness, name string, mutate func() func()) {
	GinkgoHelper()
	restore := mutate()
	defer restore()
	before := preparedQualificationSnapshot(root)
	out := preparedQualificationInvoke(program, root, revision, "verify", map[string]string{"PREPARED_PROGRAM_WITNESS": witness, "GITHUB_RUN_ATTEMPT": "2"})
	AddReportEntry("prepared-qualification.refusal", map[string]any{"case": name, "status": out.status, "stdout": out.stdout, "stderr": out.stderr}, ReportEntryVisibilityNever)
	Expect(out.status).To(Equal(1), "%s: %+v", name, out)
	Expect(out.stderr).NotTo(BeEmpty(), name)
	Expect(preparedQualificationSnapshot(root)).To(Equal(before), name)
	_, err := os.Lstat(witness)
	Expect(os.IsNotExist(err)).To(BeTrue(), "verify executed cached input in %s", name)
}

var _ = Describe("Prepared Go qualification custody", Ordered, func() {
	var base, program, root, revision, witness string
	var receipt []byte
	BeforeAll(func() {
		var err error
		base, err = os.MkdirTemp("", "factory-prepared-qualification-")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(cleanupFixtureDirectory, base)
		program = filepath.Join(base, "prepare-suite")
		built := imageProcess("..", nil, "go", "build", "-o", program, "./internal/qualification/cmd/prepare-suite")
		Expect(built.status).To(BeZero(), "private CLI compilation is setup, not behavioral RED: %+v", built)
		root, revision = preparedQualificationFixture(base, "checkout")
		witness = filepath.Join(base, "cached-program-was-executed")
	})

	// per docs/adr/0151-reliable-ci-qualification.md:38
	// per docs/adr/0151-reliable-ci-qualification.md:49
	// per docs/adr/0151-reliable-ci-qualification.md:85
	// per docs/adr/0151-reliable-ci-qualification.md:146
	It("prepares real race-instrumented executables and verifies same-run reuse before executing exact reported criteria", func() {
		// per docs/adr/0151-reliable-ci-qualification.md:68
		persisted := filepath.Join(base, "persisted-go-env")
		writeFixture(persisted, []byte("GOFLAGS=-toolexec=/fixture-persisted-compiler-must-not-run\n"), 0600)
		out := preparedQualificationInvoke(program, root, revision, "prepare", map[string]string{"PREPARED_PROGRAM_WITNESS": witness, "GOENV": persisted, "GOFLAGS": "-toolexec=/fixture-inherited-compiler-must-not-run", "GOWORK": filepath.Join(base, "absent.work"), "GOTOOLCHAIN": "invalid", "GOOS": "foreign-fixture", "GOARCH": "foreign-fixture", "CGO_ENABLED": "0"})
		AddReportEntry("prepared-qualification.prepare", map[string]any{"status": out.status, "stdout": out.stdout, "stderr": out.stderr}, ReportEntryVisibilityNever)
		Expect(out.status).To(BeZero(), "%+v", out)
		var err error
		receipt, err = os.ReadFile(filepath.Join(root, preparedQualificationReceiptPath))
		Expect(err).NotTo(HaveOccurred())
		recorded := preparedQualificationJSON(receipt)
		Expect(preparedQualificationJSON([]byte(out.stdout))).To(Equal(recorded))
		module, err := os.ReadFile(filepath.Join(root, "go.mod"))
		Expect(err).NotTo(HaveOccurred())
		match := regexp.MustCompile(`(?m)^\s*github\.com/onsi/ginkgo/v2\s+(v\S+)`).FindSubmatch(module)
		Expect(match).To(HaveLen(2))
		for key, want := range map[string]any{"schema": float64(1), "run_id": "12345", "producer_attempt": "1", "revision": revision, "go_os": runtime.GOOS, "go_arch": runtime.GOARCH, "go_version": runtime.Version(), "ginkgo_version": string(match[1]), "race": true} {
			Expect(recorded).To(HaveKeyWithValue(key, want), key)
		}
		for _, name := range []string{"go.mod", "go.sum"} {
			data, err := os.ReadFile(filepath.Join(root, name))
			Expect(err).NotTo(HaveOccurred())
			hash := sha256.Sum256(data)
			Expect(recorded).To(HaveKeyWithValue(strings.ReplaceAll(name, ".", "_")+"_sha256", hex.EncodeToString(hash[:])))
		}
		for key, path := range map[string]string{"suite": preparedQualificationSuitePath, "ginkgo": preparedQualificationRunnerPath} {
			entry, ok := recorded[key].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(entry["path"]).To(Equal(path))
			data, err := os.ReadFile(filepath.Join(root, path))
			Expect(err).NotTo(HaveOccurred())
			hash := sha256.Sum256(data)
			Expect(entry["size"]).To(Equal(float64(len(data))))
			Expect(entry["sha256"]).To(Equal(hex.EncodeToString(hash[:])))
			metadata, err := buildinfo.ReadFile(filepath.Join(root, path))
			Expect(err).NotTo(HaveOccurred())
			Expect(metadata.GoVersion).To(Equal(runtime.Version()))
			settings := map[string]string{}
			for _, setting := range metadata.Settings {
				settings[setting.Key] = setting.Value
			}
			Expect(settings).To(HaveKeyWithValue("GOOS", runtime.GOOS))
			Expect(settings).To(HaveKeyWithValue("GOARCH", runtime.GOARCH))
			if key == "suite" {
				Expect(settings).To(HaveKeyWithValue("-race", "true"))
			}
		}
		before := preparedQualificationSnapshot(root)
		for _, attempt := range []string{"1", "2"} {
			verified := preparedQualificationInvoke(program, root, revision, "verify", map[string]string{"PREPARED_PROGRAM_WITNESS": witness, "GITHUB_RUN_ATTEMPT": attempt})
			Expect(verified.status).To(BeZero(), "attempt %s: %+v", attempt, verified)
			Expect(preparedQualificationJSON([]byte(verified.stdout))).To(Equal(recorded))
			Expect(preparedQualificationSnapshot(root)).To(Equal(before))
		}
		_, err = os.Lstat(witness)
		Expect(os.IsNotExist(err)).To(BeTrue(), "preparation and verification must not execute the cached suite")
		for _, mode := range []string{"passed", "empty", "failed"} {
			report := filepath.Join(root, ".factory", mode+".json")
			junit := filepath.Join(root, ".factory", mode+".xml")
			args := []string{"--procs=1", "--timeout=30s", "--fail-on-empty", "--json-report=" + report, "--junit-report=" + junit}
			environment := map[string]string{"PREPARED_PROGRAM_WITNESS": witness}
			if mode == "empty" {
				args = append(args, "--focus=does-not-exist")
			}
			if mode == "failed" {
				environment["PREPARED_INTENTIONAL_FAILURE"] = "yes"
			}
			args = append(args, filepath.Join(root, preparedQualificationSuitePath))
			executed := imageProcess(root, environment, filepath.Join(root, preparedQualificationRunnerPath), args...)
			AddReportEntry("prepared-qualification.execute", map[string]any{"mode": mode, "status": executed.status, "stdout": executed.stdout, "stderr": executed.stderr}, ReportEntryVisibilityNever)
			if mode == "passed" {
				Expect(executed.status).To(BeZero(), "%+v", executed)
			} else {
				Expect(executed.status).NotTo(BeZero(), "%+v", executed)
			}
			data, err := os.ReadFile(report)
			Expect(err).NotTo(HaveOccurred())
			var reports []types.Report
			Expect(json.Unmarshal(data, &reports)).To(Succeed())
			Expect(reports).To(HaveLen(1))
			Expect(reports[0].PreRunStats.TotalSpecs).To(Equal(2))
			switch mode {
			case "passed":
				Expect(reports[0].SpecReports.CountWithState(types.SpecStatePassed)).To(Equal(2))
			case "empty":
				Expect(reports[0].PreRunStats.SpecsThatWillRun).To(BeZero())
			case "failed":
				Expect(reports[0].SpecReports.CountWithState(types.SpecStateFailed)).To(Equal(1))
			}
			info, err := os.Stat(junit)
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Size()).To(BeNumerically(">", 0))
		}
		Expect(os.Remove(witness)).To(Succeed())
	})

	// per docs/adr/0151-reliable-ci-qualification.md:59
	// per docs/adr/0151-reliable-ci-qualification.md:63
	It("refuses missing malformed relocated and unsafe cached inputs without execution or repair", func() {
		receiptPath := filepath.Join(root, preparedQualificationReceiptPath)
		for _, test := range []struct {
			name string
			body []byte
		}{{"missing", nil}, {"malformed", []byte("{")}, {"trailing", append(append([]byte{}, receipt...), []byte("\n{}")...)}, {"unknown field", []byte(strings.Replace(string(receipt), `"schema":`, `"unknown":true,"schema":`, 1))}} {
			preparedQualificationReject(program, root, revision, witness, test.name, func() func() {
				if test.body == nil {
					Expect(os.Remove(receiptPath)).To(Succeed())
				} else {
					writeFixture(receiptPath, test.body, 0600)
				}
				return func() { writeFixture(receiptPath, receipt, 0600) }
			})
		}
		for _, relative := range []string{preparedQualificationSuitePath, preparedQualificationRunnerPath, preparedQualificationReceiptPath} {
			path := filepath.Join(root, relative)
			original, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			info, err := os.Stat(path)
			Expect(err).NotTo(HaveOccurred())
			for _, mode := range []string{"missing", "bytes", "symlink", "directory", "writable", "not-executable", "owner-not-executable"} {
				if relative == preparedQualificationReceiptPath && (mode == "not-executable" || mode == "owner-not-executable") {
					continue
				}
				preparedQualificationReject(program, root, revision, witness, relative+"/"+mode, func() func() {
					Expect(os.Remove(path)).To(Succeed())
					switch mode {
					case "missing":
					case "bytes":
						writeFixture(path, []byte("#!/bin/sh\nprintf executed > \"$PREPARED_PROGRAM_WITNESS\"\n"), 0700)
					case "symlink":
						target := filepath.Join(base, "borrowed-cached-input")
						writeFixture(target, original, info.Mode().Perm())
						Expect(os.Symlink(target, path)).To(Succeed())
					case "directory":
						Expect(os.Mkdir(path, 0700)).To(Succeed())
					case "writable":
						writeFixture(path, original, 0777)
						Expect(os.Chmod(path, 0777)).To(Succeed())
					case "not-executable":
						writeFixture(path, original, 0600)
					case "owner-not-executable":
						writeFixture(path, original, 0645)
						Expect(os.Chmod(path, 0645)).To(Succeed())
					}
					return func() { Expect(os.RemoveAll(path)).To(Succeed()); writeFixture(path, original, info.Mode().Perm()) }
				})
			}
		}
		// Consistent forged size/digest claims cannot turn a real non-race runner
		// into the expected race suite. No extra build is needed for this control.
		// per docs/adr/0151-reliable-ci-qualification.md:71
		suitePath := filepath.Join(root, preparedQualificationSuitePath)
		suite, err := os.ReadFile(suitePath)
		Expect(err).NotTo(HaveOccurred())
		runner, err := os.ReadFile(filepath.Join(root, preparedQualificationRunnerPath))
		Expect(err).NotTo(HaveOccurred())
		suiteInfo, err := os.Stat(suitePath)
		Expect(err).NotTo(HaveOccurred())
		preparedQualificationReject(program, root, revision, witness, "coherent wrong binary metadata", func() func() {
			writeFixture(suitePath, runner, suiteInfo.Mode().Perm())
			changed := preparedQualificationJSON(receipt)
			record := changed["suite"].(map[string]any)
			digest := sha256.Sum256(runner)
			record["size"], record["sha256"] = len(runner), hex.EncodeToString(digest[:])
			data, err := json.Marshal(changed)
			Expect(err).NotTo(HaveOccurred())
			writeFixture(receiptPath, data, 0600)
			return func() {
				writeFixture(suitePath, suite, suiteInfo.Mode().Perm())
				writeFixture(receiptPath, receipt, 0600)
			}
		})
		for _, relative := range []string{"acceptance", preparedQualificationDirectory} {
			path := filepath.Join(root, relative)
			moved := filepath.Join(base, "relocated-"+filepath.Base(path))
			preparedQualificationReject(program, root, revision, witness, "symlinked ancestor "+relative, func() func() {
				Expect(os.Rename(path, moved)).To(Succeed())
				Expect(os.Symlink(moved, path)).To(Succeed())
				return func() { Expect(os.Remove(path)).To(Succeed()); Expect(os.Rename(moved, path)).To(Succeed()) }
			})
		}
	})

	// per docs/adr/0151-reliable-ci-qualification.md:61
	// per docs/adr/0151-reliable-ci-qualification.md:79
	It("rejects receipt identity drift and dirty or different checkouts while preserving all evidence", func() {
		path := filepath.Join(root, preparedQualificationReceiptPath)
		for key, value := range map[string]any{"schema": float64(2), "run_id": "99999", "revision": strings.Repeat("a", 40), "go_os": "other", "go_arch": "other", "go_version": "go0.0.0", "ginkgo_version": "v0.0.0", "go_mod_sha256": strings.Repeat("0", 64), "go_sum_sha256": strings.Repeat("0", 64), "race": false} {
			preparedQualificationReject(program, root, revision, witness, "identity "+key, func() func() {
				changed := preparedQualificationJSON(receipt)
				changed[key] = value
				data, err := json.Marshal(changed)
				Expect(err).NotTo(HaveOccurred())
				writeFixture(path, data, 0600)
				return func() { writeFixture(path, receipt, 0600) }
			})
		}
		for _, record := range []string{"suite", "ginkgo"} {
			for key, value := range map[string]any{"path": "../foreign-executable", "size": float64(0), "sha256": strings.Repeat("0", 64)} {
				preparedQualificationReject(program, root, revision, witness, record+" "+key, func() func() {
					changed := preparedQualificationJSON(receipt)
					changed[record].(map[string]any)[key] = value
					data, err := json.Marshal(changed)
					Expect(err).NotTo(HaveOccurred())
					writeFixture(path, data, 0600)
					return func() { writeFixture(path, receipt, 0600) }
				})
			}
		}
		for _, name := range []string{"go.mod", "go.sum", "acceptance/fixture_suite_test.go", "untracked.go"} {
			target := filepath.Join(root, name)
			original, err := os.ReadFile(target)
			exists := err == nil
			if !exists {
				Expect(os.IsNotExist(err)).To(BeTrue())
			}
			preparedQualificationReject(program, root, revision, witness, "changed source "+name, func() func() {
				writeFixture(target, append(append([]byte{}, original...), []byte("\n// changed source input\n")...), 0600)
				return func() {
					if exists {
						writeFixture(target, original, 0600)
					} else {
						Expect(os.Remove(target)).To(Succeed())
					}
				}
			})
		}
		preparedQualificationReject(program, root, strings.Repeat("b", 40), witness, "different expected checkout", func() func() { return func() {} })
	})

	// per docs/adr/0151-reliable-ci-qualification.md:55
	// per docs/adr/0151-reliable-ci-qualification.md:146
	It("retains failed real compiler diagnostics and never publishes an admissible partial preparation", func() {
		broken, _ := preparedQualificationFixture(base, "broken-checkout")
		writeFixture(filepath.Join(broken, "acceptance/broken_test.go"), []byte("package fixture_test\nvar _ = independentBrokenQualificationSymbol\n"), 0600)
		imageGit(broken, "add", "acceptance/broken_test.go")
		imageGit(broken, "-c", "core.hooksPath="+os.DevNull, "-c", "user.name=Qualification fixture", "-c", "user.email=qualification@example.invalid", "commit", "--quiet", "-m", "test: actual compiler refusal")
		commit := strings.TrimSpace(imageGit(broken, "rev-parse", "HEAD"))
		writeFixture(filepath.Join(broken, preparedQualificationReceiptPath), receipt, 0600)
		out := preparedQualificationInvoke(program, broken, commit, "prepare", nil)
		AddReportEntry("prepared-qualification.failed-prepare", map[string]any{"status": out.status, "stdout": out.stdout, "stderr": out.stderr}, ReportEntryVisibilityNever)
		Expect(out.status).To(Equal(1), "%+v", out)
		_, err := os.Lstat(filepath.Join(broken, preparedQualificationReceiptPath))
		Expect(os.IsNotExist(err)).To(BeTrue(), "a stale receipt cannot survive failed preparation")
		log, err := os.ReadFile(filepath.Join(broken, preparedQualificationDirectory, "prepare.log"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(log)).To(ContainSubstring("independentBrokenQualificationSymbol"))
		rejected := preparedQualificationInvoke(program, broken, commit, "verify", nil)
		Expect(rejected.status).To(Equal(1), "%+v", rejected)
	})
})
