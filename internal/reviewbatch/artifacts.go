package reviewbatch

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/anoop2811/software-factory-template/internal/fileinput"
	"github.com/anoop2811/software-factory-template/internal/filepublish"
)

// Pin each directory descriptor and reject linked ancestors before mutation.
// Untrusted filenames never select an artifact name. docs/adr/0100-budgeted-batched-adversarial-review.md:44.
func privateDirectory(name string, exclusive bool) (*os.Root, error) {
	for _, component := range strings.Split(name, string(os.PathSeparator)) {
		if component == ".." {
			return nil, errors.New("parent traversal is not an artifact directory")
		}
	}
	abs, err := filepath.Abs(name)
	if err != nil || abs == string(os.PathSeparator) {
		return nil, errors.New("invalid artifact directory")
	}
	current, err := os.OpenRoot(string(os.PathSeparator))
	if err != nil {
		return nil, err
	}
	parts := strings.Split(strings.TrimPrefix(abs, string(os.PathSeparator)), string(os.PathSeparator))
	for index, part := range parts {
		last := index == len(parts)-1
		if last {
			err = current.Mkdir(part, 0700)
			if err != nil && (exclusive || !errors.Is(err, os.ErrExist)) {
				_ = current.Close()
				return nil, errors.New("artifact directory must be new and unlinked")
			}
		}
		before, statErr := current.Lstat(part)
		if statErr != nil || !before.IsDir() || before.Mode()&os.ModeSymlink != 0 {
			_ = current.Close()
			return nil, errors.New("linked or unavailable artifact directory")
		}
		next, openErr := current.OpenRoot(part)
		_ = current.Close()
		if openErr != nil {
			return nil, errors.New("cannot pin artifact directory")
		}
		after, statErr := next.Stat(".")
		if statErr != nil || !os.SameFile(before, after) || (last && (after.Mode().Perm()&0077 != 0 || !owned(after))) {
			_ = next.Close()
			return nil, errors.New("artifact directory changed or is not private")
		}
		current = next
	}
	return current, nil
}

func owned(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid()) // #nosec G115 -- geteuid returns the platform uid represented by Stat_t.Uid.
}

func publish(directory *os.Root, name string, data []byte) error {
	// Cancellation still requires a truthful final report; publication has its own
	// short bound and never starts another network request.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	stage, err := filepublish.Prepare(ctx, directory, ".review-", data, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = stage.Cleanup() }()
	return stage.Publish(ctx, name)
}

type cachedResult struct {
	Identity string          `json:"identity"`
	Findings string          `json:"findings"`
	Usage    json.RawMessage `json:"usage"`
}

type cacheEnvelope struct {
	Version int          `json:"version"`
	Result  cachedResult `json:"result"`
	MAC     string       `json:"mac"`
}

func signature(key string, result cachedResult) string {
	derive := hmac.New(sha256.New, []byte(key))
	_, _ = derive.Write([]byte("factory-review-cache-v1"))
	mac := hmac.New(sha256.New, derive.Sum(nil))
	_, _ = mac.Write(jsonBytes(result))
	return hex.EncodeToString(mac.Sum(nil))
}

func loadCache(ctx context.Context, directory *os.Root, id, key string) (cachedResult, bool) {
	if directory == nil {
		return cachedResult{}, false
	}
	file, err := directory.OpenFile(id+".json", os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return cachedResult{}, false
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || !owned(info) {
		return cachedResult{}, false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Nlink != 1 {
		return cachedResult{}, false
	}
	data, err := fileinput.ReadFile(ctx, file, 3<<20)
	if err != nil {
		return cachedResult{}, false
	}
	var entry cacheEnvelope
	if json.Unmarshal(data, &entry) != nil || entry.Version != 1 || entry.Result.Identity != id || strings.TrimSpace(entry.Result.Findings) == "" || len(entry.Result.Findings) > 2<<20 {
		return cachedResult{}, false
	}
	actual, err := hex.DecodeString(entry.MAC)
	expected, _ := hex.DecodeString(signature(key, entry.Result))
	if err != nil || !hmac.Equal(actual, expected) {
		return cachedResult{}, false
	}
	return entry.Result, true
}

func saveCache(directory *os.Root, b batch, key string) error {
	if directory == nil {
		return nil
	}
	result := cachedResult{Identity: b.ID, Findings: b.Findings, Usage: b.Usage}
	return publish(directory, b.ID+".json", jsonBytes(cacheEnvelope{Version: 1, Result: result, MAC: signature(key, result)}))
}

func reportedUsage(raw json.RawMessage) json.RawMessage {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return nil
	}
	clean := map[string]float64{}
	for _, key := range []string{"prompt_tokens", "completion_tokens", "total_tokens", "cost"} {
		var value *float64
		if json.Unmarshal(fields[key], &value) == nil && value != nil && *value >= 0 && !math.IsInf(*value, 0) && !math.IsNaN(*value) {
			clean[key] = *value
		}
	}
	if len(clean) == 0 {
		return nil
	}
	return jsonBytes(clean)
}

func addUsage(total, next json.RawMessage) json.RawMessage {
	if len(next) == 0 {
		return nil
	}
	if len(total) == 0 {
		return append(json.RawMessage(nil), next...)
	}
	values := map[string]float64{}
	_ = json.Unmarshal(total, &values)
	if values == nil {
		values = map[string]float64{}
	}
	addition := map[string]float64{}
	_ = json.Unmarshal(next, &addition)
	for key, value := range values {
		increment, known := addition[key]
		if !known {
			delete(values, key)
			continue
		}
		values[key] = value + increment
	}
	if len(values) == 0 {
		return nil
	}
	return jsonBytes(values)
}

func (r report) markdown() []byte {
	var out strings.Builder
	fmt.Fprintf(&out, "## Adversarial review: %s\n\nRepository: `%s`\n\nBase: `%s`  \nHead: `%s`\n\n", r.Status, r.Repository, r.Base, r.Head)
	fmt.Fprintf(&out, "Coverage: %d/%d files, %d/%d fragments, %d/%d batches completed (%d cached).\n\n", r.Counts.FilesReviewed, r.Counts.FilesTotal, r.Counts.FragmentsReviewed, r.Counts.FragmentsTotal, r.Counts.BatchesCompleted, r.Counts.BatchesPlanned, r.Counts.BatchesCached)
	fmt.Fprintf(&out, "Budget reservations: %d/%d requests, %d/%d response tokens, %d/%d request bytes. These are allowances, not reported billing.\n\n", r.Budget.RequestsUsed, r.Budget.MaxRequests, r.Budget.TokensReserved, r.Budget.MaxTokens, r.Budget.InputReserved, r.Budget.MaxInput)
	if r.Error != "" {
		fmt.Fprintf(&out, "Reason: %s\n\n", r.Error)
	}
	for i, b := range r.Batches {
		fmt.Fprintf(&out, "### Batch %d: %s", i+1, b.Status)
		if b.Cached {
			out.WriteString(" (authenticated cache)")
		}
		out.WriteString("\n\n")
		for _, filename := range b.Files {
			fmt.Fprintf(&out, "- `%s`\n", filename)
		}
		out.WriteByte('\n')
		if b.Status == "complete" {
			out.WriteString(b.Findings)
			out.WriteString("\n\n")
		} else if b.Error != "" {
			fmt.Fprintf(&out, "%s\n\n", b.Error)
		}
	}
	for _, file := range r.Files {
		if file.Status != "complete" {
			fmt.Fprintf(&out, "Uncovered: `%s` (%s)", file.Path, file.Status)
			if file.Error != "" {
				fmt.Fprintf(&out, ": %s", file.Error)
			}
			out.WriteByte('\n')
		}
	}
	return []byte(out.String())
}

func writeReport(directory *os.Root, r report, stdout io.Writer) error {
	if err := publish(directory, "report.json", append(jsonBytes(r), '\n')); err != nil {
		return errors.New("cannot publish review report")
	}
	markdown := r.markdown()
	if err := publish(directory, "review.md", markdown); err != nil {
		return errors.New("cannot publish review markdown")
	}
	_, err := stdout.Write(markdown)
	return err
}
