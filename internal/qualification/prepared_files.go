package qualification

import (
	"bytes"
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func preparedDirectories(root, relative string, create bool) error {
	path := root
	for _, part := range strings.Split(relative, "/") {
		path = filepath.Join(path, part)
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) && create {
			if err = os.Mkdir(path, 0700); err != nil {
				return fmt.Errorf("create qualification directory: %w", err)
			}
			info, err = os.Lstat(path)
		}
		if err != nil {
			return fmt.Errorf("inspect qualification directory: %w", err)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0022 != 0 {
			return fmt.Errorf("unsafe qualification directory: %s", path)
		}
	}
	return nil
}

func preparedRegularFile(path string, executable bool) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect prepared input: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 || info.Mode().Perm()&0022 != 0 || (executable && info.Mode().Perm()&0100 == 0) {
		return nil, fmt.Errorf("unsafe prepared input type or mode: %s", path)
	}
	return info, nil
}

func preparedWritableFile(path string) error {
	_, err := preparedRegularFile(path, false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// Read build metadata from the same opened executable whose bytes are hashed.
// docs/adr/0151-reliable-ci-qualification.md:61.
func inspectPreparedExecutable(ctx context.Context, root, relative string, expected PreparedReceipt, race bool) (PreparedExecutable, error) {
	if err := ctx.Err(); err != nil {
		return PreparedExecutable{}, err
	}
	if err := preparedDirectories(root, filepath.ToSlash(filepath.Dir(relative)), false); err != nil {
		return PreparedExecutable{}, err
	}
	path := filepath.Join(root, relative)
	before, err := preparedRegularFile(path, true)
	if err != nil {
		return PreparedExecutable{}, err
	}
	file, err := os.Open(path) // #nosec G304 -- private callers supply only the fixed suite/runner paths; checked ancestors and leaf must remain the same opened regular executable.
	if err != nil {
		return PreparedExecutable{}, fmt.Errorf("open prepared executable: %w", err)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return PreparedExecutable{}, fmt.Errorf("inspect opened executable: %w", err)
	}
	if !os.SameFile(before, opened) {
		return PreparedExecutable{}, errors.New("prepared executable changed while opening")
	}
	metadata, err := buildinfo.Read(file)
	if err != nil {
		return PreparedExecutable{}, fmt.Errorf("read prepared executable build information: %w", err)
	}
	settings := make(map[string]string, len(metadata.Settings))
	for _, setting := range metadata.Settings {
		settings[setting.Key] = setting.Value
	}
	if metadata.GoVersion != expected.GoVersion || settings["GOOS"] != expected.GOOS || settings["GOARCH"] != expected.GOARCH || (settings["-race"] == "true") != race || settings["CGO_ENABLED"] != "1" || settings["-tags"] != "" || settings["-overlay"] != "" {
		return PreparedExecutable{}, errors.New("prepared executable compiler, platform or build flags do not match")
	}
	const ginkgoModule = "github.com/onsi/ginkgo/v2"
	found := false
	if race {
		if metadata.Path != "github.com/anoop2811/software-factory-template/acceptance.test" {
			return PreparedExecutable{}, errors.New("prepared suite has an unexpected package identity")
		}
		for _, dependency := range metadata.Deps {
			if dependency.Path == ginkgoModule && dependency.Version == expected.GinkgoVersion && dependency.Replace == nil {
				found = true
			}
		}
	} else {
		found = metadata.Path == ginkgoModule+"/ginkgo" && metadata.Main.Path == ginkgoModule && metadata.Main.Version == expected.GinkgoVersion && metadata.Main.Replace == nil
	}
	if !found {
		return PreparedExecutable{}, errors.New("prepared executable does not contain the pinned Ginkgo module")
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return PreparedExecutable{}, fmt.Errorf("read executable bytes: %w", err)
	}
	digest := sha256.New()
	buffer := make([]byte, 64<<10)
	var size int64
	for {
		if err = ctx.Err(); err != nil {
			return PreparedExecutable{}, err
		}
		count, readErr := file.Read(buffer)
		if count > 0 {
			_, _ = digest.Write(buffer[:count])
			size += int64(count)
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return PreparedExecutable{}, fmt.Errorf("hash prepared executable: %w", readErr)
		}
	}
	if size == 0 || size != opened.Size() {
		return PreparedExecutable{}, errors.New("prepared executable size changed while reading")
	}
	return PreparedExecutable{Path: relative, Size: size, SHA256: hex.EncodeToString(digest.Sum(nil))}, ctx.Err()
}

func readPreparedReceipt(ctx context.Context, root string) (PreparedReceipt, error) {
	if err := ctx.Err(); err != nil {
		return PreparedReceipt{}, err
	}
	if err := preparedDirectories(root, preparedDirectory, false); err != nil {
		return PreparedReceipt{}, err
	}
	path := filepath.Join(root, preparedReceiptPath)
	info, err := preparedRegularFile(path, false)
	if err != nil {
		return PreparedReceipt{}, err
	}
	if info.Size() <= 0 || info.Size() > 64<<10 {
		return PreparedReceipt{}, errors.New("invalid prepared receipt size")
	}
	data, err := os.ReadFile(path) // #nosec G304 -- fixed receipt path beneath the canonical root; ancestor directories, regular leaf, permissions and size were checked above.
	if err != nil {
		return PreparedReceipt{}, fmt.Errorf("read prepared receipt: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var receipt PreparedReceipt
	if err = decoder.Decode(&receipt); err != nil {
		return PreparedReceipt{}, fmt.Errorf("decode prepared receipt: %w", err)
	}
	var trailing any
	if err = decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return PreparedReceipt{}, errors.New("prepared receipt has trailing data")
	}
	return receipt, nil
}
