// Package output writes one checked, flushed presentation event.
package output

import (
	"context"
	"errors"
	"io"
	"os"

	"github.com/anoop2811/software-factory-template/internal/streamfile"
)

var ErrWrite = errors.New("cannot write output")
var ErrFlush = errors.New("cannot flush output")

// WriteEvent refuses short writes and discards private writer error details.
// docs/adr/0072-go-budget-argument-compatibility.md:103.
func WriteEvent(ctx context.Context, writer io.Writer, data []byte) (returned error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	// Borrow only pollable streams; custom writers retain their own blocking
	// contract. docs/adr/0088-go-native-metrics.md:86.
	if file, ok := writer.(*os.File); ok {
		info, err := file.Stat()
		if err != nil {
			return ErrWrite
		}
		if info.Mode()&(os.ModeNamedPipe|os.ModeSocket) != 0 {
			owned, cleanup, err := streamfile.Borrow(ctx, file, nil)
			if err != nil {
				return ErrWrite
			}
			defer func() {
				if err := cleanup(); err != nil {
					returned = errors.Join(returned, ErrWrite)
				}
			}()
			writer = owned
		}
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
