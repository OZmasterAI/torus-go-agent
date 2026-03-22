package commands

import (
	"path/filepath"
	"strings"
	"testing"

	"torus_go_agent/internal/core"
	"torus_go_agent/internal/types"
)

// newTestDAG creates a DAG backed by a temp file for a single test.
func newTestDAG(t *testing.T) *core.DAG {
	t.Helper()
	dag, err := core.NewDAG(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("NewDAG: %v", err)
	}
	t.Cleanup(func() { dag.Close() })
	return dag
}

// addNode is a helper that adds a node and fails the test on error.
func addNode(t *testing.T, dag *core.DAG, parentID string, role types.Role, text string) string {
	t.Helper()
	id, err := dag.AddNode(parentID, role, []types.ContentBlock{{Type: "text", Text: text}}, "", "", 0)
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}
	return id
}

// headID returns the current head and fails if it is empty.
func headID(t *testing.T, dag *core.DAG) string {
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
// ParseForkArgs
// ---------------------------------------------------------------------------

func TestParseForkArgs(t *testing.T) {
	tests := []struct {
		input      string
		wantAction string
		wantValue  string
	}{
		{"", "head", ""},
		{"-back 3", "back", "3"},
		{"-b 1", "back", "1"},
		{"branch", "branch", ""},
		{"some_node_id", "node", "some_node_id"},
		{"nd_abcdef01", "node", "nd_abcdef01"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			action, value := ParseForkArgs(tt.input)
			if action != tt.wantAction || value != tt.wantValue {
				t.Errorf("ParseForkArgs(%q) = (%q, %q), want (%q, %q)",
					tt.input, action, value, tt.wantAction, tt.wantValue)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ParseSwitchArgs
// ---------------------------------------------------------------------------

func TestParseSwitchArgs(t *testing.T) {
	tests := []struct {
		input    string
		wantMode string
		wantVal  string
	}{
		{"", "list", ""},
		{"1", "index", "1"},
		{"42", "index", "42"},
		{"br_abc", "id", "br_abc"},
		{"main", "id", "main"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			mode, value := ParseSwitchArgs(tt.input)
			if mode != tt.wantMode || value != tt.wantVal {
				t.Errorf("ParseSwitchArgs(%q) = (%q, %q), want (%q, %q)",
					tt.input, mode, value, tt.wantMode, tt.wantVal)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Alias
// ---------------------------------------------------------------------------

func TestAlias_SetOnHead(t *testing.T) {
	dag := newTestDAG(t)
	nodeID := addNode(t, dag, "", types.RoleUser, "hello")

	msg, err := Alias(dag, "", "my-alias")
	if err != nil {
		t.Fatalf("Alias set on head: %v", err)
	}
	if !strings.Contains(msg, "my-alias") {
		t.Errorf("expected result to contain alias name, got %q", msg)
	}

	// Resolve should work.
	resolved, err := dag.ResolveAlias("my-alias")
	if err != nil {
		t.Fatalf("ResolveAlias: %v", err)
	}
	if resolved != nodeID {
		t.Errorf("alias resolved to %q, want %q", resolved, nodeID)
	}
}

func TestAlias_SetOnSpecificNode(t *testing.T) {
	dag := newTestDAG(t)
	node1 := addNode(t, dag, "", types.RoleUser, "first")
	_ = addNode(t, dag, node1, types.RoleAssistant, "second") // head is now node2

	// Set alias by explicit node ID (not head).
	msg, err := Alias(dag, node1, "first-node")
	if err != nil {
		t.Fatalf("Alias set on specific node: %v", err)
	}
	if !strings.Contains(msg, "first-node") {
		t.Errorf("expected result to contain alias name, got %q", msg)
	}

	resolved, err := dag.ResolveAlias("first-node")
	if err != nil {
		t.Fatalf("ResolveAlias: %v", err)
	}
	if resolved != node1 {
		t.Errorf("alias resolved to %q, want %q", resolved, node1)
	}
}

func TestAlias_RemoveAliases(t *testing.T) {
	dag := newTestDAG(t)
	nodeID := addNode(t, dag, "", types.RoleUser, "hello")

	// Set two aliases.
	if err := dag.SetAlias(nodeID, "alpha"); err != nil {
		t.Fatalf("SetAlias alpha: %v", err)
	}
	if err := dag.SetAlias(nodeID, "beta"); err != nil {
		t.Fatalf("SetAlias beta: %v", err)
	}

	// Remove via empty name (uses head).
	msg, err := Alias(dag, "", "")
	if err != nil {
		t.Fatalf("Alias remove: %v", err)
	}
	if !strings.Contains(msg, "Removed") {
		t.Errorf("expected removal message, got %q", msg)
	}

	// Both aliases should be gone.
	aliases, _ := dag.GetAliases(nodeID)
	if len(aliases) != 0 {
		t.Errorf("expected 0 aliases after remove, got %v", aliases)
	}
}

func TestAlias_NoHead_ReturnsError(t *testing.T) {
	dag := newTestDAG(t)
	// Branch is empty — no head.
	_, err := Alias(dag, "", "whatever")
	if err == nil {
		t.Fatal("expected error when no head exists, got nil")
	}
}

// ---------------------------------------------------------------------------
// New
// ---------------------------------------------------------------------------

func TestNew_CreatesBranch(t *testing.T) {
	dag := newTestDAG(t)
	origID := dag.CurrentBranchID()

	newID, err := New(dag, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if newID == "" {
		t.Fatal("New returned empty branch ID")
	}
	if newID == origID {
		t.Errorf("New returned the same branch ID %q", newID)
	}
	// DAG should now be on the new branch.
	if dag.CurrentBranchID() != newID {
		t.Errorf("current branch = %q, want %q", dag.CurrentBranchID(), newID)
	}
}

func TestNew_BranchHasEmptyHead(t *testing.T) {
	dag := newTestDAG(t)
	// Add a node on the original branch first.
	_ = addNode(t, dag, "", types.RoleUser, "original message")

	_, err := New(dag, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	head, err := dag.GetHead()
	if err != nil {
		t.Fatalf("GetHead: %v", err)
	}
	if head != "" {
		t.Errorf("new branch head should be empty, got %q", head)
	}
}

// ---------------------------------------------------------------------------
// Clear
// ---------------------------------------------------------------------------

func TestClear_ResetsHead(t *testing.T) {
	dag := newTestDAG(t)
	_ = addNode(t, dag, "", types.RoleUser, "message 1")
	// Head is non-empty.
	before := headID(t, dag)
	if before == "" {
		t.Fatal("head should be set before Clear")
	}

	if err := Clear(dag, nil); err != nil {
		t.Fatalf("Clear: %v", err)
	}

	after, err := dag.GetHead()
	if err != nil {
		t.Fatalf("GetHead after Clear: %v", err)
	}
	if after != "" {
		t.Errorf("head after Clear = %q, want empty", after)
	}
}

func TestClear_EmptyBranch_NoError(t *testing.T) {
	dag := newTestDAG(t)
	// Branch has no nodes — Clear should succeed without error.
	if err := Clear(dag, nil); err != nil {
		t.Errorf("Clear on empty branch returned error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Fork / ForkFromHead / ForkBack
// ---------------------------------------------------------------------------

func TestForkFromHead_CreatesBranch(t *testing.T) {
	dag := newTestDAG(t)
	origBranch := dag.CurrentBranchID()
	_ = addNode(t, dag, "", types.RoleUser, "root message")

	newBranchID, err := ForkFromHead(dag, "my-fork")
	if err != nil {
		t.Fatalf("ForkFromHead: %v", err)
	}
	if newBranchID == "" || newBranchID == origBranch {
		t.Errorf("ForkFromHead returned unexpected branch ID %q", newBranchID)
	}
}

func TestForkFromHead_NoHead_ReturnsError(t *testing.T) {
	dag := newTestDAG(t)
	_, err := ForkFromHead(dag, "fork-from-empty")
	if err == nil {
		t.Fatal("expected error when forking from empty branch, got nil")
	}
}

func TestFork_ByNodeID(t *testing.T) {
	dag := newTestDAG(t)
	node1 := addNode(t, dag, "", types.RoleUser, "node 1")
	_ = addNode(t, dag, node1, types.RoleAssistant, "node 2")

	newBranchID, err := Fork(dag, node1, "fork-from-node1")
	if err != nil {
		t.Fatalf("Fork by node ID: %v", err)
	}
	if newBranchID == "" {
		t.Fatal("Fork returned empty branch ID")
	}
}

func TestFork_ByAlias(t *testing.T) {
	dag := newTestDAG(t)
	node1 := addNode(t, dag, "", types.RoleUser, "aliased node")
	if err := dag.SetAlias(node1, "checkpoint"); err != nil {
		t.Fatalf("SetAlias: %v", err)
	}
	_ = addNode(t, dag, node1, types.RoleAssistant, "later node")

	newBranchID, err := Fork(dag, "checkpoint", "fork-from-alias")
	if err != nil {
		t.Fatalf("Fork by alias: %v", err)
	}
	if newBranchID == "" {
		t.Fatal("Fork returned empty branch ID")
	}
}

func TestForkBack_OneStep(t *testing.T) {
	dag := newTestDAG(t)
	n1 := addNode(t, dag, "", types.RoleUser, "msg1")
	n2 := addNode(t, dag, n1, types.RoleAssistant, "msg2")
	_ = n2

	newBranchID, err := ForkBack(dag, 1, "back-1")
	if err != nil {
		t.Fatalf("ForkBack 1: %v", err)
	}
	if newBranchID == "" {
		t.Fatal("ForkBack returned empty branch ID")
	}
}

func TestForkBack_MoreThanAvailable_ClampsToRoot(t *testing.T) {
	dag := newTestDAG(t)
	n1 := addNode(t, dag, "", types.RoleUser, "only node")
	_ = n1

	// Ask for 10 steps back when only 1 message exists — should clamp to root.
	newBranchID, err := ForkBack(dag, 10, "back-clamped")
	if err != nil {
		t.Fatalf("ForkBack clamped: %v", err)
	}
	if newBranchID == "" {
		t.Fatal("ForkBack clamped returned empty branch ID")
	}
}

func TestForkBack_NoHead_ReturnsError(t *testing.T) {
	dag := newTestDAG(t)
	_, err := ForkBack(dag, 1, "no-head")
	if err == nil {
		t.Fatal("expected error when forking back with no head, got nil")
	}
}

// ---------------------------------------------------------------------------
// ListBranches
// ---------------------------------------------------------------------------

func TestListBranches_InitialState(t *testing.T) {
	dag := newTestDAG(t)
	branches, err := ListBranches(dag)
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	if len(branches) != 1 {
		t.Fatalf("expected 1 branch initially, got %d", len(branches))
	}
	if !branches[0].IsCurrent {
		t.Error("initial branch should be marked current")
	}
}

func TestListBranches_AfterNew(t *testing.T) {
	dag := newTestDAG(t)
	_, err := New(dag, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	branches, err := ListBranches(dag)
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	if len(branches) != 2 {
		t.Fatalf("expected 2 branches, got %d", len(branches))
	}
	// Exactly one should be current.
	currentCount := 0
	for _, b := range branches {
		if b.IsCurrent {
			currentCount++
		}
	}
	if currentCount != 1 {
		t.Errorf("expected exactly 1 current branch, got %d", currentCount)
	}
}

func TestListBranches_MessageCountIncludesAncestors(t *testing.T) {
	dag := newTestDAG(t)
	n1 := addNode(t, dag, "", types.RoleUser, "msg1")
	_ = addNode(t, dag, n1, types.RoleAssistant, "msg2")
	_ = addNode(t, dag, headID(t, dag), types.RoleUser, "msg3")

	branches, err := ListBranches(dag)
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	if len(branches) != 1 {
		t.Fatalf("expected 1 branch, got %d", len(branches))
	}
	if branches[0].Messages != 3 {
		t.Errorf("expected 3 messages, got %d", branches[0].Messages)
	}
}

// ---------------------------------------------------------------------------
// ListMessages
// ---------------------------------------------------------------------------

func TestListMessages_OrderAndContent(t *testing.T) {
	dag := newTestDAG(t)
	n1 := addNode(t, dag, "", types.RoleUser, "hello world")
	n2 := addNode(t, dag, n1, types.RoleAssistant, "hi there")
	_ = n2

	branchID := dag.CurrentBranchID()
	msgs, err := ListMessages(dag, branchID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "user" {
		t.Errorf("msgs[0].Role = %q, want \"user\"", msgs[0].Role)
	}
	if msgs[1].Role != "assistant" {
		t.Errorf("msgs[1].Role = %q, want \"assistant\"", msgs[1].Role)
	}
	if msgs[0].Index != 0 {
		t.Errorf("msgs[0].Index = %d, want 0", msgs[0].Index)
	}
}

func TestListMessages_PreviewTruncation(t *testing.T) {
	dag := newTestDAG(t)
	long := strings.Repeat("a", 200)
	_ = addNode(t, dag, "", types.RoleUser, long)

	branchID := dag.CurrentBranchID()
	msgs, err := ListMessages(dag, branchID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	// extractPreview truncates at 80 chars. The ellipsis "…" is 3 UTF-8 bytes,
	// so the byte length of a truncated preview is at most 79 + 3 = 82.
	// Assert that the preview is shorter than the original 200-char input.
	preview := msgs[0].Preview
	if len([]rune(preview)) > 80 {
		t.Errorf("preview rune count too large: %d", len([]rune(preview)))
	}
	if !strings.HasSuffix(preview, "…") {
		t.Errorf("truncated preview should end with ellipsis, got %q", preview)
	}
}

func TestListMessages_IncludesAliases(t *testing.T) {
	dag := newTestDAG(t)
	nodeID := addNode(t, dag, "", types.RoleUser, "aliased message")
	if err := dag.SetAlias(nodeID, "tagged"); err != nil {
		t.Fatalf("SetAlias: %v", err)
	}

	branchID := dag.CurrentBranchID()
	msgs, err := ListMessages(dag, branchID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs[0].Aliases) == 0 {
		t.Error("expected aliases to be populated")
	}
	if msgs[0].Aliases[0] != "tagged" {
		t.Errorf("alias = %q, want \"tagged\"", msgs[0].Aliases[0])
	}
}

func TestListMessages_EmptyBranch_ReturnsError(t *testing.T) {
	dag := newTestDAG(t)
	branchID := dag.CurrentBranchID()
	_, err := ListMessages(dag, branchID)
	if err == nil {
		t.Fatal("expected error for branch with no messages, got nil")
	}
}

// ---------------------------------------------------------------------------
// Steering
// ---------------------------------------------------------------------------

func TestSteering_DefaultIsmild(t *testing.T) {
	dag := newTestDAG(t)
	agent := core.NewAgent(types.AgentConfig{}, nil, nil, dag)

	result := Steering(agent, "")
	if !strings.Contains(result, "mild") {
		t.Errorf("default steering mode should be mild, got %q", result)
	}
}

func TestSteering_SetAggressive(t *testing.T) {
	dag := newTestDAG(t)
	agent := core.NewAgent(types.AgentConfig{}, nil, nil, dag)

	result := Steering(agent, "aggressive")
	if !strings.Contains(result, "aggressive") {
		t.Errorf("expected aggressive in result, got %q", result)
	}
	if agent.GetSteeringMode() != "aggressive" {
		t.Errorf("GetSteeringMode = %q, want \"aggressive\"", agent.GetSteeringMode())
	}
}

func TestSteering_SetMild(t *testing.T) {
	dag := newTestDAG(t)
	agent := core.NewAgent(types.AgentConfig{}, nil, nil, dag)
	agent.SetSteeringMode("aggressive") // start in aggressive

	result := Steering(agent, "mild")
	if !strings.Contains(result, "mild") {
		t.Errorf("expected mild in result, got %q", result)
	}
	if agent.GetSteeringMode() != "mild" {
		t.Errorf("GetSteeringMode = %q, want \"mild\"", agent.GetSteeringMode())
	}
}

func TestSteering_UnknownMode_ReturnsError(t *testing.T) {
	dag := newTestDAG(t)
	agent := core.NewAgent(types.AgentConfig{}, nil, nil, dag)

	result := Steering(agent, "extreme")
	if !strings.Contains(result, "Unknown") {
		t.Errorf("expected Unknown in result for bad mode, got %q", result)
	}
	// Mode should remain unchanged (mild default).
	if agent.GetSteeringMode() != "mild" {
		t.Errorf("steering mode changed unexpectedly to %q", agent.GetSteeringMode())
	}
}

func TestSteering_CaseInsensitive(t *testing.T) {
	dag := newTestDAG(t)
	agent := core.NewAgent(types.AgentConfig{}, nil, nil, dag)

	Steering(agent, "AGGRESSIVE")
	if agent.GetSteeringMode() != "aggressive" {
		t.Errorf("GetSteeringMode = %q, want \"aggressive\"", agent.GetSteeringMode())
	}
}
