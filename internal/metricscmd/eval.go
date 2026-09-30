package metricscmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/anoop2811/software-factory-template/internal/jsonvalue"
)

type evalTask struct {
	name  string
	score any
}
type evalFile struct {
	harness, fingerprint string
	tasks                []evalTask
}

func (c collector) evals(ctx context.Context) (agents, error) {
	a := agents{Tasks: []task{}}
	taskRoot := c.path("eval/golden-tasks")
	if info, err := os.Stat(taskRoot); err == nil {
		a.Scaffold = info.IsDir()
		if a.Scaffold {
			list, err := entries(ctx, taskRoot)
			if err != nil {
				return a, err
			}
			for _, item := range list {
				if err := ctx.Err(); err != nil {
					return a, err
				}
				if strings.HasPrefix(item.Name(), ".") {
					continue
				}
				info, err := os.Stat(taskRoot + "/" + item.Name())
				if err == nil && info.IsDir() {
					a.TaskCount++
				} else if err != nil && !errors.Is(err, os.ErrNotExist) {
					return a, err
				}
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return a, err
	}
	list, err := entries(ctx, c.path("eval/results"))
	if err != nil {
		return a, err
	}
	slices.SortFunc(list, func(a, b os.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	total, rows := 0, 0
	for _, item := range list {
		if err := ctx.Err(); err != nil {
			return a, err
		}
		name := item.Name()
		if strings.HasPrefix(name, ".") || !strings.HasSuffix(name, "-baseline.json") {
			continue
		}
		a.Harnesses++
		baseline, valid, err := readEval(ctx, c.path("eval/results/"+name), &total, &rows)
		if err != nil {
			return a, err
		}
		if !valid {
			continue
		}
		current, valid, err := readEval(ctx, c.path("eval/results/"+strings.TrimSuffix(name, "-baseline.json")+"-current.json"), &total, &rows)
		if err != nil {
			return a, err
		}
		byTask := map[string]any{}
		if valid {
			for _, item := range current.tasks {
				byTask[item.name] = item.score
			}
			if baseline.fingerprint != "" && current.fingerprint != "" && baseline.fingerprint != current.fingerprint {
				a.Stale++
			}
		}
		for _, item := range baseline.tasks {
			if err := ctx.Err(); err != nil {
				return a, err
			}
			mock := baseline.harness == "mock"
			a.Tasks = append(a.Tasks, task{baseline.harness, item.name, item.score, byTask[item.name], mock})
			if !mock {
				a.Measured++
			}
		}
	}
	return a, nil
}

func readEval(ctx context.Context, path string, total, rows *int) (evalFile, bool, error) {
	data, err := optionalRead(ctx, path)
	if err != nil {
		return evalFile{}, false, err
	}
	*total += len(data)
	if *total > 64<<20 {
		return evalFile{}, false, errors.New("aggregate eval input limit exceeded")
	}
	if len(data) == 0 {
		return evalFile{}, false, nil
	}
	// JSON extensions and invalid Unicode are not valid versioned metrics data.
	if !utf8.Valid(data) || !json.Valid(data) {
		return evalFile{}, false, nil
	}
	value, err := jsonvalue.Decode(ctx, data)
	if err != nil {
		if ctx.Err() != nil {
			return evalFile{}, false, ctx.Err()
		}
		return evalFile{}, false, nil
	}
	doc, ok := value.(map[string]any)
	if !ok {
		return evalFile{}, false, nil
	}
	result := evalFile{harness: "?", tasks: []evalTask{}}
	if harness, present := doc["harness"]; present {
		result.harness, ok = harness.(string)
		if !ok || !utf8.ValidString(result.harness) {
			return evalFile{}, false, nil
		}
	}
	if fingerprint, present := doc["inputs_fingerprint"]; present && fingerprint != nil {
		result.fingerprint, ok = fingerprint.(string)
		if !ok {
			return evalFile{}, false, nil
		}
	}
	items := []any{}
	if raw, present := doc["tasks"]; present {
		items, ok = raw.([]any)
		if !ok {
			return evalFile{}, false, nil
		}
	}
	*rows += len(items)
	if *rows > entryLimit {
		return evalFile{}, false, errors.New("aggregate eval task row limit exceeded")
	}
	for _, raw := range items {
		if err := ctx.Err(); err != nil {
			return evalFile{}, false, err
		}
		item, ok := raw.(map[string]any)
		if !ok {
			return evalFile{}, false, nil
		}
		name, ok := item["task"].(string)
		if !ok || !utf8.ValidString(name) {
			return evalFile{}, false, nil
		}
		score, ok := finiteScore(item["score"])
		if !ok {
			return evalFile{}, false, nil
		}
		result.tasks = append(result.tasks, evalTask{name, score})
	}
	return result, true, nil
}

func finiteScore(value any) (any, bool) {
	if value == nil {
		return nil, true
	}
	number, ok := value.(json.Number)
	if !ok {
		return nil, false
	}
	if !strings.ContainsAny(string(number), ".eE") {
		return number, true
	}
	f, err := number.Float64()
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return nil, false
	}
	return json.Number(jsonvalue.FloatText(f)), true
}

func scoreText(value any) string {
	if value == nil {
		return "None"
	}
	return fmt.Sprint(value)
}
