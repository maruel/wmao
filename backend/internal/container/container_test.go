package container

import "testing"

func TestNewLib(t *testing.T) {
	lib, err := NewLib("")
	if err != nil {
		t.Fatal(err)
	}
	if lib.Client == nil {
		t.Fatal("NewLib returned Lib with nil Client")
	}
}

func TestParseList(t *testing.T) {
	raw := `md-wmao-wmao-fix-auth   running   5 minutes ago
md-wmao-wmao-add-tests  stopped   1 hour ago
something-else          running   2 minutes ago`

	entries := parseList(raw)
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[0].Name != "md-wmao-wmao-fix-auth" {
		t.Errorf("entries[0].Name = %q", entries[0].Name)
	}
	if entries[0].Status != "running" {
		t.Errorf("entries[0].Status = %q", entries[0].Status)
	}
	if entries[1].Name != "md-wmao-wmao-add-tests" {
		t.Errorf("entries[1].Name = %q", entries[1].Name)
	}
}

func TestParseListEmpty(t *testing.T) {
	entries := parseList("")
	if len(entries) != 0 {
		t.Fatalf("got %d entries, want 0", len(entries))
	}
}

func TestContainerName(t *testing.T) {
	tests := []struct {
		repo, branch, want string
	}{
		{"wmao", "wmao/w0", "md-wmao-wmao-w0"},
		{"wmao", "main", "md-wmao-main"},
		{"md", "wmao/w0", "md-md-wmao-w0"},
		{"myrepo", "feature/xyz", "md-myrepo-feature-xyz"},
	}
	for _, tt := range tests {
		if got := containerName(tt.repo, tt.branch); got != tt.want {
			t.Errorf("containerName(%q, %q) = %q, want %q", tt.repo, tt.branch, got, tt.want)
		}
	}
}

func TestContainerNameRoundTrip(t *testing.T) {
	// containerName and BranchFromContainer must be inverses for wmao/ branches.
	cases := []struct {
		repo, branch string
	}{
		{"wmao", "wmao/fix-auth"},
		{"wmao", "wmao/w0"},
		{"myrepo", "wmao/fix"},
	}
	for _, tt := range cases {
		name := containerName(tt.repo, tt.branch)
		got, ok := BranchFromContainer(name, tt.repo)
		if !ok {
			t.Errorf("BranchFromContainer(%q, %q) returned !ok", name, tt.repo)
		}
		if got != tt.branch {
			t.Errorf("round-trip(%q, %q): got branch %q, want %q", tt.repo, tt.branch, got, tt.branch)
		}
	}
}

func TestBranchFromContainer(t *testing.T) {
	tests := []struct {
		name      string
		container string
		repo      string
		wantBr    string
		wantOK    bool
	}{
		{
			name:      "standard wmao branch",
			container: "md-wmao-wmao-fix-auth",
			repo:      "wmao",
			wantBr:    "wmao/fix-auth",
			wantOK:    true,
		},
		{
			name:      "non-wmao branch",
			container: "md-wmao-feature-xyz",
			repo:      "wmao",
			wantBr:    "feature-xyz",
			wantOK:    true,
		},
		{
			name:      "wrong repo prefix",
			container: "md-other-wmao-fix",
			repo:      "wmao",
			wantBr:    "",
			wantOK:    false,
		},
		{
			name:      "no md prefix",
			container: "notmd-wmao-fix",
			repo:      "wmao",
			wantBr:    "",
			wantOK:    false,
		},
		{
			name:      "different repo",
			container: "md-myrepo-wmao-fix",
			repo:      "myrepo",
			wantBr:    "wmao/fix",
			wantOK:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			br, ok := BranchFromContainer(tt.container, tt.repo)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if br != tt.wantBr {
				t.Errorf("branch = %q, want %q", br, tt.wantBr)
			}
		})
	}
}
