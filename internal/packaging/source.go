package packaging

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/anoop2811/software-factory-template/internal/native"
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
	objects, err := boundedGitOutput(ctx, options.Source, []string{"rev-parse", "--path-format=absolute", "--git-path", "objects"})
	if err != nil {
		return "", fmt.Errorf("locate source objects: %w", err)
	}
	objectPath := strings.TrimSuffix(string(objects), "\n")
	if !filepath.IsAbs(objectPath) || strings.ContainsAny(objectPath, "\r\n") {
		return "", errors.New("unsupported source object directory")
	}
	output, initErr := boundedGitOutput(ctx, workspace.Name(), []string{"init", "--bare", "--quiet", "--template=", "repository"})
	if initErr != nil {
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
	output, err := boundedGitOutput(ctx, repository, []string{"cat-file", "-t", options.Revision})
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
	return extractGitArchive(ctx, repository, options.Revision, workspace, root)
}

// Spool the archive without an in-memory size ceiling, then consume it only
// after confirmed Git ownership. docs/adr/0110-local-compiler-process-ownership.md:89.
func extractGitArchive(ctx context.Context, repository, revision string, workspace, source *os.Root) (err error) {
	archive, err := workspace.OpenFile("committed-source.tar", os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, archive.Close()) }()
	diagnostics, err := workspace.OpenFile("committed-source.stderr", os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, diagnostics.Close()) }()
	input, err := os.Open(os.DevNull)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, input.Close()) }()
	directory, err := os.Getwd()
	if err != nil {
		return err
	}
	command := gitCommand(ctx, repository, "archive", "--format=tar", revision)
	result, runErr := native.ExecuteInherited(ctx, directory, command.Args, environmentMap(command.Env), native.InheritedStreams{Stdin: input, Stdout: archive, Stderr: diagnostics}, 0)
	if runErr != nil || result.Outcome != "completed" || !result.ExitConfirmed || result.OwnershipUnconfirmed || result.ExitCode == nil || *result.ExitCode != 0 {
		if runErr == nil {
			runErr = errors.New("git archive did not establish completed process ownership")
		}
		output, readErr := io.ReadAll(io.NewSectionReader(diagnostics, 0, 64<<10))
		return fmt.Errorf("archive source: %w: %s", errors.Join(ctx.Err(), runErr, readErr), output)
	}
	if _, err = archive.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err = extractArchive(ctx, source, tar.NewReader(archive)); err != nil {
		return fmt.Errorf("extract committed source: %w", err)
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
