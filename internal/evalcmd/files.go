package evalcmd

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"syscall"

	"github.com/anoop2811/software-factory-template/internal/fileinput"
	"github.com/anoop2811/software-factory-template/internal/filepublish"
)

// POSIX cksum uses the forward CRC polynomial and appends the little-endian
// byte count before complementing. docs/adr/0099-native-factory-orchestration.md:106.
func checksum(ctx context.Context, data []byte) (string, error) {
	var crc uint32
	step := func(value byte) {
		crc ^= uint32(value) << 24
		for range 8 {
			if crc&0x80000000 != 0 {
				crc = (crc << 1) ^ 0x04c11db7
			} else {
				crc <<= 1
			}
		}
	}
	for i, value := range data {
		if i%4096 == 0 {
			if err := ctx.Err(); err != nil {
				return "", err
			}
		}
		step(value)
	}
	for size := len(data); size != 0; size >>= 8 {
		step(byte(size))
	}
	return fmt.Sprintf("%d-%d", ^crc, len(data)), ctx.Err()
}

func concatenate(ctx context.Context, paths ...string) ([]byte, error) {
	var data []byte
	for _, path := range paths {
		part, err := fileinput.Read(ctx, path, fileLimit)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if len(part) > fileLimit-len(data) {
			return nil, errors.New("evaluation input limit exceeded")
		}
		data = append(data, part...)
	}
	return data, ctx.Err()
}

func copyTask(ctx context.Context, source, destination string) (returned error) {
	origin, err := os.OpenRoot(source)
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, origin.Close()) }()
	target, err := os.OpenRoot(destination)
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, target.Close()) }()
	var total int64
	return fs.WalkDir(origin.FS(), ".", func(relative string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if relative == "." {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		switch {
		case info.IsDir():
			return target.Mkdir(relative, 0700)
		case info.Mode().IsRegular():
			file, err := origin.OpenFile(relative, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
			if err != nil {
				return err
			}
			data, readErr := fileinput.ReadFile(ctx, file, fileLimit)
			err = errors.Join(readErr, file.Close())
			if err != nil {
				return err
			}
			total += int64(len(data))
			if total > fileLimit {
				return errors.New("evaluation task copy limit exceeded")
			}
			return target.WriteFile(relative, data, info.Mode().Perm()|0600)
		case info.Mode()&os.ModeSymlink != 0:
			link, err := origin.Readlink(relative)
			if err != nil {
				return err
			}
			return target.Symlink(link, relative)
		default:
			return errors.New("evaluation task contains a special file")
		}
	})
}

func publish(ctx context.Context, directoryPath, name string, data []byte) (returned error) {
	directory, err := os.OpenRoot(directoryPath)
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, directory.Close()) }()
	if info, err := directory.Lstat(name); err == nil {
		if !info.Mode().IsRegular() {
			return errors.New("unsafe evaluation output")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	stage, err := filepublish.Prepare(ctx, directory, ".factory-eval-", data, 0600)
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, stage.Cleanup()) }()
	return stage.Publish(ctx, name)
}
