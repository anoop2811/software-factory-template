package reportcmd

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/anoop2811/software-factory-template/internal/output"
)

func render(ctx context.Context, out io.Writer, environment map[string]string, gates int, blocks uint64, events, review string, estimate uint64) error {
	var writeErr error
	printf := func(format string, args ...any) {
		if writeErr == nil {
			writeErr = output.WriteEvent(ctx, out, []byte(fmt.Sprintf(format, args...)))
		}
	}
	printf("Factory report\n\nFacts (deterministic, 0 model tokens)\n")
	printf("  Deterministic gates installed:  %d  (run 'factory doctor' for armed/inert)\n", gates)
	if profile := environment["COST_PROFILE"]; profile != "" {
		printf("  Cost profile:                   %s\n", profile)
	}
	if model := environment["OPENCODE_DEFAULT_MODEL"]; model != "" {
		frontier, economy := environment["OPENCODE_FRONTIER_MODEL"], environment["OPENCODE_ECONOMY_MODEL"]
		if frontier == "" {
			frontier = "?"
		}
		if economy == "" {
			economy = "?"
		}
		printf("  opencode tiers:                 frontier=%s default=%s economy=%s\n", frontier, model, economy)
	}
	printf("  Gate blocks recorded:           %d\n", blocks)
	if blocks > 0 {
		for events != "" {
			if err := ctx.Err(); err != nil {
				return err
			}
			if writeErr != nil {
				return writeErr
			}
			line, rest, terminated := strings.Cut(events, "\n")
			if !terminated {
				break
			}
			ts, gate, reason := eventFields(line)
			if gate != "" {
				printf("    - %s  %s  %s\n", ts, gate, reason)
			}
			events = rest
		}
		printf("  (run 'factory report --clear' to reset this window)\n")
	}
	printf("\nEstimate (labeled)\n")
	if blocks > 0 {
		printf("  Review-spend avoided:  ~%d tokens\n", estimate)
		printf("    = %d block(s) x ~%s tokens per LLM review pass (R; set FACTORY_REVIEW_TOKENS).\n", blocks, review)
		printf("    Each block is a catch a model reviewer would have had to make.\n    Method: docs/COST_AND_TOKENS.md.\n")
	} else {
		printf("  No blocks recorded yet — nothing to estimate. The gates still run on every\n")
		printf("  commit and push at 0 model tokens; an LLM reviewer would cost ~%s per pass.\n", review)
	}
	printf("\nMeasured token spend\n  The factory does not meter tokens — your harness does. See its usage output;\n")
	printf("  opencode, Claude Code, and Codex each report per-session token counts.\n\n")
	printf("Not shown: a \"tokens saved\" headline. That compares this run to one that never\nhappened. A real saved figure comes from an A/B eval (docs/COST_AND_TOKENS.md).\n")
	return writeErr
}
