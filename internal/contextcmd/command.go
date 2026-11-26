// Package contextcmd previews local task material without invoking a harness.
package contextcmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/anoop2811/software-factory-template/internal/output"
	"github.com/spf13/cobra"
)

// Decision 185. specs/003-task-context-and-skills.md:465.
const (
	defaultPayloadBytes = 32 << 10
	maxPayloadBytes     = 1 << 20
	fileLimit           = 1 << 20
	totalLimit          = 4 << 20
	selectionLimit      = 128
	diagnosticLimit     = 1024
)

type invalidRequest string

func (e invalidRequest) Error() string { return string(e) }

func invalid(format string, args ...any) error {
	return invalidRequest(fmt.Sprintf(format, args...))
}

func safeName(s string) string { return strconv.Quote(s) }

// Run owns the new command's arguments and exit statuses. Spec 003, §8:
// specs/003-task-context-and-skills.md:371.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	var files []string
	var maxBytes int
	var asJSON bool
	root := &cobra.Command{
		Use: "context", Short: "Inspect project material for a task",
		SilenceErrors: true, SilenceUsage: true, DisableSuggestions: true,
		Args:              cobra.ArbitraryArgs,
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
		RunE:              func(*cobra.Command, []string) error { return invalid("expected context preview --file PATH") },
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs(append([]string{}, args...))
	cmd := &cobra.Command{
		Use: "preview --file PATH", Short: "Preview explicit files from this Git worktree",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 0 {
				return invalid("use --file PATH for each selection")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := previewFiles(cmd.Context(), files, maxBytes)
			if err != nil {
				return err
			}
			var data []byte
			if asJSON {
				data, err = json.Marshal(p)
				if err != nil {
					return err
				}
				data = append(data, '\n')
			} else {
				data = []byte(p.human())
			}
			return output.WriteEvent(cmd.Context(), stdout, data)
		},
	}
	cmd.Flags().StringArrayVar(&files, "file", nil, "Worktree-relative file (repeatable)")
	cmd.Flags().IntVar(&maxBytes, "max-bytes", defaultPayloadBytes, "Maximum canonical payload bytes")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit the v1 preview object")
	root.SetFlagErrorFunc(func(*cobra.Command, error) error {
		return invalid("invalid option or value; see context preview --help")
	})
	root.AddCommand(cmd)
	err := root.ExecuteContext(ctx)
	if err == nil {
		return 0
	}
	status := 1
	var invalidInput invalidRequest
	if errors.As(err, &invalidInput) {
		status = 2
	}
	message := "factory context: " + err.Error()
	if len(message) >= diagnosticLimit {
		message = message[:diagnosticLimit-4]
		for !utf8.ValidString(message) {
			message = message[:len(message)-1]
		}
		message += "..."
	}
	_, _ = fmt.Fprintln(stderr, message)
	return status
}

func (p preview) human() string {
	var out strings.Builder
	fmt.Fprintf(&out, "Context preview · %d payload bytes · %d omitted\nDigest %s\n", p.PayloadBytes, len(p.Omitted), p.Digest)
	for i, item := range p.Payload.Items {
		c := item.Citations[0]
		fmt.Fprintf(&out, "\nSource evidence (%s): %s", p.Included[i].Reason, safeName(c.Path))
		if c.Lines != nil {
			fmt.Fprintf(&out, " lines %d–%d", c.Lines[0], c.Lines[1])
		}
		fmt.Fprintf(&out, "\nSHA-256 %s\n", c.SHA256)
		for line := range strings.Lines(item.Text) {
			quoted := strconv.Quote(strings.TrimSuffix(line, "\n"))
			fmt.Fprintf(&out, "  | %s\n", quoted[1:len(quoted)-1])
		}
	}
	return out.String()
}
