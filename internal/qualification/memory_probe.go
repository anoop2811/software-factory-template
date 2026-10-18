package qualification

import (
	"errors"
	"io"
	"io/fs"
	"path"
	"strconv"
	"strings"
)

// linuxMemory includes remaining allowance of visible ancestor hard limits.
// Kernel cgroup-v2 memory.max/current are absent at the genuine hierarchy root.
// docs/adr/0104-adaptive-qualification-workers.md:45.
func linuxMemory(files fs.FS) (uint64, error) {
	body, err := probeRead(files, "proc/meminfo")
	if err != nil {
		return 0, err
	}
	available, err := memAvailable(body)
	if err != nil {
		return 0, err
	}
	body, err = probeRead(files, "proc/self/cgroup")
	if err != nil {
		return 0, err
	}
	current, err := unifiedGroup(body)
	if err != nil {
		return 0, err
	}
	body, err = probeRead(files, "proc/self/mountinfo")
	if err != nil {
		return 0, err
	}
	mount, err := unifiedMount(body)
	if err != nil {
		return 0, err
	}
	directory := path.Join(mount, current)
	// path.Join does not discard mount for an absolute current path in Go.
	// Both operands are canonical absolute paths, with no escaping proc entries.
	for depth := 0; depth < 128; depth++ {
		limit, err := probeRead(files, strings.TrimPrefix(path.Join(directory, "memory.max"), "/"))
		switch {
		case errors.Is(err, fs.ErrNotExist) && directory == mount:
			// unifiedMount accepts only a '/' hierarchy root. A namespaced non-root
			// with memory.max present must still be enforced, even at the mountpoint.
		case err != nil:
			return 0, err
		case strings.TrimSpace(limit) == "max":
		default:
			maximum, err := probeUint(limit)
			if err != nil {
				return 0, err
			}
			usage, err := probeRead(files, strings.TrimPrefix(path.Join(directory, "memory.current"), "/"))
			if err != nil {
				return 0, err
			}
			used, err := probeUint(usage)
			if err != nil {
				return 0, err
			}
			remaining := uint64(0)
			if used < maximum {
				remaining = maximum - used
			}
			available = min(available, remaining)
		}
		if directory == mount {
			return available, nil
		}
		directory = path.Dir(directory)
	}
	return 0, errors.New("cgroup hierarchy exceeds probe bound")
}

func probeRead(files fs.FS, name string) (string, error) {
	file, err := files.Open(name)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	const limit = 64 << 10
	body, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return "", err
	}
	if len(body) > limit {
		return "", errors.New("resource metadata exceeds probe bound")
	}
	return string(body), nil
}

func memAvailable(body string) (uint64, error) {
	found := false
	var available uint64
	for line := range strings.SplitSeq(body, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != "MemAvailable:" {
			continue
		}
		if found || len(fields) != 3 || fields[2] != "kB" {
			return 0, errors.New("invalid MemAvailable")
		}
		n, err := probeUint(fields[1])
		if err != nil || n > ^uint64(0)/1024 {
			return 0, errors.New("invalid MemAvailable")
		}
		available, found = n*1024, true
	}
	if !found {
		return 0, errors.New("MemAvailable unavailable")
	}
	return available, nil
}

func unifiedGroup(body string) (string, error) {
	current := ""
	for line := range strings.SplitSeq(strings.TrimSpace(body), "\n") {
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 {
			return "", errors.New("invalid cgroup membership")
		}
		for controller := range strings.SplitSeq(parts[1], ",") {
			if controller == "memory" {
				return "", errors.New("cgroup-v1 memory is unsupported")
			}
		}
		if parts[0] != "0" || parts[1] != "" {
			continue
		}
		if current != "" || !canonicalAbsolute(parts[2]) {
			return "", errors.New("invalid unified cgroup path")
		}
		current = parts[2]
	}
	if current == "" {
		return "", errors.New("unified cgroup membership unavailable")
	}
	return current, nil
}

func unifiedMount(body string) (string, error) {
	mount := ""
	for line := range strings.SplitSeq(body, "\n") {
		left, right, found := strings.Cut(line, " - ")
		kind := strings.Fields(right)
		if !found || len(kind) == 0 || kind[0] != "cgroup2" {
			continue
		}
		fields := strings.Fields(left)
		if mount != "" || len(fields) < 6 || fields[3] != "/" || !canonicalAbsolute(fields[4]) {
			return "", errors.New("unsupported unified cgroup mount")
		}
		mount = fields[4]
	}
	if mount == "" {
		return "", errors.New("unified cgroup mount unavailable")
	}
	return mount, nil
}

func canonicalAbsolute(value string) bool {
	return strings.HasPrefix(value, "/") && path.Clean(value) == value && !strings.ContainsAny(value, "\\\t\r\n ")
}

func probeUint(value string) (uint64, error) {
	value = strings.TrimSpace(value)
	n, err := strconv.ParseUint(value, 10, 64)
	if err != nil || strconv.FormatUint(n, 10) != value {
		return 0, errors.New("invalid resource number")
	}
	return n, nil
}
