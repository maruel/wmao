// Package agent manages Claude Code processes via the streaming JSON protocol.
package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
)

// Run executes Claude Code over SSH in the given container with the task
// prompt. It streams NDJSON from stdout and returns the final Result.
//
// All intermediate messages are sent to msgCh for logging/observability.
// If logW is non-nil, every raw NDJSON line (input and output) is written to it.
func Run(ctx context.Context, container, task string, maxTurns int, msgCh chan<- Message, logW io.Writer) (*ResultMessage, error) {
	args := []string{
		container,
		"claude", "-p",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--verbose",
		"--dangerously-skip-permissions",
	}
	if maxTurns > 0 {
		args = append(args, "--max-turns", strconv.Itoa(maxTurns))
	}

	cmd := exec.CommandContext(ctx, "ssh", args...) //nolint:gosec // args are not user-controlled.
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = nil // Let stderr go to /dev/null; errors come via JSON.
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start claude: %w", err)
	}

	// Send the user message as NDJSON on stdin, then close to signal EOF.
	if err := writeUserMessage(stdin, task, logW); err != nil {
		return nil, fmt.Errorf("write prompt: %w", err)
	}

	result, parseErr := readMessages(stdout, msgCh, logW)

	if err := cmd.Wait(); err != nil {
		// If we got a result, prefer it over the exit error.
		if result != nil {
			return result, nil
		}
		return nil, fmt.Errorf("claude exited: %w", err)
	}
	if parseErr != nil {
		return nil, fmt.Errorf("parse: %w", parseErr)
	}
	if result == nil {
		return nil, errors.New("claude exited without a result message")
	}
	return result, nil
}

// userInputMessage is the NDJSON message sent to Claude Code via stdin.
type userInputMessage struct {
	Type    string           `json:"type"`
	Message userInputContent `json:"message"`
}

type userInputContent struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// writeUserMessage writes a single user message to w and closes it.
// If logW is non-nil, the same JSON line is also written to the log.
func writeUserMessage(w io.WriteCloser, prompt string, logW io.Writer) error {
	msg := userInputMessage{
		Type:    "user",
		Message: userInputContent{Role: "user", Content: prompt},
	}
	data, err := json.Marshal(msg)
	if err != nil {
		_ = w.Close()
		return err
	}
	data = append(data, '\n')
	if _, err := w.Write(data); err != nil {
		_ = w.Close()
		return err
	}
	if logW != nil {
		_, _ = logW.Write(data)
	}
	return w.Close()
}

// readMessages reads NDJSON lines from r, dispatches to msgCh, and returns
// the terminal ResultMessage. If logW is non-nil, each raw line is written to it.
func readMessages(r io.Reader, msgCh chan<- Message, logW io.Writer) (*ResultMessage, error) {
	scanner := bufio.NewScanner(r)
	// Claude can produce long lines (e.g., base64 images in tool results).
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<20)

	var result *ResultMessage
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		if logW != nil {
			_, _ = logW.Write(line)
			_, _ = logW.Write([]byte{'\n'})
		}
		msg, err := parseMessage(line)
		if err != nil {
			slog.Warn("skipping unparseable message", "err", err, "line", string(line))
			continue
		}
		if msgCh != nil {
			msgCh <- msg
		}
		if rm, ok := msg.(*ResultMessage); ok {
			result = rm
		}
	}
	return result, scanner.Err()
}

// parseMessage decodes a single NDJSON line into a typed Message.
func parseMessage(line []byte) (Message, error) {
	var envelope struct {
		Type    string `json:"type"`
		Subtype string `json:"subtype"`
	}
	if err := json.Unmarshal(line, &envelope); err != nil {
		return nil, fmt.Errorf("unmarshal envelope: %w", err)
	}
	switch envelope.Type {
	case "system":
		switch envelope.Subtype {
		case "init":
			var m SystemInitMessage
			if err := json.Unmarshal(line, &m); err != nil {
				return nil, err
			}
			return &m, nil
		default:
			var m SystemMessage
			if err := json.Unmarshal(line, &m); err != nil {
				return nil, err
			}
			return &m, nil
		}
	case "assistant":
		var m AssistantMessage
		if err := json.Unmarshal(line, &m); err != nil {
			return nil, err
		}
		return &m, nil
	case "user":
		var m UserMessage
		if err := json.Unmarshal(line, &m); err != nil {
			return nil, err
		}
		return &m, nil
	case "result":
		var m ResultMessage
		if err := json.Unmarshal(line, &m); err != nil {
			return nil, err
		}
		return &m, nil
	default:
		// stream_event, tool_progress, etc. — pass through as raw.
		return &RawMessage{MessageType: envelope.Type, Raw: append([]byte(nil), line...)}, nil
	}
}

// TextFromAssistant extracts all text blocks from an assistant message's content.
func TextFromAssistant(m *AssistantMessage) string {
	var parts []string
	for _, b := range m.Message.Content {
		if b.Type == "text" && b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}
