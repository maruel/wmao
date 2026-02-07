package agent

import (
	"strings"
	"testing"
)

func TestParseMessage(t *testing.T) {
	t.Run("SystemInit", func(t *testing.T) {
		line := `{"type":"system","subtype":"init","cwd":"/home/user","session_id":"abc-123","tools":["Bash","Read"],"model":"claude-opus-4-6","claude_code_version":"2.1.34","uuid":"uuid-1"}`
		msg, err := parseMessage([]byte(line))
		if err != nil {
			t.Fatal(err)
		}
		m, ok := msg.(*SystemInitMessage)
		if !ok {
			t.Fatalf("got %T, want *SystemInitMessage", msg)
		}
		if m.Model != "claude-opus-4-6" {
			t.Errorf("model = %q, want %q", m.Model, "claude-opus-4-6")
		}
		if len(m.Tools) != 2 {
			t.Errorf("tools = %v, want 2 items", m.Tools)
		}
	})
	t.Run("Assistant", func(t *testing.T) {
		line := `{"type":"assistant","message":{"model":"claude-opus-4-6","id":"msg_01","role":"assistant","content":[{"type":"text","text":"hello world"}],"usage":{"input_tokens":10,"output_tokens":5}},"session_id":"abc","uuid":"u1"}`
		msg, err := parseMessage([]byte(line))
		if err != nil {
			t.Fatal(err)
		}
		m, ok := msg.(*AssistantMessage)
		if !ok {
			t.Fatalf("got %T, want *AssistantMessage", msg)
		}
		text := TextFromAssistant(m)
		if text != "hello world" {
			t.Errorf("text = %q, want %q", text, "hello world")
		}
	})
	t.Run("Result", func(t *testing.T) {
		line := `{"type":"result","subtype":"success","is_error":false,"duration_ms":1234,"duration_api_ms":1200,"num_turns":3,"result":"done","session_id":"abc","total_cost_usd":0.05,"usage":{"input_tokens":100,"output_tokens":50},"uuid":"u2"}`
		msg, err := parseMessage([]byte(line))
		if err != nil {
			t.Fatal(err)
		}
		m, ok := msg.(*ResultMessage)
		if !ok {
			t.Fatalf("got %T, want *ResultMessage", msg)
		}
		if m.Subtype != "success" {
			t.Errorf("subtype = %q, want %q", m.Subtype, "success")
		}
		if m.TotalCostUSD != 0.05 {
			t.Errorf("cost = %f, want 0.05", m.TotalCostUSD)
		}
		if m.NumTurns != 3 {
			t.Errorf("turns = %d, want 3", m.NumTurns)
		}
	})
	t.Run("RawFallback", func(t *testing.T) {
		line := `{"type":"stream_event","event":{"type":"message_start"}}`
		msg, err := parseMessage([]byte(line))
		if err != nil {
			t.Fatal(err)
		}
		m, ok := msg.(*RawMessage)
		if !ok {
			t.Fatalf("got %T, want *RawMessage", msg)
		}
		if m.Type() != "stream_event" {
			t.Errorf("type = %q, want %q", m.Type(), "stream_event")
		}
	})
}

func TestReadMessages(t *testing.T) {
	t.Run("FullStream", func(t *testing.T) {
		input := strings.Join([]string{
			`{"type":"system","subtype":"init","cwd":"/","session_id":"s","tools":[],"model":"m","claude_code_version":"1","uuid":"u"}`,
			`{"type":"assistant","message":{"model":"m","id":"i","role":"assistant","content":[{"type":"text","text":"hi"}],"usage":{}},"session_id":"s","uuid":"u"}`,
			`{"type":"result","subtype":"success","is_error":false,"duration_ms":100,"num_turns":1,"result":"hi","session_id":"s","total_cost_usd":0.01,"usage":{},"uuid":"u"}`,
		}, "\n")

		ch := make(chan Message, 16)
		result, err := readMessages(strings.NewReader(input), ch)
		close(ch)
		if err != nil {
			t.Fatal(err)
		}
		if result == nil {
			t.Fatal("expected result, got nil")
		}
		if result.Result != "hi" {
			t.Errorf("result = %q, want %q", result.Result, "hi")
		}

		var count int
		for range ch {
			count++
		}
		if count != 3 {
			t.Errorf("message count = %d, want 3", count)
		}
	})
}
