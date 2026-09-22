package budget

import (
	"context"
	"errors"
	"io"
	"math"
	"math/big"
	"os"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
)

const promptLimit = 16 << 20

// File prompts use universal newlines; direct prompts remain literal overrides.
// docs/adr/0070-go-budget-execution-controller.md:48.
func loadPrompt(ctx context.Context, input RunInput) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", controllerError("cannot load budget prompt", err)
	}
	var prompt string
	if input.Prompt != nil {
		prompt = *input.Prompt
	} else {
		fd, err := syscall.Open(input.PromptFile, syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
		if err != nil {
			return "", errors.New("cannot load budget prompt")
		}
		file := os.NewFile(uintptr(fd), input.PromptFile)
		defer file.Close()
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() || info.Size() > promptLimit {
			return "", errors.New("invalid budget prompt file")
		}
		data, err := io.ReadAll(io.LimitReader(&contextReader{ctx, file}, promptLimit+1))
		if err != nil {
			return "", controllerError("cannot load budget prompt", err)
		}
		if len(data) > promptLimit {
			return "", errors.New("budget prompt exceeds its limit")
		}
		prompt = strings.ReplaceAll(strings.ReplaceAll(string(data), "\r\n", "\n"), "\r", "\n")
	}
	if len(prompt) > promptLimit || !utf8.ValidString(prompt) || strings.ContainsRune(prompt, 0) {
		return "", errors.New("invalid budget prompt content")
	}
	if err := ctx.Err(); err != nil {
		return "", controllerError("cannot load budget prompt", err)
	}
	return prompt, nil
}

// Exact conversion floors fractional nanoseconds and never wraps at MaxInt64.
// docs/adr/0070-go-budget-execution-controller.md:43.
func executionDuration(seconds float64) (time.Duration, error) {
	if !finite(seconds) || seconds <= 0 {
		return 0, errors.New("invalid budget execution duration")
	}
	scaled := new(big.Rat).Mul(new(big.Rat).SetFloat64(seconds), big.NewRat(int64(time.Second), 1))
	nanoseconds := new(big.Int).Quo(scaled.Num(), scaled.Denom())
	if !nanoseconds.IsInt64() || nanoseconds.Sign() <= 0 {
		return 0, errors.New("unrepresentable budget execution duration")
	}
	return time.Duration(nanoseconds.Int64()), nil
}
func executionConfig(ctx context.Context, c Config) (Config, error) {
	if err := ctx.Err(); err != nil {
		return Config{}, err
	}
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return Config{}, context.DeadlineExceeded
		}
		c.TimeoutSeconds = math.Min(c.TimeoutSeconds, remaining.Seconds())
	}
	if _, err := executionDuration(c.TimeoutSeconds); err != nil {
		return Config{}, err
	}
	return c, nil
}
func executionAdmission(ctx context.Context, c Config, remaining float64) (Config, error) {
	c, err := executionConfig(ctx, c)
	if err != nil {
		return Config{}, err
	}
	if _, err := executionDuration(math.Min(c.TimeoutSeconds, remaining)); err != nil {
		return Config{}, err
	}
	return c, nil
}
