package container

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/maruel/wmao/backend/internal/gitutil"
)

// CLI implements Ops by shelling out to the md CLI.
type CLI struct{}

// Start creates and starts an md container for the current branch.
//
// It does not SSH into it (--no-ssh). Labels are passed as --label flags.
func (CLI) Start(ctx context.Context, dir string, labels []string) (string, error) {
	args := make([]string, 0, 2+2*len(labels))
	args = append(args, "start", "--no-ssh")
	for _, l := range labels {
		args = append(args, "--label", l)
	}
	cmd := exec.CommandContext(ctx, "md", args...) //nolint:gosec // args are constructed from trusted labels, not user input.
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("md start: %w: %s", err, stderr.String())
	}
	name, err := cliContainerName(ctx, dir)
	if err != nil {
		return "", err
	}
	return name, nil
}

// Diff runs `md diff` and returns the diff output.
func (CLI) Diff(ctx context.Context, dir string, args ...string) (string, error) {
	cmdArgs := append([]string{"diff"}, args...)
	cmd := exec.CommandContext(ctx, "md", cmdArgs...) //nolint:gosec // args are not user-controlled.
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("md diff: %w", err)
	}
	return string(out), nil
}

// Pull pulls changes from the container to the local branch.
func (CLI) Pull(ctx context.Context, dir string) error {
	cmd := exec.CommandContext(ctx, "md", "pull")
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("md pull: %w: %s", err, stderr.String())
	}
	return nil
}

// Push pushes local changes into the container.
func (CLI) Push(ctx context.Context, dir string) error {
	cmd := exec.CommandContext(ctx, "md", "push")
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("md push: %w: %s", err, stderr.String())
	}
	return nil
}

// Kill stops and removes the container.
func (CLI) Kill(ctx context.Context, dir string) error {
	cmd := exec.CommandContext(ctx, "md", "kill")
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("md kill: %w: %s", err, stderr.String())
	}
	return nil
}

// List returns all md containers by parsing `md list` output.
func (CLI) List(ctx context.Context) ([]Entry, error) {
	cmd := exec.CommandContext(ctx, "md", "list")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("md list: %w", err)
	}
	return parseList(string(out)), nil
}

// parseList parses md list output into entries.
func parseList(raw string) []Entry {
	var entries []Entry
	for line := range strings.SplitSeq(strings.TrimSpace(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.HasPrefix(fields[0], "md-") {
			entries = append(entries, Entry{Name: fields[0], Status: fields[1]})
		}
	}
	return entries
}

// cliContainerName returns the md container name for the current repo+branch
// by reading the checked-out branch and matching the exact container name.
func cliContainerName(ctx context.Context, dir string) (string, error) {
	branch, err := gitutil.CurrentBranch(ctx, dir)
	if err != nil {
		return "", err
	}
	c := CLI{}
	entries, err := c.List(ctx)
	if err != nil {
		return "", err
	}
	want := containerName(filepath.Base(dir), branch)
	for _, e := range entries {
		if e.Name == want {
			return want, nil
		}
	}
	return "", errors.New("no md container found: " + want)
}

// containerName builds the expected md container name for a repo and branch.
// md uses the format "md-<repo>-<branch>" with "/" replaced by "-".
func containerName(repo, branch string) string {
	return "md-" + repo + "-" + strings.ReplaceAll(branch, "/", "-")
}
