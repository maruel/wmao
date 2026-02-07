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
	flag.Parse()

	// Web UI mode.
	if *addr != "" {
		return serveHTTP(ctx, *addr, *maxTurns)
	}

	// CLI mode.
	args := flag.Args()
	if len(args) == 0 {
		return errors.New("usage: wmao [-max-turns N] [-http :8080] <task> [task...]")
	}
	return runCLI(ctx, args, *maxTurns)
}

func serveHTTP(ctx context.Context, addr string, maxTurns int) error {
	srv, err := server.New(ctx, maxTurns)
	if err != nil {
		return err
	}
	err = srv.ListenAndServe(ctx, addr)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func runCLI(ctx context.Context, args []string, maxTurns int) error {
	baseBranch, err := gitutil.CurrentBranch(ctx)
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

	runner := &task.Runner{BaseBranch: baseBranch, MaxTurns: maxTurns}

	results := make([]task.Result, len(tasks))
	var wg sync.WaitGroup
	for i, t := range tasks {
		wg.Go(func() {
			results[i] = runner.Run(ctx, t)
		})
	}
	wg.Wait()

	_ = gitutil.CheckoutBranch(ctx, baseBranch)

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
