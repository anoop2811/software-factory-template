package releasecmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/anoop2811/software-factory-template/internal/filepublish"
	"github.com/anoop2811/software-factory-template/internal/installationfs"
)

const reviewClientName = "factory-review-client"

// Retain the entire physical ancestry: an absolute OpenRoot alone permits links
// in its input path. docs/adr/0153-authenticated-review-client-bootstrap.md:31.
type reviewClientDirectory struct {
	root     *os.Root
	name     string
	identity os.FileInfo
}

type reviewClientOutput struct {
	path        string
	chain       []reviewClientDirectory
	parent      *os.Root
	name        string
	reservation os.FileInfo
	reserved    bool
	published   bool
	closed      bool
	closeError  error
	clientTree  *installationfs.Tree
	client      *installationfs.File
	digest      string
}

func openReviewClientOutput(ctx context.Context, path string) (result *reviewClientOutput, returned error) {
	if !reviewClientPath(path) {
		return nil, errors.New("review-client output must be an absolute canonical non-root path")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cwd, err := os.Stat(".")
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot("/")
	if err != nil {
		return nil, err
	}
	identity, err := root.Stat(".")
	if err != nil {
		return nil, errors.Join(err, root.Close())
	}
	owner := &reviewClientOutput{path: path, chain: []reviewClientDirectory{{root: root, identity: identity}}}
	defer func() {
		if result == nil {
			returned = errors.Join(returned, owner.close(context.WithoutCancel(ctx)))
		}
	}()
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for _, name := range parts[:len(parts)-1] {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		parent := owner.chain[len(owner.chain)-1]
		if os.SameFile(parent.identity, cwd) {
			return nil, errors.New("review-client output must be outside the working-directory tree")
		}
		info, err := parent.root.Lstat(name)
		if err != nil {
			return nil, fmt.Errorf("review-client output parent unavailable: %w", err)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("review-client output ancestry must contain only physical directories")
		}
		next, err := parent.root.OpenRoot(name)
		if err != nil {
			return nil, err
		}
		opened, err := next.Stat(".")
		if err != nil || !sameReviewClientDirectory(info, opened) {
			return nil, errors.Join(errors.New("review-client output ancestry changed while opening"), err, next.Close())
		}
		owner.chain = append(owner.chain, reviewClientDirectory{root: next, name: name, identity: opened})
	}
	parent := owner.chain[len(owner.chain)-1]
	if os.SameFile(parent.identity, cwd) {
		return nil, errors.New("review-client output must be outside the working-directory tree")
	}
	if !safeReviewClientParent(parent.identity) {
		return nil, errors.New("review-client output parent must be caller-owned and not group or other writable")
	}
	if err := owner.check(ctx); err != nil {
		return nil, err
	}
	owner.parent, owner.name = parent.root, parts[len(parts)-1]
	if err := owner.parent.Mkdir(owner.name, 0700); err != nil {
		return nil, fmt.Errorf("review-client output requires a new directory: %w", err)
	}
	owner.reserved = true
	owner.reservation, err = owner.parent.Lstat(owner.name)
	if err != nil {
		return nil, err
	}
	if !safeReviewClientParent(owner.reservation) || owner.reservation.Mode().Perm() != 0700 {
		return nil, fmt.Errorf("review-client reservation custody uncertain at %q", path)
	}
	directory, err := owner.parent.OpenRoot(owner.name)
	if err != nil {
		return nil, err
	}
	opened, err := directory.Stat(".")
	if err != nil || !sameReviewClientDirectory(owner.reservation, opened) {
		return nil, errors.Join(errors.New("review-client output changed while opening"), err, directory.Close())
	}
	owner.chain = append(owner.chain, reviewClientDirectory{root: directory, name: owner.name, identity: opened})
	if err := owner.check(ctx); err != nil {
		return nil, err
	}
	return owner, nil
}

func safeReviewClientParent(info os.FileInfo) bool {
	if info == nil || !info.IsDir() || info.Mode()&(os.ModeSymlink|os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 || info.Mode().Perm()&0022 != 0 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int64(stat.Uid) == int64(os.Geteuid())
}

func sameReviewClientDirectory(before, after os.FileInfo) bool {
	if before == nil || after == nil || !after.IsDir() || after.Mode()&os.ModeSymlink != 0 || !os.SameFile(before, after) || before.Mode() != after.Mode() {
		return false
	}
	a, aok := before.Sys().(*syscall.Stat_t)
	b, bok := after.Sys().(*syscall.Stat_t)
	return aok && bok && a.Uid == b.Uid && a.Gid == b.Gid
}

func (owner *reviewClientOutput) check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if owner.closed {
		return errors.New("review-client output custody is closed")
	}
	for index, node := range owner.chain {
		opened, err := node.root.Stat(".")
		if err != nil || !sameReviewClientDirectory(node.identity, opened) {
			return errors.Join(fmt.Errorf("review-client directory custody changed; preserve evidence at %q", owner.path), err)
		}
		var named os.FileInfo
		if index == 0 {
			named, err = os.Lstat("/")
		} else {
			named, err = owner.chain[index-1].root.Lstat(node.name)
		}
		if err != nil || !sameReviewClientDirectory(node.identity, named) {
			return errors.Join(fmt.Errorf("review-client directory location changed; preserve evidence at %q", owner.path), err)
		}
	}
	return ctx.Err()
}

// Checked staging never exposes a partial executable under the final name.
// docs/adr/0153-authenticated-review-client-bootstrap.md:71.
func (owner *reviewClientOutput) publish(ctx context.Context, input io.Reader, size int64, digest string) (path string, returned error) {
	if err := owner.check(ctx); err != nil {
		return "", err
	}
	if !owner.reserved || owner.published {
		return "", errors.New("review-client publication ownership unavailable")
	}
	directory := owner.chain[len(owner.chain)-1].root
	stage, err := filepublish.PrepareReader(ctx, directory, ".review-client-", input, size, digest, 0700)
	if err != nil {
		return "", err
	}
	defer func() { returned = errors.Join(returned, stage.Cleanup()) }()
	if err := owner.check(ctx); err != nil {
		return "", err
	}
	if err := stage.PublishNoReplace(ctx, reviewClientName); err != nil {
		return "", err
	}
	owner.published = true
	if err := owner.check(ctx); err != nil {
		return "", err
	}
	// Open without following links, qualify the authenticated bytes, and retain
	// this leaf through final receipt custody. Directory identity alone cannot
	// detect an executable replaced after publication.
	// docs/adr/0153-authenticated-review-client-bootstrap.md:73.
	owner.clientTree, err = installationfs.Open(ctx, owner.path)
	if err != nil {
		return "", err
	}
	opened, err := owner.clientTree.Root.Root.Stat(".")
	if err != nil || !sameReviewClientDirectory(owner.reservation, opened) {
		return "", errors.Join(errors.New("review-client publication directory changed"), err)
	}
	owner.client, err = owner.clientTree.Root.OpenFile(ctx, reviewClientName, 256<<20)
	if err != nil {
		return "", err
	}
	metadata := owner.client.Metadata()
	if metadata.Size != size || metadata.Mode&07777 != 0700 {
		return "", fmt.Errorf("review-client executable size or permissions changed at %q", owner.path)
	}
	owner.digest = digest
	if err := owner.checkContents(ctx); err != nil {
		return "", err
	}
	return filepath.Join(owner.path, reviewClientName), nil
}

func (owner *reviewClientOutput) checkContents(ctx context.Context) (returned error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	directory, err := owner.chain[len(owner.chain)-1].root.Open(".")
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, directory.Close()) }()
	entries, err := directory.ReadDir(2)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if len(entries) != 1 || entries[0].Name() != reviewClientName {
		return fmt.Errorf("review-client output contains unexpected entries; preserve evidence at %q", owner.path)
	}
	if owner.client == nil {
		return fmt.Errorf("review-client executable custody unavailable; preserve evidence at %q", owner.path)
	}
	digest, _, err := owner.client.Digest(ctx, 256<<20)
	if err != nil || digest != owner.digest {
		return errors.Join(fmt.Errorf("review-client executable changed; preserve evidence at %q", owner.path), err)
	}
	return ctx.Err()
}

// Cleanup ignores cancellation only to release proven ownership; it never removes
// a populated or replaced directory. docs/adr/0153-authenticated-review-client-bootstrap.md:92.
func (owner *reviewClientOutput) close(ctx context.Context) error {
	if owner.closed {
		return owner.closeError
	}
	cleanup := context.WithoutCancel(ctx)
	err := owner.check(cleanup)
	if owner.reserved && err == nil {
		if owner.published {
			err = owner.checkContents(cleanup)
		} else {
			named, checkErr := owner.parent.Lstat(owner.name)
			if checkErr != nil || !sameReviewClientDirectory(owner.reservation, named) || !safeReviewClientParent(named) {
				err = errors.Join(fmt.Errorf("review-client reservation changed; preserve evidence at %q", owner.path), checkErr)
			} else if removeErr := owner.parent.Remove(owner.name); removeErr != nil {
				err = fmt.Errorf("review-client empty-directory cleanup failed; preserve evidence at %q: %w", owner.path, removeErr)
			}
		}
	}
	if owner.client != nil {
		err = errors.Join(err, owner.client.Close())
	}
	if owner.clientTree != nil {
		err = errors.Join(err, owner.clientTree.Close())
	}
	for index := len(owner.chain) - 1; index >= 0; index-- {
		err = errors.Join(err, owner.chain[index].root.Close())
	}
	owner.closed, owner.closeError = true, err
	return err
}
