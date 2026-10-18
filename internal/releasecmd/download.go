package releasecmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Observe file growth while the existing native supervisor owns the downloader.
// Final pinned snapshots independently enforce the exact input limits again.
// docs/adr/0101-go-release-handoff.md:119.
func boundedDownload(parent context.Context, directory string, names []string, limits []int64, run func(context.Context) error) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	stop := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				result <- nil
				return
			case <-ctx.Done():
				result <- ctx.Err()
				return
			case <-ticker.C:
				for index, name := range names {
					info, err := os.Lstat(filepath.Join(directory, name))
					if errors.Is(err, os.ErrNotExist) {
						continue
					}
					if err != nil || !info.Mode().IsRegular() || info.Size() > limits[index] {
						cancel()
						result <- errors.Join(err, fmt.Errorf("downloaded candidate asset exceeds its regular-file bound: %s", name))
						return
					}
				}
			}
		}
	}()
	err := run(ctx)
	close(stop)
	return errors.Join(err, <-result)
}
