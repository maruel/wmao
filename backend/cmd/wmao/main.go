package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"

	"github.com/maruel/wmao/backend/internal/gitutil"
	"github.com/maruel/wmao/backend/internal/server"
	"github.com/maruel/wmao/backend/internal/task"
)

func mainImpl() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	maxTurns := flag.Int("max-turns", 0, "max agentic turns per task (0=unlimited)")
	addr := flag.String("http", "", "start web UI on this address (e.g. :8080)")
	logDir := flag.String("logs", "logs", "directory for session JSONL logs (empty to disable)")
	root := flag.String("root", "", "parent directory containing git repos")
	flag.Parse()

	// Web UI mode.
	if *addr != "" {
		if *root == "" {
			return errors.New("-root is required in HTTP mode")
		}
		return serveHTTP(ctx, *addr, *root, *maxTurns, *logDir)
	}

	// CLI mode.
	args := flag.Args()
	if len(args) == 0 {
		return errors.New("usage: wmao [-max-turns N] [-http :8080] [-logs dir] [-root dir] <task> [task...]")
	}
	return runCLI(ctx, args, *root, *maxTurns, *logDir)
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

	baseBranch, err := gitutil.CurrentBranch(ctx, dir)
	if err != nil {
		return fmt.Errorf("determining current branch: %w", err)
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
