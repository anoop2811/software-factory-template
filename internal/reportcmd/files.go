package reportcmd

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"syscall"
)

func gateCount(ctx context.Context, path string) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return 0, err
	}
	if info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return 0, nil
	}
	if !info.IsDir() {
		return 0, errors.New("unsafe hooks directory")
	}
	// Pin the directory without following a replacement symlink or blocking on a FIFO.
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return 0, err
	}
	directory := os.NewFile(uintptr(fd), path)
	defer func() { _ = directory.Close() }()
	opened, err := directory.Stat()
	if err != nil {
		return 0, err
	}
	if !os.SameFile(info, opened) {
		return 0, errors.New("hooks directory changed during inspection")
	}
	entries, err := directory.ReadDir(4097)
	if err != nil && !errors.Is(err, io.EOF) {
		return 0, err
	}
	if len(entries) > 4096 {
		return 0, errors.New("hooks directory entry limit exceeded")
	}
	count := 0
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if strings.HasSuffix(entry.Name(), ".sh") {
			count++
		}
	}
	return count, ctx.Err()
}

func blockCount(ctx context.Context, events string) (uint64, error) {
	var count uint64
	for events != "" {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		line, rest, _ := strings.Cut(events, "\n")
		if line != "" {
			count++
		}
		events = rest
	}
	return count, nil
}

// Bash's tab-only IFS collapses delimiter runs for the first two assignments;
// the final variable retains interior delimiters and drops trailing tabs.
// docs/adr/0087-go-native-report.md:42.
func eventFields(line string) (string, string, string) {
	line = strings.Trim(line, "\t")
	ts, line, _ := strings.Cut(line, "\t")
	line = strings.TrimLeft(line, "\t")
	gate, reason, _ := strings.Cut(line, "\t")
	return ts, gate, strings.TrimLeft(reason, "\t")
}
