package qualification

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/anoop2811/software-factory-template/internal/artifact"
	"golang.org/x/mod/modfile"
)

func preparedRoot(ctx context.Context, options PreparedOptions) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !filepath.IsAbs(options.Root) || !positiveDecimal(options.RunID) || len(options.Revision) != 40 || strings.Trim(options.Revision, "0123456789abcdef") != "" {
		return "", errors.New("preparation requires an absolute root, full lowercase commit SHA and positive workflow run ID")
	}
	root, err := filepath.EvalSymlinks(options.Root)
	if err != nil {
		return "", fmt.Errorf("resolve qualification root: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("inspect qualification root: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("qualification root is not a directory")
	}
	return root, nil
}

func positiveDecimal(value string) bool {
	return value != "" && value[0] >= '1' && value[0] <= '9' && strings.Trim(value, "0123456789") == ""
}

// The run/checkout/module identity is independent of the receipt supplied by the cache.
// docs/adr/0151-reliable-ci-qualification.md:61.
func preparedIdentity(ctx context.Context, root string, options PreparedOptions) (PreparedReceipt, error) {
	environment := preparedEnvironment(options.Environment)
	for _, check := range []struct {
		arguments []string
		expected  string
	}{
		{[]string{"rev-parse", "--show-toplevel"}, root},
		{[]string{"rev-parse", "--verify", "HEAD"}, options.Revision},
		{[]string{"status", "--porcelain=v1", "--untracked-files=all"}, ""},
	} {
		arguments := append([]string{"git", "--no-optional-locks", "-c", "core.fsmonitor=false"}, check.arguments...)
		output, err := preparedCommand(ctx, root, environment, nil, arguments...)
		if err != nil {
			return PreparedReceipt{}, err
		}
		if strings.TrimSpace(string(output)) != check.expected {
			return PreparedReceipt{}, errors.New("qualification requires the exact clean committed checkout, including untracked inputs")
		}
	}
	inputs := make(map[string][]byte, 2)
	for _, name := range []string{"go.mod", "go.sum"} {
		path := filepath.Join(root, name)
		if _, err := preparedRegularFile(path, false); err != nil {
			return PreparedReceipt{}, err
		}
		data, err := os.ReadFile(path) // #nosec G304 -- only literal go.mod/go.sum beneath the independently checked canonical Git root; symlink and unsafe-mode leaves were refused above.
		if err != nil {
			return PreparedReceipt{}, fmt.Errorf("read module input: %w", err)
		}
		inputs[name] = data
	}
	module, err := modfile.Parse("go.mod", inputs["go.mod"], nil)
	if err != nil {
		return PreparedReceipt{}, fmt.Errorf("parse qualification module: %w", err)
	}
	if module.Module == nil || module.Module.Mod.Path != "github.com/anoop2811/software-factory-template" {
		return PreparedReceipt{}, errors.New("qualification requires the factory module")
	}
	version := ""
	for _, requirement := range module.Require {
		if requirement.Mod.Path == "github.com/onsi/ginkgo/v2" {
			version = requirement.Mod.Version
		}
	}
	if version == "" {
		return PreparedReceipt{}, errors.New("qualification requires a pinned Ginkgo module")
	}
	for _, replacement := range module.Replace {
		if replacement.Old.Path == "github.com/onsi/ginkgo/v2" {
			return PreparedReceipt{}, errors.New("qualification does not accept a replaced Ginkgo module")
		}
	}
	modHash, sumHash := sha256.Sum256(inputs["go.mod"]), sha256.Sum256(inputs["go.sum"])
	return PreparedReceipt{Schema: 1, RunID: options.RunID, Revision: options.Revision, GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, GoVersion: artifact.SourceGoVersion, GinkgoVersion: version, GoModSHA256: hex.EncodeToString(modHash[:]), GoSumSHA256: hex.EncodeToString(sumHash[:]), Race: true}, ctx.Err()
}

func samePreparedIdentity(actual, expected PreparedReceipt) bool {
	return actual.Schema == expected.Schema && actual.RunID == expected.RunID && actual.Revision == expected.Revision && actual.GOOS == expected.GOOS && actual.GOARCH == expected.GOARCH && actual.GoVersion == expected.GoVersion && actual.GinkgoVersion == expected.GinkgoVersion && actual.GoModSHA256 == expected.GoModSHA256 && actual.GoSumSHA256 == expected.GoSumSHA256 && actual.Race == expected.Race
}

// docs/adr/0151-reliable-ci-qualification.md:68 -- fixed Go flags with hosted toolchain inputs.
func preparedEnvironment(supplied map[string]string) map[string]string {
	if supplied == nil {
		supplied = make(map[string]string)
		for _, item := range os.Environ() {
			key, value, _ := strings.Cut(item, "=")
			supplied[key] = value
		}
	}
	environment := make(map[string]string, len(supplied)+7)
	for key, value := range supplied {
		// Checkout identity must not be redirected through inherited Git selectors.
		if !strings.HasPrefix(key, "GIT_") {
			environment[key] = value
		}
	}
	for key, value := range map[string]string{"GOENV": "off", "GOTOOLCHAIN": "local", "GOWORK": "off", "GOFLAGS": "", "GOOS": runtime.GOOS, "GOARCH": runtime.GOARCH, "CGO_ENABLED": "1"} {
		environment[key] = value
	}
	return environment
}
