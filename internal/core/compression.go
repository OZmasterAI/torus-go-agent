package core

import (
	"fmt"
	"strings"

	t "torus_go_agent/internal/types"
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
func ScoreMessage(m t.Message) MessageScore {
	if len(m.Content) == 0 {
		return ScoreZero
	}

	// Thinking nodes are always expendable — drop first.
	if m.Role == "thinking" {
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
	if m.Role == t.RoleUser && strings.ContainsAny(text, "?") {
		return ScoreHigh
	}

	// Assistant messages with decisions/answers (longer = more likely important)
	if m.Role == t.RoleAssistant && textLen > 200 {
		return ScoreHigh
	}

	// User instructions (longer messages are usually more important)
	if m.Role == t.RoleUser && textLen > 100 {
		return ScoreHigh
	}

	return ScoreMedium
}

// ToolTruncLimit defines head+tail line limits for a specific tool type.
type ToolTruncLimit struct {
	HeadLines int
	TailLines int
}

// DefaultToolLimits maps tool names to their truncation line limits.
var DefaultToolLimits = map[string]ToolTruncLimit{
	"bash":  {HeadLines: 30, TailLines: 15},
	"read":  {HeadLines: 60, TailLines: 20},
	"glob":  {HeadLines: 30, TailLines: 0},
	"grep":  {HeadLines: 30, TailLines: 10},
	"write": {HeadLines: 5, TailLines: 0},
	"edit":  {HeadLines: 20, TailLines: 10},
}

// CompressMessage shortens a message's text content to maxChars using
// head+tail preservation. Tool results use per-tool line limits when available,
// falling back to percentage-based head+tail. Text blocks keep the
// first ~70% and last ~20% of characters with a truncation marker.
//
// toolNames is an optional map from ToolUseID to tool name for per-tool limits.
// Pass nil to use percentage-based truncation for all tool results.
func CompressMessage(m t.Message, maxChars int, toolNames ...map[string]string) t.Message {
	compressed := t.Message{Role: m.Role}
	// Build tool name lookup: first check variadic map, then scan this message's blocks
	var toolNameMap map[string]string
	if len(toolNames) > 0 && toolNames[0] != nil {
		toolNameMap = toolNames[0]
	}
	// Also scan for tool_use blocks in this message (handles combined messages)
	for _, b := range m.Content {
		if b.Type == "tool_use" && b.Name != "" && b.ID != "" {
			if toolNameMap == nil {
				toolNameMap = make(map[string]string)
			}
			toolNameMap[b.ID] = b.Name
		}
	}
	for _, b := range m.Content {
		switch b.Type {
		case "tool_result":
			content := b.Content
			toolName := ""
			if toolNameMap != nil {
				toolName = toolNameMap[b.ToolUseID]
			}
			if len(content) > maxChars {
				if limits, ok := DefaultToolLimits[toolName]; ok {
					content = compressToolResultByLines(content, limits.HeadLines, limits.TailLines, len(b.Content))
				} else {
					content = compressToolResult(content, maxChars, len(b.Content))
				}
			}
			compressed.Content = append(compressed.Content, t.ContentBlock{
				Type:      "tool_result",
				ToolUseID: b.ToolUseID,
				Content:   content,
				IsError:   b.IsError,
			})
		case "text":
			text := b.Text
			if len(text) > maxChars {
				text = compressText(text, maxChars)
			}
			compressed.Content = append(compressed.Content, t.ContentBlock{Type: "text", Text: text})
		default:
			// Keep tool_use and other blocks as-is
			compressed.Content = append(compressed.Content, b)
		}
	}
	return compressed
}

// compressToolResultByLines applies head+tail truncation using explicit line counts.
// Used when per-tool limits are available (e.g., bash: 30 head + 15 tail lines).
func compressToolResultByLines(content string, headCount, tailCount, origLen int) string {
	lines := strings.Split(content, "\n")
	total := len(lines)
	if headCount+tailCount >= total {
		return content // fits within limits
	}

	var head, tail []string
	if headCount > 0 {
		if headCount > total {
			headCount = total
		}
		head = lines[:headCount]
	}
	if tailCount > 0 {
		start := total - tailCount
		if start < headCount {
			start = headCount
		}
		tail = lines[start:]
	}

	skipped := total - len(head) - len(tail)
	if skipped <= 0 {
		return content
	}

	result := strings.Join(head, "\n")
	result += fmt.Sprintf("\n[...%d lines truncated from %d total]\n", skipped, total)
	if len(tail) > 0 {
		result += strings.Join(tail, "\n")
	}
	return result
}

// compressToolResult applies head+tail truncation by lines.
// Keeps ~60% of maxChars from the top lines and ~30% from the bottom lines,
// with a truncation notice showing how many lines were removed.
func compressToolResult(content string, maxChars, origLen int) string {
	lines := strings.Split(content, "\n")
	if len(lines) <= 2 {
		// Too few lines to split by line; fall back to char-based truncation
		if len(content) > maxChars {
			return content[:maxChars] + fmt.Sprintf("\n[...truncated from %d chars]", origLen)
		}
		return content
	}

	headBudget := maxChars * 60 / 100
	tailBudget := maxChars * 30 / 100

	// Collect head lines
	var headLines []string
	headChars := 0
	for _, line := range lines {
		cost := len(line) + 1 // +1 for newline
		if headChars+cost > headBudget && len(headLines) > 0 {
			break
		}
		headLines = append(headLines, line)
		headChars += cost
	}

	// Collect tail lines (backwards from end)
	var tailLines []string
	tailChars := 0
	for i := len(lines) - 1; i >= len(headLines); i-- {
		cost := len(lines[i]) + 1
		if tailChars+cost > tailBudget && len(tailLines) > 0 {
			break
		}
		tailLines = append([]string{lines[i]}, tailLines...)
		tailChars += cost
	}

	skipped := len(lines) - len(headLines) - len(tailLines)
	if skipped <= 0 {
		// Head+tail covers everything; no truncation needed
		return content
	}

	head := strings.Join(headLines, "\n")
	tail := strings.Join(tailLines, "\n")
	marker := fmt.Sprintf("\n[...%d lines truncated from %d chars]\n", skipped, origLen)

	return head + marker + tail
}

// compressText applies head+tail truncation by characters.
// Keeps ~70% of maxChars from the beginning and ~20% from the end,
// with a truncation marker in the middle.
func compressText(text string, maxChars int) string {
	headBudget := maxChars * 70 / 100
	tailBudget := maxChars * 20 / 100

	head := text[:headBudget]
	tail := text[len(text)-tailBudget:]

	return head + "\n[...truncated]\n" + tail
}

// UnifiedCompressConfig configures the unified compression pipeline.
type UnifiedCompressConfig struct {
	KeepLast      int // recent messages kept verbatim (default 10)
	MinMessages   int // minimum message count before compression activates (0 = use KeepLast)
	ContextWindow int // total context window in tokens (default 128000)
	MaxTokens     int // reserved for model output (default 8192)
	ArchivePct    int // percentage of usable budget for system+archive zone (default 25)
}

// UnifiedCompress merges operation-aware compression and zone budgeting into
// a single pipeline. Scoring + budget make retention decisions together.
// keepLast operations get a score floor (min 0.3) instead of a hard wall.
//
// Pipeline:
//  1. Group ALL messages into operations
//  2. Score ALL operations (no keepLast wall — keepLast applies a score floor)
//  3. Compute zone budgets: Zone1 (system+archive), Zone2 (active ops)
//  4. Fill Zone2 by score (highest first), with per-op 50% cap for non-active ops
//  5. Active op always included (truncated, never compacted)
//  6. Remaining ops: score >= 0.3 → template, score < 0.3 → archive to working memory
func UnifiedCompress(messages []t.Message, cfg UnifiedCompressConfig) []t.Message {
	if cfg.KeepLast <= 0 {
		cfg.KeepLast = 10
	}
	if cfg.ContextWindow <= 0 {
		cfg.ContextWindow = 128000
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 8192
	}
	if cfg.ArchivePct <= 0 {
		cfg.ArchivePct = 25
	}

	n := len(messages)
	if n <= cfg.KeepLast {
		return messages
	}
	if cfg.MinMessages > 0 && n < cfg.MinMessages {
		return messages
	}

	// Step 1: Group all messages into operations
	ops := GroupOperations(messages)
	if len(ops) == 0 {
		return messages
	}

	// Step 2: Score ALL operations — no keepLast wall
	activeOp := ops[len(ops)-1]
	activeFiles := activeOp.Files
	totalOps := len(ops)

	type scoredOp struct {
		op    Operation
		score float64
		index int // original position in ops slice
	}
	scored := make([]scoredOp, len(ops))

	for i, op := range ops {
		if i == len(ops)-1 {
			// Active op always gets score 1.0
			scored[i] = scoredOp{op: op, score: 1.0, index: i}
			continue
		}
		age := len(ops) - 1 - i
		var laterSlice []Operation
		if i+1 < len(ops) {
			laterSlice = ops[i+1:]
		}
		s := ScoreOperation(op, age, totalOps, activeFiles, laterSlice)

		// keepLast score floor: operations in the recent window get min 0.3
		if op.StartIdx >= n-cfg.KeepLast {
			if s < 0.3 {
				s = 0.3
			}
		}
		scored[i] = scoredOp{op: op, score: s, index: i}
	}

	// Step 3: Compute zone budgets
	usable := cfg.ContextWindow - cfg.MaxTokens
	if usable <= 0 {
		return messages
	}
	zone1Budget := usable * cfg.ArchivePct / 100
	zone2Budget := usable * 25 / 100

	// Zone 1 actual usage: system prompt
	zone1Tokens := EstimateTokens(messages[0:1])
	zone1Unused := zone1Budget - zone1Tokens
	if zone1Unused < 0 {
		zone1Unused = 0
	}
	zone2Effective := zone2Budget + zone1Unused

	// Build ToolUseID→toolName map for per-tool truncation
	toolNameMap := make(map[string]string)
	for _, op := range ops {
		for _, msg := range op.Messages {
			for _, b := range msg.Content {
				if b.Type == "tool_use" && b.ID != "" && b.Name != "" {
					toolNameMap[b.ID] = b.Name
				}
			}
		}
	}

	// Step 4: Fill Zone 2 by score — active op first, then by score descending
	// Sort non-active ops by score descending
	type opBudgetEntry struct {
		scored scoredOp
		tokens int
	}

	// Estimate tokens for each op and pre-truncate tool output
	activeEntry := opBudgetEntry{scored: scored[len(scored)-1]}
	var activeMessages []t.Message
	for _, msg := range activeOp.Messages {
		compressed := CompressMessage(msg, 2000, toolNameMap)
		activeMessages = append(activeMessages, compressed)
	}
	activeEntry.tokens = EstimateTokens(activeMessages)

	// Collect non-active scored ops with their token costs
	var candidates []opBudgetEntry
	for i := 0; i < len(scored)-1; i++ {
		var opMsgs []t.Message
		for _, msg := range scored[i].op.Messages {
			opMsgs = append(opMsgs, CompressMessage(msg, 2000, toolNameMap))
		}
		tokens := EstimateTokens(opMsgs)
		candidates = append(candidates, opBudgetEntry{scored: scored[i], tokens: tokens})
	}

	// Sort candidates by score descending (stable to preserve order for equal scores)
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].scored.score > candidates[i].scored.score {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	// Per-op cap: no single non-active op can exceed 50% of zone2
	perOpCap := zone2Effective / 2
	if perOpCap <= 0 {
		perOpCap = 1
	}

	// Active op always gets in first
	zone2Used := activeEntry.tokens
	// Track which ops made it into zone 2
	type retained struct {
		index    int // original ops index
		messages []t.Message
	}
	var zone2Ops []retained

	for _, c := range candidates {
		// 50% cap for non-active ops
		if c.tokens > perOpCap {
			continue // will be archived or templated below
		}
		if zone2Used+c.tokens > zone2Effective {
			continue // doesn't fit
		}
		zone2Used += c.tokens
		var opMsgs []t.Message
		for _, msg := range c.scored.op.Messages {
			opMsgs = append(opMsgs, CompressMessage(msg, 2000, toolNameMap))
		}
		zone2Ops = append(zone2Ops, retained{index: c.scored.index, messages: opMsgs})
	}

	// Step 5: Build which ops are retained in zone 2
	retainedSet := make(map[int]bool)
	retainedSet[len(ops)-1] = true // active op
	for _, r := range zone2Ops {
		retainedSet[r.index] = true
	}

	// Step 6: Classify remaining ops (not retained in zone 2)
	var archivedOps []Operation
	var templateMessages []t.Message
	for i := 0; i < len(ops)-1; i++ {
		if retainedSet[i] {
			continue
		}
		if scored[i].score >= 0.3 {
			templateMessages = append(templateMessages, OperationToMessage(ops[i]))
		} else {
			archivedOps = append(archivedOps, ops[i])
		}
	}

	// Step 7: Assemble result in order: system prompt + templates + zone2 ops (by position) + active op
	sysMsg := AppendWorkingMemory(messages[0], archivedOps)
	result := make([]t.Message, 0, 1+len(templateMessages)+len(zone2Ops)*4+len(activeMessages))
	result = append(result, sysMsg)
	result = append(result, templateMessages...)

	// Sort zone2Ops by original index to maintain conversation order
	for i := 0; i < len(zone2Ops); i++ {
		for j := i + 1; j < len(zone2Ops); j++ {
			if zone2Ops[j].index < zone2Ops[i].index {
				zone2Ops[i], zone2Ops[j] = zone2Ops[j], zone2Ops[i]
			}
		}
	}
	for _, r := range zone2Ops {
		result = append(result, r.messages...)
	}

	// Active op always last
	result = append(result, activeMessages...)

	return result
}

// continuousCompress is the V1 implementation — kept as internal fallback.
// Production uses ContinuousCompressV2. Unexported to prevent external use.
func continuousCompress(messages []t.Message, keepLast, minMessages int) []t.Message {
	if keepLast <= 0 {
		keepLast = 10
	}
	n := len(messages)
	if n <= keepLast {
		return messages
	}
	if minMessages > 0 && n < minMessages {
		return messages
	}

	result := make([]t.Message, n)

	// Recent messages: keep verbatim
	verbatimStart := n - keepLast
	copy(result[verbatimStart:], messages[verbatimStart:])

	// Older messages: compress based on age + score
	// Skip index 0 (schema message) — never compress the first message
	result[0] = messages[0]
	for i := 1; i < verbatimStart; i++ {
		score := ScoreMessage(messages[i])
		age := verbatimStart - i // how far back from the verbatim boundary

		// Determine compression level based on age and score
		var maxChars int
		switch {
		case score == ScoreZero:
			// Drop entirely — replace with empty
			result[i] = t.Message{Role: messages[i].Role}
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
// zoneBudget is the V1 budget struct — kept as internal fallback.
type zoneBudget struct {
	ContextWindow  int
	ArchivePercent int
	OutputReserve  int
}

// applyZoneBudget is the V1 implementation — kept as internal fallback.
// Production uses ApplyZoneBudgetV2. Unexported to prevent external use.
func applyZoneBudget(messages []t.Message, budget zoneBudget) []t.Message {
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
	currentTokens := EstimateTokens(messages[n-1:])
	historyBudget -= currentTokens
	if historyBudget < 0 {
		historyBudget = 0
	}

	// Split: find the boundary between archive and history
	// History = messages that fit within historyBudget, working backwards from second-to-last
	historyStart := n - 1 // start with just the current message
	historyTokens := 0
	for i := n - 2; i >= 0; i-- {
		msgTokens := EstimateTokens(messages[i : i+1])
		if historyTokens+msgTokens > historyBudget {
			break
		}
		historyTokens += msgTokens
		historyStart = i
	}

	// Archive = messages before historyStart, trimmed to archiveBudget
	// Always include index 0 (schema message) regardless of budget
	var archive []t.Message
	archiveTokens := 0
	if historyStart > 0 {
		archive = append(archive, messages[0])
		archiveTokens += EstimateTokens(messages[0:1])
	}
	for i := 1; i < historyStart; i++ {
		msgTokens := EstimateTokens(messages[i : i+1])
		if archiveTokens+msgTokens > archiveBudget {
			break
		}
		archiveTokens += msgTokens
		archive = append(archive, messages[i])
	}

	// Assemble: archive + history (includes current message)
	result := make([]t.Message, 0, len(archive)+n-historyStart)
	result = append(result, archive...)
	result = append(result, messages[historyStart:]...)
	return result
}

// ZoneBudgetV2 configures 3-zone token allocation with dynamic rebalancing.
type ZoneBudgetV2 struct {
	ContextWindow    int // total token budget
	SystemArchivePct int // percentage for system prompt + archived one-liners (default 25)
	ActiveOpsPct     int // percentage for active/recent operations (default 25)
	HeadroomPct      int // percentage reserved for model output + next tools (default 50)
	OutputReserve    int // minimum tokens reserved for output (default 4096)
}

// Deprecated: use UnifiedCompress instead. Kept for backward compatibility.
//
// ApplyZoneBudgetV2 distributes messages across 3 zones with dynamic rebalancing.
// Zone 1: system prompt + archived content (SystemArchivePct)
// Zone 2: active/recent operations (ActiveOpsPct)
// Zone 3: headroom for output (HeadroomPct, reserved -- not filled)
// Unused budget from Zone 1 flows to Zone 2.
// Per-operation cap: no single operation can use more than 50% of Zone 2.
// activeFiles is passed for scoring context (can be nil).
func ApplyZoneBudgetV2(messages []t.Message, budget ZoneBudgetV2, activeFiles []string) []t.Message {
	if len(messages) == 0 {
		return nil
	}

	// Apply defaults
	if budget.SystemArchivePct <= 0 {
		budget.SystemArchivePct = 25
	}
	if budget.ActiveOpsPct <= 0 {
		budget.ActiveOpsPct = 25
	}
	if budget.HeadroomPct <= 0 {
		budget.HeadroomPct = 50
	}
	if budget.OutputReserve <= 0 {
		budget.OutputReserve = 4096
	}
	if budget.ContextWindow <= 0 {
		return messages
	}

	// Calculate usable budget (total minus output reserve)
	usable := budget.ContextWindow - budget.OutputReserve
	if usable <= 0 {
		// Not enough room -- return at least messages[0] + last message
		if len(messages) <= 2 {
			return messages
		}
		return []t.Message{messages[0], messages[len(messages)-1]}
	}

	// Zone budgets
	zone1Budget := usable * budget.SystemArchivePct / 100
	zone2Budget := usable * budget.ActiveOpsPct / 100

	// --- Zone 1: system prompt (messages[0]) ---
	zone1Tokens := EstimateTokens(messages[0:1])
	zone1Unused := zone1Budget - zone1Tokens
	if zone1Unused < 0 {
		zone1Unused = 0
	}

	// --- Dynamic rebalancing: unused Zone 1 flows to Zone 2 ---
	zone2Effective := zone2Budget + zone1Unused

	// Per-operation cap: no single op can exceed 50% of Zone 2
	perOpCap := zone2Effective / 2
	if perOpCap <= 0 {
		perOpCap = 1
	}

	// --- Zone 2: fill from the end (most recent first) ---
	// Track operations by user-message boundaries to enforce per-op cap.
	// Walk backwards from the last message, grouping by user-message starts.
	n := len(messages)
	zone2Tokens := 0
	zone2Start := n // index where Zone 2 begins (exclusive of messages[0])

	// Track per-operation token usage while scanning backwards
	currentOpTokens := 0
	inOperation := false

	for i := n - 1; i >= 1; i-- {
		msg := messages[i]
		msgTokens := EstimateTokens(messages[i : i+1])

		// Detect operation boundary: a user message with text starts a new op
		isOpStart := msg.Role == t.RoleUser && hasTextContent(msg)

		if isOpStart && inOperation {
			// We just finished scanning one operation (backwards).
			// The previous operation's tokens are in currentOpTokens.
			// Check per-op cap for the operation we just finished.
			if currentOpTokens > perOpCap {
				// This op exceeds cap -- skip it entirely by not
				// moving zone2Start back past it. But we still need
				// to try the current message's operation.
				// Undo: restore zone2Tokens and zone2Start to before this op.
				zone2Tokens -= currentOpTokens
				zone2Start = i + 1 // exclude the oversized op
				// But continue scanning -- earlier ops may fit.
			}
			currentOpTokens = 0
		}

		// Check if adding this message would exceed Zone 2 effective budget
		if zone2Tokens+msgTokens > zone2Effective {
			break
		}

		zone2Tokens += msgTokens
		currentOpTokens += msgTokens
		zone2Start = i
		inOperation = true
	}

	// Handle the last operation scanned (the earliest one we reached)
	if inOperation && currentOpTokens > perOpCap && zone2Start > 1 {
		// The earliest operation we included exceeds per-op cap.
		// Find where it ends and exclude it.
		zone2Tokens -= currentOpTokens
		// Scan forward to find the next operation start
		for j := zone2Start + 1; j < n; j++ {
			msg := messages[j]
			if msg.Role == t.RoleUser && hasTextContent(msg) {
				zone2Start = j
				break
			}
		}
		// Recalculate zone2Tokens from zone2Start
		zone2Tokens = 0
		for j := zone2Start; j < n; j++ {
			zone2Tokens += EstimateTokens(messages[j : j+1])
		}
	}

	// --- Assemble result: Zone 1 (messages[0]) + Zone 2 (zone2Start..n) ---
	result := make([]t.Message, 0, 1+(n-zone2Start))
	result = append(result, messages[0])

	if zone2Start < n && zone2Start > 0 {
		result = append(result, messages[zone2Start:]...)
	} else if zone2Start == 0 {
		// Everything fits -- but messages[0] is already added
		result = append(result, messages[1:]...)
	}

	return result
}

// hasTextContent returns true if the message has at least one non-empty text content block.
func hasTextContent(m t.Message) bool {
	for _, b := range m.Content {
		if b.Type == "text" && b.Text != "" {
			return true
		}
	}
	return false
}

// Deprecated: use UnifiedCompress instead. Kept for backward compatibility.
//
// ContinuousCompressV2 applies operation-aware compression with grouping,
// template summaries, scoring, and working memory. Returns a new slice.
//
// Pipeline:
//  1. GroupOperations -- segment messages into semantic operations
//  2. ScoreOperation -- score each non-active op (exponential decay + file overlap)
//  3. Classify: score >= 0.3 -> replace with template summary in message array
//     score < 0.3 -> archive to working memory one-liner in messages[0]
//  4. Assemble result
//
// keepLast controls how many recent messages are always kept verbatim (operations
// that fall entirely within this window are not scored). minMessages sets the
// minimum message count before compression activates (0 = use keepLast).
func ContinuousCompressV2(messages []t.Message, keepLast, minMessages int) []t.Message {
	if keepLast <= 0 {
		keepLast = 10
	}
	n := len(messages)
	if n <= keepLast {
		return messages
	}
	if minMessages > 0 && n < minMessages {
		return messages
	}

	// Step 1: Group messages into operations (messages[0] is system prompt, excluded)
	ops := GroupOperations(messages)
	if len(ops) == 0 {
		return messages
	}

	// Step 2: Identify the verbatim boundary and active files.
	// Operations whose StartIdx >= verbatimStart are kept verbatim.
	verbatimStart := n - keepLast
	if verbatimStart < 1 {
		verbatimStart = 1 // never compress the system prompt
	}

	// Find the active operation (last one) and collect its files for scoring.
	activeOp := ops[len(ops)-1]
	activeFiles := activeOp.Files

	// Separate operations into verbatim (recent) and compressible (old).
	var verbatimOps []Operation
	var compressibleOps []Operation
	for i := range ops {
		if ops[i].StartIdx >= verbatimStart {
			verbatimOps = append(verbatimOps, ops[i])
		} else {
			compressibleOps = append(compressibleOps, ops[i])
		}
	}

	// If nothing to compress, return original.
	if len(compressibleOps) == 0 {
		return messages
	}

	// Step 3: Score and classify compressible operations.
	type classifiedOp struct {
		op    Operation
		tier  string // "keep", "template", "archive"
		score float64
	}
	totalOps := len(ops)
	var classified []classifiedOp

	for i, cop := range compressibleOps {
		// Age = distance from end of compressible ops (oldest = highest age)
		age := len(compressibleOps) - i
		// Pass later ops (everything after this one) for causal dependency scoring
		var laterSlice []Operation
		if i+1 < len(compressibleOps) {
			laterSlice = append(laterSlice, compressibleOps[i+1:]...)
		}
		laterSlice = append(laterSlice, verbatimOps...)
		score := ScoreOperation(cop, age, totalOps, activeFiles, laterSlice)

		tier := "archive"
		if score >= 0.3 {
			tier = "template"
		}
		classified = append(classified, classifiedOp{op: cop, tier: tier, score: score})
	}

	// Step 4: Build result
	var archivedOps []Operation
	var middleMessages []t.Message

	for _, c := range classified {
		switch c.tier {
		case "template":
			// Replace entire operation with a single template message
			middleMessages = append(middleMessages, OperationToMessage(c.op))
		case "archive":
			// Collect for working memory in system prompt
			archivedOps = append(archivedOps, c.op)
		}
	}

	// Verbatim operations: keep all their messages as-is (with head+tail truncation on tool output)
	// Build ToolUseID→toolName map from all operations for per-tool truncation
	toolNameMap := make(map[string]string)
	for _, op := range ops {
		for _, msg := range op.Messages {
			for _, b := range msg.Content {
				if b.Type == "tool_use" && b.ID != "" && b.Name != "" {
					toolNameMap[b.ID] = b.Name
				}
			}
		}
	}
	var verbatimMessages []t.Message
	for _, vop := range verbatimOps {
		for _, msg := range vop.Messages {
			verbatimMessages = append(verbatimMessages, CompressMessage(msg, 2000, toolNameMap))
		}
	}

	// Step 5: Assemble
	// System prompt (with working memory appended) + template messages + verbatim messages
	sysMsg := AppendWorkingMemory(messages[0], archivedOps)
	result := make([]t.Message, 0, 1+len(middleMessages)+len(verbatimMessages))
	result = append(result, sysMsg)
	result = append(result, middleMessages...)
	result = append(result, verbatimMessages...)

	return result
}
