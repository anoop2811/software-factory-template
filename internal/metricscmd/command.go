// Package metricscmd computes and presents local repository measurements.
package metricscmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/anoop2811/software-factory-template/internal/fileinput"
	"github.com/anoop2811/software-factory-template/internal/native"
	"github.com/anoop2811/software-factory-template/internal/output"
	"golang.org/x/sys/unix"
)

const fileLimit = 16 << 20
const entryLimit = 4096

type options struct {
	format, digits string
	days           int
	noOpen         bool
}
type argumentError struct {
	message string
	status  int
}

func (e *argumentError) Error() string { return e.message }

// Run retains local-only collection and explicitly gated browser handoff.
// docs/adr/0088-go-native-metrics.md:16.
func Run(parent context.Context, args []string, invocation string, environment map[string]string, stdout, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	broken := make(chan os.Signal, 1)
	signal.Notify(broken, syscall.SIGPIPE)
	defer signal.Stop(broken)
	err := run(ctx, args, invocation, environment, stdout)
	if ctx.Err() != nil {
		err = fmt.Errorf("metrics canceled: %w", ctx.Err())
	}
	if err == nil {
		return 0
	}
	_, _ = fmt.Fprintln(stderr, "factory metrics: "+err.Error())
	var argument *argumentError
	if errors.As(err, &argument) {
		return argument.status
	}
	return 1
}

func parse(ctx context.Context, args []string) (options, error) {
	o := options{format: "text", digits: "30", days: 30}
	for i := 0; i < len(args); i++ {
		if err := ctx.Err(); err != nil {
			return o, err
		}
		switch arg := args[i]; {
		case arg == "--json":
			o.format = "json"
		case arg == "--html":
			o.format = "html"
		case arg == "--no-open":
			o.noOpen = true
		case arg == "--days":
			if i+1 == len(args) {
				return o, &argumentError{"--days requires an operand", 1}
			}
			i++
			o.digits = args[i]
		case strings.HasPrefix(arg, "--days="):
			o.digits = strings.TrimPrefix(arg, "--days=")
		default:
			return o, &argumentError{fmt.Sprintf("unknown argument '%s'", arg), 2}
		}
	}
	if o.digits == "" || strings.Trim(o.digits, "0123456789") != "" {
		o.digits = "30"
	}
	n, err := strconv.ParseUint(o.digits, 10, 31)
	if err != nil {
		return o, &argumentError{"days exceeds maximum 2147483647", 2}
	}
	o.days = int(n)
	if o.noOpen && o.format != "html" {
		return o, &argumentError{"--no-open applies to --html only", 2}
	}
	return o, nil
}

func run(ctx context.Context, args []string, invocation string, environment map[string]string, stdout io.Writer) error {
	o, err := parse(ctx, args)
	if err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	c := collector{root: cwd, environment: environment}
	root, err := c.git(ctx, "rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	if root != "" {
		c.root = strings.TrimRight(root, "\n")
	}
	data, err := c.collect(ctx, o)
	if err != nil {
		return err
	}
	switch o.format {
	case "json":
		raw, err := encodeJSON(ctx, data)
		if err != nil {
			return err
		}
		return output.WriteEvent(ctx, stdout, raw)
	case "html":
		return c.html(ctx, invocation, stdout, o, data)
	default:
		return renderText(ctx, stdout, environment, o, data)
	}
}

type collector struct {
	root        string
	environment map[string]string
}

func (c collector) path(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return c.root + "/" + path
}
func (c collector) git(ctx context.Context, args ...string) (string, error) {
	r, err := native.ExecuteCommand(ctx, c.root, append([]string{"git"}, args...), c.environment, 10*time.Second)
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if r.OwnershipUnconfirmed || r.Outcome == "timeout" || r.Outcome == "interrupted" || r.Outcome == "output_limit" {
		return "", errors.New("git metrics collection did not complete safely")
	}
	if err == nil && r.ExitConfirmed && r.ExitCode != nil && *r.ExitCode == 0 {
		return string(r.Stdout), nil
	}
	return "", nil
}
func optionalRead(ctx context.Context, path string) ([]byte, error) {
	data, err := fileinput.Read(ctx, path, fileLimit)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return data, err
}
func entries(ctx context.Context, path string) ([]os.DirEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NONBLOCK, 0) // #nosec G304 -- Read-only caller-selected directory, bounded below.
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	list, err := f.ReadDir(entryLimit + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(list) > entryLimit {
		return nil, errors.New("directory entry limit exceeded")
	}
	return list, ctx.Err()
}
func terminal(out io.Writer) bool {
	f, ok := out.(*os.File)
	if !ok {
		return false
	}
	_, err := unix.IoctlGetWinsize(int(f.Fd()), unix.TIOCGWINSZ)
	return err == nil
}

type metrics struct {
	Schema       string       `json:"schema"`
	WindowDays   int          `json:"window_days"`
	Enforcement  enforcement  `json:"enforcement"`
	Loop         loopMetrics  `json:"loop"`
	Verification verification `json:"verification"`
	Agents       agents       `json:"agents"`
	NotMeasured  []string     `json:"not_measured"`
}
type gateBlocks struct {
	Gate   string `json:"gate"`
	Blocks int    `json:"blocks"`
}
type enforcement struct {
	Installed int          `json:"gates_installed"`
	Armed     int          `json:"gates_armed"`
	Inert     int          `json:"gates_inert"`
	Reporting int          `json:"gates_reporting"`
	Mute      int          `json:"gates_mute"`
	Total     int          `json:"blocks_total"`
	Window    int          `json:"blocks_in_window"`
	Repeat    int          `json:"repeat_block_hours"`
	ByGate    []gateBlocks `json:"blocks_by_gate"`
}
type loopMetrics struct {
	Commits  int `json:"commits"`
	Merges   int `json:"merges"`
	Reverts  int `json:"reverts"`
	Authors  int `json:"authors"`
	Touched  int `json:"files_touched"`
	Reworked int `json:"files_reworked"`
}
type verification struct {
	Claims int `json:"claim_commits"`
	Cited  int `json:"cited_commits"`
}
type task struct {
	Harness  string `json:"harness"`
	Task     string `json:"task"`
	Baseline any    `json:"baseline"`
	Current  any    `json:"current"`
	Mock     bool   `json:"is_mock"`
}
type agents struct {
	Tasks     []task `json:"tasks"`
	Stale     int    `json:"stale"`
	Harnesses int    `json:"harnesses"`
	TaskCount int    `json:"task_count"`
	Scaffold  bool   `json:"scaffold"`
	Measured  int    `json:"measured_tasks"`
}
