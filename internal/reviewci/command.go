// Package reviewci coordinates fixed-endpoint GitHub data and model coverage.
// docs/adr/0100-batched-review.md:103.
package reviewci

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/anoop2811/software-factory-template/internal/fileinput"
	"github.com/anoop2811/software-factory-template/internal/reviewbatch"
	"github.com/spf13/pflag"
)

const apiLimit = 8 << 20

var repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*/[A-Za-z0-9][A-Za-z0-9_.-]*$`)
var revisionPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

type options struct {
	pr                                    int
	repository, base, head, output, cache string
}

type pull struct {
	Number       int    `json:"number"`
	State        string `json:"state"`
	ChangedFiles int    `json:"changed_files"`
	Base         struct {
		SHA  string `json:"sha"`
		Ref  string `json:"ref"`
		Repo struct {
			FullName string `json:"full_name"`
		} `json:"repo"`
	} `json:"base"`
	Head struct {
		SHA  string `json:"sha"`
		Repo struct {
			FullName string `json:"full_name"`
		} `json:"repo"`
	} `json:"head"`
}

type client struct {
	http              *http.Client
	token, repository string
}

// Run never checks out PR content or executes a path supplied by its diff.
// docs/adr/0100-batched-review.md:106.
func Run(ctx context.Context, args []string, environment map[string]string, stdout, stderr io.Writer) int {
	var o options
	flags := pflag.NewFlagSet("review-batch github", pflag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.IntVar(&o.pr, "pr", 0, "same-repository pull request number")
	flags.StringVar(&o.repository, "repository", "", "owner/repository")
	flags.StringVar(&o.base, "expected-base", "", "expected base, or bind initial metadata")
	flags.StringVar(&o.head, "expected-head", "", "expected head, or bind current metadata")
	flags.StringVar(&o.output, "output", "", "new private result directory")
	flags.StringVar(&o.cache, "cache", "", "authenticated review cache directory")
	if flags.Parse(args) != nil || flags.NArg() != 0 || o.pr < 1 || o.pr > 1000000000 ||
		!repositoryPattern.MatchString(o.repository) || (o.base != "" && !revisionPattern.MatchString(o.base)) ||
		(o.head != "" && !revisionPattern.MatchString(o.head)) || o.output == "" {
		fmt.Fprintln(stderr, "review-batch github: invalid arguments")
		return 2
	}
	if strings.TrimSpace(environment["GITHUB_TOKEN"]) == "" {
		fmt.Fprintln(stderr, "review-batch github: missing GITHUB_TOKEN")
		return 1
	}
	if _, err := os.Lstat(o.output); err == nil || !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintln(stderr, "review-batch github: output must be a new directory")
		return 1
	}
	seconds := 1200
	if value := environment["REVIEW_TIMEOUT_SECONDS"]; value != "" {
		var err error
		seconds, err = strconv.Atoi(value)
		if err != nil || strings.Trim(value, "0123456789") != "" || seconds < 1 || seconds > 1200 {
			fmt.Fprintln(stderr, "review-batch github: invalid total deadline")
			return 2
		}
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(seconds)*time.Second)
	defer cancel()
	transport := &http.Transport{Proxy: http.ProxyFromEnvironment, DialContext: (&net.Dialer{Timeout: 15 * time.Second}).DialContext, TLSHandshakeTimeout: 15 * time.Second, ResponseHeaderTimeout: 15 * time.Second, MaxResponseHeaderBytes: 1 << 20}
	defer transport.CloseIdleConnections()
	c := client{http: &http.Client{Transport: transport, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, token: environment["GITHUB_TOKEN"], repository: o.repository}
	if err := c.run(ctx, o, environment, stdout, stderr); err != nil {
		fmt.Fprintln(stderr, "review-batch github:", err)
		return 1
	}
	return 0
}

func (c client) api(ctx context.Context, method, path, accept string, body any) ([]byte, error) {
	var encoded []byte
	if body != nil {
		var err error
		encoded, err = json.Marshal(body)
		if err != nil || len(encoded) > 1<<20 {
			return nil, errors.New("invalid GitHub payload")
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, "https://api.github.com/repos/"+c.repository+path, bytes.NewReader(encoded))
	if err != nil {
		return nil, errors.New("invalid GitHub request")
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("Accept", accept)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "software-factory-review")
	response, err := c.http.Do(request)
	if err != nil {
		return nil, errors.New("GitHub transport failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return nil, &apiStatusError{code: response.StatusCode}
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, apiLimit+1))
	if err != nil || len(data) > apiLimit || !utf8.Valid(data) {
		return nil, errors.New("GitHub response invalid or oversized")
	}
	return data, nil
}

func (c client) metadata(ctx context.Context, o options, branch string) (pull, error) {
	data, err := c.api(ctx, http.MethodGet, "/pulls/"+strconv.Itoa(o.pr), "application/vnd.github+json", nil)
	if err != nil {
		return pull{}, err
	}
	var p pull
	if json.Unmarshal(data, &p) != nil || p.Number != o.pr || p.State != "open" || p.ChangedFiles < 0 || p.ChangedFiles > 3000 ||
		p.Base.Ref != branch || !revisionPattern.MatchString(p.Base.SHA) || (o.base != "" && p.Base.SHA != o.base) || !revisionPattern.MatchString(p.Head.SHA) ||
		(o.head != "" && p.Head.SHA != o.head) ||
		!strings.EqualFold(p.Head.Repo.FullName, o.repository) || !strings.EqualFold(p.Base.Repo.FullName, o.repository) {
		return pull{}, errors.New("stale or unsupported pull request identity")
	}
	return p, nil
}

func (c client) run(ctx context.Context, o options, environment map[string]string, stdout, stderr io.Writer) (result error) {
	data, err := c.api(ctx, http.MethodGet, "", "application/vnd.github+json", nil)
	if err != nil {
		return err
	}
	var repository struct {
		FullName      string `json:"full_name"`
		DefaultBranch string `json:"default_branch"`
	}
	if json.Unmarshal(data, &repository) != nil || !strings.EqualFold(repository.FullName, o.repository) || repository.DefaultBranch == "" {
		return errors.New("invalid repository identity")
	}
	p, err := c.metadata(ctx, o, repository.DefaultBranch)
	if err != nil {
		return err
	}
	o.head = p.Head.SHA
	o.base = p.Base.SHA
	link := "https://github.com/" + o.repository + "/actions"
	if runID := environment["GITHUB_RUN_ID"]; len(runID) > 0 && len(runID) < 25 && strings.Trim(runID, "0123456789") == "" {
		link += "/runs/" + runID
	}
	created, err := c.api(ctx, http.MethodPost, "/check-runs", "application/vnd.github+json", map[string]any{"name": "Adversarial review coverage", "head_sha": o.head, "status": "in_progress", "details_url": link, "output": map[string]string{"title": "Review in progress", "summary": "Coverage is incomplete until all batches finish for this exact revision."}})
	if err != nil {
		return err
	}
	var check struct {
		ID int64 `json:"id"`
	}
	if json.Unmarshal(created, &check) != nil || check.ID <= 0 {
		return errors.New("invalid completion check identity")
	}
	conclusion, title, summary := "failure", "Review incomplete", "Review preparation or publication did not finish."
	defer func() {
		cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer stop()
		if ctx.Err() != nil {
			conclusion, title = "cancelled", "Review cancelled"
		}
		_, updateErr := c.api(cleanup, http.MethodPatch, "/check-runs/"+strconv.FormatInt(check.ID, 10), "application/vnd.github+json", map[string]any{"status": "completed", "conclusion": conclusion, "output": map[string]string{"title": title, "summary": summary}})
		if result == nil && updateErr != nil {
			result = updateErr
		}
	}()
	files, err := c.files(ctx, o.pr, p.ChangedFiles)
	if err != nil {
		return err
	}
	diff, err := c.completeDiff(ctx, o, files)
	if err != nil {
		return err
	}
	if _, err = c.metadata(ctx, o, repository.DefaultBranch); err != nil {
		return err
	}
	temporary, err := os.MkdirTemp("", "factory-review-data-")
	if err != nil {
		return errors.New("cannot prepare private review input")
	}
	defer os.RemoveAll(temporary)
	diffPath, filesPath := filepath.Join(temporary, "changes.diff"), filepath.Join(temporary, "files.json")
	if os.WriteFile(diffPath, diff, 0600) != nil || os.WriteFile(filesPath, files, 0600) != nil {
		return errors.New("cannot write private review input")
	}
	args := []string{"--diff", diffPath, "--files", filesPath, "--base", o.base, "--head", o.head, "--repository", o.repository, "--output", o.output}
	if o.cache != "" {
		args = append(args, "--cache", o.cache)
	}
	status := reviewbatch.Run(ctx, args, environment, stdout, stderr)
	reportBody, err := fileinput.Read(context.WithoutCancel(ctx), filepath.Join(o.output, "report.json"), apiLimit)
	if err != nil {
		return errors.New("review report unavailable")
	}
	var report struct {
		Version    int    `json:"version"`
		Status     string `json:"status"`
		Repository string `json:"repository"`
		Base       string `json:"base"`
		Head       string `json:"head"`
	}
	if json.Unmarshal(reportBody, &report) != nil || report.Repository != o.repository || report.Base != o.base || report.Head != o.head {
		return errors.New("review report identity mismatch")
	}
	markdown, err := fileinput.Read(context.WithoutCancel(ctx), filepath.Join(o.output, "review.md"), apiLimit)
	if err != nil {
		return errors.New("review findings artifact unavailable")
	}
	if _, err = c.metadata(ctx, o, repository.DefaultBranch); err != nil {
		return err
	}
	comment := "## Adversarial review (advisory)\n\nRepository: `" + o.repository + "`\nBase: `" + o.base + "`\nHead: `" + o.head + "`\nStatus: **" + report.Status + "**\n\n"
	if len(markdown) > 45000 {
		prefix := markdown[:45000]
		for !utf8.Valid(prefix) {
			prefix = prefix[:len(prefix)-1]
		}
		comment += "**Excerpt:** full findings and coverage are in the [run artifacts](" + link + ").\n\n" + string(prefix)
	} else {
		comment += string(markdown)
	}
	comment += "\n\n<sub>Advisory findings; completion measures model coverage, not approval. [Full report artifacts](" + link + ").</sub>"
	if _, err = c.api(ctx, http.MethodPost, "/issues/"+strconv.Itoa(o.pr)+"/comments", "application/vnd.github+json", map[string]string{"body": comment}); err != nil {
		return err
	}
	summary = "Repository: `" + o.repository + "`; head: `" + o.head + "`; reported status: **" + report.Status + "**. [Coverage and findings](" + link + ")."
	if report.Status == "complete" && status == 0 {
		conclusion, title = "success", "Review coverage complete"
		return nil
	}
	if report.Status == "skipped" {
		conclusion, title = "skipped", "Review skipped"
	}
	return errors.New("review coverage incomplete")
}

func (c client) files(ctx context.Context, pr, expected int) ([]byte, error) {
	all := make([]json.RawMessage, 0, expected)
	bytesRead := 0
	for page := 1; page <= 31; page++ {
		data, err := c.api(ctx, http.MethodGet, "/pulls/"+strconv.Itoa(pr)+"/files?per_page=100&page="+strconv.Itoa(page), "application/vnd.github+json", nil)
		if err != nil {
			return nil, err
		}
		bytesRead += len(data)
		if bytesRead > apiLimit {
			return nil, errors.New("changed-file manifest oversized")
		}
		var entries []json.RawMessage
		if json.Unmarshal(data, &entries) != nil || len(entries) > 100 || len(all)+len(entries) > expected || len(all)+len(entries) > 3000 {
			return nil, errors.New("incomplete changed-file manifest")
		}
		all = append(all, entries...)
		if len(entries) < 100 || len(all) == expected {
			if len(all) != expected {
				return nil, errors.New("incomplete changed-file manifest")
			}
			result, err := json.Marshal(all)
			if err != nil || len(result) > apiLimit {
				return nil, errors.New("changed-file manifest oversized")
			}
			return result, nil
		}
	}
	return nil, errors.New("changed-file manifest pagination limit")
}
