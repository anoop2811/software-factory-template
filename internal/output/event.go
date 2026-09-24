// Package output writes one checked, flushed presentation event.
package output

import (
	"context"
	"errors"
	"io"
)

var ErrWrite = errors.New("cannot write output")
var ErrFlush = errors.New("cannot flush output")

// WriteEvent refuses short writes and discards private writer error details.
// docs/adr/0072-go-budget-argument-compatibility.md:103.
func WriteEvent(ctx context.Context, writer io.Writer, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	count, err := writer.Write(data)
	if err != nil || count != len(data) {
		return ErrWrite
	}
	if flusher, ok := writer.(interface{ Flush() error }); ok {
		if err := flusher.Flush(); err != nil {
			return ErrFlush
		}
	}
	return ctx.Err()
}
