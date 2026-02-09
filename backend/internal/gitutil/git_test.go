package gitutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

func TestDiscoverRepos(t *testing.T) {
	root := t.TempDir()

	// Create repos at various depths.
	mkGit := func(parts ...string) {
		t.Helper()
		p := append(append([]string{root}, parts...), ".git")
		if err := os.MkdirAll(filepath.Join(p...), 0o750); err != nil {
			t.Fatal(err)
		}
	}

	mkGit("repoA")
	mkGit("org", "repoB")
	mkGit("org", "repoC")
	mkGit("deep", "nested", "repoD")
	mkGit("deep", "nested", "too", "repoE") // depth 4 — excluded at maxDepth=3

	// Hidden directory should be skipped.
	mkGit(".hidden", "repoF")

	// Nested repo inside a repo — recursion should stop at repoA.
	mkGit("repoA", "sub", ".git")

	repos, err := DiscoverRepos(root, 3)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		filepath.Join(root, "deep", "nested", "repoD"),
		filepath.Join(root, "org", "repoB"),
		filepath.Join(root, "org", "repoC"),
		filepath.Join(root, "repoA"),
	}
	slices.Sort(repos)
	slices.Sort(want)

	if !slices.Equal(repos, want) {
		t.Errorf("repos = %v\n want %v", repos, want)
	}
}

func TestDiscoverReposDepthZero(t *testing.T) {
	root := t.TempDir()

	// Root itself is a repo.
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}

	repos, err := DiscoverRepos(root, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 || repos[0] != root {
		t.Errorf("repos = %v, want [%s]", repos, root)
	}
}

func TestMaxBranchSeqNum(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()

	// Initialize a real git repo.
	for _, args := range [][]string{
		{"init"},
		{"commit", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // test helper, args are constant.
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	// No wmao branches → -1.
	n, err := MaxBranchSeqNum(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if n != -1 {
		t.Fatalf("got %d, want -1", n)
	}

	// Create some branches.
	for _, b := range []string{"wmao/w0", "wmao/w3", "wmao/w7", "other/branch"} {
		cmd := exec.CommandContext(ctx, "git", "branch", b) //nolint:gosec // test helper, args are constant.
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git branch %s: %v\n%s", b, err, out)
		}
	}

	n, err = MaxBranchSeqNum(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if n != 7 {
		t.Fatalf("got %d, want 7", n)
	}
}

func TestDiscoverReposEmpty(t *testing.T) {
	root := t.TempDir()
	repos, err := DiscoverRepos(root, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 0 {
		t.Errorf("repos = %v, want empty", repos)
	}
}
