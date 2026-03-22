package commands

import (
	"path/filepath"
	"strings"
	"testing"

	"torus_go_agent/internal/core"
	"torus_go_agent/internal/types"
)

// newTestDAG creates a DAG backed by a temp file for a single test.
func newEdgeTestDAG(t *testing.T) *core.DAG {
	t.Helper()
	dag, err := core.NewDAG(filepath.Join(t.TempDir(), "edge_test.db"))
	if err != nil {
		t.Fatalf("NewDAG: %v", err)
	}
	t.Cleanup(func() { dag.Close() })
	return dag
}

// addEdgeNode is a helper that adds a node and fails the test on error.
func addEdgeNode(t *testing.T, dag *core.DAG, parentID string, role types.Role, text string) string {
	t.Helper()
	id, err := dag.AddNode(parentID, role, []types.ContentBlock{{Type: "text", Text: text}}, "", "", 0)
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	return id
}

// headEdgeID returns the current head and fails if it is empty.
func headEdgeID(t *testing.T, dag *core.DAG) string {
	t.Helper()
	h, err := dag.GetHead()
	if err != nil {
		t.Fatalf("GetHead: %v", err)
	}
	if h == "" {
		t.Fatal("GetHead: empty head")
	}
	return h
}

// ---------------------------------------------------------------------------
// ParseForkArgs Edge Cases
// ---------------------------------------------------------------------------

func TestCommandsEdge_ParseForkArgs_BackWithoutNumber(t *testing.T) {
	action, value := ParseForkArgs("-back")
	// Without a space separator, -back is treated as a node ID
	if action != "node" || value != "-back" {
		t.Errorf("ParseForkArgs(\"-back\") = (%q, %q), want (\"node\", \"-back\")", action, value)
	}
}

func TestCommandsEdge_ParseForkArgs_BackWithSpaces(t *testing.T) {
	action, value := ParseForkArgs("  -back 5  ")
	if action != "back" || value != "5" {
		t.Errorf("ParseForkArgs(\"  -back 5  \") = (%q, %q), want (\"back\", \"5\")", action, value)
	}
}

func TestCommandsEdge_ParseForkArgs_BackShortForm(t *testing.T) {
	action, value := ParseForkArgs("-b")
	// Without a space separator, -b is treated as a node ID
	if action != "node" || value != "-b" {
		t.Errorf("ParseForkArgs(\"-b\") = (%q, %q), want (\"node\", \"-b\")", action, value)
	}
}

func TestCommandsEdge_ParseForkArgs_BranchWithLeadingSpace(t *testing.T) {
	action, value := ParseForkArgs("  branch  ")
	if action != "branch" || value != "" {
		t.Errorf("ParseForkArgs(\"  branch  \") = (%q, %q), want (\"branch\", \"\")", action, value)
	}
}

func TestCommandsEdge_ParseForkArgs_NodeIDWithSpecialChars(t *testing.T) {
	action, value := ParseForkArgs("nd_abc123-def")
	if action != "node" || value != "nd_abc123-def" {
		t.Errorf("ParseForkArgs(\"nd_abc123-def\") = (%q, %q), want (\"node\", \"nd_abc123-def\")", action, value)
	}
}

// ---------------------------------------------------------------------------
// ParseSwitchArgs Edge Cases
// ---------------------------------------------------------------------------

func TestCommandsEdge_ParseSwitchArgs_LargeIndex(t *testing.T) {
	mode, value := ParseSwitchArgs("999999")
	if mode != "index" || value != "999999" {
		t.Errorf("ParseSwitchArgs(\"999999\") = (%q, %q), want (\"index\", \"999999\")", mode, value)
	}
}

func TestCommandsEdge_ParseSwitchArgs_ZeroIndex(t *testing.T) {
	mode, value := ParseSwitchArgs("0")
	if mode != "index" || value != "0" {
		t.Errorf("ParseSwitchArgs(\"0\") = (%q, %q), want (\"index\", \"0\")", mode, value)
	}
}

func TestCommandsEdge_ParseSwitchArgs_NegativeIndex(t *testing.T) {
	mode, value := ParseSwitchArgs("-5")
	// strconv.Atoi parses negative numbers, so -5 is a valid index
	if mode != "index" || value != "-5" {
		t.Errorf("ParseSwitchArgs(\"-5\") = (%q, %q), want (\"index\", \"-5\")", mode, value)
	}
}

func TestCommandsEdge_ParseSwitchArgs_IDWithSpaces(t *testing.T) {
	mode, value := ParseSwitchArgs("  br_main  ")
	if mode != "id" || value != "br_main" {
		t.Errorf("ParseSwitchArgs(\"  br_main  \") = (%q, %q), want (\"id\", \"br_main\")", mode, value)
	}
}

func TestCommandsEdge_ParseSwitchArgs_NumericStringInID(t *testing.T) {
	mode, value := ParseSwitchArgs("br_123")
	if mode != "id" || value != "br_123" {
		t.Errorf("ParseSwitchArgs(\"br_123\") = (%q, %q), want (\"id\", \"br_123\")", mode, value)
	}
}

// ---------------------------------------------------------------------------
// Switch Edge Cases
// ---------------------------------------------------------------------------

func TestCommandsEdge_SwitchByIndex_OutOfRange_TooHigh(t *testing.T) {
	dag := newEdgeTestDAG(t)
	_ = addEdgeNode(t, dag, "", types.RoleUser, "message")

	_, err := SwitchByIndex(dag, 999)
	if err == nil {
		t.Fatal("expected error for index out of range, got nil")
	}
	if !strings.Contains(err.Error(), "invalid") {
		t.Errorf("error should mention invalid, got: %v", err)
	}
}

func TestCommandsEdge_SwitchByIndex_ZeroIndex(t *testing.T) {
	dag := newEdgeTestDAG(t)
	_ = addEdgeNode(t, dag, "", types.RoleUser, "message")

	_, err := SwitchByIndex(dag, 0)
	if err == nil {
		t.Fatal("expected error for zero index, got nil")
	}
}

func TestCommandsEdge_SwitchByIndex_NegativeIndex(t *testing.T) {
	dag := newEdgeTestDAG(t)
	_ = addEdgeNode(t, dag, "", types.RoleUser, "message")

	_, err := SwitchByIndex(dag, -1)
	if err == nil {
		t.Fatal("expected error for negative index, got nil")
	}
}

func TestCommandsEdge_SwitchByIndex_SingleBranch(t *testing.T) {
	dag := newEdgeTestDAG(t)
	_ = addEdgeNode(t, dag, "", types.RoleUser, "message")

	name, err := SwitchByIndex(dag, 1)
	if err != nil {
		t.Fatalf("SwitchByIndex to only branch: %v", err)
	}
	if name == "" {
		t.Error("expected non-empty branch name")
	}
}

// ---------------------------------------------------------------------------
// FormatBranchList Edge Cases
// ---------------------------------------------------------------------------

func TestCommandsEdge_FormatBranchList_Empty(t *testing.T) {
	result := FormatBranchList([]BranchSummary{})
	if result != "" {
		t.Errorf("FormatBranchList([]) should be empty, got %q", result)
	}
}

func TestCommandsEdge_FormatBranchList_VeryLongName(t *testing.T) {
	branches := []BranchSummary{
		{
			ID:         "br_123",
			Name:       strings.Repeat("a", 100),
			Messages:   5,
			IsCurrent:  false,
			HeadNodeID: "nd_abc",
		},
	}
	result := FormatBranchList(branches)
	// Name should be truncated to ~30 chars with ellipsis
	if len(result) == 0 {
		t.Error("FormatBranchList should produce output")
	}
	if !strings.Contains(result, "…") {
		t.Errorf("long name should be truncated with ellipsis, got: %s", result)
	}
}

func TestCommandsEdge_FormatBranchList_CurrentMarker(t *testing.T) {
	branches := []BranchSummary{
		{
			ID:         "br_1",
			Name:       "branch1",
			Messages:   1,
			IsCurrent:  false,
			HeadNodeID: "nd_a",
		},
		{
			ID:         "br_2",
			Name:       "branch2",
			Messages:   2,
			IsCurrent:  true,
			HeadNodeID: "nd_b",
		},
	}
	result := FormatBranchList(branches)
	// Current branch should have "●" marker
	lines := strings.Split(result, "\n")
	if len(lines) < 2 {
		t.Fatal("expected at least 2 lines")
	}
	if !strings.Contains(lines[1], "●") {
		t.Errorf("current branch should have bullet marker, got line: %s", lines[1])
	}
}

func TestCommandsEdge_FormatBranchList_ZeroMessages(t *testing.T) {
	branches := []BranchSummary{
		{
			ID:         "br_empty",
			Name:       "empty_branch",
			Messages:   0,
			IsCurrent:  true,
			HeadNodeID: "",
		},
	}
	result := FormatBranchList(branches)
	if !strings.Contains(result, "0 msgs") {
		t.Errorf("should show 0 msgs for empty branch, got: %s", result)
	}
}

// ---------------------------------------------------------------------------
// FormatMessageList Edge Cases
// ---------------------------------------------------------------------------

func TestCommandsEdge_FormatMessageList_Empty(t *testing.T) {
	result := FormatMessageList([]MessageSummary{})
	if result != "" {
		t.Errorf("FormatMessageList([]) should be empty, got %q", result)
	}
}

func TestCommandsEdge_FormatMessageList_LongRole(t *testing.T) {
	messages := []MessageSummary{
		{
			NodeID:    "nd_123",
			Role:      "very_long_role_name",
			Preview:   "test message",
			Index:     0,
			Timestamp: 0,
		},
	}
	result := FormatMessageList(messages)
	// Role should be truncated to 9 chars
	if !strings.Contains(result, "very_long") {
		t.Errorf("role should be truncated, got: %s", result)
	}
}

func TestCommandsEdge_FormatMessageList_WithAliases(t *testing.T) {
	messages := []MessageSummary{
		{
			NodeID:    "nd_456",
			Role:      "user",
			Preview:   "message with aliases",
			Index:     2,
			Timestamp: 0,
			Aliases:   []string{"checkpoint", "save_point"},
		},
	}
	result := FormatMessageList(messages)
	if !strings.Contains(result, "checkpoint") || !strings.Contains(result, "save_point") {
		t.Errorf("aliases should be in output, got: %s", result)
	}
}

func TestCommandsEdge_FormatMessageList_NoAliases(t *testing.T) {
	messages := []MessageSummary{
		{
			NodeID:    "nd_789",
			Role:      "assistant",
			Preview:   "response",
			Index:     1,
			Timestamp: 0,
			Aliases:   []string{},
		},
	}
	result := FormatMessageList(messages)
	// Should not have parentheses for empty aliases
	if strings.Count(result, "(") > 0 && strings.Contains(result, "()") {
		t.Errorf("should not have empty alias parentheses, got: %s", result)
	}
}

// ---------------------------------------------------------------------------
// Alias Edge Cases
// ---------------------------------------------------------------------------

func TestCommandsEdge_Alias_ResolveByAlias(t *testing.T) {
	dag := newEdgeTestDAG(t)
	nodeID := addEdgeNode(t, dag, "", types.RoleUser, "message")
	if err := dag.SetAlias(nodeID, "my-alias"); err != nil {
		t.Fatalf("SetAlias: %v", err)
	}

	// Resolve the same node using its alias
	msg, err := Alias(dag, "my-alias", "new-alias")
	if err != nil {
		t.Fatalf("Alias with alias as nodeID: %v", err)
	}
	if !strings.Contains(msg, "new-alias") {
		t.Errorf("expected new-alias in result, got: %s", msg)
	}
}

func TestCommandsEdge_Alias_InvalidNodeID(t *testing.T) {
	dag := newEdgeTestDAG(t)
	_, err := Alias(dag, "nonexistent_node_id", "alias-name")
	if err == nil {
		t.Fatal("expected error for invalid node ID, got nil")
	}
}

func TestCommandsEdge_Alias_MultipleAliasesOnNode(t *testing.T) {
	dag := newEdgeTestDAG(t)
	nodeID := addEdgeNode(t, dag, "", types.RoleUser, "message")

	// Set multiple aliases
	if err := dag.SetAlias(nodeID, "first"); err != nil {
		t.Fatalf("SetAlias first: %v", err)
	}
	if err := dag.SetAlias(nodeID, "second"); err != nil {
		t.Fatalf("SetAlias second: %v", err)
	}

	aliases, _ := dag.GetAliases(nodeID)
	if len(aliases) != 2 {
		t.Errorf("expected 2 aliases, got %d", len(aliases))
	}

	// Remove all at once
	msg, err := Alias(dag, nodeID, "")
	if err != nil {
		t.Fatalf("Alias remove: %v", err)
	}
	if !strings.Contains(msg, "Removed") {
		t.Errorf("expected 'Removed' in message, got: %s", msg)
	}

	aliases, _ = dag.GetAliases(nodeID)
	if len(aliases) != 0 {
		t.Errorf("expected 0 aliases after removal, got %d", len(aliases))
	}
}

// ---------------------------------------------------------------------------
// Fork Edge Cases
// ---------------------------------------------------------------------------

func TestCommandsEdge_Fork_InvalidNodeID(t *testing.T) {
	dag := newEdgeTestDAG(t)
	_, err := Fork(dag, "nonexistent_node", "fork-name")
	if err == nil {
		t.Fatal("expected error for invalid node ID, got nil")
	}
}

func TestCommandsEdge_Fork_EmptyName(t *testing.T) {
	dag := newEdgeTestDAG(t)
	nodeID := addEdgeNode(t, dag, "", types.RoleUser, "message")

	// Empty name should be auto-generated
	branchID, err := Fork(dag, nodeID, "")
	if err != nil {
		t.Fatalf("Fork with empty name: %v", err)
	}
	if branchID == "" {
		t.Error("Fork should return a branch ID even with empty name")
	}
}

func TestCommandsEdge_ForkBack_ZeroSteps(t *testing.T) {
	dag := newEdgeTestDAG(t)
	n1 := addEdgeNode(t, dag, "", types.RoleUser, "msg1")
	n2 := addEdgeNode(t, dag, n1, types.RoleAssistant, "msg2")
	_ = n2

	branchID, err := ForkBack(dag, 0, "fork-back-0")
	if err != nil {
		t.Fatalf("ForkBack(0): %v", err)
	}
	if branchID == "" {
		t.Error("ForkBack should return a branch ID")
	}
}

// ---------------------------------------------------------------------------
// New / Clear Edge Cases
// ---------------------------------------------------------------------------

func TestCommandsEdge_New_WithHooks(t *testing.T) {
	dag := newEdgeTestDAG(t)
	hooks := core.NewHookRegistry()
	if hooks == nil {
		t.Fatal("NewHookRegistry returned nil")
	}

	newID, err := New(dag, hooks)
	if err != nil {
		t.Fatalf("New with hooks: %v", err)
	}
	if newID == "" {
		t.Error("New should return a branch ID")
	}
}

func TestCommandsEdge_Clear_WithHooks(t *testing.T) {
	dag := newEdgeTestDAG(t)
	_ = addEdgeNode(t, dag, "", types.RoleUser, "message")
	hooks := core.NewHookRegistry()

	err := Clear(dag, hooks)
	if err != nil {
		t.Fatalf("Clear with hooks: %v", err)
	}

	head, _ := dag.GetHead()
	if head != "" {
		t.Errorf("head should be empty after Clear, got %q", head)
	}
}

// ---------------------------------------------------------------------------
// ListMessages Edge Cases
// ---------------------------------------------------------------------------

func TestCommandsEdge_ListMessages_SingleMessage(t *testing.T) {
	dag := newEdgeTestDAG(t)
	_ = addEdgeNode(t, dag, "", types.RoleUser, "only message")

	branchID := dag.CurrentBranchID()
	msgs, err := ListMessages(dag, branchID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != 1 {
		t.Errorf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Index != 0 {
		t.Errorf("first message should have Index=0, got %d", msgs[0].Index)
	}
}

func TestCommandsEdge_ListMessages_WithEmptyContent(t *testing.T) {
	dag := newEdgeTestDAG(t)
	// Add a node with empty text
	id, err := dag.AddNode("", types.RoleUser, []types.ContentBlock{{Type: "text", Text: ""}}, "", "", 0)
	if err != nil {
		t.Fatalf("AddNode with empty text: %v", err)
	}

	branchID := dag.CurrentBranchID()
	msgs, err := ListMessages(dag, branchID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Preview != "(empty)" {
		t.Errorf("empty content preview should be \"(empty)\", got %q", msgs[0].Preview)
	}
	if msgs[0].NodeID != id {
		t.Errorf("message NodeID should match, got %q want %q", msgs[0].NodeID, id)
	}
}

func TestCommandsEdge_ListMessages_DifferentBranch(t *testing.T) {
	dag := newEdgeTestDAG(t)
	_ = addEdgeNode(t, dag, "", types.RoleUser, "branch1 msg")

	// Create a new branch and add different message
	newBranchID, err := New(dag, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_ = addEdgeNode(t, dag, "", types.RoleUser, "branch2 msg")

	// List messages from current branch
	_, err = ListMessages(dag, dag.CurrentBranchID())
	if err != nil && !strings.Contains(err.Error(), "no messages") {
		t.Fatalf("ListMessages: %v", err)
	}

	// After ListMessages, current branch should still be the new one
	if dag.CurrentBranchID() != newBranchID {
		t.Errorf("current branch changed unexpectedly")
	}
}

// ---------------------------------------------------------------------------
// Steering Edge Cases
// ---------------------------------------------------------------------------

func TestCommandsEdge_Steering_CaseSensitivity_Mixed(t *testing.T) {
	dag := newEdgeTestDAG(t)
	agent := core.NewAgent(types.AgentConfig{}, nil, nil, dag)

	result := Steering(agent, "MiLd")
	if !strings.Contains(result, "mild") {
		t.Errorf("MiLd should be converted to mild, got: %s", result)
	}
}

func TestCommandsEdge_Steering_WithLeadingTrailingSpace(t *testing.T) {
	dag := newEdgeTestDAG(t)
	agent := core.NewAgent(types.AgentConfig{}, nil, nil, dag)

	result := Steering(agent, "  aggressive  ")
	if !strings.Contains(result, "aggressive") {
		t.Errorf("expected aggressive in result, got: %s", result)
	}
}

func TestCommandsEdge_Steering_InvalidModes(t *testing.T) {
	dag := newEdgeTestDAG(t)
	agent := core.NewAgent(types.AgentConfig{}, nil, nil, dag)

	invalidModes := []string{"passive", "extreme", "normal", "gentle", "strict"}
	for _, mode := range invalidModes {
		result := Steering(agent, mode)
		if !strings.Contains(result, "Unknown") {
			t.Errorf("mode %q should be Unknown, got: %s", mode, result)
		}
	}
}

func TestCommandsEdge_Steering_ShowMode_DoesNotChange(t *testing.T) {
	dag := newEdgeTestDAG(t)
	agent := core.NewAgent(types.AgentConfig{}, nil, nil, dag)

	agent.SetSteeringMode("aggressive")
	result := Steering(agent, "")
	if !strings.Contains(result, "aggressive") {
		t.Errorf("show mode should display aggressive, got: %s", result)
	}
	if agent.GetSteeringMode() != "aggressive" {
		t.Errorf("mode should remain aggressive, got %q", agent.GetSteeringMode())
	}
}
