package container

import (
	"bytes"
	"context"
	"io"
	"os"

	"github.com/maruel/md"
)

// Lib implements Ops using the md Go library.
type Lib struct {
	Client *md.Client
}

// NewLib creates a Lib backed by an md.Client.
func NewLib(tag string) (*Lib, error) {
	c, err := md.New(tag)
	if err != nil {
		return nil, err
	}
	// c.W = io.Discard
	c.W = os.Stderr
	return &Lib{Client: c}, nil
}

func (l *Lib) container(dir, branch string) *md.Container {
	return l.Client.Container(dir, branch)
}

// Start creates and starts an md container for the current branch.
func (l *Lib) Start(ctx context.Context, dir string, labels []string) (string, error) {
	// The branch is determined by the checked-out branch in dir, same as the
	// CLI implementation. We read it the same way md does internally.
	branch, err := md.GitCurrentBranch(ctx, dir)
	if err != nil {
		return "", err
	}
	c := l.container(dir, branch)
	if err := c.Start(ctx, &md.StartOpts{NoSSH: true, Labels: labels}); err != nil {
		return "", err
	}
	return c.Name, nil
}

// Diff returns the diff output from the container.
func (l *Lib) Diff(ctx context.Context, dir string, args ...string) (string, error) {
	branch, err := md.GitCurrentBranch(ctx, dir)
	if err != nil {
		return "", err
	}
	var stdout bytes.Buffer
	if err := l.container(dir, branch).Diff(ctx, &stdout, io.Discard, args); err != nil {
		return "", err
	}
	return stdout.String(), nil
}

// Pull pulls changes from the container to the local branch.
func (l *Lib) Pull(ctx context.Context, dir string) error {
	branch, err := md.GitCurrentBranch(ctx, dir)
	if err != nil {
		return err
	}
	return l.container(dir, branch).Pull(ctx)
}

// Push pushes local changes into the container.
func (l *Lib) Push(ctx context.Context, dir string) error {
	branch, err := md.GitCurrentBranch(ctx, dir)
	if err != nil {
		return err
	}
	return l.container(dir, branch).Push(ctx)
}

// Kill stops and removes the container.
func (l *Lib) Kill(ctx context.Context, dir string) error {
	branch, err := md.GitCurrentBranch(ctx, dir)
	if err != nil {
		return err
	}
	return l.container(dir, branch).Kill(ctx)
}

// List returns all md containers.
func (l *Lib) List(ctx context.Context) ([]Entry, error) {
	containers, err := l.Client.List(ctx)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, len(containers))
	for i, c := range containers {
		entries[i] = Entry{Name: c.Name, Status: c.State}
	}
	return entries, nil
}
