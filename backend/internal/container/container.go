// Package container wraps md container lifecycle operations.
package container

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/maruel/md"
)

// New creates an md.Client for container operations.
func New(tag string) (*md.Client, error) {
	c, err := md.New(tag)
	if err != nil {
		return nil, err
	}
	c.W = os.Stderr
	return c, nil
}

// LabelValue returns the value of a Docker label on a running container.
//
// Returns empty string if the label is not set.
func LabelValue(ctx context.Context, containerName, label string) (string, error) {
	format := fmt.Sprintf("{{index .Config.Labels %q}}", label)
	cmd := exec.CommandContext(ctx, "docker", "inspect", containerName, "--format", format) //nolint:gosec // containerName and format are not user-controlled.
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("docker inspect label %q on %s: %w", label, containerName, err)
	}
	v := strings.TrimSpace(string(out))
	if v == "<no value>" {
		return "", nil
	}
	return v, nil
}

// BranchFromContainer derives the git branch name from a container name by
// stripping the "md-<repo>-" prefix and restoring the "caic/" prefix that was
// flattened to "caic-" by md.
func BranchFromContainer(containerName, repoName string) (string, bool) {
	prefix := "md-" + repoName + "-"
	if !strings.HasPrefix(containerName, prefix) {
		return "", false
	}
	slug := containerName[len(prefix):]
	// md replaces "/" with "-", so "caic/foo" becomes "caic-foo".
	if strings.HasPrefix(slug, "caic-") {
		return "caic/" + slug[len("caic-"):], true
	}
	return slug, true
}
