package task

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maruel/wmao/backend/internal/agent"
)

func writeLogFile(t *testing.T, dir, name string, lines ...string) {
	t.Helper()
	data := make([]byte, 0, len(lines)*64)
	for _, l := range lines {
		data = append(data, l...)
		data = append(data, '\n')
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestLoadBranchLogs(t *testing.T) {
	t.Run("EmptyDir", func(t *testing.T) {
		if lt := LoadBranchLogs("", "wmao/w0"); lt != nil {
			t.Error("expected nil for empty logDir")
		}
	})
	t.Run("NoMatch", func(t *testing.T) {
		dir := t.TempDir()
		meta := mustJSON(t, agent.MetaMessage{MessageType: "wmao_meta", Prompt: "other", Branch: "wmao/w9"})
		writeLogFile(t, dir, "20260101T000000-wmao-w9.jsonl", meta)

		if lt := LoadBranchLogs(dir, "wmao/w0"); lt != nil {
			t.Error("expected nil when no files match branch")
		}
	})
	t.Run("SingleFile", func(t *testing.T) {
		dir := t.TempDir()
		meta := mustJSON(t, agent.MetaMessage{MessageType: "wmao_meta", Prompt: "fix bug", Repo: "test", Branch: "wmao/w0"})
		init := mustJSON(t, agent.SystemInitMessage{MessageType: "system", Subtype: "init", SessionID: "sid-1"})
		asst := mustJSON(t, agent.AssistantMessage{MessageType: "assistant"})
		result := mustJSON(t, agent.ResultMessage{MessageType: "result", Result: "done"})
		writeLogFile(t, dir, "20260101T000000-wmao-w0.jsonl", meta, init, asst, result)

		lt := LoadBranchLogs(dir, "wmao/w0")
		if lt == nil {
			t.Fatal("expected non-nil LoadedTask")
		}
		if lt.Prompt != "fix bug" {
			t.Errorf("Prompt = %q, want %q", lt.Prompt, "fix bug")
		}
		if len(lt.Msgs) != 3 {
			t.Fatalf("Msgs len = %d, want 3", len(lt.Msgs))
		}
		if lt.Msgs[0].Type() != "system" {
			t.Errorf("Msgs[0].Type() = %q, want %q", lt.Msgs[0].Type(), "system")
		}
	})
	t.Run("MultipleFiles", func(t *testing.T) {
		dir := t.TempDir()

		// First session.
		meta1 := mustJSON(t, agent.MetaMessage{MessageType: "wmao_meta", Prompt: "fix bug", Repo: "test", Branch: "wmao/w0"})
		asst1 := mustJSON(t, agent.AssistantMessage{MessageType: "assistant"})
		writeLogFile(t, dir, "20260101T000000-wmao-w0.jsonl", meta1, asst1)

		// Second session.
		meta2 := mustJSON(t, agent.MetaMessage{MessageType: "wmao_meta", Prompt: "fix bug", Repo: "test", Branch: "wmao/w0"})
		init2 := mustJSON(t, agent.SystemInitMessage{MessageType: "system", Subtype: "init", SessionID: "sid-2"})
		asst2 := mustJSON(t, agent.AssistantMessage{MessageType: "assistant"})
		writeLogFile(t, dir, "20260101T010000-wmao-w0.jsonl", meta2, init2, asst2)

		lt := LoadBranchLogs(dir, "wmao/w0")
		if lt == nil {
			t.Fatal("expected non-nil LoadedTask")
		}
		if len(lt.Msgs) != 3 {
			t.Fatalf("Msgs len = %d, want 3", len(lt.Msgs))
		}
		if lt.Prompt != "fix bug" {
			t.Errorf("Prompt = %q, want %q", lt.Prompt, "fix bug")
		}
	})
	t.Run("NonexistentDir", func(t *testing.T) {
		if lt := LoadBranchLogs("/nonexistent/path", "wmao/w0"); lt != nil {
			t.Error("expected nil for nonexistent dir")
		}
	})
	t.Run("NoFalseMatch", func(t *testing.T) {
		dir := t.TempDir()
		// Log file for wmao/w10 should NOT match wmao/w1.
		meta := mustJSON(t, agent.MetaMessage{MessageType: "wmao_meta", Prompt: "task", Branch: "wmao/w10"})
		asst := mustJSON(t, agent.AssistantMessage{MessageType: "assistant"})
		writeLogFile(t, dir, "20260101T000000-wmao-w10.jsonl", meta, asst)

		if lt := LoadBranchLogs(dir, "wmao/w1"); lt != nil {
			t.Error("wmao/w10 log should not match wmao/w1")
		}
	})
}

func TestLoadLogs(t *testing.T) {
	t.Run("Valid", func(t *testing.T) {
		dir := t.TempDir()
		meta := mustJSON(t, agent.MetaMessage{MessageType: "wmao_meta", Prompt: "task1", Repo: "r", Branch: "wmao/w0"})
		asst := mustJSON(t, agent.AssistantMessage{MessageType: "assistant"})
		trailer := mustJSON(t, agent.MetaResultMessage{MessageType: "wmao_result", State: "done"})
		writeLogFile(t, dir, "20260101T000000-wmao-w0.jsonl", meta, asst, trailer)

		// Non-jsonl file should be ignored.
		if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hello"), 0o600); err != nil {
			t.Fatal(err)
		}

		tasks, err := LoadLogs(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(tasks) != 1 {
			t.Fatalf("len = %d, want 1", len(tasks))
		}
		if tasks[0].Prompt != "task1" {
			t.Errorf("Prompt = %q, want %q", tasks[0].Prompt, "task1")
		}
		if tasks[0].State != StateDone {
			t.Errorf("State = %v, want %v", tasks[0].State, StateDone)
		}
	})
	t.Run("NotExist", func(t *testing.T) {
		tasks, err := LoadLogs(filepath.Join(t.TempDir(), "nope"))
		if err != nil {
			t.Fatal(err)
		}
		if tasks != nil {
			t.Error("expected nil for nonexistent dir")
		}
	})
	t.Run("BadHeader", func(t *testing.T) {
		dir := t.TempDir()
		writeLogFile(t, dir, "bad.jsonl", `{"type":"not_meta"}`)

		tasks, err := LoadLogs(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(tasks) != 0 {
			t.Errorf("len = %d, want 0", len(tasks))
		}
	})
}

func TestParseState(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want State
	}{
		{"done", StateDone},
		{"failed", StateFailed},
		{"ended", StateEnded},
		{"unknown", StateFailed},
	} {
		t.Run(tt.in, func(t *testing.T) {
			if got := parseState(tt.in); got != tt.want {
				t.Errorf("parseState(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestStringError(t *testing.T) {
	e := stringError("boom")
	if !strings.Contains(e.Error(), "boom") {
		t.Errorf("Error() = %q, want containing %q", e.Error(), "boom")
	}
}
