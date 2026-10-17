// Package eventlog records best-effort gate events with bounded retention.
package eventlog

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/anoop2811/software-factory-template/internal/fileinput"
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
	data, err := fileinput.Read(ctx, path, 64<<20)
	if err != nil || int64(bytes.Count(data, []byte{'\n'})) <= maximum {
		return err
	}
	keep := max(maximum/2, 1)
	start := len(data)
	for remaining := keep; remaining > 0 && start > 0; remaining-- {
		start = bytes.LastIndexByte(data[:start-1], '\n') + 1
	}
	directory, err := os.MkdirTemp(filepath.Dir(path), ".events.")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(directory) }()
	temporary := directory + "/trim"
	if err := os.WriteFile(temporary, data[start:], 0600); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}
