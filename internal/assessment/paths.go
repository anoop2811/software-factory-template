package assessment

import (
	"context"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

type directory struct {
	file     *os.File
	name     string
	identity unix.Stat_t
}
type directories []directory

func (d directories) last() *os.File { return d[len(d)-1].file }
func (d directories) close() {
	for i := len(d) - 1; i >= 0; i-- {
		_ = d[i].file.Close()
	}
}
func sameIdentity(a, b unix.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Mode&unix.S_IFMT == b.Mode&unix.S_IFMT
}
func descriptor(file *os.File) (unix.Stat_t, error) {
	var stat unix.Stat_t
	err := unix.Fstat(int(file.Fd()), &stat)
	return stat, err
}
func named(parent *os.File, name string) (unix.Stat_t, error) {
	var stat unix.Stat_t
	err := unix.Fstatat(int(parent.Fd()), name, &stat, unix.AT_SYMLINK_NOFOLLOW)
	return stat, err
}
func openAt(parent *os.File, name string, flags int) (*os.File, error) {
	fd, err := unix.Openat(int(parent.Fd()), name, flags|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), "assessment-entry"), nil
}
func (d directories) valid() bool {
	for i, entry := range d {
		current, err := descriptor(entry.file)
		if err != nil || !sameIdentity(current, entry.identity) {
			return false
		}
		if i == 0 {
			if entry.name == "" {
				continue
			}
			var location unix.Stat_t
			if unix.Fstatat(unix.AT_FDCWD, entry.name, &location, unix.AT_SYMLINK_NOFOLLOW) != nil || !sameIdentity(location, entry.identity) {
				return false
			}
		} else {
			location, err := named(d[i-1].file, entry.name)
			if err != nil || !sameIdentity(location, entry.identity) {
				return false
			}
		}
	}
	return true
}

// Every component is pinned without following links; traversal is refused before
// any normalization. docs/adr/0079-go-installation-reference-assessment.md:81.
func openRoot(ctx context.Context, path string) (directories, error) {
	if path == "" || strings.ContainsRune(path, 0) {
		return nil, failure(2, "invalid assessment root")
	}
	components := strings.Split(path, "/")
	for _, part := range components {
		if part == ".." {
			return nil, failure(2, "invalid assessment root")
		}
	}
	start := "."
	if strings.HasPrefix(path, "/") {
		start = "/"
	}
	fd, err := unix.Open(start, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, failure(1, "cannot open assessment root")
	}
	file := os.NewFile(uintptr(fd), "assessment-root")
	stat, err := descriptor(file)
	if err != nil {
		_ = file.Close()
		return nil, failure(1, "cannot open assessment root")
	}
	chain := directories{{file: file, name: start, identity: stat}}
	fail := func(err error) (directories, error) { chain.close(); return nil, err }
	for _, part := range components {
		if part == "" || part == "." {
			continue
		}
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		before, err := named(chain.last(), part)
		if err != nil {
			if classification(err) == "unsafe" {
				return fail(failure(2, "unsafe assessment root"))
			}
			return fail(failure(1, "cannot open assessment root"))
		}
		if before.Mode&unix.S_IFMT != unix.S_IFDIR {
			return fail(failure(2, "unsafe assessment root"))
		}
		child, err := openAt(chain.last(), part, unix.O_RDONLY|unix.O_DIRECTORY)
		if err != nil {
			if classification(err) == "unsafe" || classification(err) == "missing" {
				return fail(failure(2, "unsafe assessment root"))
			}
			return fail(failure(1, "cannot open assessment root"))
		}
		after, err := descriptor(child)
		if err != nil {
			_ = child.Close()
			return fail(failure(1, "cannot open assessment root"))
		}
		if !sameIdentity(before, after) {
			_ = child.Close()
			return fail(failure(2, "unsafe assessment root"))
		}
		chain = append(chain, directory{file: child, name: part, identity: after})
	}
	if !chain.valid() {
		return fail(failure(2, "unsafe assessment root"))
	}
	return chain, nil
}
