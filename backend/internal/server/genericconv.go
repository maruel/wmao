// Backend-neutral conversion from agent.Message to dto.EventMessage for SSE.
// Every backend (Claude, Gemini, Codex, …) uses this converter to produce the
// generic event stream served on /api/v1/tasks/{id}/events. The harness-
// specific raw streams (e.g. eventconv.go for Claude) are separate.
package server

import (
	"encoding/json"
	"time"

	"github.com/maruel/caic/backend/internal/agent"
	"github.com/maruel/caic/backend/internal/server/dto"
)

// genericToolTimingTracker mirrors toolTimingTracker but emits
// EventMessage events with a harness field on init.
type genericToolTimingTracker struct {
	harness agent.Harness
	pending map[string]time.Time
}

func newGenericToolTimingTracker(harness agent.Harness) *genericToolTimingTracker {
	return &genericToolTimingTracker{harness: harness, pending: make(map[string]time.Time)}
}

// convertMessage converts an agent.Message into zero or more EventMessages.
func (gt *genericToolTimingTracker) convertMessage(msg agent.Message, now time.Time) []dto.EventMessage {
	ts := now.UnixMilli()
	switch m := msg.(type) {
	case *agent.SystemInitMessage:
		if m.Subtype == "init" {
			return []dto.EventMessage{{
				Kind: dto.EventKindInit,
				Ts:   ts,
				Init: &dto.EventInit{
					Model:        m.Model,
					AgentVersion: m.Version,
					SessionID:    m.SessionID,
					Tools:        m.Tools,
					Cwd:          m.Cwd,
					Harness:      string(gt.harness),
				},
			}}
		}
		return []dto.EventMessage{{
			Kind:   dto.EventKindSystem,
			Ts:     ts,
			System: &dto.EventSystem{Subtype: m.Subtype},
		}}
	case *agent.SystemMessage:
		return []dto.EventMessage{{
			Kind:   dto.EventKindSystem,
			Ts:     ts,
			System: &dto.EventSystem{Subtype: m.Subtype},
		}}
	case *agent.AssistantMessage:
		return gt.convertAssistant(m, ts, now)
	case *agent.UserMessage:
		return gt.convertUser(m, ts, now)
	case *agent.ResultMessage:
		return []dto.EventMessage{{
			Kind: dto.EventKindResult,
			Ts:   ts,
			Result: &dto.EventResult{
				Subtype:      m.Subtype,
				IsError:      m.IsError,
				Result:       m.Result,
				DiffStat:     toDTODiffStat(m.DiffStat),
				TotalCostUSD: m.TotalCostUSD,
				Duration:     float64(m.DurationMs) / 1e3,
				DurationAPI:  float64(m.DurationAPIMs) / 1e3,
				NumTurns:     m.NumTurns,
				Usage: dto.EventUsage{
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
			return []dto.EventMessage{{
				Kind:      dto.EventKindTextDelta,
				Ts:        ts,
				TextDelta: &dto.EventTextDelta{Text: m.Event.Delta.Text},
			}}
		}
		return nil
	case *agent.DiffStatMessage:
		return []dto.EventMessage{{
			Kind:     dto.EventKindDiffStat,
			Ts:       ts,
			DiffStat: &dto.EventDiffStat{DiffStat: toDTODiffStat(m.DiffStat)},
		}}
	default:
		return nil
	}
}

func (gt *genericToolTimingTracker) convertAssistant(m *agent.AssistantMessage, ts int64, now time.Time) []dto.EventMessage {
	var events []dto.EventMessage
	for _, block := range m.Message.Content {
		switch block.Type {
		case "text":
			if block.Text != "" {
				events = append(events, dto.EventMessage{
					Kind: dto.EventKindText,
					Ts:   ts,
					Text: &dto.EventText{Text: block.Text},
				})
			}
		case "tool_use":
			gt.pending[block.ID] = now
			switch block.Name {
			case "AskUserQuestion":
				events = append(events, dto.EventMessage{
					Kind: dto.EventKindAsk,
					Ts:   ts,
					Ask: &dto.EventAsk{
						ToolUseID: block.ID,
						Questions: parseAskInput(block.Input),
					},
				})
			case "TodoWrite":
				if todo := parseTodoInput(block.ID, block.Input); todo != nil {
					events = append(events, dto.EventMessage{
						Kind: dto.EventKindTodo,
						Ts:   ts,
						Todo: todo,
					})
				}
			default:
				events = append(events, dto.EventMessage{
					Kind: dto.EventKindToolUse,
					Ts:   ts,
					ToolUse: &dto.EventToolUse{
						ToolUseID: block.ID,
						Name:      block.Name,
						Input:     block.Input,
					},
				})
			}
		}
	}
	u := m.Message.Usage
	if u.InputTokens > 0 || u.OutputTokens > 0 {
		events = append(events, dto.EventMessage{
			Kind: dto.EventKindUsage,
			Ts:   ts,
			Usage: &dto.EventUsage{
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

func (gt *genericToolTimingTracker) convertUser(m *agent.UserMessage, ts int64, now time.Time) []dto.EventMessage {
	if m.ParentToolUseID == nil {
		ui := extractUserInput(m.Message)
		if ui.Text == "" && len(ui.Images) == 0 {
			return nil
		}
		return []dto.EventMessage{{
			Kind:      dto.EventKindUserInput,
			Ts:        ts,
			UserInput: &dto.EventUserInput{Text: ui.Text, Images: ui.Images},
		}}
	}
	toolUseID := *m.ParentToolUseID
	var duration float64
	if started, ok := gt.pending[toolUseID]; ok {
		duration = now.Sub(started).Seconds()
		delete(gt.pending, toolUseID)
	}
	errText := extractToolError(m.Message)
	return []dto.EventMessage{{
		Kind: dto.EventKindToolResult,
		Ts:   ts,
		ToolResult: &dto.EventToolResult{
			ToolUseID: toolUseID,
			Duration:  duration,
			Error:     errText,
		},
	}}
}

// marshalEvent is a convenience wrapper for json.Marshal on EventMessage.
func marshalEvent(ev *dto.EventMessage) ([]byte, error) {
	return json.Marshal(ev)
}
