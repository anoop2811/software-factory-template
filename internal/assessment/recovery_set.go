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

func inspectRecoverySet(ctx context.Context, parent *os.File, name string, operations ops) (result RecoverySet, returned error) {
	var pins recoveryPins
	defer func() {
		if !pins.valid() {
			result = recoveryRow(name, "unsafe", result.Held)
		}
		pins.close()
	}()
	result, _, returned = inspectRecoverySetPinned(ctx, parent, name, operations, &pins)
	return result, returned
}

// Caller-owned pins keep saved identities valid across later installed observations.
// docs/adr/0092-go-recovery-restoration-planning.md:44.
func inspectRecoverySetPinned(ctx context.Context, parent *os.File, name string, operations ops, pins *recoveryPins) (result RecoverySet, manifest recoveryManifest, returned error) {
	result = recoveryRow(name, "unrecognized", nil)
	pin, class := openRecoveryDirectory(ctx, parent, name, false, operations)
	if err := ctx.Err(); err != nil {
		if pin.file != nil {
			_ = pin.file.Close()
		}
		return RecoverySet{}, recoveryManifest{}, err
	}
	if class != "" {
		if class == "missing" {
			class = "unsafe"
		}
		return recoveryRow(name, class, nil), recoveryManifest{}, nil
	}
	*pins = append(*pins, pin)
	if !recoveryID(name) {
		return result, recoveryManifest{}, nil
	}
	entries, class := recoveryEntries(ctx, pin.file, 32)
	if err := ctx.Err(); err != nil {
		return RecoverySet{}, recoveryManifest{}, err
	}
	if class != "" {
		return recoveryRow(name, class, nil), recoveryManifest{}, nil
	}
	data, filePin, class := readRecoveryFile(ctx, pin.file, "manifest.json", 16<<10, operations)
	if filePin.file != nil {
		*pins = append(*pins, filePin)
	}
	if err := ctx.Err(); err != nil {
		return RecoverySet{}, recoveryManifest{}, err
	}
	if class != "" {
		if class == "missing" {
			class = "unrecognized"
		}
		return recoveryRow(name, class, nil), recoveryManifest{}, nil
	}
	manifest, valid := parseRecoveryManifest(ctx, data, name)
	if err := ctx.Err(); err != nil {
		return RecoverySet{}, recoveryManifest{}, err
	}
	if !valid {
		return result, recoveryManifest{}, nil
	}
	held := manifest.held
	result = recoveryRow(name, "integrity_checked", &held)
	tree := &recoveryNode{children: map[string]*recoveryNode{}}
	for _, reference := range manifest.assets {
		node := tree
		for _, part := range strings.Split("files/"+reference.Path, "/") {
			if node.children == nil {
				node.children = map[string]*recoveryNode{}
			}
			if node.children[part] == nil {
				node.children[part] = &recoveryNode{}
			}
			node = node.children[part]
		}
		observation := reference.Reference
		node.reference = &observation
	}
	total := len(entries)
	class = walkRecovery(ctx, pin.file, tree, entries, 0, &total, pins, operations)
	if err := ctx.Err(); err != nil {
		return RecoverySet{}, recoveryManifest{}, err
	}
	if class != "" {
		return recoveryRow(name, class, &held), recoveryManifest{}, nil
	}
	result.FileCount = len(manifest.assets)
	for _, asset := range manifest.assets {
		result.Bytes += asset.Reference.Bytes
	}
	return result, manifest, nil
}

type recoveryNode struct {
	children  map[string]*recoveryNode
	reference *Observation
}

func mergeRecoveryClass(current, next string) string {
	priority := map[string]int{"": 0, "incomplete": 1, "unsafe": 2, "limit_exceeded": 3, "assessment_error": 4}
	if priority[next] > priority[current] {
		return next
	}
	return current
}
func walkRecovery(ctx context.Context, parent *os.File, node *recoveryNode, entries []os.DirEntry, depth int, total *int, pins *recoveryPins, operations ops) string {
	seen := map[string]bool{}
	result := ""
	for _, entry := range entries {
		if ctx.Err() != nil {
			return "assessment_error"
		}
		name := entry.Name()
		if depth == 0 && name == "manifest.json" {
			continue
		}
		lookup := named
		if node.children[name] == nil && operations.named != nil {
			lookup = operations.named
		}
		before, err := lookup(parent, name)
		if err != nil {
			return "assessment_error"
		}
		child := node.children[name]
		if child == nil {
			*pins = append(*pins, recoveryPin{parent: parent, name: name, stat: before})
			safe := recoveryFile(before, assetLimit) || recoveryDirectory(before, false)
			class := "incomplete"
			if !safe {
				class = "unsafe"
			}
			result = mergeRecoveryClass(result, class)
			continue
		}
		seen[name] = true
		if depth+1 > 4 {
			return "limit_exceeded"
		}
		if child.reference != nil {
			data, pin, class := readRecoveryFile(ctx, parent, name, assetLimit, operations)
			if pin.file != nil {
				*pins = append(*pins, pin)
			}
			if class == "missing" {
				class = "unsafe"
			}
			if class == "" {
				digest := sha256.Sum256(data)
				if int64(len(data)) != child.reference.Bytes || hex.EncodeToString(digest[:]) != child.reference.SHA256 {
					class = "incomplete"
				}
			}
			result = mergeRecoveryClass(result, class)
			continue
		}
		pin, class := openRecoveryDirectory(ctx, parent, name, false, operations)
		if class != "" {
			if class == "missing" {
				class = "unsafe"
			}
			result = mergeRecoveryClass(result, class)
			continue
		}
		*pins = append(*pins, pin)
		contents, class := recoveryEntries(ctx, pin.file, 32-*total)
		if class != "" {
			result = mergeRecoveryClass(result, class)
			continue
		}
		*total += len(contents)
		result = mergeRecoveryClass(result, walkRecovery(ctx, pin.file, child, contents, depth+1, total, pins, operations))
	}
	for name := range node.children {
		if !seen[name] {
			result = mergeRecoveryClass(result, "incomplete")
		}
	}
	return result
}

// Known payloads reuse confined opens and metadata identity helpers. Handles stay
// pinned until the whole set is rechecked, including directory listing metadata.
// docs/adr/0083-go-recovery-set-inspection.md:102.
func readRecoveryFile(ctx context.Context, parent *os.File, name string, limit int64, operations ops) ([]byte, recoveryPin, string) {
	if ctx.Err() != nil {
		return nil, recoveryPin{}, "assessment_error"
	}
	before, err := named(parent, name)
	if err != nil {
		return nil, recoveryPin{}, classification(err)
	}
	if before.Mode&unix.S_IFMT != unix.S_IFREG || before.Nlink != 1 || before.Mode&07777 != 0600 || int64(before.Uid) != int64(os.Geteuid()) || before.Size < 0 {
		return nil, recoveryPin{}, "unsafe"
	}
	if before.Size > limit {
		return nil, recoveryPin{}, recoverySizeClass(name)
	}
	if ctx.Err() != nil {
		return nil, recoveryPin{}, "assessment_error"
	}
	file, err := recoveryOpen(operations, parent, name, unix.O_RDONLY)
	if err != nil {
		class := classification(err)
		if class == "missing" {
			class = "unsafe"
		}
		return nil, recoveryPin{}, class
	}
	after, err := descriptor(file)
	if err != nil {
		_ = file.Close()
		return nil, recoveryPin{}, "assessment_error"
	}
	if !recoveryUnchanged(before, after) {
		_ = file.Close()
		return nil, recoveryPin{}, "unsafe"
	}
	pin := recoveryPin{parent: parent, file: file, name: name, stat: after}
	read := operations.read
	if read == nil {
		read = func(file *os.File, data []byte) (int, error) { return file.Read(data) }
	}
	var data []byte
	var buffer [32 << 10]byte
	for {
		if ctx.Err() != nil {
			return nil, pin, "assessment_error"
		}
		n, err := read(file, buffer[:])
		if n < 0 || n > len(buffer) {
			return nil, pin, "assessment_error"
		}
		if int64(len(data))+int64(n) > limit {
			return nil, pin, recoverySizeClass(name)
		}
		data = append(data, buffer[:n]...)
		if ctx.Err() != nil {
			return nil, pin, "assessment_error"
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil || n == 0 {
			return nil, pin, "assessment_error"
		}
	}
	current, err := descriptor(file)
	if err != nil {
		return nil, pin, "assessment_error"
	}
	location, err := named(parent, name)
	if err != nil || !recoveryUnchanged(after, current) || !recoveryUnchanged(current, location) || int64(len(data)) != current.Size {
		return nil, pin, "unsafe"
	}
	return data, pin, ""
}

func recoverySizeClass(name string) string {
	if name == "manifest.json" {
		return "limit_exceeded"
	}
	return "unsafe"
}
