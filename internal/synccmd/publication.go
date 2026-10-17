package synccmd

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"

	"github.com/anoop2811/software-factory-template/internal/filepublish"
	"github.com/anoop2811/software-factory-template/internal/installationfs"
)

type publication struct {
	tree       *installationfs.Tree
	files      []*destination
	messages   []string
	claudeLink bool
}
type destination struct {
	path     string
	data     []byte
	mode     os.FileMode
	previous *installationfs.File
	parent   *installationfs.Node
	name     string
	stage    *filepublish.Stage
	link     bool
	linkName string
	linkInfo os.FileInfo
}

func (plan *publication) add(path string, data []byte) {
	plan.files = append(plan.files, &destination{path: path, data: data})
}

// Inspection is read-only across the complete selection, including destinations
// reached after a currently missing directory. No staging precedes validation.
// docs/adr/0099-native-factory-orchestration.md:65.
func (plan *publication) inspect(ctx context.Context) error {
	for _, target := range plan.files {
		if err := plan.inspectFile(ctx, target); err != nil {
			return err
		}
	}
	if plan.claudeLink {
		return plan.inspectClaudeLink(ctx)
	}
	return nil
}

func (plan *publication) inspectFile(ctx context.Context, target *destination) error {
	parent, name, _, err := plan.tree.Parent(ctx, target.path, false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return unsafe(err)
	}
	target.parent, target.name = parent, name
	_, err = parent.Root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return unsafe(err)
	}
	previous, err := plan.tree.OpenFile(ctx, target.path, inputLimit)
	if err != nil {
		return unsafe(fmt.Errorf("inspect destination %s: %w", target.path, err))
	}
	target.previous = previous
	return nil
}

func (plan *publication) inspectClaudeLink(ctx context.Context) error {
	parent := plan.tree.Root
	info, err := parent.Root.Lstat("CLAUDE.md")
	if errors.Is(err, os.ErrNotExist) {
		plan.files = append(plan.files, &destination{path: "CLAUDE.md", parent: parent, name: "CLAUDE.md", link: true})
		plan.claudeMessage("sync-claude: symlinked CLAUDE.md → AGENTS.md")
		return nil
	}
	if err != nil {
		return unsafe(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		link, err := parent.Root.Readlink("CLAUDE.md")
		if err != nil || link != "AGENTS.md" {
			return unsafe(errors.Join(errors.New("unrelated CLAUDE.md link"), err))
		}
		after, err := parent.Root.Lstat("CLAUDE.md")
		if err != nil || !os.SameFile(info, after) {
			return unsafe(errors.Join(errors.New("CLAUDE.md link changed during inspection"), err))
		}
		return plan.tree.Check(ctx)
	}
	target := &destination{path: "CLAUDE.md", parent: parent, name: "CLAUDE.md", link: true}
	if err := plan.inspectFile(ctx, target); err != nil {
		return err
	}
	if target.previous == nil {
		return unsafe(errors.New("CLAUDE.md disappeared during inspection"))
	}
	// Retain the original descriptor even when backup selection later refuses.
	plan.files = append(plan.files, target)
	_, data, err := target.previous.Digest(ctx, 2<<20)
	if err != nil {
		return unsafe(err)
	}
	if len(data) != 0 {
		name := "CLAUDE.md.replaced-by-symlink"
		if _, err := parent.Root.Lstat(name); err == nil {
			name += "." + rand.Text()
		} else if !errors.Is(err, os.ErrNotExist) {
			return unsafe(err)
		}
		if _, err := parent.Root.Lstat(name); !errors.Is(err, os.ErrNotExist) {
			return unsafe(errors.Join(errors.New("CLAUDE.md backup name is occupied"), err))
		}
		original, err := target.previous.File.Stat()
		if err != nil {
			return unsafe(err)
		}
		backup := &destination{path: name, parent: parent, name: name, data: data, mode: original.Mode().Perm()}
		// The backup must become durable before the original is replaced.
		plan.files[len(plan.files)-1] = backup
		plan.files = append(plan.files, target)
		plan.claudeMessage("sync-claude: kept your CLAUDE.md as " + name)
	}
	plan.claudeMessage("sync-claude: symlinked CLAUDE.md → AGENTS.md")
	return nil
}

func (plan *publication) claudeMessage(message string) {
	for i, existing := range plan.messages {
		if existing == "sync-claude: done" {
			plan.messages = append(plan.messages[:i:i], append([]string{message}, plan.messages[i:]...)...)
			return
		}
	}
}

func (plan *publication) prepare(ctx context.Context) error {
	for _, target := range plan.files {
		parent, name, _, err := plan.tree.Parent(ctx, target.path, true)
		if err != nil {
			return unsafe(err)
		}
		target.parent, target.name = parent, name
		if err := plan.checkDestination(ctx, target); err != nil {
			return err
		}
		if target.link {
			target.linkName = ".factory-sync-link-" + rand.Text()
			if err := parent.Root.Symlink("AGENTS.md", target.linkName); err != nil {
				return unsafe(err)
			}
			target.linkInfo, err = parent.Root.Lstat(target.linkName)
			if err != nil {
				return unsafe(err)
			}
			continue
		}
		mode := target.mode
		if mode == 0 {
			mode = 0644
		}
		if target.previous != nil {
			info, err := target.previous.File.Stat()
			if err != nil {
				return unsafe(err)
			}
			mode = info.Mode().Perm()
		}
		target.stage, err = filepublish.Prepare(ctx, parent.Root, ".factory-sync-", target.data, mode)
		if err != nil {
			return unsafe(err)
		}
	}
	return nil
}

func (plan *publication) checkDestination(ctx context.Context, target *destination) error {
	if err := plan.tree.Check(ctx); err != nil {
		return unsafe(err)
	}
	if target.previous != nil {
		return unsafe(target.previous.Check(ctx))
	}
	if _, err := target.parent.Root.Lstat(target.name); !errors.Is(err, os.ErrNotExist) {
		return unsafe(errors.Join(errors.New("destination appeared after inspection: "+target.path), err))
	}
	return nil
}

func (plan *publication) publish(ctx context.Context) error {
	for _, target := range plan.files {
		if err := plan.checkDestination(ctx, target); err != nil {
			return err
		}
		var err error
		switch {
		case target.link:
			info, inspectErr := target.parent.Root.Lstat(target.linkName)
			if inspectErr != nil || !os.SameFile(target.linkInfo, info) {
				return unsafe(errors.Join(errors.New("prepared instruction link changed"), inspectErr))
			}
			if target.previous == nil {
				// Creating the final symlink is exclusive and atomic; the prepared
				// sibling remains owned until cleanup.
				err = target.parent.Root.Symlink("AGENTS.md", target.name)
			} else {
				err = target.parent.Root.Rename(target.linkName, target.name)
				if err == nil {
					target.linkName = ""
				}
			}
		case target.previous == nil:
			err = target.stage.PublishNoReplace(ctx, target.name)
		default:
			err = target.stage.Publish(ctx, target.name)
		}
		if err != nil {
			return unsafe(fmt.Errorf("publish %s: %w", target.path, err))
		}
		if err := target.parent.Sync(ctx); err != nil {
			return unsafe(err)
		}
	}
	return nil
}

func (plan *publication) close() error {
	var failures []error
	for _, target := range plan.files {
		if target.stage != nil {
			failures = append(failures, target.stage.Cleanup())
		}
		if target.linkName != "" && target.linkInfo != nil {
			info, err := target.parent.Root.Lstat(target.linkName)
			if err == nil && os.SameFile(target.linkInfo, info) {
				failures = append(failures, target.parent.Root.Remove(target.linkName))
			} else if !errors.Is(err, os.ErrNotExist) {
				failures = append(failures, errors.Join(errors.New("instruction-link cleanup ownership unavailable"), err))
			}
		}
		if target.previous != nil {
			failures = append(failures, target.previous.Close())
		}
	}
	return unsafe(errors.Join(failures...))
}
