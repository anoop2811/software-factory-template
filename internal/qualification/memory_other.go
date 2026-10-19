//go:build !darwin && !linux

package qualification

import "errors"

func platformMemory() (uint64, error) { return 0, errors.New("unsupported memory probe") }
