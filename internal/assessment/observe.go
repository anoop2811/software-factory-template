package assessment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

const assetLimit = 1 << 20

func ordinary(stat unix.Stat_t) bool {
	return stat.Mode&unix.S_IFMT == unix.S_IFREG && stat.Nlink == 1 && stat.Size >= 0 && stat.Size <= assetLimit
}
func unchanged(before, after unix.Stat_t) bool {
	return sameIdentity(before, after) && before.Mode == after.Mode && before.Nlink == after.Nlink && before.Size == after.Size && before.Mtim == after.Mtim
}

// Hash only bounded ordinary descriptors and recheck their named identity and
// ancestry afterward. docs/adr/0079-go-installation-reference-assessment.md:90.
func observe(ctx context.Context, root *os.File, reference referenceAsset, operations ops) (result Asset, returned error) {
	result = row(reference, "assessment_error", nil)
	rootInfo, err := descriptor(root)
	if err != nil {
		return result, nil
	}
	chain := directories{{file: root, identity: rootInfo}}
	defer func() {
		if !chain.valid() {
			result = row(reference, "unsafe", nil)
		}
		chain[1:].close()
	}()
	components := strings.Split(reference.Path, "/")
	for _, name := range components[:len(components)-1] {
		if err := ctx.Err(); err != nil {
			return Asset{}, err
		}
		before, err := named(chain.last(), name)
		if err != nil {
			return row(reference, classification(err), nil), nil
		}
		if before.Mode&unix.S_IFMT != unix.S_IFDIR {
			return row(reference, "unsafe", nil), nil
		}
		child, err := openAt(chain.last(), name, unix.O_RDONLY|unix.O_DIRECTORY)
		if err != nil {
			class := classification(err)
			if class == "missing" {
				class = "unsafe"
			}
			return row(reference, class, nil), nil
		}
		after, err := descriptor(child)
		if err != nil {
			_ = child.Close()
			return result, nil
		}
		if !sameIdentity(before, after) {
			_ = child.Close()
			return row(reference, "unsafe", nil), nil
		}
		chain = append(chain, directory{file: child, name: name, identity: after})
	}
	name := components[len(components)-1]
	parent := chain.last()
	before, err := named(parent, name)
	if err != nil {
		return row(reference, classification(err), nil), nil
	}
	if !ordinary(before) {
		return row(reference, "unsafe", nil), nil
	}
	open := operations.open
	if open == nil {
		open = openAt
	}
	file, err := open(parent, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC)
	if err != nil {
		class := classification(err)
		if class == "missing" {
			class = "unsafe"
		}
		return row(reference, class, nil), nil
	}
	defer file.Close()
	opened, err := descriptor(file)
	if err != nil {
		return result, nil
	}
	if !ordinary(opened) || !unchanged(before, opened) {
		return row(reference, "unsafe", nil), nil
	}
	read := operations.read
	if read == nil {
		read = func(file *os.File, buffer []byte) (int, error) { return file.Read(buffer) }
	}
	hash := sha256.New()
	var buffer [32 << 10]byte
	var size int64
	for {
		if err := ctx.Err(); err != nil {
			return Asset{}, err
		}
		n, readErr := read(file, buffer[:])
		if n < 0 || n > len(buffer) {
			return result, nil
		}
		size += int64(n)
		if size > assetLimit {
			return row(reference, "unsafe", nil), nil
		}
		_, _ = hash.Write(buffer[:n])
		if err := ctx.Err(); err != nil {
			return Asset{}, err
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil || n == 0 {
			return result, nil
		}
	}
	after, err := descriptor(file)
	if err != nil {
		return result, nil
	}
	current, err := named(parent, name)
	if err != nil || !ordinary(after) || !ordinary(current) || !unchanged(opened, after) || !unchanged(after, current) || size != after.Size {
		return row(reference, "unsafe", nil), nil
	}
	observed := &Observation{SHA256: hex.EncodeToString(hash.Sum(nil)), Mode: permissions(after), Bytes: size}
	class := "customized"
	if *observed == reference.Reference && after.Mode&07000 == 0 {
		class = "matching_reference"
	}
	return row(reference, class, observed), nil
}
