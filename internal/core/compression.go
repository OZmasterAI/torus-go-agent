package core

import (
	"fmt"
	"strings"
)

// MessageScore represents the importance of a message.
type MessageScore int

const (
	ScoreZero    MessageScore = 0
	ScoreLow     MessageScore = 1
	ScoreMedium  MessageScore = 2
	ScoreHigh    MessageScore = 3
)

// ScoreMessage assigns an importance score to a message based on heuristics.
func ScoreMessage(m Message) MessageScore {
	if len(m.Content) == 0 {
		return ScoreZero
	}

	// Collect all text content
	var text string
	hasToolUse := false
	hasToolResult := false
	for _, b := range m.Content {
		if b.Type == "text" {
			text += b.Text
		}
		if b.Type == "tool_use" {
			hasToolUse = true
		}
		if b.Type == "tool_result" {
			hasToolResult = true
		}
	}

	textLower := strings.ToLower(text)
	textLen := len(text)

	// Empty or duplicate blocks
	if textLen == 0 && !hasToolUse && !hasToolResult {
		return ScoreZero
	}

	// Tool results with actual output — medium
	if hasToolResult {
		if textLen > 50 {
			return ScoreMedium
		}
		return ScoreLow
	}

	// Tool calls (just invocations) — low
	if hasToolUse {
		return ScoreLow
	}

	// Short acknowledgments — very low
	if textLen < 30 {
		for _, ack := range []string{"ok", "thanks", "got it", "sure", "yes", "no", "done", "good", "nice", "great"} {
			if strings.TrimSpace(textLower) == ack || strings.HasPrefix(textLower, ack+",") || strings.HasPrefix(textLower, ack+".") {
				return ScoreLow
			}
		}
	}

	// User messages with questions — high
	if m.Role == RoleUser && strings.ContainsAny(text, "?") {
		return ScoreHigh
	}

	// Assistant messages with decisions/answers (longer = more likely important)
	if m.Role == RoleAssistant && textLen > 200 {
		return ScoreHigh
	}

	// User instructions (longer messages are usually more important)
	if m.Role == RoleUser && textLen > 100 {
		return ScoreHigh
	}

	return ScoreMedium
}

// CompressMessage shortens a message's text content to maxChars.
// Tool results get summarized to their first line + truncation notice.
// Regular text gets truncated with "..." suffix.
func CompressMessage(m Message, maxChars int) Message {
	compressed := Message{Role: m.Role}
	for _, b := range m.Content {
		switch b.Type {
		case "tool_result":
			content := b.Content
			if len(content) > maxChars {
				// Keep first line + truncation
				firstLine := content
				if idx := strings.Index(content, "\n"); idx > 0 {
					firstLine = content[:idx]
				}
				if len(firstLine) > maxChars {
					firstLine = firstLine[:maxChars]
				}
				content = fmt.Sprintf("%s\n[...truncated from %d chars]", firstLine, len(b.Content))
			}
			compressed.Content = append(compressed.Content, ContentBlock{
				Type:      "tool_result",
				ToolUseID: b.ToolUseID,
				Content:   content,
				IsError:   b.IsError,
			})
		case "text":
			text := b.Text
			if len(text) > maxChars {
				text = text[:maxChars] + "\n[...truncated]"
			}
			compressed.Content = append(compressed.Content, ContentBlock{Type: "text", Text: text})
		default:
			// Keep tool_use and other blocks as-is
			compressed.Content = append(compressed.Content, b)
		}
	}
	return compressed
}

// ContinuousCompress applies gradual compression to a message list based on
// message age and importance score. Recent messages stay verbatim; older messages
// get progressively shorter. Returns a new slice — does not modify the originals.
//
// The keepLast parameter controls how many recent messages are always kept verbatim.
func ContinuousCompress(messages []Message, keepLast int) []Message {
	if keepLast <= 0 {
		keepLast = 10
	}
	n := len(messages)
	if n <= keepLast {
		return messages
	}

	result := make([]Message, n)

	// Recent messages: keep verbatim
	verbatimStart := n - keepLast
	copy(result[verbatimStart:], messages[verbatimStart:])

	// Older messages: compress based on age + score
	for i := 0; i < verbatimStart; i++ {
		score := ScoreMessage(messages[i])
		age := verbatimStart - i // how far back from the verbatim boundary

		// Determine compression level based on age and score
		var maxChars int
		switch {
		case score == ScoreZero:
			// Drop entirely — replace with empty
			result[i] = Message{Role: messages[i].Role}
			continue
		case age > 30 && score <= ScoreLow:
			maxChars = 50 // 1 line
		case age > 20 && score <= ScoreMedium:
			maxChars = 100 // 2-3 lines
		case age > 20 && score == ScoreHigh:
			maxChars = 300 // short paragraph
		case age > 10 && score <= ScoreLow:
			maxChars = 100
		case age > 10 && score <= ScoreMedium:
			maxChars = 200
		case age > 10 && score == ScoreHigh:
			maxChars = 500
		default:
			// age <= 10, not in verbatim zone: light compression
			maxChars = 800
		}

		result[i] = CompressMessage(messages[i], maxChars)
	}

	return result
}

// ZoneBudget configures the token allocation for zone-based context assembly.
type ZoneBudget struct {
	ContextWindow  int // total token budget
	ArchivePercent int // percentage of usable budget for archive zone (default 30)
	OutputReserve  int // tokens reserved for model output (default 4096)
}

// ApplyZoneBudget splits messages into archive (old, capped) and history (recent, flexible)
// zones, trimming each to fit within its token budget. The current turn's message (last in
// the slice) is always preserved in full.
//
// Zone layout:
//   [archive messages — capped at ArchivePercent of usable budget]
//   [history messages — fills remaining budget]
//
// Messages should already be compressed via ContinuousCompress before calling this.
func ApplyZoneBudget(messages []Message, budget ZoneBudget) []Message {
	if budget.ContextWindow <= 0 {
		return messages
	}
	if budget.ArchivePercent <= 0 {
		budget.ArchivePercent = 30
	}
	if budget.OutputReserve <= 0 {
		budget.OutputReserve = 4096
	}

	usable := budget.ContextWindow - budget.OutputReserve
	if usable <= 0 {
		return messages
	}

	archiveBudget := usable * budget.ArchivePercent / 100
	historyBudget := usable - archiveBudget

	n := len(messages)
	if n == 0 {
		return messages
	}

	// Current message (last) is always kept — subtract its cost from history budget
	currentTokens := estimateTokens(messages[n-1:])
	historyBudget -= currentTokens
	if historyBudget < 0 {
		historyBudget = 0
	}

	// Split: find the boundary between archive and history
	// History = messages that fit within historyBudget, working backwards from second-to-last
	historyStart := n - 1 // start with just the current message
	historyTokens := 0
	for i := n - 2; i >= 0; i-- {
		msgTokens := estimateTokens(messages[i : i+1])
		if historyTokens+msgTokens > historyBudget {
			break
		}
		historyTokens += msgTokens
		historyStart = i
	}

	// Archive = messages before historyStart, trimmed to archiveBudget
	var archive []Message
	archiveTokens := 0
	for i := 0; i < historyStart; i++ {
		msgTokens := estimateTokens(messages[i : i+1])
		if archiveTokens+msgTokens > archiveBudget {
			break
		}
		archiveTokens += msgTokens
		archive = append(archive, messages[i])
	}

	// Assemble: archive + history (includes current message)
	result := make([]Message, 0, len(archive)+n-historyStart)
	result = append(result, archive...)
	result = append(result, messages[historyStart:]...)
	return result
}
