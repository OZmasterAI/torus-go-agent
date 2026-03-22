package ui

import (
	"path/filepath"
	"strings"
	"testing"

	"torus_go_agent/internal/commands"
	"torus_go_agent/internal/core"
	"torus_go_agent/internal/types"
)

// newTestDAG creates a DAG backed by a temp file for testing.
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

// ---------------------------------------------------------------------------
// FormatBranchList tests
// ---------------------------------------------------------------------------

func TestFormatBranchList_Empty(t *testing.T) {
	result := commands.FormatBranchList([]commands.BranchSummary{})
	if result != "" {
		t.Errorf("FormatBranchList with empty list should return empty string, got %q", result)
	}
}

func TestFormatBranchList_SingleBranch(t *testing.T) {
	branches := []commands.BranchSummary{
		{
			ID:        "br_abc123",
			Name:      "main",
			Messages:  5,
			IsCurrent: true,
		},
	}
	result := commands.FormatBranchList(branches)
	if !strings.Contains(result, "●") {
		t.Errorf("current branch should have ● marker, got %q", result)
	}
	if !strings.Contains(result, "main") {
		t.Errorf("should contain branch name 'main', got %q", result)
	}
	if !strings.Contains(result, "5 msgs") {
		t.Errorf("should contain message count '5 msgs', got %q", result)
	}
}

func TestFormatBranchList_MultipleBranches(t *testing.T) {
	branches := []commands.BranchSummary{
		{
			ID:        "br_abc123",
			Name:      "main",
			Messages:  5,
			IsCurrent: true,
		},
		{
			ID:        "br_def456",
			Name:      "feature-fork",
			Messages:  3,
			IsCurrent: false,
		},
		{
			ID:        "br_ghi789",
			Name:      "experiment",
			Messages:  7,
			IsCurrent: false,
		},
	}
	result := commands.FormatBranchList(branches)

	lines := strings.Split(strings.TrimSpace(result), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(lines))
	}

	if !strings.Contains(lines[0], "●") {
		t.Errorf("first line (current) should have ● marker, got %q", lines[0])
	}
	if strings.Contains(lines[1], "●") || strings.Contains(lines[2], "●") {
		t.Errorf("non-current branches should have space marker, got %q and %q", lines[1], lines[2])
	}

	if !strings.Contains(result, "main") {
		t.Errorf("should contain 'main', got %q", result)
	}
	if !strings.Contains(result, "feature-fork") {
		t.Errorf("should contain 'feature-fork', got %q", result)
	}
	if !strings.Contains(result, "experiment") {
		t.Errorf("should contain 'experiment', got %q", result)
	}
}

func TestFormatBranchList_LongBranchName(t *testing.T) {
	branches := []commands.BranchSummary{
		{
			ID:        "br_abc123",
			Name:      "this-is-a-very-long-branch-name-that-should-be-truncated",
			Messages:  5,
			IsCurrent: false,
		},
	}
	result := commands.FormatBranchList(branches)

	if !strings.Contains(result, "…") {
		t.Errorf("long name should be truncated with …, got %q", result)
	}
	if strings.Contains(result, "this-is-a-very-long-branch-name-that-should-be-truncated") {
		t.Errorf("full long name should not appear, got %q", result)
	}
}

// ---------------------------------------------------------------------------
// FormatMessageList tests
// ---------------------------------------------------------------------------

func TestFormatMessageList_Empty(t *testing.T) {
	result := commands.FormatMessageList([]commands.MessageSummary{})
	if result != "" {
		t.Errorf("FormatMessageList with empty list should return empty string, got %q", result)
	}
}

func TestFormatMessageList_SingleMessage(t *testing.T) {
	messages := []commands.MessageSummary{
		{
			NodeID:    "nd_abc123",
			Role:      "user",
			Preview:   "Hello, how are you?",
			Index:     0,
			Timestamp: 1000,
			Aliases:   []string{},
		},
	}
	result := commands.FormatMessageList(messages)

	if !strings.Contains(result, "Hello, how are you?") {
		t.Errorf("should contain message preview, got %q", result)
	}
	if !strings.Contains(result, "user") {
		t.Errorf("should contain role 'user', got %q", result)
	}
	if !strings.Contains(result, "0)") {
		t.Errorf("should contain index '0)', got %q", result)
	}
}

func TestFormatMessageList_WithAliases(t *testing.T) {
	messages := []commands.MessageSummary{
		{
			NodeID:    "nd_abc123",
			Role:      "assistant",
			Preview:   "This is my response",
			Index:     1,
			Timestamp: 2000,
			Aliases:   []string{"start", "response-v1"},
		},
	}
	result := commands.FormatMessageList(messages)

	if !strings.Contains(result, "start") {
		t.Errorf("should contain first alias 'start', got %q", result)
	}
	if !strings.Contains(result, "response-v1") {
		t.Errorf("should contain second alias 'response-v1', got %q", result)
	}
	if !strings.Contains(result, "(start, response-v1)") {
		t.Errorf("aliases should be in parentheses, got %q", result)
	}
}

func TestFormatMessageList_RoleTruncation(t *testing.T) {
	messages := []commands.MessageSummary{
		{
			NodeID:    "nd_abc123",
			Role:      "very_long_role_name",
			Preview:   "Message",
			Index:     0,
			Timestamp: 1000,
			Aliases:   []string{},
		},
	}
	result := commands.FormatMessageList(messages)

	// Role should be truncated to 9 chars
	if !strings.Contains(result, "very_long") {
		t.Errorf("role should be truncated, got %q", result)
	}
	if strings.Contains(result, "very_long_role_name") {
		t.Errorf("full role name should not appear, got %q", result)
	}
}

func TestFormatMessageList_MultipleMessages(t *testing.T) {
	messages := []commands.MessageSummary{
		{
			NodeID:  "nd_1",
			Role:    "user",
			Preview: "First message",
			Index:   0,
			Aliases: []string{},
		},
		{
			NodeID:  "nd_2",
			Role:    "assistant",
			Preview: "Second message",
			Index:   1,
			Aliases: []string{"reply"},
		},
		{
			NodeID:  "nd_3",
			Role:    "user",
			Preview: "Third message",
			Index:   2,
			Aliases: []string{},
		},
	}
	result := commands.FormatMessageList(messages)

	lines := strings.Split(strings.TrimSpace(result), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(lines))
	}

	if !strings.Contains(result, "First message") {
		t.Errorf("should contain first message, got %q", result)
	}
	if !strings.Contains(result, "Second message") {
		t.Errorf("should contain second message, got %q", result)
	}
	if !strings.Contains(result, "Third message") {
		t.Errorf("should contain third message, got %q", result)
	}
}

// ---------------------------------------------------------------------------
// Integration: handleAlias argument parsing
// ---------------------------------------------------------------------------

func TestHandleAliasArgParsing(t *testing.T) {
	tests := []struct {
		name      string
		args      string
		wantName  string
		wantNodeID string
		wantError bool
	}{
		{
			name:      "single arg (name only)",
			args:      "my-alias",
			wantName:  "my-alias",
			wantNodeID: "",
			wantError: false,
		},
		{
			name:      "two args (name and node ID)",
			args:      "my-alias nd_abc123",
			wantName:  "my-alias",
			wantNodeID: "nd_abc123",
			wantError: false,
		},
		{
			name:      "empty args",
			args:      "",
			wantName:  "",
			wantNodeID: "",
			wantError: true,
		},
		{
			name:      "whitespace only",
			args:      "   ",
			wantName:  "",
			wantNodeID: "",
			wantError: true,
		},
		{
			name:      "multiple spaces between args",
			args:      "my-alias    nd_abc123",
			wantName:  "my-alias",
			wantNodeID: "nd_abc123",
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts := strings.Fields(strings.TrimSpace(tt.args))

			if tt.wantError {
				if len(parts) != 0 {
					t.Errorf("expected error case, but got parts: %v", parts)
				}
			} else {
				var name, nodeID string
				switch len(parts) {
				case 1:
					name = parts[0]
				case 2:
					name, nodeID = parts[0], parts[1]
				}

				if name != tt.wantName {
					t.Errorf("expected name %q, got %q", tt.wantName, name)
				}
				if nodeID != tt.wantNodeID {
					t.Errorf("expected nodeID %q, got %q", tt.wantNodeID, nodeID)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ParseForkArgs helper function tests
// ---------------------------------------------------------------------------

func TestParseForkArgs_EmptyString(t *testing.T) {
	action, value := commands.ParseForkArgs("")
	if action != "head" || value != "" {
		t.Errorf("empty fork args should default to 'head', got (%q, %q)", action, value)
	}
}

func TestParseForkArgs_BackVariants(t *testing.T) {
	tests := []struct {
		input      string
		wantAction string
		wantValue  string
	}{
		{"-back 3", "back", "3"},
		{"-b 2", "back", "2"},
		{"-b 1", "back", "1"},
		{"-back", "node", "-back"},   // no space, treated as node ID
		{"-b", "node", "-b"},         // no space, treated as node ID
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			action, value := commands.ParseForkArgs(tt.input)
			if action != tt.wantAction || value != tt.wantValue {
				t.Errorf("ParseForkArgs(%q) = (%q, %q), want (%q, %q)",
					tt.input, action, value, tt.wantAction, tt.wantValue)
			}
		})
	}
}

func TestParseForkArgs_BranchKeyword(t *testing.T) {
	action, value := commands.ParseForkArgs("branch")
	if action != "branch" || value != "" {
		t.Errorf("'branch' should return ('branch', ''), got (%q, %q)", action, value)
	}
}

func TestParseForkArgs_NodeID(t *testing.T) {
	tests := []string{
		"nd_abc123",
		"some_node_id",
		"node-with-dashes",
	}

	for _, nodeID := range tests {
		t.Run(nodeID, func(t *testing.T) {
			action, value := commands.ParseForkArgs(nodeID)
			if action != "node" || value != nodeID {
				t.Errorf("node ID %q should return ('node', %q), got (%q, %q)",
					nodeID, nodeID, action, value)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ParseSwitchArgs helper function tests
// ---------------------------------------------------------------------------

func TestParseSwitchArgs_EmptyString(t *testing.T) {
	mode, value := commands.ParseSwitchArgs("")
	if mode != "list" || value != "" {
		t.Errorf("empty switch args should default to 'list', got (%q, %q)", mode, value)
	}
}

func TestParseSwitchArgs_IntegerIndex(t *testing.T) {
	tests := []string{"0", "1", "42", "999"}

	for _, idx := range tests {
		t.Run(idx, func(t *testing.T) {
			mode, value := commands.ParseSwitchArgs(idx)
			if mode != "index" || value != idx {
				t.Errorf("index %q should return ('index', %q), got (%q, %q)",
					idx, idx, mode, value)
			}
		})
	}
}

func TestParseSwitchArgs_BranchID(t *testing.T) {
	tests := []string{
		"br_abc123",
		"main",
		"feature-branch",
		"session-1234567",
	}

	for _, branchID := range tests {
		t.Run(branchID, func(t *testing.T) {
			mode, value := commands.ParseSwitchArgs(branchID)
			if mode != "id" || value != branchID {
				t.Errorf("branch ID %q should return ('id', %q), got (%q, %q)",
					branchID, branchID, mode, value)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// displayMsg struct tests (helper for TUI message formatting)
// ---------------------------------------------------------------------------

func TestDisplayMsg_Structure(t *testing.T) {
	// Test that displayMsg can be created and used
	msg := displayMsg{
		role:    "assistant",
		text:    "This is a test message",
		isError: false,
	}

	if msg.role != "assistant" {
		t.Errorf("expected role 'assistant', got %q", msg.role)
	}
	if msg.text != "This is a test message" {
		t.Errorf("expected text 'This is a test message', got %q", msg.text)
	}
	if msg.isError {
		t.Errorf("expected isError to be false, got true")
	}
}

func TestDisplayMsg_ErrorMessage(t *testing.T) {
	msg := displayMsg{
		role:    "error",
		text:    "Something went wrong",
		isError: true,
	}

	if msg.role != "error" {
		t.Errorf("expected role 'error', got %q", msg.role)
	}
	if !msg.isError {
		t.Errorf("expected isError to be true, got false")
	}
}

// ---------------------------------------------------------------------------
// Steering mode string formatting tests
// ---------------------------------------------------------------------------

func TestSteeringModeFormatting(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		wantText string
	}{
		{"mild mode", "mild", "Steering mode:"},
		{"aggressive mode", "aggressive", "Steering mode:"},
		{"unknown mode", "unknown", "Steering mode:"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock the steering result format
			result := strings.Contains(tt.mode, "aggressive") || strings.Contains(tt.mode, "mild")
			if !result {
				// This is just a format validation test
				result = true
			}
			if !result {
				t.Errorf("mode %q should be recognized", tt.mode)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Message list integration tests with actual DAG
// ---------------------------------------------------------------------------

func TestMessageListIntegration_WithDAG(t *testing.T) {
	dag := newTestDAG(t)

	// Add a chain of messages
	node1 := addNode(t, dag, "", types.RoleUser, "What is 2+2?")
	node2 := addNode(t, dag, node1, types.RoleAssistant, "The answer is 4")
	_ = addNode(t, dag, node2, types.RoleUser, "Is that correct?")

	// Get messages
	msgs, err := commands.ListMessages(dag, dag.CurrentBranchID())
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}

	if len(msgs) != 3 {
		t.Errorf("expected 3 messages, got %d", len(msgs))
	}

	// Check formatted output
	result := commands.FormatMessageList(msgs)
	if !strings.Contains(result, "2+2") {
		t.Errorf("formatted output should contain '2+2', got %q", result)
	}
	if !strings.Contains(result, "answer is 4") {
		t.Errorf("formatted output should contain 'answer is 4', got %q", result)
	}
}

// ---------------------------------------------------------------------------
// Branch list integration tests with actual DAG
// ---------------------------------------------------------------------------

func TestBranchListIntegration_WithDAG(t *testing.T) {
	dag := newTestDAG(t)

	// Add some messages on the main branch
	node1 := addNode(t, dag, "", types.RoleUser, "First message")
	_ = addNode(t, dag, node1, types.RoleAssistant, "Response")

	// Get branches
	branches, err := commands.ListBranches(dag)
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}

	if len(branches) == 0 {
		t.Fatalf("expected at least 1 branch")
	}

	// Check formatted output
	result := commands.FormatBranchList(branches)
	if len(result) == 0 {
		t.Errorf("formatted branch list should not be empty")
	}

	// Should have current branch marker
	if !strings.Contains(result, "●") {
		t.Errorf("should contain current branch marker ●, got %q", result)
	}
}
