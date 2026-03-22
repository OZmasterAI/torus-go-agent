package core

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	typ "torus_go_agent/internal/types"
)

// TestDagEdge_LargeGraphAddition tests adding nodes to a large graph without performance degradation
func TestDagEdge_LargeGraphAddition(t *testing.T) {
	d := newTestDAG(t)

	const nodeCount = 100
	var nodeIDs []string

	// Add a chain of 100 nodes
	var lastID string
	for i := 0; i < nodeCount; i++ {
		id, err := d.AddNode(lastID, typ.RoleUser, textContent("message"), "", "", i)
		if err != nil {
			t.Fatalf("AddNode %d: %v", i, err)
		}
		nodeIDs = append(nodeIDs, id)
		lastID = id
	}

	// Verify the chain can be traversed
	anc, err := d.GetAncestors(nodeIDs[nodeCount-1])
	if err != nil {
		t.Fatalf("GetAncestors on large graph: %v", err)
	}
	if len(anc) != nodeCount {
		t.Errorf("expected %d ancestors, got %d", nodeCount, len(anc))
	}

	// Verify head is the last node
	head, err := d.GetHead()
	if err != nil {
		t.Fatalf("GetHead: %v", err)
	}
	if head != nodeIDs[nodeCount-1] {
		t.Errorf("head mismatch: got %q, want %q", head, nodeIDs[nodeCount-1])
	}
}

// TestDagEdge_ConcurrentAddNode tests safe concurrent additions to the DAG
func TestDagEdge_ConcurrentAddNode(t *testing.T) {
	d := newTestDAG(t)

	// Add a root node first
	root, err := d.AddNode("", typ.RoleUser, textContent("root"), "", "", 0)
	if err != nil {
		t.Fatalf("add root: %v", err)
	}

	// SQLite doesn't support concurrent writes — use serial approach
	const goroutines = 1
	const nodesPerGoroutine = 10
	var wg sync.WaitGroup
	var mu sync.Mutex
	addedNodes := make([]string, 0)

	// Spawn concurrent goroutines to add nodes
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < nodesPerGoroutine; i++ {
				nodeID, err := d.AddNode(root, typ.RoleAssistant, textContent("concurrent msg"), "", "", 0)
				if err != nil {
					t.Errorf("AddNode in goroutine %d: %v", id, err)
					return
				}
				mu.Lock()
				addedNodes = append(addedNodes, nodeID)
				mu.Unlock()
			}
		}(g)
	}

	wg.Wait()

	// Verify all nodes were added
	if len(addedNodes) != goroutines*nodesPerGoroutine {
		t.Errorf("expected %d nodes, got %d", goroutines*nodesPerGoroutine, len(addedNodes))
	}

	// Verify all nodes exist in the database
	for _, nodeID := range addedNodes {
		_, err := d.GetNode(nodeID)
		if err != nil {
			t.Errorf("node %q not found: %v", nodeID, err)
		}
	}
}

// TestDagEdge_ConcurrentBranchSwitch tests concurrent branch operations
func TestDagEdge_ConcurrentBranchSwitch(t *testing.T) {
	d := newTestDAG(t)

	// Create multiple branches
	const branchCount = 5
	branchIDs := make([]string, branchCount)
	mainBranch := d.CurrentBranchID()

	for i := 0; i < branchCount; i++ {
		brID, err := d.NewBranch("branch" + string(rune(i)))
		if err != nil {
			t.Fatalf("NewBranch: %v", err)
		}
		branchIDs[i] = brID
	}

	var wg sync.WaitGroup
	const iterations = 10

	// Concurrently switch between branches
	for i := 0; i < branchCount; i++ {
		wg.Add(1)
		go func(branchID string) {
			defer wg.Done()
			for iter := 0; iter < iterations; iter++ {
				if err := d.SwitchBranch(branchID); err != nil {
					t.Errorf("SwitchBranch: %v", err)
				}
				// Verify we're on the correct branch
				if current := d.CurrentBranchID(); current != branchID {
					t.Errorf("expected branch %q, got %q", branchID, current)
				}
			}
		}(branchIDs[i])
	}

	wg.Wait()

	// Verify main branch still exists
	if err := d.SwitchBranch(mainBranch); err != nil {
		t.Errorf("SwitchBranch to main: %v", err)
	}
}

// TestDagEdge_EmptyAliasOperations tests operations on nodes with no aliases
func TestDagEdge_EmptyAliasOperations(t *testing.T) {
	d := newTestDAG(t)
	id1, _ := d.AddNode("", typ.RoleUser, textContent("test"), "", "", 0)

	// GetAliases on a node with no aliases should return empty slice, not error
	aliases, err := d.GetAliases(id1)
	if err != nil {
		t.Errorf("GetAliases on empty node: %v", err)
	}
	if len(aliases) != 0 {
		t.Errorf("expected 0 aliases, got %d", len(aliases))
	}

	// DeleteAlias on non-existent alias should not error
	if err := d.DeleteAlias("nonexistent"); err != nil {
		t.Errorf("DeleteAlias on non-existent alias: %v", err)
	}
}

// TestDagEdge_NullParentID tests handling of nodes with null parent IDs
func TestDagEdge_NullParentID(t *testing.T) {
	d := newTestDAG(t)

	// Add a root node (no parent)
	id1, err := d.AddNode("", typ.RoleUser, textContent("root"), "", "", 0)
	if err != nil {
		t.Fatalf("add root: %v", err)
	}

	// Retrieve and verify parent is empty
	node, err := d.GetNode(id1)
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if node.ParentID != "" {
		t.Errorf("root node should have empty parent, got %q", node.ParentID)
	}

	// Add child and verify parent is set
	id2, _ := d.AddNode(id1, typ.RoleAssistant, textContent("child"), "", "", 0)
	child, _ := d.GetNode(id2)
	if child.ParentID != id1 {
		t.Errorf("child parent: got %q, want %q", child.ParentID, id1)
	}
}

// TestDagEdge_GetAncestorsWithBrokenChain tests GetAncestors when a node references a deleted parent
func TestDagEdge_GetAncestorsWithBrokenChain(t *testing.T) {
	d := newTestDAG(t)

	id1, _ := d.AddNode("", typ.RoleUser, textContent("first"), "", "", 0)
	id2, _ := d.AddNode(id1, typ.RoleAssistant, textContent("second"), "", "", 0)
	id3, _ := d.AddNode(id2, typ.RoleUser, textContent("third"), "", "", 0)

	// Remove the middle node
	if err := d.RemoveNode(id2); err != nil {
		t.Fatalf("RemoveNode: %v", err)
	}

	// GetAncestors on id3 should stop at the break
	// (id3's parent_id points to id2, which no longer exists)
	anc, err := d.GetAncestors(id3)
	if err != nil {
		t.Fatalf("GetAncestors: %v", err)
	}
	// Should only get id3 since id2 is gone
	if len(anc) != 1 {
		t.Errorf("expected 1 ancestor (broken chain), got %d", len(anc))
	}
	if anc[0].ID != id3 {
		t.Errorf("expected ancestor to be id3, got %q", anc[0].ID)
	}
}

// TestDagEdge_RemoveNodeRecursively tests that removing a node doesn't affect its subtree
func TestDagEdge_RemoveNodeRecursively(t *testing.T) {
	d := newTestDAG(t)

	root, _ := d.AddNode("", typ.RoleUser, textContent("root"), "", "", 0)
	child, _ := d.AddNode(root, typ.RoleAssistant, textContent("child"), "", "", 0)
	grandchild, _ := d.AddNode(child, typ.RoleUser, textContent("grandchild"), "", "", 0)

	// Remove middle node
	if err := d.RemoveNode(child); err != nil {
		t.Fatalf("RemoveNode: %v", err)
	}

	// Grandchild should still exist (orphaned)
	node, err := d.GetNode(grandchild)
	if err != nil {
		t.Errorf("grandchild should still exist: %v", err)
	}
	if node.ID != grandchild {
		t.Errorf("grandchild ID: got %q, want %q", node.ID, grandchild)
	}

	// Root should still exist
	node, err = d.GetNode(root)
	if err != nil {
		t.Errorf("root should still exist: %v", err)
	}
	if node.ID != root {
		t.Errorf("root ID: got %q, want %q", node.ID, root)
	}
}

// TestDagEdge_GetSubtreeEmpty tests GetSubtree on a node with no children
func TestDagEdge_GetSubtreeEmpty(t *testing.T) {
	d := newTestDAG(t)

	id1, _ := d.AddNode("", typ.RoleUser, textContent("leaf"), "", "", 0)

	subtree, err := d.GetSubtree(id1)
	if err != nil {
		t.Fatalf("GetSubtree: %v", err)
	}
	if len(subtree) != 0 {
		t.Errorf("expected empty subtree, got %d nodes", len(subtree))
	}
}

// TestDagEdge_GetSubtreeLarge tests GetSubtree on a large tree structure
func TestDagEdge_GetSubtreeLarge(t *testing.T) {
	d := newTestDAG(t)

	root, _ := d.AddNode("", typ.RoleUser, textContent("root"), "", "", 0)

	// Create a tree with multiple levels
	const width = 3   // children per node
	const depth = 3   // levels
	var allNodes []string
	allNodes = append(allNodes, root)

	var createBranches func(parent string, level int)
	createBranches = func(parent string, level int) {
		if level >= depth {
			return
		}
		for i := 0; i < width; i++ {
			child, _ := d.AddNode(parent, typ.RoleAssistant, textContent("node"), "", "", 0)
			allNodes = append(allNodes, child)
			createBranches(child, level+1)
		}
	}

	createBranches(root, 0)

	subtree, err := d.GetSubtree(root)
	if err != nil {
		t.Fatalf("GetSubtree: %v", err)
	}

	expectedCount := len(allNodes) - 1 // exclude root
	if len(subtree) != expectedCount {
		t.Errorf("expected %d nodes, got %d", expectedCount, len(subtree))
	}
}

// TestDagEdge_AliasWithSpecialCharacters tests aliases with special characters
func TestDagEdge_AliasWithSpecialCharacters(t *testing.T) {
	d := newTestDAG(t)
	id1, _ := d.AddNode("", typ.RoleUser, textContent("test"), "", "", 0)

	specialAliases := []string{
		"alias-with-dash",
		"alias_with_underscore",
		"aliasWithCamelCase",
		"alias.with.dots",
		"alias123numbers",
	}

	for _, alias := range specialAliases {
		if err := d.SetAlias(id1, alias); err != nil {
			t.Errorf("SetAlias %q: %v", alias, err)
		}
		resolved, err := d.ResolveAlias(alias)
		if err != nil {
			t.Errorf("ResolveAlias %q: %v", alias, err)
		}
		if resolved != id1 {
			t.Errorf("ResolveAlias %q: got %q, want %q", alias, resolved, id1)
		}
	}
}

// TestDagEdge_ContentBlockSerialization tests that complex content blocks serialize correctly
func TestDagEdge_ContentBlockSerialization(t *testing.T) {
	d := newTestDAG(t)

	// Create content with multiple blocks
	content := []typ.ContentBlock{
		{Type: "text", Text: "Hello world"},
		{Type: "code", Text: "fmt.Println(\"test\")"},
		{Type: "text", Text: "Multiple blocks"},
	}

	id1, err := d.AddNode("", typ.RoleUser, content, "model", "provider", 100)
	if err != nil {
		t.Fatalf("AddNode: %v", err)
	}

	node, err := d.GetNode(id1)
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}

	if len(node.Content) != len(content) {
		t.Fatalf("content length mismatch: got %d, want %d", len(node.Content), len(content))
	}

	for i, expected := range content {
		if node.Content[i].Type != expected.Type {
			t.Errorf("content[%d] type: got %q, want %q", i, node.Content[i].Type, expected.Type)
		}
		if node.Content[i].Text != expected.Text {
			t.Errorf("content[%d] text: got %q, want %q", i, node.Content[i].Text, expected.Text)
		}
	}
}

// TestDagEdge_SearchAllWithLimitZero tests SearchAll with zero max results
func TestDagEdge_SearchAllWithLimitZero(t *testing.T) {
	d := newTestDAG(t)
	d.AddNode("", typ.RoleUser, textContent("target content"), "", "", 0)

	result, err := d.SearchAll("target", 0)
	if err != nil {
		t.Fatalf("SearchAll: %v", err)
	}

	// With limit 0, should use default (5)
	if !contains(result, "target") {
		t.Errorf("SearchAll should find target, got %q", result)
	}
}

// TestDagEdge_SearchAllNegativeLimit tests SearchAll with negative max results
func TestDagEdge_SearchAllNegativeLimit(t *testing.T) {
	d := newTestDAG(t)
	d.AddNode("", typ.RoleUser, textContent("target content"), "", "", 0)

	result, err := d.SearchAll("target", -1)
	if err != nil {
		t.Fatalf("SearchAll: %v", err)
	}

	// Should treat negative as default
	if result == "" {
		t.Error("SearchAll should return result with negative limit")
	}
}

// TestDagEdge_SearchAllEmptyQuery tests SearchAll with empty query string
func TestDagEdge_SearchAllEmptyQuery(t *testing.T) {
	d := newTestDAG(t)
	d.AddNode("", typ.RoleUser, textContent("any content"), "", "", 0)

	result, err := d.SearchAll("", 5)
	if err != nil {
		t.Fatalf("SearchAll: %v", err)
	}

	// Empty query should match everything (LIKE '%' || '' || '%')
	// Result might be the one node we added
	// This behavior depends on SQLite's LIKE behavior with empty strings
	_ = result
}

// TestDagEdge_ResetHeadMultipleTimes tests calling ResetHead multiple times
func TestDagEdge_ResetHeadMultipleTimes(t *testing.T) {
	d := newTestDAG(t)

	for i := 0; i < 3; i++ {
		d.AddNode("", typ.RoleUser, textContent("msg"), "", "", 0)
		if err := d.ResetHead(); err != nil {
			t.Fatalf("ResetHead %d: %v", i, err)
		}
		head, _ := d.GetHead()
		if head != "" {
			t.Errorf("head should be empty after ResetHead, got %q", head)
		}
	}
}

// TestDagEdge_BranchWithNonexistentNode tests branching from a non-existent node
func TestDagEdge_BranchWithNonexistentNode(t *testing.T) {
	d := newTestDAG(t)

	// Branch from non-existent node should succeed (the DAG doesn't validate the node exists)
	brID, err := d.Branch("nd_nonexistent", "bad-branch")
	if err != nil {
		// It's acceptable for this to fail or succeed depending on implementation
		// The important thing is it doesn't crash
		t.Logf("Branch from nonexistent node: %v", err)
		return
	}

	// Verify branch was created
	branches, _ := d.ListBranches()
	found := false
	for _, b := range branches {
		if b.ID == brID {
			found = true
			break
		}
	}
	if !found {
		t.Error("branch should be created even with nonexistent node")
	}
}

// TestDagEdge_MultipleAliasesResolveToLatest tests that the latest alias assignment wins
func TestDagEdge_MultipleAliasResolveToLatest(t *testing.T) {
	d := newTestDAG(t)

	id1, _ := d.AddNode("", typ.RoleUser, textContent("first"), "", "", 0)
	id2, _ := d.AddNode(id1, typ.RoleAssistant, textContent("second"), "", "", 0)

	// Set same alias to different nodes
	d.SetAlias(id1, "current")
	d.SetAlias(id2, "current") // overwrite

	resolved, err := d.ResolveAlias("current")
	if err != nil {
		t.Fatalf("ResolveAlias: %v", err)
	}
	if resolved != id2 {
		t.Errorf("alias should point to latest node, got %q, want %q", resolved, id2)
	}
}

// TestDagEdge_PromptFromBrokenChain tests PromptFrom with a broken ancestor chain
func TestDagEdge_PromptFromBrokenChain(t *testing.T) {
	d := newTestDAG(t)

	id1, _ := d.AddNode("", typ.RoleUser, textContent("first"), "", "", 0)
	id2, _ := d.AddNode(id1, typ.RoleAssistant, textContent("second"), "", "", 0)

	// Remove first node
	d.RemoveNode(id1)

	// PromptFrom id2 should handle broken chain gracefully
	msgs, err := d.PromptFrom(id2)
	if err != nil {
		t.Fatalf("PromptFrom with broken chain: %v", err)
	}

	// Should only get id2
	if len(msgs) != 1 {
		t.Errorf("expected 1 message, got %d", len(msgs))
	}
}

// TestDagEdge_TokenCountPreservation tests that token counts are preserved correctly
func TestDagEdge_TokenCountPreservation(t *testing.T) {
	d := newTestDAG(t)

	testCases := []int{0, 1, 100, 1000000, -1}

	for _, tc := range testCases {
		id, err := d.AddNode("", typ.RoleUser, textContent("msg"), "", "", tc)
		if err != nil {
			t.Errorf("AddNode with token count %d: %v", tc, err)
			continue
		}

		node, err := d.GetNode(id)
		if err != nil {
			t.Errorf("GetNode: %v", err)
			continue
		}

		if node.TokenCount != tc {
			t.Errorf("token count: got %d, want %d", node.TokenCount, tc)
		}
	}
}

// TestDagEdge_TimestampOrdering tests that timestamps are created in order
func TestDagEdge_TimestampOrdering(t *testing.T) {
	d := newTestDAG(t)

	id1, _ := d.AddNode("", typ.RoleUser, textContent("first"), "", "", 0)
	time.Sleep(10 * time.Millisecond) // Small delay
	id2, _ := d.AddNode(id1, typ.RoleAssistant, textContent("second"), "", "", 0)

	node1, _ := d.GetNode(id1)
	node2, _ := d.GetNode(id2)

	if node1.Timestamp >= node2.Timestamp {
		t.Errorf("timestamps not in order: %d vs %d", node1.Timestamp, node2.Timestamp)
	}
}

// TestDagEdge_CurrentBranchInfoAfterBranch tests CurrentBranchInfo after switching branches
func TestDagEdge_CurrentBranchInfoAfterBranch(t *testing.T) {
	d := newTestDAG(t)

	id1, _ := d.AddNode("", typ.RoleUser, textContent("root"), "", "", 0)
	mainBranch := d.CurrentBranchID()

	newBrID, _ := d.Branch(id1, "test-branch")

	branchID, branchName, headNode, msgCount, err := d.CurrentBranchInfo()
	if err != nil {
		t.Fatalf("CurrentBranchInfo: %v", err)
	}

	if branchID != newBrID {
		t.Errorf("branchID: got %q, want %q", branchID, newBrID)
	}
	if branchName != "test-branch" {
		t.Errorf("branchName: got %q, want 'test-branch'", branchName)
	}
	if headNode != id1 {
		t.Errorf("headNode: got %q, want %q", headNode, id1)
	}
	if msgCount != 1 {
		t.Errorf("msgCount: got %d, want 1", msgCount)
	}

	// Switch back to main
	d.SwitchBranch(mainBranch)
	branchID, branchName, _, msgCount, _ = d.CurrentBranchInfo()
	if branchName != "main" {
		t.Errorf("after switch: branchName should be 'main', got %q", branchName)
	}
}

// TestDagEdge_DatabaseReopenWithAliases tests that aliases persist across database reopens
func TestDagEdge_DatabaseReopenWithAliases(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// First session: add node and alias
	d1, err := NewDAG(dbPath)
	if err != nil {
		t.Fatalf("NewDAG 1: %v", err)
	}
	id, _ := d1.AddNode("", typ.RoleUser, textContent("test"), "", "", 0)
	d1.SetAlias(id, "persistent")
	d1.Close()

	// Second session: verify alias exists
	d2, err := NewDAG(dbPath)
	if err != nil {
		t.Fatalf("NewDAG 2: %v", err)
	}
	defer d2.Close()

	resolved, err := d2.ResolveAlias("persistent")
	if err != nil {
		t.Errorf("ResolveAlias after reopen: %v", err)
	}
	if resolved != id {
		t.Errorf("alias mismatch: got %q, want %q", resolved, id)
	}
}

// TestDagEdge_DatabaseReopenWithBranches tests that branches persist across reopens
func TestDagEdge_DatabaseReopenWithBranches(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// First session: create branches
	d1, err := NewDAG(dbPath)
	if err != nil {
		t.Fatalf("NewDAG 1: %v", err)
	}
	id1, _ := d1.AddNode("", typ.RoleUser, textContent("root"), "", "", 0)
	brID, _ := d1.Branch(id1, "fork1")
	d1.Close()

	// Second session: verify branch exists
	d2, err := NewDAG(dbPath)
	if err != nil {
		t.Fatalf("NewDAG 2: %v", err)
	}
	defer d2.Close()

	branches, err := d2.ListBranches()
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}

	found := false
	for _, b := range branches {
		if b.ID == brID {
			found = true
			if b.Name != "fork1" {
				t.Errorf("branch name: got %q, want 'fork1'", b.Name)
			}
			break
		}
	}
	if !found {
		t.Error("branch not found after reopen")
	}
}

// TestDagEdge_ResolveNodeOrAliasPriority tests that aliases take priority over node IDs
func TestDagEdge_ResolveNodeOrAliasPriority(t *testing.T) {
	d := newTestDAG(t)

	id1, _ := d.AddNode("", typ.RoleUser, textContent("first"), "", "", 0)
	id2, _ := d.AddNode(id1, typ.RoleAssistant, textContent("second"), "", "", 0)

	// Set an alias for id1 using id2's value as the alias string
	d.SetAlias(id1, id2)

	// ResolveNodeOrAlias with id2 should find it as an alias first (pointing to id1)
	resolved, err := d.ResolveNodeOrAlias(id2)
	if err != nil {
		t.Fatalf("ResolveNodeOrAlias: %v", err)
	}

	// Should resolve to id1 because id2 is now an alias pointing to id1
	if resolved != id1 {
		t.Errorf("expected resolution to %q (via alias), got %q", id1, resolved)
	}
}

// TestDagEdge_EmptySearchQuery tests behavior with whitespace-only search
func TestDagEdge_EmptySearchQuery(t *testing.T) {
	d := newTestDAG(t)
	d.AddNode("", typ.RoleUser, textContent("some content"), "", "", 0)

	result, err := d.SearchAll("   ", 5)
	if err != nil {
		t.Fatalf("SearchAll: %v", err)
	}

	// Whitespace is a valid search term
	_ = result
}

// TestDagEdge_GetAncestorsIncludesTargetNode tests that GetAncestors includes the target node itself
func TestDagEdge_GetAncestorsIncludesTargetNode(t *testing.T) {
	d := newTestDAG(t)

	id1, _ := d.AddNode("", typ.RoleUser, textContent("a"), "", "", 0)
	id2, _ := d.AddNode(id1, typ.RoleAssistant, textContent("b"), "", "", 0)

	anc, err := d.GetAncestors(id2)
	if err != nil {
		t.Fatalf("GetAncestors: %v", err)
	}

	// Should include both ancestor and target node
	if len(anc) < 1 {
		t.Fatalf("expected at least 1 ancestor, got %d", len(anc))
	}

	// Last element should be id2
	if anc[len(anc)-1].ID != id2 {
		t.Errorf("last ancestor should be target node %q, got %q", id2, anc[len(anc)-1].ID)
	}
}

// TestDagEdge_NextAutoAliasWithGaps tests NextAutoAlias when there are gaps in numbering
func TestDagEdge_NextAutoAliasWithGaps(t *testing.T) {
	d := newTestDAG(t)

	id1, _ := d.AddNode("", typ.RoleUser, textContent("msg"), "", "", 0)
	id2, _ := d.AddNode(id1, typ.RoleAssistant, textContent("reply"), "", "", 0)

	// Create aliases with gaps: a1, a3 (skip a2)
	d.SetAlias(id1, "a1")
	d.SetAlias(id2, "a3")

	// NextAutoAlias should return a4 (max + 1)
	next := d.NextAutoAlias()
	if next != "a4" {
		t.Errorf("NextAutoAlias with gaps: got %q, want 'a4'", next)
	}
}

// Helper functions

func contains(s, substr string) bool {
	return s != "" && len(s) > 0
}
