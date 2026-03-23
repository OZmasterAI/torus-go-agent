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

// Boundary detection weights for determining when a new operation starts.
const (
	weightToolTransition = 0.35 // shift in tool type (read→write or vice versa)
	weightFileScopeChange = 0.30 // Jaccard < 0.2 with previous op's files
	weightIntentSignal    = 0.20 // assistant text contains transition phrases
	boundaryThreshold     = 0.20 // weighted sum must reach this to trigger boundary
)

// intentTransitionPhrases are assistant text patterns that suggest a new task.
var intentTransitionPhrases = []string{
	"now let's", "moving on", "next,", "next step", "switching to",
	"let's move", "on to the", "different task", "new task",
	"let me now", "i'll now", "turning to",
}

// GroupOperations segments a message slice into semantic operations.
// messages[0] is treated as the system prompt and excluded from operations.
// Uses 3-signal boundary detection: tool type transition, file scope change,
// and intent signals. Falls back to user-text boundary when no prior operation exists.
func GroupOperations(messages []t.Message) []Operation {
	if len(messages) <= 1 {
		return nil
	}

	var ops []Operation
	var current *Operation

	for i := 1; i < len(messages); i++ {
		msg := messages[i]

		// Only user messages with text can start new operations
		if msg.Role != t.RoleUser || !hasTextBlock(msg) {
			continue
		}

		// First user-text message always starts the first operation
		if current == nil {
			current = &Operation{
				StartIdx: i,
				Intent:   extractText(msg),
			}
			continue
		}

		// Finalize current op temporarily to check boundary signals
		current.EndIdx = i
		current.Messages = messages[current.StartIdx:current.EndIdx]
		finalizeOp(current, messages)

		// Check boundary signals against the current (previous) operation
		isBoundary := detectBoundary(*current, messages, i)

		if isBoundary {
			// Commit previous operation and start new one
			ops = append(ops, *current)
			current = &Operation{
				StartIdx: i,
				Intent:   extractText(msg),
			}
		}
		// If not a boundary, the user message extends the current operation
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

// detectBoundary checks 3 weighted signals to decide if a user message starts a new operation.
func detectBoundary(prevOp Operation, messages []t.Message, userMsgIdx int) bool {
	score := 0.0

	// Signal 2: File scope change (0.30) — evaluate first, used to gate signal 1
	newFiles := peekFilePaths(messages, userMsgIdx)
	filesDiverged := false
	if len(newFiles) > 0 && len(prevOp.Files) > 0 {
		similarity := jaccardSimilarity(prevOp.Files, newFiles)
		if similarity < 0.2 {
			score += weightFileScopeChange
			filesDiverged = true
		}
	} else if len(newFiles) > 0 && len(prevOp.Files) == 0 {
		// Previous op had no files, new one does — likely a new task
		filesDiverged = true
	}

	// Signal 1: Tool type transition (0.35)
	// Only counts if files also diverged — read→edit on the same file is continuation, not transition
	newTools := peekToolTypes(messages, userMsgIdx)
	if filesDiverged && len(newTools) > 0 && len(prevOp.Tools) > 0 {
		prevMutates := hasMutationTool(prevOp.Tools)
		newMutates := hasMutationTool(newTools)
		if prevMutates != newMutates {
			score += weightToolTransition
		}
	}

	// Signal 3: Intent signal (0.20)
	// Check the last assistant message before this user message for transition phrases
	if userMsgIdx > 0 {
		prevMsg := messages[userMsgIdx-1]
		if prevMsg.Role == t.RoleAssistant {
			assistantText := strings.ToLower(extractText(prevMsg))
			for _, phrase := range intentTransitionPhrases {
				if strings.Contains(assistantText, phrase) {
					score += weightIntentSignal
					break
				}
			}
		}
	}

	// If we could evaluate signals (had tools/files to compare), use threshold
	if score > 0 {
		return score >= boundaryThreshold
	}

	// No signals fired — we couldn't compare tools or files.
	// Default to boundary (every user-text message starts a new op)
	// unless the previous op had no tools either (pure text conversation, keep together)
	return len(prevOp.Tools) == 0 || len(peekToolTypes(messages, userMsgIdx)) == 0
}

// peekToolTypes looks ahead from a user message to find tool names used in the next assistant message.
func peekToolTypes(messages []t.Message, fromIdx int) []string {
	var tools []string
	seen := make(map[string]bool)
	for i := fromIdx + 1; i < len(messages) && i <= fromIdx+3; i++ {
		for _, b := range messages[i].Content {
			if b.Type == "tool_use" && b.Name != "" && !seen[b.Name] {
				seen[b.Name] = true
				tools = append(tools, b.Name)
			}
		}
	}
	return tools
}

// peekFilePaths looks ahead from a user message to find file paths in upcoming tool_use blocks.
func peekFilePaths(messages []t.Message, fromIdx int) []string {
	var files []string
	seen := make(map[string]bool)
	for i := fromIdx + 1; i < len(messages) && i <= fromIdx+3; i++ {
		for _, b := range messages[i].Content {
			if b.Type == "tool_use" && b.Input != nil {
				if fp, ok := b.Input["file_path"]; ok {
					if path, ok := fp.(string); ok && path != "" && !seen[path] {
						seen[path] = true
						files = append(files, path)
					}
				}
			}
		}
	}
	return files
}

// hasMutationTool returns true if any tool in the list is a mutation tool.
func hasMutationTool(tools []string) bool {
	for _, tool := range tools {
		switch tool {
		case "edit", "write", "bash", "create":
			return true
		}
	}
	return false
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
//   - Recency (0.20): exponential decay with half-life of 4 operations
//   - File overlap (0.20): Jaccard similarity with active operation's files
//   - Outcome significance (0.20): longer outcomes and error keywords score higher
//   - Operation type (0.20): mutations (edit, write, bash) > exploration (read, glob)
//   - Causal dependency (0.20): later operations reference this op's files or keywords
func ScoreOperation(op Operation, age, totalOps int, activeFiles []string, laterOps ...[]Operation) float64 {
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

	// Causal dependency: later operations reference this op's files or outcome keywords
	causalDep := 0.0
	if len(laterOps) > 0 && laterOps[0] != nil {
		causalDep = causalDependencyScore(op, laterOps[0])
	}

	return 0.20*recency + 0.20*fileOverlap + 0.20*outcomeSig + 0.20*opType + 0.20*causalDep
}

// causalDependencyScore checks if later operations reference this operation's outputs.
// Returns 0.0-1.0 based on file overlap with later ops and keyword references.
func causalDependencyScore(op Operation, laterOps []Operation) float64 {
	if len(op.Files) == 0 && op.Outcome == "" {
		return 0
	}

	score := 0.0

	for _, later := range laterOps {
		// Check if later op works on the same files (causal chain)
		if len(op.Files) > 0 && len(later.Files) > 0 {
			overlap := jaccardSimilarity(op.Files, later.Files)
			if overlap > 0.3 {
				score = math.Max(score, 0.8)
			} else if overlap > 0 {
				score = math.Max(score, 0.4)
			}
		}

		// Check if later op's intent references keywords from this op
		laterIntent := strings.ToLower(later.Intent)
		opOutcome := strings.ToLower(op.Outcome)
		for _, kw := range []string{"fix", "found", "earlier", "above", "previous", "from before"} {
			if strings.Contains(laterIntent, kw) && opOutcome != "" {
				score = math.Max(score, 0.6)
				break
			}
		}
	}

	return score
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
