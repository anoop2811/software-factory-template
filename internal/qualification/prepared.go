package qualification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anoop2811/software-factory-template/internal/native"
)

const (
	preparedDirectory   = ".factory/qualification/prepared-v1"
	preparedReceiptPath = preparedDirectory + "/receipt.json"
	preparedRunnerPath  = preparedDirectory + "/ginkgo"
	preparedSuitePath   = "acceptance/qualification.test"
)

// PreparedOptions identifies one immutable workflow checkout; attempts are provenance only.
// docs/adr/0151-reliable-ci-qualification.md:85.
type PreparedOptions struct {
	Root, Revision, RunID, ProducerAttempt string
	Environment                            map[string]string
}

type PreparedExecutable struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type PreparedReceipt struct {
	Schema          int                `json:"schema"`
	RunID           string             `json:"run_id"`
	ProducerAttempt string             `json:"producer_attempt"`
	Revision        string             `json:"revision"`
	GOOS            string             `json:"go_os"`
	GOARCH          string             `json:"go_arch"`
	GoVersion       string             `json:"go_version"`
	GinkgoVersion   string             `json:"ginkgo_version"`
	GoModSHA256     string             `json:"go_mod_sha256"`
	GoSumSHA256     string             `json:"go_sum_sha256"`
	Race            bool               `json:"race"`
	Suite           PreparedExecutable `json:"suite"`
	Ginkgo          PreparedExecutable `json:"ginkgo"`
}

// Prepare compiles inputs without running them and publishes the receipt last.
// docs/adr/0151-reliable-ci-qualification.md:55.
func Prepare(ctx context.Context, options PreparedOptions) (PreparedReceipt, error) {
	root, err := preparedRoot(ctx, options)
	if err != nil {
		return PreparedReceipt{}, err
	}
	if !positiveDecimal(options.ProducerAttempt) {
		return PreparedReceipt{}, errors.New("invalid producer attempt")
	}
	if err = preparedDirectories(root, preparedDirectory, true); err != nil {
		return PreparedReceipt{}, err
	}
	// A previous success must not survive a failed replacement preparation.
	receiptPath := filepath.Join(root, preparedReceiptPath)
	if err = os.Remove(receiptPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return PreparedReceipt{}, fmt.Errorf("invalidate prepared receipt: %w", err)
	}
	logPath := filepath.Join(root, preparedDirectory, "prepare.log")
	if err = preparedWritableFile(logPath); err != nil {
		return PreparedReceipt{}, err
	}
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600) // #nosec G304 -- fixed preparation log beneath the explicitly selected canonical root; ancestor and leaf types were checked above.
	if err != nil {
		return PreparedReceipt{}, fmt.Errorf("create preparation diagnostics: %w", err)
	}
	receipt, buildErr := prepareExecutables(ctx, root, options, log)
	if buildErr != nil {
		_, writeErr := fmt.Fprintln(log, "preparation failed:", buildErr)
		buildErr = errors.Join(buildErr, writeErr)
	}
	if err = errors.Join(buildErr, log.Close()); err != nil {
		return PreparedReceipt{}, err
	}
	if err = publishPreparedReceipt(ctx, root, receipt); err != nil {
		return PreparedReceipt{}, err
	}
	return receipt, nil
}

func prepareExecutables(ctx context.Context, root string, options PreparedOptions, log io.Writer) (PreparedReceipt, error) {
	receipt, err := preparedIdentity(ctx, root, options)
	if err != nil {
		return PreparedReceipt{}, err
	}
	receipt.ProducerAttempt = options.ProducerAttempt
	if err = preparedDirectories(root, "acceptance", false); err != nil {
		return PreparedReceipt{}, err
	}
	for _, path := range []string{preparedRunnerPath, preparedSuitePath} {
		if err = preparedWritableFile(filepath.Join(root, path)); err != nil {
			return PreparedReceipt{}, err
		}
	}
	environment := preparedEnvironment(options.Environment)
	version, err := preparedCommand(ctx, root, environment, log, "go", "version")
	if err != nil {
		return PreparedReceipt{}, err
	}
	if strings.TrimSpace(string(version)) != "go version "+receipt.GoVersion+" "+receipt.GOOS+"/"+receipt.GOARCH {
		return PreparedReceipt{}, errors.New("preparation requires the qualified native Go compiler")
	}
	commands := [][]string{
		{"go", "build", "-mod=readonly", "-o", filepath.Join(root, preparedRunnerPath), "github.com/onsi/ginkgo/v2/ginkgo"},
		{"go", "test", "-c", "-race", "-mod=readonly", "-o", filepath.Join(root, preparedSuitePath), "./acceptance"},
	}
	for _, argv := range commands {
		if _, err = preparedCommand(ctx, root, environment, log, argv...); err != nil {
			return PreparedReceipt{}, err
		}
	}
	if receipt.Ginkgo, err = inspectPreparedExecutable(ctx, root, preparedRunnerPath, receipt, false); err != nil {
		return PreparedReceipt{}, err
	}
	if receipt.Suite, err = inspectPreparedExecutable(ctx, root, preparedSuitePath, receipt, true); err != nil {
		return PreparedReceipt{}, err
	}
	final, err := preparedIdentity(ctx, root, options)
	if err != nil {
		return PreparedReceipt{}, err
	}
	if !samePreparedIdentity(receipt, final) {
		return PreparedReceipt{}, errors.New("source identity changed during preparation")
	}
	return receipt, nil
}

// VerifyPrepared only reads the checkout and cached inputs; it never repairs or executes them.
// docs/adr/0151-reliable-ci-qualification.md:59.
func VerifyPrepared(ctx context.Context, options PreparedOptions) (PreparedReceipt, error) {
	root, err := preparedRoot(ctx, options)
	if err != nil {
		return PreparedReceipt{}, err
	}
	receipt, err := readPreparedReceipt(ctx, root)
	if err != nil {
		return PreparedReceipt{}, err
	}
	expected, err := preparedIdentity(ctx, root, options)
	if err != nil {
		return PreparedReceipt{}, err
	}
	if !positiveDecimal(receipt.ProducerAttempt) || !samePreparedIdentity(receipt, expected) {
		return PreparedReceipt{}, errors.New("prepared qualification identity does not match this checkout and run")
	}
	for _, entry := range []struct {
		record PreparedExecutable
		path   string
		race   bool
	}{{receipt.Suite, preparedSuitePath, true}, {receipt.Ginkgo, preparedRunnerPath, false}} {
		if entry.record.Path != entry.path {
			return PreparedReceipt{}, errors.New("unexpected prepared executable placement")
		}
		actual, inspectErr := inspectPreparedExecutable(ctx, root, entry.path, receipt, entry.race)
		if inspectErr != nil {
			return PreparedReceipt{}, inspectErr
		}
		if actual != entry.record {
			return PreparedReceipt{}, fmt.Errorf("prepared executable changed: %s", entry.path)
		}
	}
	return receipt, ctx.Err()
}

func preparedCommand(ctx context.Context, root string, environment map[string]string, log io.Writer, argv ...string) ([]byte, error) {
	started := time.Now()
	result, err := native.ExecuteCommandBounded(ctx, root, argv, environment, 1<<20, 1<<20)
	elapsed := time.Since(started)
	if result.ExitCode != nil && *result.ExitCode != 0 {
		err = errors.Join(err, fmt.Errorf("command exited with status %d", *result.ExitCode))
	}
	if result.Outcome != "completed" || !result.ExitConfirmed || result.OwnershipUnconfirmed || result.ExitCode == nil {
		err = errors.Join(err, errors.New("command process ownership was not confirmed complete"))
	}
	if log != nil {
		_, writeErr := fmt.Fprintf(log, "command: %q\nelapsed: %s\nstdout:\n%sstderr:\n%s\n", argv, elapsed, result.Stdout, result.Stderr)
		err = errors.Join(err, writeErr)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", strings.Join(argv, " "), errors.Join(err, ctx.Err()))
	}
	return result.Stdout, nil
}

func publishPreparedReceipt(ctx context.Context, root string, receipt PreparedReceipt) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Join(root, preparedDirectory), ".receipt-*")
	if err != nil {
		return fmt.Errorf("create receipt staging file: %w", err)
	}
	defer os.Remove(file.Name())
	err = json.NewEncoder(file).Encode(receipt)
	if err == nil {
		err = file.Sync()
	}
	if err = errors.Join(err, file.Close(), ctx.Err()); err != nil {
		return fmt.Errorf("write prepared receipt: %w", err)
	}
	if err = os.Rename(file.Name(), filepath.Join(root, preparedReceiptPath)); err != nil {
		return fmt.Errorf("publish prepared receipt: %w", err)
	}
	return nil
}
