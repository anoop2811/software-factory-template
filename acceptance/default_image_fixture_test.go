package acceptance_test

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/anoop2811/software-factory-template/internal/artifact"
	"github.com/anoop2811/software-factory-template/internal/native"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type defaultImageSelected struct {
	binary  []byte
	assets  []imageAssetOracle
	payload map[string][]byte
}

// Consumer fixtures independently encode their real committed source. The
// production packager's own qualification still exercises its separate owner.
// per docs/adr/0131-bounded-ci-feedback.md:131
func defaultImagePrepare(ctx context.Context, source, revision, cache string) *defaultImageSelected {
	GinkgoHelper()
	selected := &defaultImageSelected{}
	defaultImagePhase("catalog", revision, func() {
		selected.assets, selected.payload = defaultImageCatalog(ctx, source, revision)
	})
	defaultImagePhase("compile", revision, func() {
		build := filepath.Join(cache, "selected-source")
		Expect(os.Mkdir(build, 0700)).To(Succeed())
		for _, asset := range selected.assets {
			Expect(ctx.Err()).NotTo(HaveOccurred())
			name := asset.Path
			if name != "go.mod" && name != "go.sum" && !strings.HasPrefix(name, "cmd/factory/") && !strings.HasPrefix(name, "internal/") {
				continue
			}
			Expect(asset.Mode).NotTo(Equal("120000"), "compiler source must be an ordinary committed file: %s", name)
			writeFixture(filepath.Join(build, filepath.FromSlash(name)), selected.payload[name], 0644)
		}
		compiler, err := exec.LookPath("go")
		Expect(err).NotTo(HaveOccurred())
		compiler, err = filepath.EvalSymlinks(compiler)
		Expect(err).NotTo(HaveOccurred())
		env := defaultImageEnvironment()
		for key := range env {
			if strings.HasPrefix(key, "GO") || strings.HasPrefix(key, "CGO") {
				delete(env, key)
			}
		}
		for key, value := range map[string]string{"GOENV": "off", "GOTOOLCHAIN": "local", "GOWORK": "off", "GOFLAGS": "", "GOEXPERIMENT": "", "GOPROXY": "off", "GOSUMDB": "off", "CGO_ENABLED": "0", "GOOS": runtime.GOOS, "GOARCH": runtime.GOARCH, "GOAMD64": "v1", "GOARM64": "v8.0"} {
			env[key] = value
		}
		version := strings.Fields(string(defaultImageCommand(ctx, build, env, "", compiler, "version")))
		Expect(version).To(HaveLen(4))
		Expect(version[:3]).To(Equal([]string{"go", "version", artifact.SourceGoVersion}))
		program := filepath.Join(cache, "selected-runtime")
		// These are the current production compiler flags. Version/revision live
		// in authenticated metadata, not linker substitutions in the executable.
		// per docs/adr/0131-bounded-ci-feedback.md:133
		defaultImageCommand(ctx, build, env, "", compiler, "build", "-mod=readonly", "-trimpath", "-buildvcs=false", "-o", program, "./cmd/factory")
		selected.binary, err = os.ReadFile(program)
		Expect(err).NotTo(HaveOccurred())
		Expect(ctx.Err()).NotTo(HaveOccurred())
		Expect(selected.binary).NotTo(BeEmpty())
		Expect(string(selected.binary)).To(ContainSubstring(defaultInitMarker))
	})
	return selected
}

// Git objects, not export attributes or mutable worktree files, supply every
// catalog row and payload. The two subprocesses share the image's deadline.
// per docs/adr/0131-bounded-ci-feedback.md:135
func defaultImageCatalog(ctx context.Context, source, revision string) ([]imageAssetOracle, map[string][]byte) {
	GinkgoHelper()
	env := defaultImageEnvironment()
	listing := defaultImageCommand(ctx, source, env, "", "git", "--no-replace-objects", "ls-tree", "-r", "-z", "--full-tree", revision)
	type object struct{ mode, id, path string }
	objects := []object{}
	var requests strings.Builder
	for _, record := range strings.Split(string(listing), "\x00") {
		if record == "" {
			continue
		}
		metadata, path, found := strings.Cut(record, "\t")
		Expect(found).To(BeTrue())
		fields := strings.Fields(metadata)
		Expect(fields).To(HaveLen(3))
		Expect(fields[1]).To(Equal("blob"))
		objects = append(objects, object{fields[0], fields[2], path})
		requests.WriteString(fields[2] + "\n")
	}
	body := defaultImageCommand(ctx, source, env, requests.String(), "git", "--no-replace-objects", "cat-file", "--batch")
	reader := bufio.NewReader(bytes.NewReader(body))
	assets := make([]imageAssetOracle, 0, len(objects))
	payload := make(map[string][]byte, len(objects))
	for _, object := range objects {
		Expect(ctx.Err()).NotTo(HaveOccurred())
		header, err := reader.ReadString('\n')
		Expect(err).NotTo(HaveOccurred())
		fields := strings.Fields(header)
		Expect(fields).To(HaveLen(3))
		Expect(fields[:2]).To(Equal([]string{object.id, "blob"}))
		size, err := strconv.Atoi(fields[2])
		Expect(err).NotTo(HaveOccurred())
		Expect(size).To(BeNumerically(">=", 0))
		Expect(size).To(BeNumerically("<=", len(body)))
		data := make([]byte, size)
		_, err = io.ReadFull(reader, data)
		Expect(err).NotTo(HaveOccurred())
		separator, err := reader.ReadByte()
		Expect(err).NotTo(HaveOccurred())
		Expect(separator).To(Equal(byte('\n')))
		assets = append(assets, imageAssetOracle{object.path, object.mode, defaultImageDigest(data), int64(len(data))})
		payload[object.path] = data
	}
	_, err := reader.ReadByte()
	Expect(err).To(MatchError(io.EOF))
	sort.Slice(assets, func(i, j int) bool { return assets[i].Path < assets[j].Path })
	return assets, payload
}

// Native supervision reaps the compiler/Git group on deadline, rather than
// killing only the direct process and waiting indefinitely for its captures.
// per docs/adr/0131-bounded-ci-feedback.md:136
func defaultImageCommand(ctx context.Context, root string, environment map[string]string, input string, argv ...string) []byte {
	GinkgoHelper()
	result, err := native.ExecuteCommandWithOptions(ctx, root, argv, environment, native.CommandOptions{Input: input, StdoutLimit: 16 << 20, StderrLimit: 64 << 10})
	Expect(err).NotTo(HaveOccurred(), "fixture command %q: %s", argv, result.Stderr)
	Expect(ctx.Err()).NotTo(HaveOccurred())
	Expect(result.Outcome).To(Equal("completed"))
	Expect(result.ExitConfirmed).To(BeTrue())
	Expect(result.OwnershipUnconfirmed).To(BeFalse())
	Expect(result.ExitCode).NotTo(BeNil())
	Expect(*result.ExitCode).To(Equal(0), "fixture command %q: %s", argv, result.Stderr)
	return result.Stdout
}

func defaultImageEnvironment() map[string]string {
	environment := map[string]string{}
	for _, entry := range os.Environ() {
		key, value, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(key, "GIT_") {
			environment[key] = value
		}
	}
	return environment
}

// Only fixture compression changes; every decoded member is still authored by
// the existing independent image oracle and consumed by the real selected CLI.
// per docs/adr/0131-bounded-ci-feedback.md:137
func defaultImageArchive(ctx context.Context, entries []stageEntry) ([]byte, error) {
	var compressed bytes.Buffer
	gz, err := gzip.NewWriterLevel(&compressed, gzip.BestSpeed)
	if err != nil {
		return nil, err
	}
	tw := tar.NewWriter(gz)
	for _, entry := range entries {
		if err = ctx.Err(); err != nil {
			break
		}
		if err = tw.WriteHeader(&tar.Header{Name: entry.name, Mode: entry.mode, Typeflag: entry.kind, Size: int64(len(entry.body))}); err != nil {
			break
		}
		if _, err = io.Copy(tw, &defaultImageReader{ctx, bytes.NewReader(entry.body)}); err != nil {
			break
		}
	}
	err = errors.Join(err, tw.Close(), gz.Close(), ctx.Err())
	return compressed.Bytes(), err
}

type defaultImageReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *defaultImageReader) Read(data []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(data)
}

func defaultImageDigest(data []byte) string { return fmt.Sprintf("%x", sha256.Sum256(data)) }

func defaultImagePhase(phase, identity string, action func()) {
	GinkgoHelper()
	start := time.Now()
	defer func() {
		AddReportEntry("default-image."+phase, map[string]any{"identity": identity, "elapsed_ns": time.Since(start).Nanoseconds()}, ReportEntryVisibilityNever)
	}()
	action()
}
