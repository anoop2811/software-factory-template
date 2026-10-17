// Package helpers implements the private literal shared-helper protocol.
package helpers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/anoop2811/software-factory-template/internal/commandenv"
	"github.com/anoop2811/software-factory-template/internal/config"
	"github.com/anoop2811/software-factory-template/internal/eventlog"
	"github.com/anoop2811/software-factory-template/internal/githooks"
	"github.com/anoop2811/software-factory-template/internal/native"
	"github.com/anoop2811/software-factory-template/internal/output"
	"github.com/anoop2811/software-factory-template/internal/presentation"
	"github.com/anoop2811/software-factory-template/internal/roles"
)

var budgetKeys = []string{
	"FACTORY_BUDGET_ROOT", "FACTORY_BUDGET_ENABLED", "FACTORY_BUDGET_MAX_ATTEMPTS",
	"FACTORY_BUDGET_MAX_SESSION_RUNS", "FACTORY_BUDGET_TIMEOUT_SECONDS", "FACTORY_BUDGET_SESSION_SECONDS",
	"FACTORY_BUDGET_MAX_CONCURRENT", "FACTORY_BUDGET_ESTIMATED_USD", "FACTORY_BUDGET_ACTION",
}

// Run validates an entire literal request before invoking any helper effects.
// docs/adr/0099-native-factory-orchestration.md:134.
func Run(ctx context.Context, args []string, environment map[string]string, stdout, stderr io.Writer) int {
	if !valid(args) {
		_, _ = fmt.Fprintln(stderr, "factory bridge: unsupported helper request")
		return 2
	}
	value, err := execute(ctx, args, environment)
	if err == nil && value != "" {
		err = output.WriteEvent(ctx, stdout, []byte(value))
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "factory bridge: helper failed")
		return 2
	}
	return 0
}

func valid(args []string) bool {
	if len(args) == 0 {
		return false
	}
	n := len(args) - 1
	switch args[0] {
	case "event":
		return n <= 2
	case "now":
		return n == 0
	case "duration":
		return n == 1 || n == 2
	case "palette":
		return n == 1 && (args[1] == "0" || args[1] == "1")
	case "action-box":
		return n >= 4
	case "hookspath":
		return n == 1
	case "budget-plan":
		for _, key := range args[1:] {
			if !config.KnownExportKey(key) {
				return false
			}
		}
		return true
	case "budget-model":
		return n == 6
	}
	return false
}

func execute(ctx context.Context, args []string, environment map[string]string) (string, error) {
	switch args[0] {
	case "now":
		return strconv.FormatInt(time.Now().Unix(), 10), nil
	case "duration":
		end := strconv.FormatInt(time.Now().Unix(), 10)
		if len(args) == 3 {
			end = args[2]
		}
		return Duration(args[1], end), nil
	case "palette":
		p := presentation.PaletteFor(args[1] == "1", environment["NO_COLOR"])
		return plan("FACTORY_HELPER_PALETTE_V1", []config.Action{
			{Key: "C_RESET", Value: p.Reset}, {Key: "C_BOLD", Value: p.Bold}, {Key: "C_DIM", Value: p.Dim},
			{Key: "C_YELLOW", Value: p.Yellow}, {Key: "C_GREEN", Value: p.Green}, {Key: "C_RED", Value: p.Red},
		})
	case "action-box":
		return presentation.Box(presentation.Palette{Reset: args[1], Bold: args[2], Yellow: args[3]}, args[4], args[5:]), nil
	case "hookspath":
		state, path, err := githooks.Status(ctx, args[1], environment)
		return state + "\t" + path, err
	case "budget-model":
		tier := roles.Resolve(args[3], roles.Tier(args[2]))
		switch tier {
		case "frontier":
			return args[4], nil
		case "economy":
			return args[6], nil
		default:
			return args[5], nil
		}
	case "budget-plan":
		return budgetPlan(ctx, environment, args[1:])
	case "event":
		gate, reason := "gate", "blocked"
		if len(args) > 1 {
			gate = args[1]
		}
		if len(args) > 2 {
			reason = args[2]
		}
		root := ""
		if environment["FACTORY_EVENT_LOG"] == "" {
			if cwd, err := os.Getwd(); err == nil {
				result, err := native.ExecuteCommand(ctx, cwd, []string{"git", "rev-parse", "--show-toplevel"}, environment, 5*time.Second)
				if err == nil && !result.OwnershipUnconfirmed && result.ExitCode != nil && *result.ExitCode == 0 {
					root = strings.TrimRight(string(result.Stdout), "\n")
				}
			}
		}
		_ = eventlog.Append(ctx, root, environment, gate, reason)
		return "", nil
	}
	return "", errors.New("unsupported helper")
}

// Duration accepts unsigned decimal epochs and avoids signed arithmetic overflow.
// docs/adr/0099-native-factory-orchestration.md:138.
func Duration(start, end string) string {
	parse := func(value string) (uint64, error) {
		if value == "" || strings.IndexFunc(value, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			return 0, errors.New("invalid epoch")
		}
		return strconv.ParseUint(value, 10, 64)
	}
	s, startErr := parse(start)
	e, endErr := parse(end)
	if startErr != nil || endErr != nil {
		return "unknown"
	}
	seconds := uint64(0)
	if e > s {
		seconds = e - s
	}
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	if seconds < 3600 {
		return fmt.Sprintf("%dm %ds", seconds/60, seconds%60)
	}
	return fmt.Sprintf("%dh %dm", seconds/3600, seconds%3600/60)
}

func budgetPlan(ctx context.Context, environment map[string]string, preserved []string) (string, error) {
	effective, path, err := commandenv.Base(ctx, environment)
	if err != nil {
		return "", err
	}
	actions := make([]config.Action, 0, len(budgetKeys))
	for _, key := range budgetKeys {
		actions = append(actions, config.Action{Key: key, Value: effective[key]})
	}
	for key := range environment {
		if config.KnownExportKey(key) {
			preserved = append(preserved, key)
		}
	}
	exports, err := config.ExportPlan(ctx, path, preserved)
	if err != nil {
		return "", err
	}
	return plan("FACTORY_BUDGET_PLAN_V1", append(actions, exports...))
}

func plan(header string, actions []config.Action) (string, error) {
	var b strings.Builder
	b.WriteString(header + "\n")
	for _, action := range actions {
		if strings.ContainsAny(action.Value, "\n\x00") {
			return "", errors.New("invalid helper plan value")
		}
		if action.ExportOnly {
			fmt.Fprintf(&b, "export\t%s\n", action.Key)
		} else {
			fmt.Fprintf(&b, "set\t%s\t%s\n", action.Key, action.Value)
		}
	}
	b.WriteString("END\n")
	if b.Len() > 1<<20 {
		return "", errors.New("helper plan limit exceeded")
	}
	return b.String(), nil
}
