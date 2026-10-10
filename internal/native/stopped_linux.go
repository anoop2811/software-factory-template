package native

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

func processStopped(ctx context.Context, pid int) (result bool, returned error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	file, err := os.Open("/proc/" + strconv.Itoa(pid) + "/stat")
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer func() { returned = errors.Join(returned, file.Close()) }()
	data, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil {
		return false, err
	}
	end := strings.LastIndexByte(string(data), ')')
	if len(data) > 4096 || end < 0 || end+2 >= len(data) {
		return false, fmt.Errorf("invalid owned child process state")
	}
	// The kernel documents pid, command then state; T denotes stopped/traced.
	// https://docs.kernel.org/filesystems/proc.html (fetched 2026-10-09).
	state := data[end+2]
	return state == 'T' || state == 't', ctx.Err()
}
