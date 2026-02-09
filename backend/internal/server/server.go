// Package server provides the HTTP server serving the API and embedded
// frontend.
package server

import (
	"context"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/maruel/wmao/backend/frontend"
	"github.com/maruel/wmao/backend/internal/gitutil"
	"github.com/maruel/wmao/backend/internal/task"
)

// Server is the HTTP server for the wmao web UI.
type Server struct {
	runner *task.Runner
	mu     sync.Mutex
	tasks  []*taskEntry
}

type taskEntry struct {
	task   *task.Task
	result *task.Result
	done   chan struct{}
}

// taskJSON is the JSON representation sent to the frontend.
type taskJSON struct {
	ID         int     `json:"id"`
	Task       string  `json:"task"`
	Branch     string  `json:"branch"`
	Container  string  `json:"container"`
	State      string  `json:"state"`
	DiffStat   string  `json:"diffStat"`
	CostUSD    float64 `json:"costUSD"`
	DurationMs int64   `json:"durationMs"`
	NumTurns   int     `json:"numTurns"`
	Error      string  `json:"error,omitempty"`
	Result     string  `json:"result,omitempty"`
}

// New creates a new Server.
func New(ctx context.Context, maxTurns int, logDir string) (*Server, error) {
	branch, err := gitutil.CurrentBranch(ctx)
	if err != nil {
		return nil, err
	}
	return &Server{
		runner: &task.Runner{BaseBranch: branch, MaxTurns: maxTurns, LogDir: logDir},
	}, nil
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/tasks", s.handleListTasks)
	mux.HandleFunc("POST /api/tasks", s.handleCreateTask(ctx))

	// Serve embedded frontend.
	dist, err := fs.Sub(frontend.Files, "dist")
	if err != nil {
		return err
	}
	mux.Handle("GET /", http.FileServerFS(dist))

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		BaseContext: func(_ net.Listener) context.Context {
			return ctx
		},
	}
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	slog.Info("listening", "addr", addr)
	return srv.ListenAndServe()
}

func (s *Server) handleListTasks(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	out := make([]taskJSON, len(s.tasks))
	for i, e := range s.tasks {
		out[i] = toJSON(i, e)
	}
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Server) handleCreateTask(ctx context.Context) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Prompt string `json:"prompt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Prompt == "" {
			http.Error(w, "prompt is required", http.StatusBadRequest)
			return
		}

		t := &task.Task{Prompt: req.Prompt}
		entry := &taskEntry{task: t, done: make(chan struct{})}

		s.mu.Lock()
		s.tasks = append(s.tasks, entry)
		s.mu.Unlock()

		// Run in background using the server context, not the request context.
		go func() {
			defer close(entry.done)
			result := s.runner.Run(ctx, t)
			s.mu.Lock()
			entry.result = &result
			s.mu.Unlock()
		}()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
	}
}

func toJSON(id int, e *taskEntry) taskJSON {
	j := taskJSON{
		ID:        id,
		Task:      e.task.Prompt,
		Branch:    e.task.Branch,
		Container: e.task.Container,
		State:     e.task.State.String(),
	}
	if e.result != nil {
		j.DiffStat = e.result.DiffStat
		j.CostUSD = e.result.CostUSD
		j.DurationMs = e.result.DurationMs
		j.NumTurns = e.result.NumTurns
		j.Result = e.result.AgentResult
		if e.result.Err != nil {
			j.Error = e.result.Err.Error()
		}
	}
	return j
}
