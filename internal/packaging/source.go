package packaging

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

func gitCommand(ctx context.Context, source string, args ...string) *exec.Cmd {
	// #nosec G204 -- Fixed Git executable and argument boundaries; revision validated as full SHA.
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", source}, args...)...)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "GIT_") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "GIT_NO_REPLACE_OBJECTS=1", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
	return cmd
}

// Use only source objects: local attributes and replacement refs must never
// change committed bytes (docs/adr/0060-runtime-source-bundles.md:13).
func isolatedRepository(ctx context.Context, options Options, workspace *os.Root) (string, error) {
	probe := gitCommand(ctx, options.Source, "rev-parse", "--path-format=absolute", "--git-path", "objects")
	objects, err := probe.Output()
	if err != nil {
		return "", fmt.Errorf("locate source objects: %w", err)
	}
	objectPath := strings.TrimSuffix(string(objects), "\n")
	if !filepath.IsAbs(objectPath) || strings.ContainsAny(objectPath, "\r\n") {
		return "", errors.New("unsupported source object directory")
	}
	command := gitCommand(ctx, workspace.Name(), "init", "--bare", "--quiet", "--template=", "repository")
	if output, initErr := command.CombinedOutput(); initErr != nil {
		return "", fmt.Errorf("initialize source view: %w: %s", initErr, output)
	}
	if err = workspace.MkdirAll("repository/objects/info", 0700); err != nil {
		return "", err
	}
	if err = workspace.MkdirAll("repository/info", 0700); err != nil {
		return "", err
	}
	if err = writeFile(workspace, "repository/objects/info/alternates", []byte(objectPath+"\n"), 0600); err != nil {
		return "", err
	}
	if err = writeFile(workspace, "repository/info/attributes", []byte("** -export-ignore -export-subst\n"), 0600); err != nil {
		return "", err
	}
	return filepath.Join(workspace.Name(), "repository"), nil
}

func extractSource(ctx context.Context, options Options, workspace *os.Root) error {
	repository, err := isolatedRepository(ctx, options, workspace)
	if err != nil {
		return err
	}
	typeCommand := gitCommand(ctx, repository, "cat-file", "-t", options.Revision)
	output, err := typeCommand.CombinedOutput()
	if err != nil {
		return fmt.Errorf("read source revision: %w: %s", err, output)
	}
	if strings.TrimSpace(string(output)) != "commit" {
		return errors.New("source revision must identify a commit")
	}
	root, err := workspace.OpenRoot("source")
	if err != nil {
		return err
	}
	defer root.Close()
	command := gitCommand(ctx, repository, "archive", "--format=tar", options.Revision)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	stdout, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	if err = command.Start(); err != nil {
		return fmt.Errorf("archive source: %w", err)
	}
	extractErr := extractArchive(ctx, root, tar.NewReader(stdout))
	if extractErr != nil {
		_ = command.Process.Kill()
	}
	waitErr := command.Wait()
	if extractErr != nil {
		return fmt.Errorf("extract committed source: %w", extractErr)
	}
	if waitErr != nil {
		return fmt.Errorf("archive source: %w: %s", waitErr, stderr.String())
	}
	return nil
}

func extractArchive(ctx context.Context, root *os.Root, archive *tar.Reader) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		name := strings.TrimSuffix(header.Name, "/")
		if path.IsAbs(name) || path.Clean(name) != name || strings.ContainsAny(name, "\\\x00") {
			return fmt.Errorf("unsafe archive path %q", header.Name)
		}
		if name != "go.mod" && name != "go.sum" && name != "cmd/factory" && name != "internal" && !strings.HasPrefix(name, "cmd/factory/") && !strings.HasPrefix(name, "internal/") {
			continue
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err = root.MkdirAll(name, 0700); err != nil {
				return err
			}
		case tar.TypeReg:
			if err = extractFile(root, name, archive); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported source archive entry %q (symlinks are forbidden)", name)
		}
	}
}

func extractFile(root *os.Root, name string, contents io.Reader) error {
	if err := root.MkdirAll(path.Dir(name), 0700); err != nil {
		return err
	}
	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, contents)
	return errors.Join(copyErr, file.Close())
}
