package installation

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/anoop2811/software-factory-template/internal/installationfs"
	"github.com/anoop2811/software-factory-template/internal/native"
	"github.com/anoop2811/software-factory-template/internal/transition"
)

// CheckTransitionLockIndex recognizes other index spellings of the same physical
// lock on case-insensitive filesystems. docs/DECISION_LOG.md:9059.
func CheckTransitionLockIndex(ctx context.Context, root string, environment map[string]string, gitOptions []string) (returned error) {
	// The CI caller supplies its hardened Git configuration. This query reads
	// only index names, with a fixed pathspec and no worktree filters.
	argv := append([]string{"git", "--no-optional-locks", "-c", "core.fsmonitor=false"}, gitOptions...)
	// Git folds ASCII case. Wildcards at s and k also admit their Unicode
	// equivalents (long s and Kelvin sign) without scanning descendants.
	argv = append(argv, "ls-files", "--cached", "-z", "--", ":(icase,glob).factory/runtime-tran*ition.loc*")
	child, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := native.ExecuteCommandBounded(child, root, argv, environment, 64<<10, 16<<10)
	if result.Outcome == "output_limit" {
		limit := conflict("transition lock index inspection exceeded its output limit")
		var ownership *native.OwnershipError
		if result.OwnershipUnconfirmed || !result.ExitConfirmed || errors.As(err, &ownership) {
			return errors.Join(limit, child.Err(), err)
		}
		return errors.Join(limit, child.Err())
	}
	if err != nil || !result.ExitConfirmed || result.OwnershipUnconfirmed || result.ExitCode == nil || *result.ExitCode != 0 {
		return errors.Join(errors.New("cannot inspect tracked transition lock"), err)
	}
	if len(result.Stdout) == 0 {
		return ctx.Err()
	}
	if result.Stdout[len(result.Stdout)-1] != 0 {
		return errors.New("invalid tracked transition lock response")
	}
	names := bytes.Split(result.Stdout[:len(result.Stdout)-1], []byte{0})
	aliases := names[:0]
	for _, name := range names {
		if string(name) == transition.LockPath {
			return conflict("private transition lock is tracked")
		}
		if strings.EqualFold(string(name), transition.LockPath) {
			aliases = append(aliases, name)
		}
	}
	if len(aliases) == 0 {
		return ctx.Err()
	}
	tree, err := installationfs.Open(ctx, root)
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, tree.Close()) }()
	lock, err := tree.Stat(ctx, transition.LockPath)
	missingLock := errors.Is(err, os.ErrNotExist)
	if err != nil && !missingLock {
		return err
	}
	parent, _, _ := strings.Cut(transition.LockPath, "/")
	for _, name := range aliases {
		alias := string(name)
		aliasParent, _, _ := strings.Cut(alias, "/")
		if aliasParent != parent {
			// Inspect the top-level entries without following links or acquiring
			// custody of an independently owned application directory.
			canonical, canonicalErr := tree.Stat(ctx, parent)
			other, otherErr := tree.Stat(ctx, aliasParent)
			if canonicalErr != nil && !errors.Is(canonicalErr, os.ErrNotExist) {
				return canonicalErr
			}
			if otherErr != nil && !errors.Is(otherErr, os.ErrNotExist) {
				return otherErr
			}
			if (canonicalErr == nil) != (otherErr == nil) {
				continue
			}
			if canonicalErr == nil && (canonical.Device != other.Device || canonical.Inode != other.Inode) {
				continue
			}
		}
		entry, err := tree.Stat(ctx, alias)
		if errors.Is(err, os.ErrNotExist) {
			if missingLock {
				return conflict("cannot distinguish an absent tracked spelling from the transition lock")
			}
			continue
		}
		if err != nil {
			return err
		}
		if !missingLock && entry.Device == lock.Device && entry.Inode == lock.Inode {
			return conflict("private transition lock is tracked")
		}
	}
	return tree.Check(ctx)
}
