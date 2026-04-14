// Package uib provides a composable Bubble Tea TUI for torus_go_agent.
// It is a clean rewrite of internal/ui with file-level separation and
// sub-model composition. The existing TUI at internal/ui remains untouched.
package uib

import "time"

// ── Stream event types ────────────────────────────────────────────────────────

// StreamEventType identifies the kind of event arriving from the agent stream.
type StreamEventType int

const (
	StreamTextDelta     StreamEventType = iota // Streaming text fragment.
	StreamThinkingDelta                        // Streaming thinking fragment.
	StreamToolStart                            // Tool execution starting.
	StreamToolEnd                              // Tool execution finished.
	StreamStatusUpdate                         // Hook-triggered status phrase.
)

// StreamEventMsg is a unified event from the agent stream.
// It replaces the 3 separate channels (deltaCh, toolCh, statusCh) used by the
// old TUI, funneling every stream event through a single tea.Msg type.
type StreamEventMsg struct {
	Type       StreamEventType
	Delta      string    // StreamTextDelta: the text fragment.
	Thinking   string    // StreamThinkingDelta: thinking text.
	Tool       ToolEvent // StreamToolEnd: completed tool details.
	StatusHook string    // StreamStatusUpdate: hook name.
}

// ToolEvent holds the outcome of a single tool invocation.
type ToolEvent struct {
	Name     string
	Args     map[string]any
	Result   string
	IsError  bool
	FilePath string
	Duration time.Duration
}

// ── Agent lifecycle messages ──────────────────────────────────────────────────

// AgentDoneMsg signals the agent finished successfully.
type AgentDoneMsg struct {
	Text            string
	TokensIn        int // Cumulative input tokens across all turns (for billing/totals).
	TokensOut       int
	Cost            float64
	Elapsed         time.Duration
	LastInputTokens int // Input tokens from the most recent API call (for CTX% display).
}

// AgentErrorMsg signals the agent hit a fatal error.
type AgentErrorMsg struct{ Err error }

// TickMsg drives the bouncing progress bar animation.
type TickMsg time.Time

// WorkflowDoneMsg carries the result of an async workflow execution.
type WorkflowDoneMsg struct {
	Text string
	Err  error
}

// ── Display messages ──────────────────────────────────────────────────────────

// DisplayMsg is a rendered chat message shown in the viewport.
type DisplayMsg struct {
	Role         string     // "user", "assistant", "error", "tool"
	Text         string
	ThinkingText string     // Finalized thinking for this response (inline display).
	IsError      bool
	Rendered     string     // Cached glamour output (invalidated on resize).
	Tool         *ToolEvent // Set when Role == "tool".
	Ts           time.Time
}

// NewDisplayMsg creates a DisplayMsg timestamped to now.
func NewDisplayMsg(role, text string) DisplayMsg {
	return DisplayMsg{Role: role, Text: text, Ts: time.Now()}
}
