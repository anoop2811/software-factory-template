package metricscmd

import (
	"context"
	"errors"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/anoop2811/software-factory-template/internal/config"
)

var blockingExit = regexp.MustCompile(`(?m)^[\t\v\f\r ]*exit[\t\v\f\r ]+[1-9]`)
var quotedEvidence = regexp.MustCompile("`[^`]+`")

func (c collector) collect(ctx context.Context, o options) (metrics, error) {
	m := metrics{Schema: "factory.metrics/v1", WindowDays: o.days, NotMeasured: []string{
		"token spend per role — your harness owns that; the factory does not meter it",
		"code quality — not honestly measurable without judgment, so it is not claimed",
	}}
	if err := c.gates(ctx, &m.Enforcement); err != nil {
		return m, err
	}
	if err := c.events(ctx, o.days, &m.Enforcement); err != nil {
		return m, err
	}
	if err := c.history(ctx, o.digits, &m.Loop, &m.Verification); err != nil {
		return m, err
	}
	a, err := c.evals(ctx)
	m.Agents = a
	return m, err
}

func (c collector) gates(ctx context.Context, e *enforcement) error {
	path := c.path("scripts/hooks")
	info, err := os.Lstat(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil && !info.Mode().IsRegular() {
		list, err := entries(ctx, path)
		if err != nil {
			return err
		}
		for _, entry := range list {
			if err := ctx.Err(); err != nil {
				return err
			}
			name := entry.Name()
			if !strings.HasSuffix(name, ".sh") {
				continue
			}
			if info.Mode()&os.ModeSymlink == 0 {
				e.Installed++
			}
			if strings.HasPrefix(name, ".") || entry.IsDir() {
				continue
			}
			if target, err := os.Stat(path + "/" + name); err == nil && target.IsDir() {
				continue
			}
			data, err := optionalRead(ctx, path+"/"+name)
			if err != nil {
				return err
			}
			text := string(data)
			if strings.Contains(text, "factory_log_event") {
				e.Reporting++
			} else if !strings.Contains(text, "factory: no-block-event") && blockingExit.MatchString(text) {
				e.Mute++
			}
		}
	}
	cfg := c.environment["FACTORY_CONFIG"]
	if cfg == "" {
		cfg = "factory.yaml"
	}
	data, err := optionalRead(ctx, c.path(cfg))
	if err != nil {
		return err
	}
	for _, key := range []string{"test_file_patterns", "citation_prefix", "protected_paths", "check_command"} {
		value, err := config.GetBytes(ctx, data, key, "")
		if err != nil {
			return err
		}
		if value != "" {
			e.Armed++
		} else {
			e.Inert++
		}
	}
	return nil
}

func (c collector) events(ctx context.Context, days int, e *enforcement) error {
	e.ByGate = []gateBlocks{}
	path := c.environment["FACTORY_EVENT_LOG"]
	if path == "" {
		path = ".factory/events.log"
	}
	data, err := optionalRead(ctx, c.path(path))
	if err != nil {
		return err
	}
	text := string(data)
	if strings.ContainsRune(text, 0) {
		return errors.New("unsafe NUL-bearing event log")
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -days).Format("2006-01-02")
	byGate := map[string]int{}
	byHour := map[string]int{}
	for text != "" {
		if err := ctx.Err(); err != nil {
			return err
		}
		line, rest, _ := strings.Cut(text, "\n")
		text = rest
		if line != "" {
			e.Total++
		}
		ts, tail, _ := strings.Cut(line, "\t")
		if ts < cutoff {
			continue
		}
		if line != "" {
			e.Window++
		}
		gate, _, _ := strings.Cut(tail, "\t")
		byGate[gate]++
		hour := []rune(ts)
		if len(hour) > 13 {
			hour = hour[:13]
		}
		byHour[string(hour)+"\t"+gate]++
	}
	for gate, count := range byGate {
		e.ByGate = append(e.ByGate, gateBlocks{gate, count})
	}
	slices.SortFunc(e.ByGate, func(a, b gateBlocks) int {
		if a.Blocks != b.Blocks {
			return b.Blocks - a.Blocks
		}
		return strings.Compare(b.Gate, a.Gate)
	})
	for _, count := range byHour {
		if count >= 3 {
			e.Repeat++
		}
	}
	return ctx.Err()
}

func (c collector) history(ctx context.Context, days string, l *loopMetrics, v *verification) error {
	since := "--since=" + days + " days ago"
	for _, q := range []struct {
		args   []string
		target *int
	}{
		{[]string{"rev-list", "--count", since, "HEAD"}, &l.Commits},
		{[]string{"rev-list", "--count", "--merges", since, "HEAD"}, &l.Merges},
	} {
		text, err := c.git(ctx, q.args...)
		if err != nil {
			return err
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		n, err := strconv.Atoi(text)
		if err != nil || n < 0 {
			return errors.New("invalid git metrics count")
		}
		*q.target = n
	}
	subjects, err := c.git(ctx, "log", since, "--format=%s")
	if err != nil {
		return err
	}
	for _, line := range strings.Split(subjects, "\n") {
		if strings.HasPrefix(strings.ToLower(line), "revert") {
			l.Reverts++
		}
	}
	authors, err := c.git(ctx, "log", since, "--format=%an")
	if err != nil {
		return err
	}
	unique := map[string]bool{}
	for _, line := range strings.Split(authors, "\n") {
		if line != "" {
			unique[line] = true
		}
	}
	l.Authors = len(unique)
	paths, err := c.git(ctx, "log", since, "--name-only", "--format=")
	if err != nil {
		return err
	}
	counts := map[string]int{}
	for _, line := range strings.Split(paths, "\n") {
		if err := ctx.Err(); err != nil {
			return err
		}
		if line != "" {
			counts[line]++
		}
	}
	l.Touched = len(counts)
	for _, count := range counts {
		if count > 1 {
			l.Reworked++
		}
	}
	messages, err := c.git(ctx, "log", since, "--format=%B%x00")
	if err != nil {
		return err
	}
	for _, message := range strings.Split(messages, "\x00") {
		if err := ctx.Err(); err != nil {
			return err
		}
		if hasClaim(message) {
			v.Claims++
			if quotedEvidence.MatchString(message) || strings.Contains(message, "→") || strings.Contains(message, "exit:") || strings.Contains(message, "NOT verified") || strings.Contains(message, "unverified") {
				v.Cited++
			}
		}
	}
	return ctx.Err()
}

// Python's word class includes Unicode letters and numbers, not just ASCII;
// its case-insensitive I also includes both dotted and dotless variants.
// docs/adr/0088-go-native-metrics.md:59.
func hasClaim(message string) bool {
	for _, word := range strings.FieldsFunc(message, func(r rune) bool { return r != '_' && !unicode.IsLetter(r) && !unicode.IsNumber(r) }) {
		word = strings.Map(func(r rune) rune {
			if r == 'İ' || r == 'ı' {
				return 'i'
			}
			return r
		}, word)
		if strings.EqualFold(word, "verified") || strings.EqualFold(word, "fixed") || strings.EqualFold(word, "works") {
			return true
		}
	}
	return false
}
