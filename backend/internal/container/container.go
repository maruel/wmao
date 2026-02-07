// Package container wraps md CLI operations for container lifecycle management.
package container

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Start creates and starts an md container for the given branch.
// It does not SSH into it (--no-ssh).
func Start(ctx context.Context, branch string) (string, error) {
	// md start --no-ssh will create the container and return.
	// The container name is md-<repo>-<branch>.
	cmd := exec.CommandContext(ctx, "md", "start", "--no-ssh")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("md start: %w: %s", err, stderr.String())
	}
	// Derive the container name. md uses the repo name from the current
	// directory and the current branch.
	name, err := containerName(ctx)
	if err != nil {
		return "", err
	}
	return name, nil
}

// Diff runs `md diff` and returns the diff output.
func Diff(ctx context.Context, args ...string) (string, error) {
	cmdArgs := append([]string{"diff"}, args...)
	cmd := exec.CommandContext(ctx, "md", cmdArgs...) //nolint:gosec // args are not user-controlled.
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("md diff: %w", err)
	}
	return string(out), nil
}

// Pull pulls changes from the container to the local branch.
func Pull(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "md", "pull")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("md pull: %w: %s", err, stderr.String())
	}
	return nil
}

// Kill stops and removes the container.
func Kill(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "md", "kill")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("md kill: %w: %s", err, stderr.String())
	}
	return nil
}

// containerName returns the md container name for the current repo+branch.
func containerName(ctx context.Context) (string, error) {
	// List containers and find the one for this repo.
	cmd := exec.CommandContext(ctx, "md", "list")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("md list: %w", err)
	}
	// md list outputs lines like: "md-<repo>-<branch>   running   5 minutes ago"
	// Take the most recently created one.
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		fields := strings.Fields(lines[i])
		if len(fields) > 0 && strings.HasPrefix(fields[0], "md-") {
			return fields[0], nil
		}
	}
	return "", errors.New("no md container found in md list output")
}
