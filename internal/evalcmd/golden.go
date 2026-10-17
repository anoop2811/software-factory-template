package evalcmd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/anoop2811/software-factory-template/internal/commandenv"
	"github.com/anoop2811/software-factory-template/internal/config"
	"github.com/anoop2811/software-factory-template/internal/fileinput"
	"github.com/anoop2811/software-factory-template/internal/native"
)

type goldenOptions struct {
	harness, runner              string
	runs, seconds                int
	explicit, save, nativeRunner bool
}
type taskResult struct {
	Task        string  `json:"task"`
	Passes      int     `json:"passes"`
	Runs        int     `json:"runs"`
	Score       float64 `json:"score"`
	Fingerprint *string `json:"fingerprint,omitempty"`
}
type goldenResult struct {
	Harness string       `json:"harness"`
	Runner  string       `json:"runner"`
	Runs    int          `json:"runs"`
	Inputs  *string      `json:"inputs_fingerprint,omitempty"`
	Tasks   []taskResult `json:"tasks"`
}

func parseGolden(args []string, stdout, stderr io.Writer) (goldenOptions, int) {
	o := goldenOptions{harness: "mock", runner: "eval/runners/mock.sh", runs: 1, seconds: 300}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--help" || arg == "-h" {
			fmt.Fprintln(stdout, "usage: golden-task-eval.sh [--harness NAME] [--runner PATH] [--runs N] [--timeout S] [--save-baseline]")
			fmt.Fprintln(stdout, "  --harness  mock (default), claude, codex, opencode, or your own name")
			fmt.Fprintln(stdout, "             a shipped runner is selected automatically by name")
			return o, 0
		}
		if arg == "--save-baseline" {
			o.save = true
			continue
		}
		key, value, equal := strings.Cut(arg, "=")
		switch key {
		case "--harness", "--runner", "--runs", "--timeout":
			if !equal {
				if i+1 >= len(args) || args[i+1] == "" {
					fmt.Fprintf(stderr, "golden-task-eval: %s needs a value\n", key)
					return o, 2
				}
				i++
				value = args[i]
			}
		default:
			fmt.Fprintf(stderr, "golden-task-eval: unknown argument '%s'\n", arg)
			return o, 2
		}
		switch key {
		case "--harness":
			o.harness = value
		case "--runner":
			o.runner = value
			o.explicit = true
		case "--runs":
			o.runs = positive(value, 1)
		case "--timeout":
			o.seconds = positive(value, 300)
		}
	}
	return o, -1
}
func positive(value string, fallback int) int {
	if value == "" || strings.Trim(value, "0123456789") != "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 {
		return fallback
	}
	return n
}

func golden(ctx context.Context, args []string, root string, environment map[string]string, stdout, stderr io.Writer) int {
	o, status := parseGolden(args, stdout, stderr)
	if status >= 0 {
		return status
	}
	if strings.ContainsAny(o.harness, "/\x00") || o.harness == ".." {
		fmt.Fprintln(stderr, "golden-task-eval: invalid harness name")
		return 2
	}
	if !o.explicit {
		candidate := "eval/runners/" + o.harness + ".sh"
		if info, err := os.Stat(filepath.Join(root, candidate)); err == nil && info.Mode().IsRegular() {
			o.runner = candidate
		} else if o.harness != "mock" {
			fmt.Fprintf(stderr, "golden-task-eval: no shipped runner for harness '%s'.\n", o.harness)
			entries, err := os.ReadDir(filepath.Join(root, "eval/runners"))
			var shipped []string
			for _, entry := range entries {
				if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sh") {
					shipped = append(shipped, strings.TrimSuffix(entry.Name(), ".sh"))
				}
			}
			switch {
			case len(shipped) > 0:
				sort.Strings(shipped)
				fmt.Fprintf(stderr, "  Shipped: %s \n", strings.Join(shipped, " "))
			case err == nil:
				fmt.Fprintln(stderr, "  Shipped: none — eval/runners is empty.")
			default:
				fmt.Fprintln(stderr, "  Shipped: none — there is no eval/runners directory in this repo.")
			}
			fmt.Fprintln(stderr, "  Pass --runner <script> for your own harness, or --harness mock to use the mock deliberately.")
			fmt.Fprintln(stderr, "  Not falling back to the mock: it solves every task, so the score would be meaningless.")
			return 2
		}
	}
	resultsDir := filepath.Join(root, "eval/results")
	if err := os.MkdirAll(resultsDir, 0700); err != nil {
		return goldenError(stderr, err)
	}
	_, configPath, err := commandenv.Base(ctx, environment)
	if err != nil {
		return goldenError(stderr, err)
	}
	model, err := config.Get(ctx, configPath, o.harness+"_frontier_model", "")
	if err != nil {
		return goldenError(stderr, err)
	}
	model = strings.TrimRight(strings.ReplaceAll(model, "\x00", ""), "\n")
	presentedModel := model
	if presentedModel == "" {
		presentedModel = "inherit"
	}
	fmt.Fprintf(stdout, "golden-task-eval: harness=%s runner=%s runs=%d model=%s\n", o.harness, o.runner, o.runs, presentedModel)
	entries, err := os.ReadDir(filepath.Join(root, "eval/golden-tasks"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return goldenError(stderr, err)
	}
	var tasks []string
	for _, entry := range entries {
		if entry.IsDir() {
			tasks = append(tasks, entry.Name())
		}
	}
	if len(tasks) == 0 {
		fmt.Fprintln(stdout, "golden-task-eval: no tasks in eval/golden-tasks/")
		fmt.Fprintln(stdout, "  A task is a red spec plus an oracle — see eval/README.md.")
		fmt.Fprintln(stdout, "  If eval/golden-tasks/ is missing entirely, 'factory upgrade' installs it.")
		return 0
	}
	runnerPath := o.runner
	if !filepath.IsAbs(runnerPath) {
		runnerPath = filepath.Join(root, runnerPath)
	}
	if info, err := os.Stat(runnerPath); err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		fmt.Fprintf(stderr, "golden-task-eval: runner not executable: %s\n", o.runner)
		return 1
	}
	o.nativeRunner, err = qualifiedRunner(ctx, o, runnerPath)
	if err != nil {
		return goldenError(stderr, err)
	}
	inputs, err := goldenInputs(ctx, root, runnerPath)
	if err != nil {
		return goldenError(stderr, err)
	}
	inputsFP, err := checksum(ctx, inputs)
	if err != nil {
		return goldenError(stderr, err)
	}
	current := goldenResult{Harness: o.harness, Runner: o.runner, Runs: o.runs, Inputs: &inputsFP, Tasks: []taskResult{}}
	variables := maps.Clone(environment)
	variables["FACTORY_EVAL_MODEL"] = model
	for _, name := range tasks {
		directory := filepath.Join(root, "eval/golden-tasks", name)
		if info, err := os.Stat(directory + "/verify.sh"); err != nil || !info.Mode().IsRegular() {
			fmt.Fprintf(stdout, "  - %s: SKIP (no verify.sh oracle)\n", name)
			continue
		}
		passes, timeouts := 0, 0
		for run := 0; run < o.runs; run++ {
			passed, timedOut, err := goldenRun(ctx, o, root, directory, runnerPath, variables)
			if err != nil {
				return goldenError(stderr, err)
			}
			if passed {
				passes++
			}
			if timedOut {
				timeouts++
			}
		}
		score, _ := strconv.ParseFloat(fmt.Sprintf("%.2f", float64(passes)/float64(o.runs)), 64)
		input, err := concatenate(ctx, directory+"/task.md", directory+"/verify.sh")
		if err != nil {
			return goldenError(stderr, err)
		}
		fp, err := checksum(ctx, input)
		if err != nil {
			return goldenError(stderr, err)
		}
		note := ""
		if timeouts > 0 {
			note = fmt.Sprintf(" — %d run(s) hit the %ds cap (a hung runner counts as failed)", timeouts, o.seconds)
		}
		fmt.Fprintf(stdout, "  - %s: %d/%d passed (score %.2f)%s\n", name, passes, o.runs, score, note)
		current.Tasks = append(current.Tasks, taskResult{Task: name, Passes: passes, Runs: o.runs, Score: score, Fingerprint: &fp})
	}
	encoded, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return goldenError(stderr, err)
	}
	if err := publish(ctx, resultsDir, o.harness+"-current.json", encoded); err != nil {
		return goldenError(stderr, err)
	}
	if o.save {
		if err := publish(ctx, resultsDir, o.harness+"-baseline.json", encoded); err != nil {
			return goldenError(stderr, err)
		}
		fmt.Fprintf(stdout, "golden-task-eval: baseline saved to eval/results/%s-baseline.json\n", o.harness)
		if o.harness == "mock" {
			fmt.Fprintln(stdout, "  Note: this is the mock runner. It calls no model and always scores 1.00 —\n  it proves the scorer, not your agents. To measure agents, point --runner\n  at a real harness (see eval/runners/example-harness.sh) and --harness at\n  its name, so the baseline records which system was scored.")
		}
		return 0
	}
	baseline, err := fileinput.Read(ctx, filepath.Join(resultsDir, o.harness+"-baseline.json"), fileLimit)
	switch {
	case errors.Is(err, os.ErrNotExist):
		fmt.Fprintf(stdout, "golden-task-eval: no baseline for '%s' — run with --save-baseline to create one\n", o.harness)
	case err != nil:
		return goldenError(stderr, err)
	default:
		var base goldenResult
		if err := json.Unmarshal(baseline, &base); err != nil {
			return goldenError(stderr, err)
		}
		if err := validateBaselineScores(baseline); err != nil {
			return goldenError(stderr, err)
		}
		if compareGolden(base, current, stdout) != 0 {
			return 1
		}
	}
	fmt.Fprintln(stdout, "golden-task-eval: done")
	return 0
}

func goldenError(stderr io.Writer, _ error) int {
	fmt.Fprintln(stderr, "golden-task-eval: evaluation could not complete")
	return 1
}

// A missing or null score is not a measured zero. Older baselines without
// fingerprints remain supported, but malformed score evidence cannot pass.
// docs/adr/0099-native-factory-orchestration.md:107.
func validateBaselineScores(data []byte) error {
	var baseline struct {
		Tasks []struct {
			Score *float64 `json:"score"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(data, &baseline); err != nil {
		return err
	}
	for _, task := range baseline.Tasks {
		if task.Score == nil {
			return errors.New("baseline task score is missing or null")
		}
	}
	return nil
}

func goldenInputs(ctx context.Context, root, runnerPath string) ([]byte, error) {
	data, err := concatenate(ctx, runnerPath, filepath.Join(root, "AGENTS.md"))
	if err != nil {
		return nil, err
	}
	for _, item := range []struct{ path, pattern string }{{"factory.yaml", `^(cost_profile|[a-z]+_(frontier|default|economy)_model):`}, {"factory.config", `^(COST_PROFILE|[A-Z]+_(FRONTIER|DEFAULT|ECONOMY)_MODEL)=`}} {
		raw, err := concatenate(ctx, filepath.Join(root, item.path))
		if err != nil {
			return nil, err
		}
		pattern := regexp.MustCompile(item.pattern)
		for _, line := range bytes.Split(raw, []byte{'\n'}) {
			if pattern.Match(line) {
				if len(line)+1 > fileLimit-len(data) {
					return nil, errors.New("evaluation input limit exceeded")
				}
				data = append(data, line...)
				data = append(data, '\n')
			}
		}
	}
	return data, ctx.Err()
}

func goldenRun(ctx context.Context, o goldenOptions, root, source, runnerPath string, environment map[string]string) (passed, timedOut bool, returned error) {
	work, err := os.MkdirTemp("", "factory-golden-")
	if err != nil {
		return false, false, err
	}
	defer func() { returned = errors.Join(returned, os.RemoveAll(work)) }()
	if err := copyTask(ctx, source, work); err != nil {
		return false, false, err
	}
	original, err := fileinput.Read(ctx, work+"/verify.sh", fileLimit)
	if err != nil {
		return false, false, err
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(o.seconds)*time.Second)
	var result native.CommandResult
	if o.nativeRunner {
		result, err = runHarness(runCtx, o.harness, work, environment)
	} else {
		result, err = native.ExecuteCommand(runCtx, root, []string{runnerPath, work}, environment, time.Duration(o.seconds)*time.Second)
	}
	timedOut = errors.Is(runCtx.Err(), context.DeadlineExceeded) || result.Outcome == "timeout"
	cancel()
	if ctx.Err() != nil {
		return false, timedOut, ctx.Err()
	}
	if result.OwnershipUnconfirmed {
		return false, timedOut, errors.New("runner ownership unconfirmed")
	}
	if err != nil || !result.ExitConfirmed || (result.Outcome != "completed" && result.Outcome != "failed") {
		return false, timedOut, nil
	}
	if timedOut {
		return false, true, nil
	}
	after, err := fileinput.Read(ctx, work+"/verify.sh", fileLimit)
	if err != nil || !bytes.Equal(original, after) {
		return false, false, nil
	}
	oracleCtx, stop := context.WithTimeout(ctx, time.Duration(o.seconds)*time.Second)
	defer stop()
	oracle, err := native.ExecuteCommand(oracleCtx, work, []string{"sh", "verify.sh"}, environment, time.Duration(o.seconds)*time.Second)
	if ctx.Err() != nil {
		return false, false, ctx.Err()
	}
	if oracle.OwnershipUnconfirmed {
		return false, false, errors.New("oracle ownership unconfirmed")
	}
	passed = err == nil && oracle.Outcome == "completed" && oracle.ExitConfirmed && oracle.ExitCode != nil && *oracle.ExitCode == 0 && oracleCtx.Err() == nil
	final, readErr := fileinput.Read(ctx, work+"/verify.sh", fileLimit)
	return passed && readErr == nil && bytes.Equal(original, final), false, nil
}

// Only the exact qualified built-in bytes permit native substitution. A named
// custom runner remains an explicit boundary, including its caller cwd and
// cost/permission guards. Digests are from immutable 4ecab79627d08d1e125391a5831eb995efd3fce4.
// docs/adr/0099-native-factory-orchestration.md:207.
func qualifiedRunner(ctx context.Context, o goldenOptions, path string) (bool, error) {
	if o.explicit {
		return false, nil
	}
	digests := map[string]string{
		"mock":     "f3ce523ea1acb0031a41fed87c670c8d8517fdb0fe117379438586b44c85f8c4",
		"claude":   "027b387803f9bc41bb3164ebee2de43e6d2d6df5e546b81cb466fcb1b81ebd69",
		"codex":    "beeacdc5d96ff4fe0ac985952234ae2cb29b33ff6178885ccc3c8cf42ec22f16",
		"opencode": "dd82b4615f54d83e17e2511fa6587eee7cbaf6d4aeef3c30ee2807ada0c280b0",
	}
	expected, ok := digests[o.harness]
	if !ok {
		return false, nil
	}
	data, err := fileinput.Read(ctx, path, fileLimit)
	if err != nil {
		return false, err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]) == expected, nil
}

func compareGolden(base, current goldenResult, stdout io.Writer) int {
	var stale []string
	if base.Inputs != nil && current.Inputs != nil && *base.Inputs != *current.Inputs {
		stale = append(stale, "  all tasks: the runner, AGENTS.md, or model tiers changed")
	}
	old := map[string]taskResult{}
	for _, task := range base.Tasks {
		old[task.Task] = task
	}
	names := map[string]bool{}
	for _, task := range current.Tasks {
		names[task.Task] = true
		if prev, ok := old[task.Task]; ok && prev.Fingerprint != nil && task.Fingerprint != nil && *prev.Fingerprint != *task.Fingerprint {
			stale = append(stale, "  "+task.Task+": its task.md or verify.sh oracle changed")
		}
	}
	for _, task := range base.Tasks {
		if !names[task.Task] {
			stale = append(stale, "  "+task.Task+": in the baseline but absent from this run (task removed?)")
		}
	}
	if len(stale) > 0 {
		fmt.Fprintln(stdout, "golden-task-eval: BASELINE STALE — cannot claim 'no regression'")
		for _, reason := range stale {
			fmt.Fprintln(stdout, reason)
		}
		fmt.Fprintln(stdout, "The saved scores were measured against different inputs. Re-run with\n--save-baseline to re-measure deliberately.")
		return 1
	}
	var regressions []string
	for _, task := range current.Tasks {
		if prev, ok := old[task.Task]; ok && task.Score < prev.Score-1e-9 {
			regressions = append(regressions, fmt.Sprintf("  %s: %.2f -> %.2f", task.Task, prev.Score, task.Score))
		}
	}
	if len(regressions) > 0 {
		fmt.Fprintln(stdout, "golden-task-eval: REGRESSION DETECTED — a task's pass rate dropped")
		for _, line := range regressions {
			fmt.Fprintln(stdout, line)
		}
		fmt.Fprintln(stdout, "If the change is intentional, re-run with --save-baseline to update.")
		return 1
	}
	if base.Inputs == nil {
		fmt.Fprintln(stdout, "golden-task-eval: baseline predates fingerprinting — staleness unchecked\n  Re-run with --save-baseline to record what the scores were measured against.")
	}
	fmt.Fprintln(stdout, "golden-task-eval: no regression from baseline")
	return 0
}
