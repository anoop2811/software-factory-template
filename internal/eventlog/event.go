// Package eventlog records best-effort gate events with bounded retention.
package eventlog

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

// Append keeps the newest half only after the configured line cap is exceeded.
// An empty root disables the implicit log outside Git; callers may ignore errors.
// docs/adr/0099-native-factory-orchestration.md:138.
func Append(ctx context.Context, root string, environment map[string]string, gate, reason string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path := environment["FACTORY_EVENT_LOG"]
	if path == "" {
		if root == "" {
			return nil
		}
		path = root + "/.factory/events.log"
	}
	if gate == "" {
		gate = "gate"
	}
	if reason == "" {
		reason = "blocked"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	// #nosec G304 -- Caller-selected event log; nonblocking open and regular-file check reject special files.
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY|syscall.O_NONBLOCK, 0600)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return errors.Join(err, errors.New("event log is not a regular file"), file.Close())
	}
	_, writeErr := fmt.Fprintf(file, "%s\t%s\t%s\n", time.Now().UTC().Format("2006-01-02T15:04:05Z"), gate, reason)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return err
	}
	maximum, err := strconv.ParseInt(environment["FACTORY_EVENT_MAX_LINES"], 10, 64)
	if err != nil || maximum <= 0 {
		maximum = 5000
	}
	return retain(ctx, path, maximum)
}

// retain scans and copies fixed-size chunks, including logs with oversized rows.
// docs/adr/0099-native-factory-orchestration.md:248.
func retain(ctx context.Context, path string, maximum int64) error {
	// #nosec G304 -- Caller-selected log; nonblocking open followed by regular-file validation.
	input, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer func() { _ = input.Close() }()
	before, err := input.Stat()
	if err != nil || !before.Mode().IsRegular() {
		return errors.Join(err, errors.New("event log is not a regular file"))
	}
	buffer := make([]byte, 32<<10)
	start, trim, err := retainedStart(ctx, input, before.Size(), maximum, buffer)
	if err != nil || !trim {
		return err
	}
	directory, err := os.MkdirTemp(filepath.Dir(path), ".events.")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(directory) }()
	temporary := directory + "/trim"
	// #nosec G304 -- Exclusive file in the private directory created above.
	output, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	copyErr := copyRetained(ctx, output, io.NewSectionReader(input, start, before.Size()-start), buffer)
	if err := errors.Join(copyErr, output.Close()); err != nil {
		return err
	}
	after, err := input.Stat()
	if err != nil {
		return err
	}
	if before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) || before.Mode() != after.Mode() {
		return errors.New("event log changed during retention")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

// Count newline-terminated rows like wc -l, locating the suffix like tail -n.
// A partial final row counts toward the retained suffix, but not the line cap.
func retainedStart(ctx context.Context, file *os.File, size, maximum int64, buffer []byte) (int64, bool, error) {
	keep := max(maximum/2, 1)
	var lines, boundaries, start int64
	for end := size; end > 0; {
		if err := ctx.Err(); err != nil {
			return 0, false, err
		}
		offset := max(end-int64(len(buffer)), 0)
		chunk := buffer[:end-offset]
		if _, err := file.ReadAt(chunk, offset); err != nil {
			return 0, false, err
		}
		if end == size && chunk[len(chunk)-1] != '\n' {
			boundaries = 1
		}
		for i := len(chunk) - 1; i >= 0; i-- {
			if chunk[i] != '\n' {
				continue
			}
			lines++
			boundaries++
			if boundaries == keep+1 {
				start = offset + int64(i) + 1
			}
			if lines > maximum {
				return start, true, nil
			}
		}
		end = offset
	}
	return 0, false, nil
}

func copyRetained(ctx context.Context, output io.Writer, input io.Reader, buffer []byte) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, readErr := input.Read(buffer)
		if n > 0 {
			if _, err := output.Write(buffer[:n]); err != nil {
				return err
			}
		}
		if errors.Is(readErr, io.EOF) {
			return ctx.Err()
		}
		if readErr != nil {
			return readErr
		}
	}
}
