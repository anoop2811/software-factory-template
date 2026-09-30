package metricscmd

import (
	"bytes"
	"context"
	"errors"

	"github.com/anoop2811/software-factory-template/internal/jsonvalue"
)

type cappedJSON struct{ bytes.Buffer }

func (b *cappedJSON) Write(data []byte) (int, error) {
	if len(data) > fileLimit-1-b.Len() {
		return 0, errors.New("generated JSON limit exceeded")
	}
	return b.Buffer.Write(data)
}

// Stream repeated metadata into a capped sink rather than materializing an
// arbitrarily amplified JSON document. docs/adr/0088-go-native-metrics.md:76.
func encodeJSON(ctx context.Context, m metrics) ([]byte, error) {
	e, l, v, a := m.Enforcement, m.Loop, m.Verification, m.Agents
	by := make([]any, 0, len(e.ByGate))
	for _, g := range e.ByGate {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		by = append(by, map[string]any{"gate": g.Gate, "blocks": g.Blocks})
	}
	tasks := make([]any, 0, len(a.Tasks))
	for _, t := range a.Tasks {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		tasks = append(tasks, map[string]any{"harness": t.Harness, "task": t.Task, "baseline": t.Baseline, "current": t.Current, "is_mock": t.Mock})
	}
	notMeasured := make([]any, 0, len(m.NotMeasured))
	for _, n := range m.NotMeasured {
		notMeasured = append(notMeasured, n)
	}
	value := map[string]any{
		"schema": m.Schema, "window_days": m.WindowDays,
		"enforcement":  map[string]any{"gates_installed": e.Installed, "gates_armed": e.Armed, "gates_inert": e.Inert, "gates_reporting": e.Reporting, "gates_mute": e.Mute, "blocks_total": e.Total, "blocks_in_window": e.Window, "repeat_block_hours": e.Repeat, "blocks_by_gate": by},
		"loop":         map[string]any{"commits": l.Commits, "merges": l.Merges, "reverts": l.Reverts, "authors": l.Authors, "files_touched": l.Touched, "files_reworked": l.Reworked},
		"verification": map[string]any{"claim_commits": v.Claims, "cited_commits": v.Cited},
		"agents":       map[string]any{"tasks": tasks, "stale": a.Stale, "harnesses": a.Harnesses, "task_count": a.TaskCount, "scaffold": a.Scaffold, "measured_tasks": a.Measured},
		"not_measured": notMeasured,
	}
	var buffer cappedJSON
	if err := jsonvalue.EncodePython(ctx, &buffer, value); err != nil {
		return nil, err
	}
	if err := buffer.WriteByte('\n'); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
