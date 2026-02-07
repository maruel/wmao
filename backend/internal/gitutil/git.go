// Package gitutil provides git operations for branch management and pushing.
package gitutil

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// CurrentBranch returns the current git branch name.
func CurrentBranch(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// CreateBranch creates a new branch from the current HEAD and checks it out.
func CreateBranch(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "git", "checkout", "-b", name)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git checkout -b %s: %w: %s", name, err, stderr.String())
	}
	return nil
}

// CheckoutBranch switches to an existing branch.
func CheckoutBranch(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "git", "checkout", name)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git checkout %s: %w: %s", name, err, stderr.String())
	}
	return nil
}

// Push pushes the branch to origin. Returns an error if it fails.
func Push(ctx context.Context, branch string) error {
	cmd := exec.CommandContext(ctx, "git", "push", "origin", branch)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git push origin %s: %w: %s", branch, err, stderr.String())
	}
	return nil
}

// RepoName returns the repository directory name (last component of the
// top-level path).
func RepoName(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse --show-toplevel: %w", err)
	}
	top := strings.TrimSpace(string(out))
	parts := strings.Split(top, "/")
	return parts[len(parts)-1], nil
}
