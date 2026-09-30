package reviewlanecmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"syscall"

	"github.com/anoop2811/software-factory-template/internal/fileinput"
)

type workflowDirectory struct {
	path       string
	roots      []*os.Root
	identities []os.FileInfo
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
	return safeFile(a) && safeFile(b) && os.SameFile(a, b) && a.Mode() == b.Mode() && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime())
}
func (d *workflowDirectory) close() error {
	var errs []error
	for i := len(d.roots) - 1; i >= 0; i-- {
		errs = append(errs, d.roots[i].Close())
	}
	return errors.Join(errs...)
}
func (d *workflowDirectory) directory() *os.Root {
	if len(d.roots) != 3 {
		return nil
	}
	return d.roots[2]
}

// Pin each actual directory; symlink ancestors are not an ownership assertion.
// docs/adr/0089-go-native-review-lane.md:66.
func openWorkflow(ctx context.Context, path string, create bool) (result *workflowDirectory, returned error) {
	d := &workflowDirectory{path: path}
	defer func() {
		if returned != nil {
			returned = errors.Join(returned, d.close())
		}
	}()
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !safeDirectory(info) {
		return nil, errors.New("unsafe review lane project directory")
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	d.roots = append(d.roots, root)
	d.identities = append(d.identities, info)
	for _, name := range []string{".github", "workflows"} {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		info, err := root.Lstat(name)
		if errors.Is(err, os.ErrNotExist) {
			if !create {
				return d, nil
			}
			if err := root.Mkdir(name, 0755); err != nil {
				return nil, err
			}
			info, err = root.Lstat(name)
		}
		if err != nil {
			return nil, err
		}
		if !safeDirectory(info) {
			return nil, errors.New("unsafe review lane workflow directory")
		}
		next, err := root.OpenRoot(name)
		if err != nil {
			return nil, err
		}
		opened, err := next.Stat(".")
		if err != nil || !os.SameFile(info, opened) {
			_ = next.Close()
			return nil, errors.New("review lane workflow directory changed")
		}
		d.roots = append(d.roots, next)
		d.identities = append(d.identities, info)
		root = next
	}
	return d, nil
}
func (d *workflowDirectory) check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	current, err := os.Lstat(d.path)
	if err != nil || !safeDirectory(current) || !os.SameFile(current, d.identities[0]) {
		return errors.New("review lane project directory changed")
	}
	for i, name := range []string{".github", "workflows"} {
		if i+1 >= len(d.roots) {
			break
		}
		current, err := d.roots[i].Lstat(name)
		if err != nil || !safeDirectory(current) || !os.SameFile(current, d.identities[i+1]) {
			return errors.New("review lane workflow directory changed")
		}
	}
	return nil
}
func (d *workflowDirectory) observe(ctx context.Context) (os.FileInfo, bool, error) {
	directory := d.directory()
	if directory == nil {
		return nil, false, nil
	}
	info, err := directory.Lstat("adversarial-review.yml")
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !safeFile(info) {
		return info, false, nil
	}
	file, err := directory.OpenFile("adversarial-review.yml", os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, false, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !sameFile(info, opened) {
		return nil, false, errors.New("review lane workflow changed")
	}
	data, err := fileinput.ReadFile(ctx, file, fileLimit)
	if err != nil {
		return nil, false, err
	}
	first, _, _ := bytes.Cut(data, []byte{'\n'})
	return info, string(first) == managedHeader, nil
}
func (d *workflowDirectory) unchanged(ctx context.Context, original os.FileInfo) error {
	if err := d.check(ctx); err != nil {
		return err
	}
	current, managed, err := d.observe(ctx)
	if err != nil {
		return err
	}
	if original == nil && current == nil {
		return nil
	}
	if original == nil || current == nil || !managed || !sameFile(original, current) {
		return errors.New("review lane workflow changed before mutation")
	}
	return nil
}
