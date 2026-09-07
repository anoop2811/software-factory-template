// Package artifact verifies inert runtime release metadata before a later
// migration stage is allowed to execute an artifact.
package artifact

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const header = "FACTORY_RUNTIME_ARTIFACT_V1"

// Metadata is the verified, deterministic identity of one runtime artifact.
type Metadata struct {
	Version string
	Target  string
	Binary  string
	SHA256  string
}

// Verify validates manifest and binary without executing or mutating either.
// The target is explicit so cross-target staging cannot silently verify the
// host target. docs/adr/0058-runtime-artifact-verification-candidate.md:20.
func Verify(ctx context.Context, manifestPath, root, target string) (Metadata, error) {
	if err := ctx.Err(); err != nil {
		return Metadata{}, err
	}
	if target == "" || !validTarget(target) {
		return Metadata{}, fmt.Errorf("invalid artifact target %q", target)
	}
	manifest, err := regularNoSymlink(manifestPath)
	if err != nil {
		return Metadata{}, fmt.Errorf("artifact manifest: %w", err)
	}
	defer manifest.Close()

	metadata, err := parseManifest(ctx, manifest)
	if err != nil {
		return Metadata{}, err
	}
	if metadata.Target != target {
		return Metadata{}, fmt.Errorf("artifact target %q does not match requested %q", metadata.Target, target)
	}
	if err := validateRoot(root); err != nil {
		return Metadata{}, err
	}
	binaryPath, err := safeChild(root, metadata.Binary)
	if err != nil {
		return Metadata{}, fmt.Errorf("artifact binary: %w", err)
	}
	actual, err := digest(ctx, binaryPath)
	if err != nil {
		return Metadata{}, fmt.Errorf("artifact binary: %w", err)
	}
	if actual != metadata.SHA256 {
		return Metadata{}, fmt.Errorf("artifact digest mismatch: manifest %s, actual %s", metadata.SHA256, actual)
	}
	return metadata, nil
}

func parseManifest(ctx context.Context, reader io.Reader) (Metadata, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 256), 64*1024)
	var metadata Metadata
	seen := map[string]bool{}
	line := 0
	for scanner.Scan() {
		line++
		if err := ctx.Err(); err != nil {
			return Metadata{}, err
		}
		value := scanner.Text()
		if line == 1 {
			if value != header {
				return Metadata{}, fmt.Errorf("invalid artifact manifest header")
			}
			continue
		}
		if value == "END" {
			if scanner.Scan() {
				return Metadata{}, fmt.Errorf("artifact manifest has data after END")
			}
			if err := scanner.Err(); err != nil {
				return Metadata{}, fmt.Errorf("read artifact manifest: %w", err)
			}
			if !seen["version"] || !seen["target"] || !seen["binary"] || !seen["sha256"] {
				return Metadata{}, fmt.Errorf("artifact manifest is missing a required field")
			}
			return metadata, nil
		}
		parts := strings.SplitN(value, "\t", 2)
		if len(parts) != 2 || seen[parts[0]] {
			return Metadata{}, fmt.Errorf("invalid artifact manifest field at line %d", line)
		}
		key, field := parts[0], parts[1]
		if field == "" || strings.ContainsAny(field, "\r\n") {
			return Metadata{}, fmt.Errorf("invalid artifact manifest value at line %d", line)
		}
		seen[key] = true
		switch key {
		case "version":
			metadata.Version = field
		case "target":
			if !validTarget(field) {
				return Metadata{}, fmt.Errorf("invalid artifact target %q", field)
			}
			metadata.Target = field
		case "binary":
			metadata.Binary = field
		case "sha256":
			if len(field) != sha256.Size*2 || strings.ToLower(field) != field {
				return Metadata{}, fmt.Errorf("invalid artifact sha256")
			}
			if _, err := hex.DecodeString(field); err != nil {
				return Metadata{}, fmt.Errorf("invalid artifact sha256: %w", err)
			}
			metadata.SHA256 = field
		default:
			return Metadata{}, fmt.Errorf("unknown artifact manifest field %q", key)
		}
	}
	if err := scanner.Err(); err != nil {
		return Metadata{}, fmt.Errorf("read artifact manifest: %w", err)
	}
	return Metadata{}, errors.New("artifact manifest is missing END")
}

func validTarget(target string) bool {
	parts := strings.Split(target, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return false
	}
	for _, part := range parts {
		for _, r := range part {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' && r != '-' {
				return false
			}
		}
	}
	return true
}

func validateRoot(root string) error {
	info, err := os.Lstat(root)
	if err != nil {
		return fmt.Errorf("artifact root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("artifact root is not a regular directory")
	}
	return nil
}

func safeChild(root, relative string) (string, error) {
	if relative == "" || filepath.IsAbs(relative) {
		return "", errors.New("binary path must be relative")
	}
	clean := filepath.Clean(relative)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("binary path escapes artifact root")
	}
	current := root
	for _, part := range strings.Split(clean, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("binary path contains a symlink")
		}
	}
	info, err := os.Stat(current)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("binary is not a regular file")
	}
	return current, nil
}

func regularNoSymlink(path string) (*os.File, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, errors.New("manifest is not a regular file")
	}
	return os.Open(path)
}

func digest(ctx context.Context, path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	buffer := make([]byte, 32*1024)
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		count, readErr := file.Read(buffer)
		if count > 0 {
			if _, err := hash.Write(buffer[:count]); err != nil {
				return "", err
			}
		}
		if errors.Is(readErr, io.EOF) {
			return hex.EncodeToString(hash.Sum(nil)), nil
		}
		if readErr != nil {
			return "", readErr
		}
	}
}
