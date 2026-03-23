package uib

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"torus_go_agent/internal/core"
)

// runAgentStream launches the agent and sends all events through a single
// channel. This is the key architectural change from the old TUI: instead of
// 3 separate channels (deltaCh, toolCh, statusCh), everything goes through
// one eventCh of type StreamEventMsg.
func runAgentStream(agent *core.Agent, input string, eventCh chan<- StreamEventMsg) tea.Cmd {
	return func() tea.Msg {
		start := time.Now()
		var finalText string
		var finalErr error
		var toolStart time.Time
		var totalIn, totalOut int
		var totalCost float64

		for ev := range agent.RunStream(context.Background(), input) {
			switch ev.Type {
			case core.EventAgentTextDelta:
				eventCh <- StreamEventMsg{Type: StreamTextDelta, Delta: ev.Text}
			case core.EventAgentThinkingDelta:
				eventCh <- StreamEventMsg{Type: StreamThinkingDelta, Thinking: ev.Text}
			case core.EventAgentToolStart:
				toolStart = time.Now()
				eventCh <- StreamEventMsg{Type: StreamToolStart}
			case core.EventAgentToolEnd:
				dur := time.Duration(0)
				if !toolStart.IsZero() {
					dur = time.Since(toolStart)
				}
				fp, _ := ev.ToolArgs["file_path"].(string)
				eventCh <- StreamEventMsg{Type: StreamToolEnd, Tool: ToolEvent{
					Name:     ev.ToolName,
					Args:     ev.ToolArgs,
					Result:   ev.ToolResult.Content,
					IsError:  ev.ToolResult.IsError,
					FilePath: fp,
					Duration: dur,
				}}
				toolStart = time.Time{}
			case core.EventAgentTurnEnd:
				if ev.Usage != nil {
					totalIn += ev.Usage.InputTokens
					totalOut += ev.Usage.OutputTokens
					totalCost += ev.Usage.Cost
				}
			case core.EventAgentDone:
				finalText = ev.Text
			case core.EventStatusUpdate:
				eventCh <- StreamEventMsg{Type: StreamStatusUpdate, StatusHook: ev.StatusHook}
			case core.EventAgentError:
				finalErr = ev.Error
			}
		}
		close(eventCh)
		if finalErr != nil {
			return AgentErrorMsg{Err: finalErr}
		}
		return AgentDoneMsg{
			Text:      finalText,
			TokensIn:  totalIn,
			TokensOut: totalOut,
			Cost:      totalCost,
			Elapsed:   time.Since(start),
		}
	}
}

// waitForStreamEvent polls the single event channel for the next event.
func waitForStreamEvent(ch <-chan StreamEventMsg) tea.Cmd {
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return ev
	}
}
