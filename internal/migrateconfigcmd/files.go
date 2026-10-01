package migrateconfigcmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"

	"github.com/anoop2811/software-factory-template/internal/fileinput"
)

type project struct {
	path      string
	directory *os.Root
	identity  os.FileInfo
}
type snapshot struct {
	data []byte
	info os.FileInfo
}

func safeDirectory(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && info.IsDir() && info.Mode().Perm()&0022 == 0 && info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) == 0 && int64(stat.Uid) == int64(os.Geteuid())
}
func safeFile(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && info.Mode().IsRegular() && info.Mode() == info.Mode().Perm() && stat.Nlink == 1 && int64(stat.Uid) == int64(os.Geteuid()) && info.Size() <= fileLimit
}
func sameFile(a, b os.FileInfo) bool {
	x, xok := a.Sys().(*syscall.Stat_t)
	y, yok := b.Sys().(*syscall.Stat_t)
	return xok && yok && safeFile(a) && safeFile(b) && os.SameFile(a, b) && a.Mode() == b.Mode() && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime()) && x.Uid == y.Uid && x.Gid == y.Gid
}

func openProject(ctx context.Context, path string) (*project, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !safeDirectory(info) {
		return nil, errors.New("unsafe project directory")
	}
	directory, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	opened, err := directory.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		return nil, errors.Join(errors.New("project directory changed while opening"), directory.Close())
	}
	return &project{path: path, directory: directory, identity: info}, nil
}

func (p *project) check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	current, err := os.Lstat(p.path)
	if err != nil || !safeDirectory(current) || !os.SameFile(p.identity, current) || p.identity.Mode() != current.Mode() {
		return errors.New("factory migrate-config: project directory changed")
	}
	return nil
}

func (p *project) read(ctx context.Context, name string) (snapshot, error) {
	if err := ctx.Err(); err != nil {
		return snapshot{}, err
	}
	before, err := p.directory.Lstat(name)
	if err != nil {
		return snapshot{}, err
	}
	if !safeFile(before) {
		return snapshot{}, errors.New("input must be a caller-owned ordinary regular file, single-link and at most 16 MiB")
	}
	file, err := p.directory.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return snapshot{}, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return snapshot{}, err
	}
	if !sameFile(before, opened) {
		return snapshot{}, errors.New("input changed while opening")
	}
	data, err := fileinput.ReadFile(ctx, file, fileLimit)
	if err != nil {
		return snapshot{}, err
	}
	final, err := file.Stat()
	if err != nil {
		return snapshot{}, err
	}
	occupant, err := p.directory.Lstat(name)
	if err != nil {
		return snapshot{}, err
	}
	if !sameFile(before, final) || !sameFile(before, occupant) {
		return snapshot{}, errors.New("input changed while reading")
	}
	return snapshot{data: data, info: final}, nil
}

func (p *project) unchanged(ctx context.Context, name string, before snapshot) error {
	after, err := p.read(ctx, name)
	if err != nil {
		return err
	}
	if !sameFile(before.info, after.info) || !bytes.Equal(before.data, after.data) {
		return fmt.Errorf("factory migrate-config: %s changed before mutation", name)
	}
	return nil
}

func (p *project) noBackup() error {
	_, err := p.directory.Lstat("factory.config.migrated")
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	//nolint:revive,staticcheck // Preserve the recovery diagnostic: docs/adr/0090-go-native-config-migration.md:62.
	return fmt.Errorf("\nfactory migrate-config: %s/factory.config.migrated already exists.\n  Refusing to overwrite it — it is the backup from an earlier migration,\n  and the recovery path documented below depends on it surviving.\n  Move or remove that file, then run this again.", p.path)
}

// Use the pinned directory descriptor for the platform's atomic no-replace.
// docs/adr/0090-go-native-config-migration.md:71.
func (p *project) renameLegacy(ctx context.Context) (returned error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	directory, err := p.directory.Open(".")
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, directory.Close()) }()
	info, err := directory.Stat()
	if err != nil || !os.SameFile(p.identity, info) {
		return errors.New("factory migrate-config: project directory changed before rename")
	}
	connection, err := directory.SyscallConn()
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	var renameErr error
	err = connection.Control(func(fd uintptr) { renameErr = renameNoReplace(int(fd), "factory.config", "factory.config.migrated") })
	return errors.Join(err, renameErr)
}
