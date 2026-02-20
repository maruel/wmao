// Conversion from internal agent.Message types to dto.ClaudeEventMessage for
// the Claude Code raw SSE stream (/api/v1/tasks/{id}/raw_events). Each backend
// has its own raw converter; this is the Claude Code one. See genericconv.go
// for the backend-neutral converter that all backends share.
package server

import (
	"encoding/json"
	"time"

	"github.com/maruel/caic/backend/internal/agent"
	"github.com/maruel/caic/backend/internal/server/dto"
	"github.com/maruel/caic/backend/internal/task"
)

// toolTimingTracker computes per-tool-call duration by recording the timestamp
// when each tool_use is seen and computing the delta when the corresponding
// UserMessage arrives.
type toolTimingTracker struct {
	pending map[string]time.Time // toolUseID → timestamp when tool_use was seen
}

func newToolTimingTracker() *toolTimingTracker {
	return &toolTimingTracker{pending: make(map[string]time.Time)}
}

// convertMessage converts an agent.Message into zero or more EventMessages.
// A single AssistantMessage can produce multiple events (one per content
// block + one usage event). Returns nil for messages that should be filtered
// (RawMessage, etc.).
func (tt *toolTimingTracker) convertMessage(msg agent.Message, now time.Time) []dto.ClaudeEventMessage {
	ts := now.UnixMilli()
	switch m := msg.(type) {
	case *agent.SystemInitMessage:
		if m.Subtype == "init" {
			return []dto.ClaudeEventMessage{{
				Kind: dto.ClaudeEventKindInit,
				Ts:   ts,
				Init: &dto.ClaudeEventInit{
					Model:        m.Model,
					AgentVersion: m.Version,
					SessionID:    m.SessionID,
					Tools:        m.Tools,
					Cwd:          m.Cwd,
				},
			}}
		}
		return []dto.ClaudeEventMessage{{
			Kind:   dto.ClaudeEventKindSystem,
			Ts:     ts,
			System: &dto.ClaudeEventSystem{Subtype: m.Subtype},
		}}
	case *agent.SystemMessage:
		return []dto.ClaudeEventMessage{{
			Kind:   dto.ClaudeEventKindSystem,
			Ts:     ts,
			System: &dto.ClaudeEventSystem{Subtype: m.Subtype},
		}}
	case *agent.AssistantMessage:
		return tt.convertAssistant(m, ts, now)
	case *agent.UserMessage:
		return tt.convertUser(m, ts, now)
	case *agent.ResultMessage:
		return []dto.ClaudeEventMessage{{
			Kind: dto.ClaudeEventKindResult,
			Ts:   ts,
			Result: &dto.ClaudeEventResult{
				Subtype:      m.Subtype,
				IsError:      m.IsError,
				Result:       m.Result,
				DiffStat:     toDTODiffStat(m.DiffStat),
				TotalCostUSD: m.TotalCostUSD,
				Duration:     float64(m.DurationMs) / 1e3,
				DurationAPI:  float64(m.DurationAPIMs) / 1e3,
				NumTurns:     m.NumTurns,
				Usage: dto.ClaudeEventUsage{
					InputTokens:              m.Usage.InputTokens,
					OutputTokens:             m.Usage.OutputTokens,
					CacheCreationInputTokens: m.Usage.CacheCreationInputTokens,
					CacheReadInputTokens:     m.Usage.CacheReadInputTokens,
					ServiceTier:              m.Usage.ServiceTier,
				},
			},
		}}
	case *agent.StreamEvent:
		if m.Event.Type == "content_block_delta" && m.Event.Delta != nil && m.Event.Delta.Type == "text_delta" && m.Event.Delta.Text != "" {
			return []dto.ClaudeEventMessage{{
				Kind:      dto.ClaudeEventKindTextDelta,
				Ts:        ts,
				TextDelta: &dto.ClaudeEventTextDelta{Text: m.Event.Delta.Text},
			}}
		}
		return nil
	case *agent.DiffStatMessage:
		return []dto.ClaudeEventMessage{{
			Kind:     dto.ClaudeEventKindDiffStat,
			Ts:       ts,
			DiffStat: &dto.ClaudeEventDiffStat{DiffStat: toDTODiffStat(m.DiffStat)},
		}}
	default:
		// RawMessage (tool_progress), MetaMessage, etc. — filtered.
		return nil
	}
}

func (tt *toolTimingTracker) convertAssistant(m *agent.AssistantMessage, ts int64, now time.Time) []dto.ClaudeEventMessage {
	var events []dto.ClaudeEventMessage
	for _, block := range m.Message.Content {
		switch block.Type {
		case "text":
			if block.Text != "" {
				events = append(events, dto.ClaudeEventMessage{
					Kind: dto.ClaudeEventKindText,
					Ts:   ts,
					Text: &dto.ClaudeEventText{Text: block.Text},
				})
			}
		case "tool_use":
			tt.pending[block.ID] = now
			switch block.Name {
			case "AskUserQuestion":
				events = append(events, dto.ClaudeEventMessage{
					Kind: dto.ClaudeEventKindAsk,
					Ts:   ts,
					Ask: &dto.ClaudeEventAsk{
						ToolUseID: block.ID,
						Questions: parseClaudeAskInput(block.Input),
					},
				})
			case "TodoWrite":
				if todo := parseClaudeTodoInput(block.ID, block.Input); todo != nil {
					events = append(events, dto.ClaudeEventMessage{
						Kind: dto.ClaudeEventKindTodo,
						Ts:   ts,
						Todo: todo,
					})
				}
			default:
				events = append(events, dto.ClaudeEventMessage{
					Kind: dto.ClaudeEventKindToolUse,
					Ts:   ts,
					ToolUse: &dto.ClaudeEventToolUse{
						ToolUseID: block.ID,
						Name:      block.Name,
						Input:     block.Input,
					},
				})
			}
		}
	}
	// Emit per-turn usage.
	u := m.Message.Usage
	if u.InputTokens > 0 || u.OutputTokens > 0 {
		events = append(events, dto.ClaudeEventMessage{
			Kind: dto.ClaudeEventKindUsage,
			Ts:   ts,
			Usage: &dto.ClaudeEventUsage{
				InputTokens:              u.InputTokens,
				OutputTokens:             u.OutputTokens,
				CacheCreationInputTokens: u.CacheCreationInputTokens,
				CacheReadInputTokens:     u.CacheReadInputTokens,
				ServiceTier:              u.ServiceTier,
				Model:                    m.Message.Model,
			},
		})
	}
	return events
}

func (tt *toolTimingTracker) convertUser(m *agent.UserMessage, ts int64, now time.Time) []dto.ClaudeEventMessage {
	// User text input (no parent tool) vs tool result.
	//
	// NOTE: Claude Code only emits UserMessage with ParentToolUseID for
	// background/async tools (e.g. Task subagent). Synchronous built-in tools
	// (Read, Edit, Grep, Glob, Bash, Write, etc.) execute internally and their
	// results are fed back to the API without emitting a "user" message on the
	// stream-json output. The frontend must infer completion for these tools
	// from subsequent events rather than waiting for an explicit toolResult.
	if m.ParentToolUseID == nil {
		ui := extractUserInput(m.Message)
		if ui.Text == "" && len(ui.Images) == 0 {
			return nil
		}
		return []dto.ClaudeEventMessage{{
			Kind:      dto.ClaudeEventKindUserInput,
			Ts:        ts,
			UserInput: &dto.ClaudeEventUserInput{Text: ui.Text, Images: ui.Images},
		}}
	}
	toolUseID := *m.ParentToolUseID
	var duration float64
	if started, ok := tt.pending[toolUseID]; ok {
		duration = now.Sub(started).Seconds()
		delete(tt.pending, toolUseID)
	}
	errText := extractToolError(m.Message)
	return []dto.ClaudeEventMessage{{
		Kind: dto.ClaudeEventKindToolResult,
		Ts:   ts,
		ToolResult: &dto.ClaudeEventToolResult{
			ToolUseID: toolUseID,
			Duration:  duration,
			Error:     errText,
		},
	}}
}

// parseTodoInput extracts typed TodoItem data from a TodoWrite tool input
// for the generic event stream.
func parseTodoInput(toolUseID string, raw json.RawMessage) *dto.EventTodo {
	var input struct {
		Todos []dto.TodoItem `json:"todos"`
	}
	if json.Unmarshal(raw, &input) != nil || len(input.Todos) == 0 {
		return nil
	}
	return &dto.EventTodo{ToolUseID: toolUseID, Todos: input.Todos}
}

// parseClaudeTodoInput extracts typed ClaudeTodoItem data from a TodoWrite
// tool input for the Claude raw stream.
func parseClaudeTodoInput(toolUseID string, raw json.RawMessage) *dto.ClaudeEventTodo {
	var input struct {
		Todos []dto.ClaudeTodoItem `json:"todos"`
	}
	if json.Unmarshal(raw, &input) != nil || len(input.Todos) == 0 {
		return nil
	}
	return &dto.ClaudeEventTodo{ToolUseID: toolUseID, Todos: input.Todos}
}

// parseAskInput extracts typed AskQuestion data from the opaque tool input
// for the generic event stream.
func parseAskInput(raw json.RawMessage) []dto.AskQuestion {
	var input struct {
		Questions []dto.AskQuestion `json:"questions"`
	}
	if json.Unmarshal(raw, &input) == nil {
		return input.Questions
	}
	return nil
}

// parseClaudeAskInput extracts typed ClaudeAskQuestion data from the opaque
// tool input for the Claude raw stream.
func parseClaudeAskInput(raw json.RawMessage) []dto.ClaudeAskQuestion {
	var input struct {
		Questions []dto.ClaudeAskQuestion `json:"questions"`
	}
	if json.Unmarshal(raw, &input) == nil {
		return input.Questions
	}
	return nil
}

// userInput holds the text and optional images extracted from a synthetic
// UserMessage. See syntheticUserInput in task.go for the two shapes:
//   - {"role":"user","content":"text"}           (text only)
//   - {"role":"user","content":[...blocks...]}   (images + optional text)
type userInput struct {
	Text   string
	Images []dto.ImageData
}

// extractUserInput extracts text and images from a user input message.
func extractUserInput(raw json.RawMessage) userInput {
	if len(raw) == 0 {
		return userInput{}
	}
	// Try text-only shape first (most common).
	var textMsg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	if json.Unmarshal(raw, &textMsg) == nil && textMsg.Role == "user" && textMsg.Content != "" {
		return userInput{Text: textMsg.Content}
	}
	// Try content-block array shape (images).
	var blockMsg struct {
		Role    string `json:"role"`
		Content []struct {
			Type   string `json:"type"`
			Text   string `json:"text,omitempty"`
			Source *struct {
				MediaType string `json:"media_type"`
				Data      string `json:"data"`
			} `json:"source,omitempty"`
		} `json:"content"`
	}
	if json.Unmarshal(raw, &blockMsg) == nil && blockMsg.Role == "user" {
		var ui userInput
		for _, b := range blockMsg.Content {
			switch b.Type {
			case "text":
				ui.Text = b.Text
			case "image":
				if b.Source != nil {
					ui.Images = append(ui.Images, dto.ImageData{
						MediaType: b.Source.MediaType,
						Data:      b.Source.Data,
					})
				}
			}
		}
		return ui
	}
	return userInput{}
}

// dtoPromptToAgent converts dto.Prompt to agent.Prompt at the server boundary.
func dtoPromptToAgent(p dto.Prompt) agent.Prompt {
	var images []agent.ImageData
	if len(p.Images) > 0 {
		images = make([]agent.ImageData, len(p.Images))
		for i, img := range p.Images {
			images[i] = agent.ImageData{MediaType: img.MediaType, Data: img.Data}
		}
	}
	return agent.Prompt{Text: p.Text, Images: images}
}

// toDTOHarness converts agent.Harness to dto.Harness at the server boundary.
func toDTOHarness(h agent.Harness) dto.Harness {
	return dto.Harness(h)
}

// toAgentHarness converts dto.Harness to agent.Harness at the server boundary.
func toAgentHarness(h dto.Harness) agent.Harness {
	return agent.Harness(h)
}

// toDTOSafetyIssues converts []task.SafetyIssue to []dto.SafetyIssue at the
// server boundary.
func toDTOSafetyIssues(issues []task.SafetyIssue) []dto.SafetyIssue {
	if len(issues) == 0 {
		return nil
	}
	out := make([]dto.SafetyIssue, len(issues))
	for i, si := range issues {
		out[i] = dto.SafetyIssue{File: si.File, Kind: si.Kind, Detail: si.Detail}
	}
	return out
}

// toDTODiffStat converts agent.DiffStat to dto.DiffStat at the server boundary.
func toDTODiffStat(ds agent.DiffStat) dto.DiffStat {
	if len(ds) == 0 {
		return nil
	}
	out := make(dto.DiffStat, len(ds))
	for i, f := range ds {
		out[i] = dto.DiffFileStat{Path: f.Path, Added: f.Added, Deleted: f.Deleted, Binary: f.Binary}
	}
	return out
}

// extractToolError checks if a UserMessage contains an error indicator.
func extractToolError(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var msg struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"is_error"`
	}
	if json.Unmarshal(raw, &msg) == nil && msg.IsError {
		for _, c := range msg.Content {
			if c.Type == "text" && c.Text != "" {
				return c.Text
			}
		}
	}
	return ""
}
