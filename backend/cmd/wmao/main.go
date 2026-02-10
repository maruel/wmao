package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/maruel/wmao/backend/internal/agent"
	"github.com/maruel/wmao/backend/internal/container"
	"github.com/maruel/wmao/backend/internal/gitutil"
	"github.com/maruel/wmao/backend/internal/server"
	"github.com/maruel/wmao/backend/internal/task"
)

func mainImpl() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	maxTurns := flag.Int("max-turns", 0, "max agentic turns per task (0=unlimited)")
	addr := flag.String("http", "", "start web UI on this address (e.g. :8080)")
	root := flag.String("root", "", "parent directory containing git repos")
	fake := flag.Bool("fake", false, "use fake container/agent ops (for e2e tests); creates a temp repo when -root is omitted")
	flag.Parse()

	if *fake {
		return serveFake(ctx, *addr, *root)
	}

	logDir := cacheDir()

	// Exit when executable is rebuilt (systemd restarts the service).
	if err := watchExecutable(ctx, cancel); err != nil {
		slog.Warn("failed to watch executable", "err", err)
	}

	// Web UI mode.
	if *addr != "" {
		if *root == "" {
			return errors.New("-root is required in HTTP mode")
		}
		return serveHTTP(ctx, *addr, *root, *maxTurns, logDir)
	}

	// CLI mode.
	args := flag.Args()
	if len(args) == 0 {
		return errors.New("usage: wmao [-max-turns N] [-http :8080] [-root dir] <task> [task...]")
	}
	return runCLI(ctx, args, *root, *maxTurns, logDir)
}

func serveHTTP(ctx context.Context, addr, rootDir string, maxTurns int, logDir string) error {
	srv, err := server.New(ctx, rootDir, maxTurns, logDir)
	if err != nil {
		return err
	}
	err = srv.ListenAndServe(ctx, addr)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func runCLI(ctx context.Context, args []string, rootDir string, maxTurns int, logDir string) error {
	// Determine the repo directory.
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	if rootDir != "" {
		// When -root is set, CWD must be inside a repo under root.
		repos, err := gitutil.DiscoverRepos(rootDir, 3)
		if err != nil {
			return fmt.Errorf("discover repos: %w", err)
		}
		found := false
		for _, r := range repos {
			if strings.HasPrefix(dir, r) {
				dir = r
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("CWD %s is not inside any repo under %s", dir, rootDir)
		}
	}

	baseBranch, err := gitutil.DefaultBranch(ctx, dir)
	if err != nil {
		return fmt.Errorf("determining default branch: %w", err)
	}

	tasks := make([]*task.Task, len(args))
	for i, prompt := range args {
		tasks[i] = &task.Task{
			Prompt:   prompt,
			MaxTurns: maxTurns,
		}
	}

	runner := &task.Runner{BaseBranch: baseBranch, Dir: dir, MaxTurns: maxTurns, LogDir: logDir}

	results := make([]task.Result, len(tasks))
	var wg sync.WaitGroup
	for i, t := range tasks {
		wg.Go(func() {
			results[i] = runner.Run(ctx, t)
		})
	}
	wg.Wait()

	_ = gitutil.CheckoutBranch(ctx, dir, baseBranch)

	fmt.Println()
	fmt.Println("=== Results ===")
	var failed int
	for i := range results {
		printResult(&results[i])
		if results[i].State == task.StateFailed {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d/%d tasks failed", failed, len(results))
	}
	return nil
}

func printResult(r *task.Result) {
	status := "OK"
	if r.State == task.StateFailed {
		status = "FAIL"
	}
	fmt.Printf("\n[%s] %s\n", status, r.Task)
	if r.Branch != "" {
		fmt.Printf("  branch:    %s\n", r.Branch)
	}
	if r.DiffStat != "" {
		fmt.Printf("  changes:\n")
		for line := range strings.SplitSeq(strings.TrimSpace(r.DiffStat), "\n") {
			fmt.Printf("    %s\n", line)
		}
	}
	if r.CostUSD > 0 {
		fmt.Printf("  cost:      $%.4f\n", r.CostUSD)
	}
	if r.DurationMs > 0 {
		fmt.Printf("  duration:  %.1fs\n", float64(r.DurationMs)/1000)
	}
	if r.NumTurns > 0 {
		fmt.Printf("  turns:     %d\n", r.NumTurns)
	}
	if r.Err != nil {
		fmt.Printf("  error:     %v\n", r.Err)
	}
	if r.AgentResult != "" {
		s := r.AgentResult
		if len(s) > 200 {
			s = s[:200] + "..."
		}
		fmt.Printf("  result:    %s\n", s)
	}
}

func main() {
	if err := mainImpl(); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintf(os.Stderr, "wmao: %v\n", err)
		os.Exit(1)
	}
}

// serveFake starts the HTTP server with fake container/agent ops and a temp
// git repo. Used for e2e testing without md CLI or SSH.
func serveFake(ctx context.Context, addr, rootDir string) error {
	if addr == "" {
		addr = ":8090"
	}

	// When -root is not provided, create a temp git repo.
	if rootDir == "" {
		tmpDir, err := os.MkdirTemp("", "wmao-e2e-*")
		if err != nil {
			return err
		}
		defer func() { _ = os.RemoveAll(tmpDir) }()
		clone, err := initFakeRepo(tmpDir)
		if err != nil {
			return fmt.Errorf("init fake repo: %w", err)
		}
		rootDir = filepath.Dir(clone)
	}

	srv, err := server.New(ctx, rootDir, 1, "")
	if err != nil {
		return fmt.Errorf("new server: %w", err)
	}
	srv.SetRunnerOps(&fakeContainer{}, fakeAgentStart)

	err = srv.ListenAndServe(ctx, addr)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// initFakeRepo creates a bare remote and a clone with one commit on main.
func initFakeRepo(tmpDir string) (string, error) {
	bare := filepath.Join(tmpDir, "remote.git")
	clone := filepath.Join(tmpDir, "clone")
	for _, args := range [][]string{
		{"init", "--bare", bare},
		{"init", clone},
		{"-C", clone, "config", "user.name", "Test"},
		{"-C", clone, "config", "user.email", "test@test.com"},
		{"-C", clone, "checkout", "-b", "main"},
	} {
		if err := runGit(args...); err != nil {
			return "", err
		}
	}
	if err := os.WriteFile(filepath.Join(clone, "README.md"), []byte("hello\n"), 0o600); err != nil {
		return "", err
	}
	for _, args := range [][]string{
		{"-C", clone, "add", "."},
		{"-C", clone, "commit", "-m", "init"},
		{"-C", clone, "remote", "add", "origin", bare},
		{"-C", clone, "push", "-u", "origin", "main"},
	} {
		if err := runGit(args...); err != nil {
			return "", err
		}
	}
	return clone, nil
}

func runGit(args ...string) error {
	out, err := exec.Command("git", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %v: %w\n%s", args, err, out)
	}
	return nil
}

// fakeContainer implements container.Ops with no-op operations.
type fakeContainer struct{}

var _ container.Ops = (*fakeContainer)(nil)

func (*fakeContainer) Start(ctx context.Context, dir string) (string, error) {
	branch, err := gitutil.CurrentBranch(ctx, dir)
	if err != nil {
		return "", err
	}
	return "md-test-" + strings.ReplaceAll(branch, "/", "-"), nil
}

func (*fakeContainer) Diff(_ context.Context, _ string, _ ...string) (string, error) {
	return "", nil
}

func (*fakeContainer) Pull(_ context.Context, _ string) error { return nil }
func (*fakeContainer) Push(_ context.Context, _ string) error { return nil }
func (*fakeContainer) Kill(_ context.Context, _ string) error { return nil }

// fakeAgentStart creates a Session backed by a shell process that emits three
// JSON messages (init, assistant, result) then exits.
func fakeAgentStart(_ context.Context, _ string, _ int, msgCh chan<- agent.Message, logW io.Writer, _ string) (*agent.Session, error) {
	script := `read line
echo '{"type":"system","subtype":"init","session_id":"test-session","cwd":"/workspace","model":"fake-model","claude_code_version":"0.0.0-test"}'
echo '{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"I completed the requested task."}]}}'
echo '{"type":"result","subtype":"success","result":"All done.","num_turns":1,"total_cost_usd":0.01,"duration_ms":500}'
`
	cmd := exec.Command("sh", "-c", script)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return agent.NewSession(cmd, stdin, stdout, msgCh, logW), nil
}

// cacheDir returns the wmao log/cache directory, using $XDG_CACHE_HOME/wmao/
// with a fallback to ~/.cache/wmao/.
func cacheDir() string {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		base = filepath.Join(home, ".cache")
	}
	return filepath.Join(base, "wmao")
}

// watchExecutable watches the current executable for modifications and calls
// stop to trigger graceful shutdown when detected. Combined with systemd's
// Restart=always, this enables seamless restarts after a rebuild.
func watchExecutable(ctx context.Context, stop context.CancelFunc) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return err
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	if err := w.Add(exe); err != nil {
		_ = w.Close()
		return err
	}
	go func() {
		defer func() { _ = w.Close() }()
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-w.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Chmod) {
					slog.Info("executable modified, shutting down")
					stop()
					return
				}
			case err, ok := <-w.Errors:
				if !ok {
					return
				}
				slog.Warn("error watching executable", "err", err)
			}
		}
	}()
	return nil
}
