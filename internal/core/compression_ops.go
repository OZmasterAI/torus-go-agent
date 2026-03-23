package core

import (
	"math"
	"strings"

	t "torus_go_agent/internal/types"
)

// Operation represents a semantic unit of work -- a user request through to its resolution.
type Operation struct {
	Messages []t.Message // all messages in this operation
	StartIdx int         // index in original message slice
	EndIdx   int         // end index (exclusive)
	Files    []string    // file paths referenced by tool_use blocks
	Tools    []string    // unique tool names used
	Intent   string      // first user message text (what was asked)
	Outcome  string      // last assistant message text (what was answered)
}

// GroupOperations segments a message slice into semantic operations.
// messages[0] is treated as the system prompt and excluded from operations.
// A new operation starts when a user message contains text content (not just tool_result).
func GroupOperations(messages []t.Message) []Operation {
	if len(messages) <= 1 {
		return nil
	}

	var ops []Operation
	var current *Operation

	for i := 1; i < len(messages); i++ {
		msg := messages[i]

		// Check if this user message starts a new operation.
		// A user message with at least one "text" block (not just tool_results) starts a new op.
		if msg.Role == t.RoleUser && hasTextBlock(msg) {
			// Finalize previous operation if any
			if current != nil {
				current.EndIdx = i
				current.Messages = messages[current.StartIdx:current.EndIdx]
				finalizeOp(current, messages)
				ops = append(ops, *current)
			}
			// Start new operation
			current = &Operation{
				StartIdx: i,
				Intent:   extractText(msg),
			}
			continue
		}

		// Non-starting messages just extend the current operation (tool_results, assistant replies, etc.)
	}

	// Finalize the last operation
	if current != nil {
		current.EndIdx = len(messages)
		current.Messages = messages[current.StartIdx:current.EndIdx]
		finalizeOp(current, messages)
		ops = append(ops, *current)
	}

	return ops
}

// hasTextBlock returns true if the message has at least one content block with type "text".
func hasTextBlock(m t.Message) bool {
	for _, b := range m.Content {
		if b.Type == "text" {
			return true
		}
	}
	return false
}

// extractText returns the concatenated text from all "text" blocks in a message.
func extractText(m t.Message) string {
	var text string
	for _, b := range m.Content {
		if b.Type == "text" {
			if text != "" {
				text += " "
			}
			text += b.Text
		}
	}
	return text
}

// finalizeOp extracts Files, Tools, and Outcome from messages in the operation's range.
func finalizeOp(op *Operation, messages []t.Message) {
	fileSeen := make(map[string]bool)
	toolSeen := make(map[string]bool)

	for i := op.StartIdx; i < op.EndIdx; i++ {
		msg := messages[i]
		for _, b := range msg.Content {
			if b.Type == "tool_use" {
				// Extract tool name (deduplicated)
				if b.Name != "" && !toolSeen[b.Name] {
					toolSeen[b.Name] = true
					op.Tools = append(op.Tools, b.Name)
				}
				// Extract file_path from Input map (deduplicated)
				if b.Input != nil {
					if fp, ok := b.Input["file_path"]; ok {
						if path, ok := fp.(string); ok && path != "" && !fileSeen[path] {
							fileSeen[path] = true
							op.Files = append(op.Files, path)
						}
					}
				}
			}
		}

		// Track the last assistant text as the outcome
		if msg.Role == t.RoleAssistant {
			text := extractText(msg)
			if text != "" {
				op.Outcome = text
			}
		}
	}
}

// RenderOperationTemplate produces a structured summary of an operation.
// Format:
//
//	[Op] <intent>
//	Files: <file1>, <file2>
//	Actions: <tool1>, <tool2>
//	Outcome: <outcome text, truncated to ~200 chars>
func RenderOperationTemplate(op Operation) string {
	var sb strings.Builder
	sb.WriteString("[Op] ")
	sb.WriteString(op.Intent)
	sb.WriteByte('\n')
	if len(op.Files) > 0 {
		sb.WriteString("Files: ")
		sb.WriteString(strings.Join(op.Files, ", "))
		sb.WriteByte('\n')
	}
	if len(op.Tools) > 0 {
		sb.WriteString("Actions: ")
		sb.WriteString(strings.Join(op.Tools, ", "))
		sb.WriteByte('\n')
	}
	outcome := op.Outcome
	if len(outcome) > 200 {
		outcome = outcome[:200] + "..."
	}
	if outcome != "" {
		sb.WriteString("Outcome: ")
		sb.WriteString(outcome)
		sb.WriteByte('\n')
	}
	return sb.String()
}

// OperationToMessage wraps a template summary as a single assistant message.
func OperationToMessage(op Operation) t.Message {
	return t.Message{
		Role:    t.RoleAssistant,
		Content: []t.ContentBlock{{Type: "text", Text: RenderOperationTemplate(op)}},
	}
}

// ScoreOperation returns a relevance score 0.0-1.0 for an operation.
// Signals (weights sum to 1.0):
//   - Recency (0.25): exponential decay with half-life of 4 operations
//   - File overlap (0.25): Jaccard similarity with active operation's files
//   - Outcome significance (0.25): longer outcomes and error keywords score higher
//   - Operation type (0.25): mutations (edit, write, bash) > exploration (read, glob)
func ScoreOperation(op Operation, age, totalOps int, activeFiles []string) float64 {
	// Recency: exponential decay, half-life = 4
	recency := math.Exp(-0.693 * float64(age) / 4.0)

	// File overlap: Jaccard similarity
	fileOverlap := jaccardSimilarity(op.Files, activeFiles)

	// Outcome significance
	outcomeSig := 0.0
	outcomeLen := len(op.Outcome)
	if outcomeLen > 200 {
		outcomeSig = 1.0
	} else if outcomeLen > 50 {
		outcomeSig = 0.5
	}
	// Error keywords boost
	outcomeLower := strings.ToLower(op.Outcome)
	for _, kw := range []string{"error", "fail", "bug", "fix", "broke"} {
		if strings.Contains(outcomeLower, kw) {
			outcomeSig = math.Max(outcomeSig, 0.8)
			break
		}
	}

	// Operation type: mutations > exploration > none
	opType := 0.0
	if len(op.Tools) > 0 {
		opType = 0.3 // exploration tools (read, glob, etc.)
		mutationTools := map[string]bool{"edit": true, "write": true, "bash": true, "create": true}
		for _, tool := range op.Tools {
			if mutationTools[tool] {
				opType = 0.8
				break
			}
		}
	}

	return 0.25*recency + 0.25*fileOverlap + 0.25*outcomeSig + 0.25*opType
}

// jaccardSimilarity computes |intersection| / |union| for two string slices.
func jaccardSimilarity(a, b []string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	setA := make(map[string]bool, len(a))
	for _, s := range a {
		setA[s] = true
	}
	setB := make(map[string]bool, len(b))
	for _, s := range b {
		setB[s] = true
	}
	intersection := 0
	for s := range setA {
		if setB[s] {
			intersection++
		}
	}
	union := len(setA)
	for s := range setB {
		if !setA[s] {
			union++
		}
	}
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

// RenderWorkingMemoryOneLiner produces a single-line summary for a fully archived operation.
// Format: "- <intent> [<outcome truncated to 80 chars>] (<files>)"
func RenderWorkingMemoryOneLiner(op Operation) string {
	outcome := op.Outcome
	if len(outcome) > 80 {
		outcome = outcome[:80] + "..."
	}
	line := "- " + op.Intent
	if outcome != "" {
		line += " [" + outcome + "]"
	}
	if len(op.Files) > 0 {
		line += " (" + strings.Join(op.Files, ", ") + ")"
	}
	return line
}

// AppendWorkingMemory appends one-liner summaries of archived operations to the system message.
// Returns a copy -- does not mutate the original.
func AppendWorkingMemory(sysMsg t.Message, archivedOps []Operation) t.Message {
	if len(archivedOps) == 0 {
		return sysMsg
	}

	// Build working memory section
	var sb strings.Builder
	sb.WriteString("\n\n## Working Memory\n")
	for _, op := range archivedOps {
		sb.WriteString(RenderWorkingMemoryOneLiner(op))
		sb.WriteByte('\n')
	}

	// Copy the message (don't mutate original)
	result := t.Message{Role: sysMsg.Role}
	result.Content = make([]t.ContentBlock, len(sysMsg.Content))
	copy(result.Content, sysMsg.Content)
	if len(result.Content) > 0 && result.Content[0].Type == "text" {
		result.Content[0] = t.ContentBlock{
			Type: "text",
			Text: result.Content[0].Text + sb.String(),
		}
	}
	return result
}
