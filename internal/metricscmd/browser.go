package metricscmd

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/anoop2811/software-factory-template/internal/output"
)

func (c collector) browser(ctx context.Context, out io.Writer, o options, path string) error {
	message := "  open it directly — it is a self-contained file, nothing is served.\n"
	if o.noOpen || c.environment["CI"] != "" || !terminal(out) {
		return output.WriteEvent(ctx, out, []byte(message))
	}
	argv := []string{}
	if browser := c.environment["BROWSER"]; browser != "" {
		if binary := c.executable(browser); binary != "" {
			argv = []string{binary}
		} else {
			fields := strings.FieldsFunc(browser, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\v' || r == '\f' })
			if len(fields) > 0 {
				if binary := c.executable(fields[0]); binary != "" {
					fields[0] = binary
					argv = fields
				}
			}
		}
	}
	if len(argv) == 0 {
		for _, name := range []string{"open", "xdg-open", "wslview"} {
			if binary := c.executable(name); binary != "" {
				argv = []string{binary}
				break
			}
		}
	}
	if len(argv) == 0 {
		return output.WriteEvent(ctx, out, []byte("  no opener found (tried $BROWSER, open, xdg-open, wslview) — open it\n  directly; it is a self-contained file, nothing is served.\n"))
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.launchBrowser(append(argv, path)) {
		message = "  opening it — pass --no-open to just write the file.\n"
	}
	return output.WriteEvent(ctx, out, []byte(message))
}

func (c collector) executable(name string) string {
	if strings.ContainsRune(name, '/') {
		path := c.path(name)
		if _, err := exec.LookPath(path); err == nil {
			return path
		}
		return ""
	}
	path, present := c.environment["PATH"]
	if !present {
		path = "/bin:/usr/bin"
	}
	for _, directory := range strings.Split(path, string(os.PathListSeparator)) {
		if directory == "" {
			directory = "."
		}
		candidate := c.path(directory + "/" + name)
		if _, err := exec.LookPath(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

// The explicitly requested browser session is a detached presentation handoff,
// rather than an owned computation. docs/adr/0088-go-native-metrics.md:114.
func (c collector) launchBrowser(argv []string) bool {
	null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return false
	}
	defer func() { _ = null.Close() }()
	command := exec.Command(argv[0], argv[1:]...) // #nosec G204 -- Literal caller-selected browser argv; never interpreted as a shell command.
	command.Dir = c.root
	command.Stdin = null
	command.Stdout = null
	command.Stderr = null
	command.Env = make([]string, 0, len(c.environment))
	for key, value := range c.environment {
		command.Env = append(command.Env, key+"="+value)
	}
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if command.Start() != nil {
		return false
	}
	go func() { _ = command.Wait() }()
	return true
}
