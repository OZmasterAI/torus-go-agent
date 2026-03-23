package core

import (
	"encoding/json"
	"fmt"
	"strings"

	t "torus_go_agent/internal/types"
)

// CompactionMode controls how the context window is managed when it fills up.
type CompactionMode string

const (
	// CompactionOff disables all compaction — the loop will error or truncate at provider limits.
	CompactionOff CompactionMode = "off"

	// CompactionSliding drops middle messages, keeping the first and last N.
	CompactionSliding CompactionMode = "sliding"

	// CompactionLLM summarises dropped messages via an LLM call before compacting.
	CompactionLLM CompactionMode = "llm"
)

// CompactionConfig configures the compaction pipeline.
type CompactionConfig struct {
	// Mode selects which compaction strategy to use.
	Mode CompactionMode

	// Threshold is the percent of the ContextWindow at which compaction triggers (0–100).
	// Default: 80
	Threshold int

	// MaxMessages triggers compaction when message count exceeds this value.
	// 0 = disabled (token-based only). Use with Trigger = "messages" or "both".
	MaxMessages int

	// Trigger controls what triggers compaction: "tokens" (default), "messages", or "both".
	Trigger string

	// KeepLastN is the number of recent messages to preserve verbatim after compaction.
	// Default: 10
	KeepLastN int

	// ContextWindow is the token ceiling for the model in use (e.g. 200000 for claude-3-5-sonnet).
	ContextWindow int
}

// defaultCompactionConfig fills in zero-value fields with sensible defaults.
func defaultCompactionConfig(cfg CompactionConfig) CompactionConfig {
	if cfg.Threshold == 0 {
		cfg.Threshold = 80
	}
	if cfg.KeepLastN == 0 {
		cfg.KeepLastN = 10
	}
	if cfg.ContextWindow == 0 {
		cfg.ContextWindow = 200_000
	}
	return cfg
}

// estimateTokens is an alias for EstimateTokens (tokenizer.go).
// Kept as a package-private shorthand used by context.go and compression.go.
var estimateTokens = EstimateTokens

// NeedsCompaction returns true when compaction should trigger based on the
// configured trigger mode: "tokens" (default), "messages", or "both".
// If actualInputTokens > 0, it uses real API token counts instead of estimating.
func NeedsCompaction(messages []t.Message, cfg CompactionConfig, actualInputTokens ...int) bool {
	cfg = defaultCompactionConfig(cfg)
	if cfg.Mode == CompactionOff {
		return false
	}

	trigger := cfg.Trigger
	if trigger == "" {
		trigger = "tokens"
	}

	tokenHit := false
	msgHit := false

	tokens := 0
	if len(actualInputTokens) > 0 && actualInputTokens[0] > 0 {
		tokens = actualInputTokens[0]
	} else {
		tokens = estimateTokens(messages)
	}
	limit := cfg.ContextWindow * cfg.Threshold / 100
	tokenHit = tokens >= limit

	if cfg.MaxMessages > 0 {
		msgHit = len(messages) >= cfg.MaxMessages
	}

	switch trigger {
	case "messages":
		return msgHit
	case "both":
		return tokenHit || msgHit
	default: // "tokens"
		return tokenHit
	}
}

// CompactSliding keeps the first message (system / initial user turn) and the
// last keepLastN messages. Everything in between is dropped.
//
// If len(messages) <= keepLastN+1 the slice is returned unchanged.
func CompactSliding(messages []t.Message, keepLastN int) []t.Message {
	if keepLastN <= 0 {
		keepLastN = 10
	}
	if len(messages) <= keepLastN+1 {
		return messages
	}
	result := make([]t.Message, 0, keepLastN+1)
	result = append(result, messages[0]) // always keep the first
	tail := messages[len(messages)-keepLastN:]
	result = append(result, tail...)
	return result
}

// CompactLLM summarises the dropped messages via an LLM call, then returns:
//
//	[ messages[0], <summary assistant message>, ...last-N messages ]
//
// The summarize callback receives the key content extracted from the middle
// messages and must return a human-readable summary string.
//
// Falls back to extractKeyContent (no LLM call) when:
//   - summarize is nil
//   - the summarize call returns an error
func CompactLLM(messages []t.Message, keepLastN int, summarize func(string) (string, error)) ([]t.Message, error) {
	if keepLastN <= 0 {
		keepLastN = 10
	}
	if len(messages) <= keepLastN+1 {
		return messages, nil
	}

	// The "middle" is everything after the first message and before the last N.
	middle := messages[1 : len(messages)-keepLastN]
	keyContent := extractKeyContent(middle)

	var summary string
	if summarize != nil {
		var err error
		summary, err = summarize(keyContent)
		if err != nil {
			// Graceful fallback — use raw key content as the summary body.
			summary = keyContent
		}
	} else {
		summary = keyContent
	}

	summaryMsg := t.Message{
		Role: t.RoleAssistant,
		Content: []t.ContentBlock{{
			Type: "text",
			Text: fmt.Sprintf("[Context Summary]\n\n%s", summary),
		}},
	}

	tail := messages[len(messages)-keepLastN:]
	result := make([]t.Message, 0, 2+len(tail))
	result = append(result, messages[0])
	result = append(result, summaryMsg)
	result = append(result, tail...)
	return result, nil
}

// extractKeyContent walks messages and builds a concise textual representation
// of the conversation, prefixed with speaker labels. The result is truncated
// to 2000 characters so it is always safe to embed in a downstream prompt.
func extractKeyContent(messages []t.Message) string {
	var sb strings.Builder
	for _, m := range messages {
		// Skip thinking nodes — they are ephemeral reasoning, not conversation content.
		if m.Role == "thinking" {
			continue
		}
		switch m.Role {
		case t.RoleUser:
			sb.WriteString("[User]\n")
		case t.RoleAssistant:
			sb.WriteString("[Assistant]\n")
		case t.RoleSystem:
			sb.WriteString("[System]\n")
		case t.RoleTool:
			// Tool messages may contain multiple blocks; label each.
		}
		for _, b := range m.Content {
			switch b.Type {
			case "text":
				if b.Text != "" {
					sb.WriteString(b.Text)
					sb.WriteByte('\n')
				}
			case "tool_use":
				sb.WriteString(fmt.Sprintf("[ToolCall: %s]\n", b.Name))
				if len(b.Input) > 0 {
					if raw, err := json.Marshal(b.Input); err == nil {
						sb.Write(raw)
						sb.WriteByte('\n')
					}
				}
			case "tool_result":
				sb.WriteString(fmt.Sprintf("[ToolResult: %s]\n", b.ToolUseID))
				if b.Content != "" {
					sb.WriteString(b.Content)
					sb.WriteByte('\n')
				}
			}
		}
	}

	out := sb.String()
	const maxChars = 2000
	if len(out) > maxChars {
		out = out[:maxChars]
	}
	return out
}

// CompactDAG is the DAG-native compaction strategy.
//
// Unlike linear compaction, it never destroys history. Instead it:
//  1. Reads all ancestors from the current HEAD.
//  2. Checks NeedsCompaction against that message list.
//  3. If compaction is needed, extracts key content from the middle messages.
//  4. Calls summarize to produce a summary string (falls back to key content).
//  5. Creates a new branch forked from the root node (first ancestor).
//  6. Adds a single assistant "context summary" node on the new branch.
//  7. Re-adds the last KeepLastN messages as new nodes on the new branch.
//  8. Switches the DAG to the new branch (original branch is fully preserved).
//
// The dag is mutated in place (branch switch). Returns nil when no compaction
// was needed.
func CompactDAG(dag *DAG, cfg CompactionConfig, summarize func(string) (string, error)) error {
	cfg = defaultCompactionConfig(cfg)
	if cfg.Mode == CompactionOff {
		return nil
	}

	// --- 1. Get current path ---
	head, err := dag.GetHead()
	if err != nil {
		return fmt.Errorf("compactDAG get head: %w", err)
	}
	if head == "" {
		return nil // empty DAG, nothing to compact
	}

	ancestors, err := dag.GetAncestors(head)
	if err != nil {
		return fmt.Errorf("compactDAG get ancestors: %w", err)
	}

	// Convert nodes → messages for token estimation.
	messages := nodesToMessages(ancestors)

	// --- 2. Check threshold ---
	if !NeedsCompaction(messages, cfg) {
		return nil
	}

	keepN := cfg.KeepLastN
	if len(messages) <= keepN+1 {
		// Not enough messages to meaningfully compact.
		return nil
	}

	// --- 3. Extract key content from the middle ---
	middle := messages[1 : len(messages)-keepN]
	keyContent := extractKeyContent(middle)

	// --- 3b. Group dropped operations and generate working memory one-liners ---
	droppedOps := GroupOperations(append([]t.Message{messages[0]}, middle...))
	var workingMemory string
	if len(droppedOps) > 0 {
		var wmLines []string
		for _, op := range droppedOps {
			wmLines = append(wmLines, RenderWorkingMemoryOneLiner(op))
		}
		workingMemory = strings.Join(wmLines, "\n")
	}

	// --- 3c. Preserve existing working memory from messages[0] (set by compression) ---
	if sysText := extractText(messages[0]); strings.Contains(sysText, "## Working Memory") {
		idx := strings.Index(sysText, "## Working Memory")
		existingWM := strings.TrimSpace(sysText[idx+len("## Working Memory"):])
		if existingWM != "" {
			if workingMemory != "" {
				workingMemory = existingWM + "\n" + workingMemory
			} else {
				workingMemory = existingWM
			}
		}
	}

	// --- 4. Summarise ---
	var summary string
	if summarize != nil {
		var serr error
		summary, serr = summarize(keyContent)
		if serr != nil {
			summary = keyContent // fallback
		}
	} else {
		summary = keyContent
	}

	// --- 5. New branch from root node ---
	rootNode := ancestors[0]
	newBranchName := fmt.Sprintf("compact-%s", genID())
	newBranchID, err := dag.Branch(rootNode.ID, newBranchName)
	if err != nil {
		return fmt.Errorf("compactDAG create branch: %w", err)
	}

	// --- 6. Add summary node (with working memory one-liners) ---
	summaryText := fmt.Sprintf("[Context Summary]\n\n%s", summary)
	if workingMemory != "" {
		summaryText += "\n\n## Working Memory\n" + workingMemory
	}
	summaryContent := []t.ContentBlock{{
		Type: "text",
		Text: summaryText,
	}}
	summaryNodeID, err := dag.AddNode(rootNode.ID, t.RoleAssistant, summaryContent, "", "", estimateTokens([]t.Message{{Role: t.RoleAssistant, Content: summaryContent}}))
	if err != nil {
		return fmt.Errorf("compactDAG add summary node: %w", err)
	}

	// --- 7. Re-add last N messages onto the new branch ---
	tail := messages[len(messages)-keepN:]
	parentID := summaryNodeID
	for _, msg := range tail {
		nodeID, err := dag.AddNode(parentID, msg.Role, msg.Content, "", "", estimateTokens([]t.Message{msg}))
		if err != nil {
			return fmt.Errorf("compactDAG re-add tail node: %w", err)
		}
		parentID = nodeID
	}

	// --- 8. Switch to the new branch (original preserved) ---
	// newBranchID already set as current by dag.Branch(), but let's be explicit.
	if err := dag.SwitchBranch(newBranchID); err != nil {
		return fmt.Errorf("compactDAG switch branch: %w", err)
	}

	return nil
}

// nodesToMessages converts a slice of DAG nodes to the Message type used by
// providers and compaction helpers.
func nodesToMessages(nodes []Node) []t.Message {
	msgs := make([]t.Message, len(nodes))
	for i, n := range nodes {
		msgs[i] = t.Message{Role: t.Role(n.Role), Content: n.Content}
	}
	return msgs
}

// sanitizeMessages cleans up a message slice before sending to a provider:
//   - Filters out empty text blocks (text: "")
//   - Deduplicates identical text blocks within the same message
//   - Merges consecutive messages with the same role
func sanitizeMessages(messages []t.Message) []t.Message {
	// Pass 1: clean blocks within each message
	for i := range messages {
		var cleaned []t.ContentBlock
		seen := map[string]bool{}
		for _, b := range messages[i].Content {
			// Skip empty text blocks
			if b.Type == "text" && b.Text == "" {
				continue
			}
			// Dedup identical text blocks
			if b.Type == "text" {
				if seen[b.Text] {
					continue
				}
				seen[b.Text] = true
			}
			cleaned = append(cleaned, b)
		}
		messages[i].Content = cleaned
	}

	// Pass 2: merge consecutive same-role messages (skip tool messages — they must stay separate)
	var merged []t.Message
	for _, m := range messages {
		if len(m.Content) == 0 {
			continue
		}
		if len(merged) > 0 && merged[len(merged)-1].Role == m.Role && m.Role != t.RoleTool {
			merged[len(merged)-1].Content = append(merged[len(merged)-1].Content, m.Content...)
		} else {
			merged = append(merged, m)
		}
	}
	return merged
}
