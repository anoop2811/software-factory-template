package metricscmd

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/anoop2811/software-factory-template/internal/output"
)

func renderText(ctx context.Context, out io.Writer, environment map[string]string, o options, m metrics) error {
	bold, dim, yellow, reset := "", "", "", ""
	if terminal(out) && environment["NO_COLOR"] == "" {
		bold = "\x1b[1m"
		dim = "\x1b[2m"
		yellow = "\x1b[33m"
		reset = "\x1b[0m"
	}
	var writeErr error
	printf := func(format string, args ...any) {
		if writeErr == nil {
			writeErr = output.WriteEvent(ctx, out, []byte(fmt.Sprintf(format, args...)))
		}
	}
	e, l, v, a := m.Enforcement, m.Loop, m.Verification, m.Agents
	printf("%sFactory metrics%s — last %s days, computed locally\n\n", bold, reset, o.digits)
	printf("%sEnforcement%s  %sis the factory real, or installed theatre?%s\n", bold, reset, dim, reset)
	printf("  %-34s %d\n", "gates installed", e.Installed)
	printf("  %-34s %d armed, %s%d inert%s\n", "config-armed gates", e.Armed, yellow, e.Inert, reset)
	if e.Inert > 0 {
		printf("  %-34s %sinert is a choice — 'factory doctor' names which%s\n", "", dim, reset)
	}
	printf("  %-34s %d in window (%d retained)\n", "blocks caught", e.Window, e.Total)
	if e.Mute > 0 {
		printf("  %-34s %s%d of %d can block without recording it%s\n", "", yellow, e.Mute, e.Installed, reset)
		printf("  %-34s %sso a low block count may be silence, not calm%s\n", "", dim, reset)
	}
	for _, gate := range e.ByGate {
		if err := ctx.Err(); err != nil {
			return err
		}
		if gate.Gate != "" {
			printf("    %s%s %d\n", gate.Gate, strings.Repeat(" ", max(0, 32-len(gate.Gate))), gate.Blocks)
		}
	}
	if e.Repeat > 0 {
		printf("  %-34s %s%d hour(s)%s %s— friction: a real habit, or a wrong gate%s\n", "same gate 3+ times in an hour", yellow, e.Repeat, reset, dim, reset)
	}
	printf("\n%sLoop health%s  %sis work converging, or circling?%s\n", bold, reset, dim, reset)
	printf("  %-34s %d across %d author(s)\n", "commits", l.Commits, l.Authors)
	printf("  %-34s %d\n", "merges", l.Merges)
	printf("  %-34s %d %s— work that had to be undone%s\n", "reverts", l.Reverts, dim, reset)
	printf("  %-34s %d of %d files %s— changed in more than one commit%s\n", "reworked", l.Reworked, l.Touched, dim, reset)
	printf("\n%sVerification discipline%s  %sare claims cited, or asserted?%s\n", bold, reset, dim, reset)
	percentage := "n/a"
	if v.Claims > 0 {
		percentage = strconv.Itoa(v.Cited*100/v.Claims) + "%"
	}
	printf("  %-34s %d of %d (%s)\n", "claims carrying evidence", v.Cited, v.Claims, percentage)
	printf("\n%sAgents%s  %sgetting better, or worse?%s\n", bold, reset, dim, reset)
	if len(a.Tasks) == 0 {
		switch {
		case !a.Scaffold:
			printf("  eval scaffold not installed — run: factory upgrade  (adds eval/golden-tasks and eval/runners)\n")
		case a.TaskCount == 0:
			printf("  no eval tasks yet — a task is a red spec plus an oracle; see eval/README.md\n")
		default:
			printf("  %d task(s), no baseline yet — run: ./scripts/golden-task-eval.sh --save-baseline\n", a.TaskCount)
		}
	} else {
		for _, task := range a.Tasks {
			if err := ctx.Err(); err != nil {
				return err
			}
			name := []rune(task.Task)
			if len(name) > 30 {
				name = name[:30]
			}
			current := "-"
			if task.Current != nil {
				current = scoreText(task.Current)
			}
			tag := ""
			if task.Mock {
				tag = "  (mock — not your agents)"
			}
			printf("    %-30s baseline %s  current %s%s\n", string(name), scoreText(task.Baseline), current, tag)
		}
		if a.Measured == 0 {
			printf("  the scorer works — no agent measured yet. The mock runner writes a\n  fixed answer and always scores 1.00; point --runner at a real harness\n  (see eval/runners/example-harness.sh) to score the agents themselves.\n")
		}
		if a.Stale > 0 {
			printf("  %d baseline(s) STALE — measured against different inputs\n", a.Stale)
		}
	}
	printf("\n%sNot measured here: token spend per role (your harness owns that), and code\n", dim)
	printf("quality (not honestly measurable without judgment, so it is not claimed).\nNothing above left this machine. --json to export, --html for a page.%s\n", reset)
	return writeErr
}
