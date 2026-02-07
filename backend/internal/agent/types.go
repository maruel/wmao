package agent

import "encoding/json"

// Message is the interface for all Claude Code streaming JSON messages.
type Message interface {
	// Type returns the message type string.
	Type() string
}

// SystemInitMessage is emitted at session start (type=system, subtype=init).
type SystemInitMessage struct {
	MessageType string   `json:"type"`
	Subtype     string   `json:"subtype"`
	Cwd         string   `json:"cwd"`
	SessionID   string   `json:"session_id"`
	Tools       []string `json:"tools"`
	Model       string   `json:"model"`
	Version     string   `json:"claude_code_version"`
	UUID        string   `json:"uuid"`
}

// Type implements Message.
func (m *SystemInitMessage) Type() string { return "system" }

// SystemMessage is a generic system message (status, compact_boundary, etc.).
type SystemMessage struct {
	MessageType string `json:"type"`
	Subtype     string `json:"subtype"`
	SessionID   string `json:"session_id"`
	UUID        string `json:"uuid"`
}

// Type implements Message.
func (m *SystemMessage) Type() string { return "system" }

// AssistantMessage contains model responses (text or tool_use blocks).
type AssistantMessage struct {
	MessageType     string     `json:"type"`
	Message         APIMessage `json:"message"`
	ParentToolUseID *string    `json:"parent_tool_use_id"`
	SessionID       string     `json:"session_id"`
	UUID            string     `json:"uuid"`
}

// Type implements Message.
func (m *AssistantMessage) Type() string { return "assistant" }

// APIMessage is the Anthropic API message structure.
type APIMessage struct {
	Model        string         `json:"model"`
	ID           string         `json:"id"`
	Role         string         `json:"role"`
	Content      []ContentBlock `json:"content"`
	StopReason   *string        `json:"stop_reason"`
	StopSequence *string        `json:"stop_sequence"`
	Usage        Usage          `json:"usage"`
}

// ContentBlock is a single block in a message's content array.
type ContentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

// Usage tracks token consumption.
type Usage struct {
	InputTokens              int    `json:"input_tokens"`
	OutputTokens             int    `json:"output_tokens"`
	CacheCreationInputTokens int    `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int    `json:"cache_read_input_tokens"`
	ServiceTier              string `json:"service_tier"`
}

// UserMessage contains tool results fed back by Claude Code.
type UserMessage struct {
	MessageType     string          `json:"type"`
	Message         json.RawMessage `json:"message"`
	ParentToolUseID *string         `json:"parent_tool_use_id"`
	SessionID       string          `json:"session_id"`
	UUID            string          `json:"uuid"`
}

// Type implements Message.
func (m *UserMessage) Type() string { return "user" }

// ResultMessage is the terminal message for a query.
type ResultMessage struct {
	MessageType   string          `json:"type"`
	Subtype       string          `json:"subtype"`
	IsError       bool            `json:"is_error"`
	DurationMs    int64           `json:"duration_ms"`
	DurationAPIMs int64           `json:"duration_api_ms"`
	NumTurns      int             `json:"num_turns"`
	Result        string          `json:"result"`
	StopReason    *string         `json:"stop_reason"`
	SessionID     string          `json:"session_id"`
	TotalCostUSD  float64         `json:"total_cost_usd"`
	Usage         Usage           `json:"usage"`
	ModelUsage    json.RawMessage `json:"modelUsage"`
	UUID          string          `json:"uuid"`
}

// Type implements Message.
func (m *ResultMessage) Type() string { return "result" }

// RawMessage is a pass-through for message types we don't need to inspect
// (stream_event, tool_progress, etc.).
type RawMessage struct {
	MessageType string
	Raw         []byte
}

// Type implements Message.
func (m *RawMessage) Type() string { return m.MessageType }
