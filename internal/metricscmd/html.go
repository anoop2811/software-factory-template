package metricscmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/anoop2811/software-factory-template/internal/fileinput"
	"github.com/anoop2811/software-factory-template/internal/filepublish"
	"github.com/anoop2811/software-factory-template/internal/output"
)

func (c collector) html(ctx context.Context, invocation string, out io.Writer, o options, data metrics) error {
	template, err := fileinput.Read(ctx, filepath.Join(invocation, "templates/metrics.html"), fileLimit)
	if err != nil {
		return fmt.Errorf("read HTML template: %w", err)
	}
	placeholder := []byte("/*__FACTORY_METRICS_JSON__*/null")
	if bytes.Count(template, placeholder) != 1 {
		return errors.New("HTML template requires exactly one placeholder")
	}
	encoded, err := encodeJSON(ctx, data)
	if err != nil {
		return err
	}
	// Escape all HTML tokenizer openers, including comments, as valid JSON.
	// Check expansion before allocation. docs/adr/0088-go-native-metrics.md:98.
	expansion := 5 * bytes.Count(encoded, []byte("<"))
	if len(encoded)+expansion > fileLimit || len(template)-len(placeholder)+len(encoded)+expansion > fileLimit {
		return errors.New("generated HTML limit exceeded")
	}
	encoded = bytes.ReplaceAll(encoded, []byte("<"), []byte("\\u003c"))
	page := bytes.Replace(template, placeholder, encoded, 1)
	if err := publishHTML(ctx, c.root, page); err != nil {
		return err
	}
	path := c.path(".factory/metrics.html")
	if err := output.WriteEvent(ctx, out, []byte("factory metrics: wrote "+path+"\n")); err != nil {
		return err
	}
	return c.browser(ctx, out, o, path)
}

func ordinaryPage(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && info.Mode().IsRegular() && info.Mode() == info.Mode().Perm() && stat.Nlink == 1
}
func samePage(a, b os.FileInfo) bool {
	return ordinaryPage(a) && ordinaryPage(b) && os.SameFile(a, b) && a.Size() == b.Size() && a.Mode() == b.Mode() && a.ModTime().Equal(b.ModTime())
}

// Publication is confined to pinned directories, with identity checks before
// replacement and before removing owned temporaries. docs/adr/0088-go-native-metrics.md:102.
func publishHTML(ctx context.Context, path string, page []byte) (returned error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	project, err := os.OpenRoot(path)
	if err != nil {
		return err
	}
	defer func() { _ = project.Close() }()
	projectInfo, err := project.Stat(".")
	if err != nil {
		return err
	}
	info, err := project.Lstat(".factory")
	if errors.Is(err, os.ErrNotExist) {
		if err = project.Mkdir(".factory", 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		info, err = project.Lstat(".factory")
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("unsafe metrics destination directory")
	}
	directory, err := project.OpenRoot(".factory")
	if err != nil {
		return err
	}
	defer func() { _ = directory.Close() }()
	opened, err := directory.Stat(".")
	if err != nil {
		return err
	}
	if !os.SameFile(info, opened) {
		return errors.New("metrics destination directory changed")
	}
	checkDirectory := func() error {
		current, err := os.Stat(path)
		if err != nil || !os.SameFile(projectInfo, current) {
			return errors.New("metrics project directory changed")
		}
		current, err = project.Lstat(".factory")
		if err != nil || !current.IsDir() || !os.SameFile(info, current) {
			return errors.New("metrics destination directory changed")
		}
		return nil
	}
	if err := checkDirectory(); err != nil {
		return err
	}
	original, err := directory.Lstat("metrics.html")
	existed := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	mode := os.FileMode(0600)
	if existed {
		if !ordinaryPage(original) {
			return errors.New("unsafe metrics destination")
		}
		mode = original.Mode().Perm()
	}
	stage, err := filepublish.Prepare(ctx, directory, ".metrics-", page, mode)
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, stage.Cleanup()) }()
	if err := checkDirectory(); err != nil {
		return err
	}
	current, err := directory.Lstat("metrics.html")
	if existed {
		if err != nil || !samePage(original, current) {
			return errors.New("metrics destination changed")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.New("metrics destination appeared during publication")
	}
	return stage.Publish(ctx, "metrics.html")
}
