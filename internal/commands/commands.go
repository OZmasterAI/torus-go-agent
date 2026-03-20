// Package commands provides channel-agnostic slash commands for DAG operations.
// Each function takes a *core.DAG and arguments, performs the operation,
// and returns a result. Channels handle display.
package commands

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"go_sdk_agent/internal/core"
)

// BranchSummary is a display-friendly branch listing.
type BranchSummary struct {
	ID         string
	Name       string
	Messages   int
	IsCurrent  bool
	HeadNodeID string
	ForkedFrom string
}

// MessageSummary is a display-friendly message listing for browsing.
type MessageSummary struct {
	NodeID    string
	Role      string
	Preview   string // first ~80 chars of text content
	Index     int    // position in the chain (0 = root)
	Timestamp int64
}

// ListBranches returns all branches with message counts and current marker.
func ListBranches(dag *core.DAG) ([]BranchSummary, error) {
	branches, err := dag.ListBranches()
	if err != nil {
		return nil, err
	}
	currentID := dag.CurrentBranchID()
	var result []BranchSummary
	for _, b := range branches {
		msgCount := 0
		if b.HeadNodeID != "" {
			anc, _ := dag.GetAncestors(b.HeadNodeID)
			msgCount = len(anc)
		}
		result = append(result, BranchSummary{
			ID:         b.ID,
			Name:       b.Name,
			Messages:   msgCount,
			IsCurrent:  b.ID == currentID,
			HeadNodeID: b.HeadNodeID,
			ForkedFrom: b.ForkedFrom,
		})
	}
	return result, nil
}

// Switch changes the active branch. Returns the branch name.
func Switch(dag *core.DAG, branchID string) (string, error) {
	if err := dag.SwitchBranch(branchID); err != nil {
		return "", err
	}
	branches, err := dag.ListBranches()
	if err != nil {
		return branchID, nil
	}
	for _, b := range branches {
		if b.ID == branchID {
			return b.Name, nil
		}
	}
	return branchID, nil
}

// SwitchByIndex switches to the Nth branch in the list (1-based).
func SwitchByIndex(dag *core.DAG, index int) (string, error) {
	branches, err := ListBranches(dag)
	if err != nil {
		return "", err
	}
	if index < 1 || index > len(branches) {
		return "", fmt.Errorf("invalid branch number %d (have %d branches)", index, len(branches))
	}
	return Switch(dag, branches[index-1].ID)
}

// ListMessages returns all messages on a branch in chain order (root first).
func ListMessages(dag *core.DAG, branchID string) ([]MessageSummary, error) {
	currentID := dag.CurrentBranchID()
	if branchID != currentID {
		if err := dag.SwitchBranch(branchID); err != nil {
			return nil, err
		}
		defer dag.SwitchBranch(currentID)
	}

	head, err := dag.GetHead()
	if err != nil || head == "" {
		return nil, fmt.Errorf("branch has no messages")
	}

	ancestors, err := dag.GetAncestors(head)
	if err != nil {
		return nil, err
	}

	var result []MessageSummary
	for i, node := range ancestors {
		preview := extractPreview(node.Content, 80)
		result = append(result, MessageSummary{
			NodeID:    node.ID,
			Role:      node.Role,
			Preview:   preview,
			Index:     i,
			Timestamp: node.Timestamp,
		})
	}
	return result, nil
}

// Fork creates a new branch from a specific node.
func Fork(dag *core.DAG, fromNodeID, name string) (string, error) {
	if name == "" {
		name = fmt.Sprintf("fork-%d", time.Now().Unix())
	}
	return dag.Branch(fromNodeID, name)
}

// ForkFromHead forks from the current head.
func ForkFromHead(dag *core.DAG, name string) (string, error) {
	head, err := dag.GetHead()
	if err != nil || head == "" {
		return "", fmt.Errorf("no head on current branch")
	}
	return Fork(dag, head, name)
}

// ForkBack creates a new branch from N messages before the current head.
func ForkBack(dag *core.DAG, stepsBack int, name string) (string, error) {
	head, err := dag.GetHead()
	if err != nil || head == "" {
		return "", fmt.Errorf("no head on current branch")
	}
	ancestors, err := dag.GetAncestors(head)
	if err != nil {
		return "", err
	}
	target := len(ancestors) - 1 - stepsBack
	if target < 0 {
		target = 0
	}
	return Fork(dag, ancestors[target].ID, name)
}

// ParseForkArgs parses /fork arguments.
// Returns: action ("head", "back", "node", "branch"), value string.
func ParseForkArgs(args string) (action, value string) {
	args = strings.TrimSpace(args)
	if args == "" {
		return "head", ""
	}
	if strings.HasPrefix(args, "-back ") || strings.HasPrefix(args, "-b ") {
		parts := strings.Fields(args)
		if len(parts) >= 2 {
			return "back", parts[1]
		}
		return "back", "1"
	}
	if args == "branch" {
		return "branch", ""
	}
	return "node", args
}

// ParseSwitchArgs parses /switch arguments.
// Returns: mode ("list", "index", "id"), value string.
func ParseSwitchArgs(args string) (mode, value string) {
	args = strings.TrimSpace(args)
	if args == "" {
		return "list", ""
	}
	if _, err := strconv.Atoi(args); err == nil {
		return "index", args
	}
	return "id", args
}

// FormatBranchList renders branch summaries as a readable string.
func FormatBranchList(branches []BranchSummary) string {
	var sb strings.Builder
	for i, b := range branches {
		marker := "  "
		if b.IsCurrent {
			marker = "● "
		}
		name := b.Name
		if len(name) > 30 {
			name = name[:27] + "…"
		}
		sb.WriteString(fmt.Sprintf("%s%d) %s (%d msgs)\n", marker, i+1, name, b.Messages))
	}
	return sb.String()
}

// FormatMessageList renders message summaries as a readable string.
func FormatMessageList(messages []MessageSummary) string {
	var sb strings.Builder
	for _, m := range messages {
		role := m.Role
		if len(role) > 9 {
			role = role[:9]
		}
		sb.WriteString(fmt.Sprintf("  %d) [%s] %s\n", m.Index, role, m.Preview))
	}
	return sb.String()
}

func extractPreview(content []core.ContentBlock, maxLen int) string {
	for _, b := range content {
		text := b.Text
		if text == "" {
			text = b.Content
		}
		if text != "" {
			text = strings.Join(strings.Fields(text), " ")
			if len(text) > maxLen {
				return text[:maxLen-1] + "…"
			}
			return text
		}
	}
	return "(empty)"
}
