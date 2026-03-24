package core

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	typ "torus_go_agent/internal/types"
)

func textContent(s string) []typ.ContentBlock {
	return []typ.ContentBlock{{Type: "text", Text: s}}
}

// ---- 1. NewDAG ----

func TestNewDAG_CreatesMainBranch(tt *testing.T) {
	d := newTestDAG(tt)

	// Must have exactly one branch named "main".
	branches, err := d.ListBranches()
	if err != nil {
		tt.Fatalf("ListBranches: %v", err)
	}
	if len(branches) != 1 {
		tt.Fatalf("expected 1 branch, got %d", len(branches))
	}
	if branches[0].Name != "main" {
		tt.Errorf("expected branch name 'main', got %q", branches[0].Name)
	}
	if !strings.HasPrefix(branches[0].ID, "br_") {
		tt.Errorf("branch ID should start with 'br_', got %q", branches[0].ID)
	}
	if d.CurrentBranchID() != branches[0].ID {
		tt.Errorf("CurrentBranchID mismatch: %q vs %q", d.CurrentBranchID(), branches[0].ID)
	}
}

func TestNewDAG_ReopenPreservesData(tt *testing.T) {
	dir := tt.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// First open: add a node.
	d1, err := NewDAG(dbPath)
	if err != nil {
		tt.Fatalf("NewDAG 1: %v", err)
	}
	nodeID, err := d1.AddNode("", typ.RoleUser, textContent("hello"), "", "", 5)
	if err != nil {
		tt.Fatalf("AddNode: %v", err)
	}
	d1.Close()

	// Second open: data survives.
	d2, err := NewDAG(dbPath)
	if err != nil {
		tt.Fatalf("NewDAG 2: %v", err)
	}
	defer d2.Close()

	n, err := d2.GetNode(nodeID)
	if err != nil {
		tt.Fatalf("GetNode after reopen: %v", err)
	}
	if n.ID != nodeID {
		tt.Errorf("node ID mismatch: %q vs %q", n.ID, nodeID)
	}
}

// ---- 2. AddNode ----

func TestAddNode_InsertsAndUpdatesHead(tt *testing.T) {
	d := newTestDAG(tt)

	id1, err := d.AddNode("", typ.RoleUser, textContent("msg1"), "model-a", "prov-a", 10)
	if err != nil {
		tt.Fatalf("AddNode 1: %v", err)
	}
	if !strings.HasPrefix(id1, "nd_") {
		tt.Errorf("node ID should start with 'nd_', got %q", id1)
	}

	// Head should be id1.
	head, err := d.GetHead()
	if err != nil {
		tt.Fatalf("GetHead: %v", err)
	}
	if head != id1 {
		tt.Errorf("expected head %q, got %q", id1, head)
	}

	// Add a second node as child of first.
	id2, err := d.AddNode(id1, typ.RoleAssistant, textContent("reply"), "model-b", "prov-b", 20)
	if err != nil {
		tt.Fatalf("AddNode 2: %v", err)
	}
	head, _ = d.GetHead()
	if head != id2 {
		tt.Errorf("expected head %q after second add, got %q", id2, head)
	}

	// Verify stored fields.
	n, err := d.GetNode(id2)
	if err != nil {
		tt.Fatalf("GetNode: %v", err)
	}
	if n.Role != string(typ.RoleAssistant) {
		tt.Errorf("role: got %q, want %q", n.Role, typ.RoleAssistant)
	}
	if n.Model != "model-b" {
		tt.Errorf("model: got %q, want %q", n.Model, "model-b")
	}
	if n.Provider != "prov-b" {
		tt.Errorf("provider: got %q, want %q", n.Provider, "prov-b")
	}
	if n.TokenCount != 20 {
		tt.Errorf("token_count: got %d, want 20", n.TokenCount)
	}
	if n.ParentID != id1 {
		tt.Errorf("parent_id: got %q, want %q", n.ParentID, id1)
	}
	if n.Timestamp == 0 {
		tt.Error("timestamp should be non-zero")
	}
	if len(n.Content) != 1 || n.Content[0].Text != "reply" {
		tt.Errorf("content mismatch: got %+v", n.Content)
	}
}

// ---- 3. RemoveNode ----

func TestRemoveNode_DeletesAndRewindsHead(tt *testing.T) {
	d := newTestDAG(tt)

	id1, _ := d.AddNode("", typ.RoleUser, textContent("first"), "", "", 0)
	id2, _ := d.AddNode(id1, typ.RoleAssistant, textContent("second"), "", "", 0)

	// Remove the second node; head should rewind to id1.
	if err := d.RemoveNode(id2); err != nil {
		tt.Fatalf("RemoveNode: %v", err)
	}
	head, _ := d.GetHead()
	if head != id1 {
		tt.Errorf("head after remove: got %q, want %q", head, id1)
	}

	// The removed node should be gone.
	_, err := d.GetNode(id2)
	if err != sql.ErrNoRows {
		tt.Errorf("expected ErrNoRows for removed node, got %v", err)
	}
}

func TestRemoveNode_RootRewindsToEmpty(tt *testing.T) {
	d := newTestDAG(tt)

	id1, _ := d.AddNode("", typ.RoleUser, textContent("only"), "", "", 0)
	if err := d.RemoveNode(id1); err != nil {
		tt.Fatalf("RemoveNode root: %v", err)
	}
	head, _ := d.GetHead()
	if head != "" {
		tt.Errorf("head after removing root: got %q, want empty", head)
	}
}

func TestRemoveNode_NotFound(tt *testing.T) {
	d := newTestDAG(tt)
	err := d.RemoveNode("nd_nonexistent")
	if err == nil {
		tt.Error("expected error removing non-existent node")
	}
}

// ---- 4. GetNode ----

func TestGetNode_NotFound(tt *testing.T) {
	d := newTestDAG(tt)
	_, err := d.GetNode("nd_doesnotexist")
	if err != sql.ErrNoRows {
		tt.Errorf("expected sql.ErrNoRows, got %v", err)
	}
}

// ---- 5. GetHead ----

func TestGetHead_EmptyOnFreshBranch(tt *testing.T) {
	d := newTestDAG(tt)
	head, err := d.GetHead()
	if err != nil {
		tt.Fatalf("GetHead: %v", err)
	}
	if head != "" {
		tt.Errorf("expected empty head on fresh branch, got %q", head)
	}
}

// ---- 6. GetAncestors ----

func TestGetAncestors_Chain(tt *testing.T) {
	d := newTestDAG(tt)

	id1, _ := d.AddNode("", typ.RoleUser, textContent("a"), "", "", 0)
	id2, _ := d.AddNode(id1, typ.RoleAssistant, textContent("b"), "", "", 0)
	id3, _ := d.AddNode(id2, typ.RoleUser, textContent("c"), "", "", 0)

	anc, err := d.GetAncestors(id3)
	if err != nil {
		tt.Fatalf("GetAncestors: %v", err)
	}
	if len(anc) != 3 {
		tt.Fatalf("expected 3 ancestors, got %d", len(anc))
	}
	// Should be in root-first order (reversed).
	if anc[0].ID != id1 || anc[1].ID != id2 || anc[2].ID != id3 {
		tt.Errorf("ancestor order: %s, %s, %s", anc[0].ID, anc[1].ID, anc[2].ID)
	}
}

func TestGetAncestors_SingleNode(tt *testing.T) {
	d := newTestDAG(tt)
	id1, _ := d.AddNode("", typ.RoleUser, textContent("solo"), "", "", 0)

	anc, err := d.GetAncestors(id1)
	if err != nil {
		tt.Fatalf("GetAncestors: %v", err)
	}
	if len(anc) != 1 || anc[0].ID != id1 {
		tt.Errorf("expected single ancestor %q, got %+v", id1, anc)
	}
}

func TestGetAncestors_NonExistentReturnsEmpty(tt *testing.T) {
	d := newTestDAG(tt)
	anc, err := d.GetAncestors("nd_ghost")
	if err != nil {
		tt.Fatalf("GetAncestors: %v", err)
	}
	if len(anc) != 0 {
		tt.Errorf("expected empty ancestors for missing node, got %d", len(anc))
	}
}

// ---- 7. Alias CRUD ----

func TestSetAlias_AndResolve(tt *testing.T) {
	d := newTestDAG(tt)
	id1, _ := d.AddNode("", typ.RoleUser, textContent("msg"), "", "", 0)

	if err := d.SetAlias(id1, "start"); err != nil {
		tt.Fatalf("SetAlias: %v", err)
	}
	resolved, err := d.ResolveAlias("start")
	if err != nil {
		tt.Fatalf("ResolveAlias: %v", err)
	}
	if resolved != id1 {
		tt.Errorf("ResolveAlias: got %q, want %q", resolved, id1)
	}
}

func TestSetAlias_Overwrite(tt *testing.T) {
	d := newTestDAG(tt)
	id1, _ := d.AddNode("", typ.RoleUser, textContent("a"), "", "", 0)
	id2, _ := d.AddNode(id1, typ.RoleAssistant, textContent("b"), "", "", 0)

	d.SetAlias(id1, "tag")
	d.SetAlias(id2, "tag") // overwrite

	resolved, err := d.ResolveAlias("tag")
	if err != nil {
		tt.Fatalf("ResolveAlias after overwrite: %v", err)
	}
	if resolved != id2 {
		tt.Errorf("expected alias to point to %q, got %q", id2, resolved)
	}
}

func TestDeleteAlias(tt *testing.T) {
	d := newTestDAG(tt)
	id1, _ := d.AddNode("", typ.RoleUser, textContent("msg"), "", "", 0)

	d.SetAlias(id1, "tmp")
	if err := d.DeleteAlias("tmp"); err != nil {
		tt.Fatalf("DeleteAlias: %v", err)
	}
	_, err := d.ResolveAlias("tmp")
	if err != sql.ErrNoRows {
		tt.Errorf("expected ErrNoRows after delete, got %v", err)
	}
}

func TestGetAliases(tt *testing.T) {
	d := newTestDAG(tt)
	id1, _ := d.AddNode("", typ.RoleUser, textContent("msg"), "", "", 0)

	d.SetAlias(id1, "alpha")
	d.SetAlias(id1, "beta")

	aliases, err := d.GetAliases(id1)
	if err != nil {
		tt.Fatalf("GetAliases: %v", err)
	}
	if len(aliases) != 2 {
		tt.Fatalf("expected 2 aliases, got %d", len(aliases))
	}
	// Check both are present (order may vary).
	found := map[string]bool{}
	for _, a := range aliases {
		found[a] = true
	}
	if !found["alpha"] || !found["beta"] {
		tt.Errorf("expected aliases alpha and beta, got %v", aliases)
	}
}

func TestGetAliases_Empty(tt *testing.T) {
	d := newTestDAG(tt)
	id1, _ := d.AddNode("", typ.RoleUser, textContent("msg"), "", "", 0)

	aliases, err := d.GetAliases(id1)
	if err != nil {
		tt.Fatalf("GetAliases: %v", err)
	}
	if len(aliases) != 0 {
		tt.Errorf("expected 0 aliases, got %d", len(aliases))
	}
}

func TestResolveAlias_NotFound(tt *testing.T) {
	d := newTestDAG(tt)
	_, err := d.ResolveAlias("nope")
	if err != sql.ErrNoRows {
		tt.Errorf("expected sql.ErrNoRows, got %v", err)
	}
}

// ---- 8. NextAutoAlias ----

func TestNextAutoAlias_Incrementing(tt *testing.T) {
	d := newTestDAG(tt)

	// No aliases yet: should start at a1.
	if got := d.NextAutoAlias(); got != "a1" {
		tt.Errorf("first NextAutoAlias: got %q, want 'a1'", got)
	}

	// Add a1 manually, next should be a2.
	id1, _ := d.AddNode("", typ.RoleUser, textContent("msg"), "", "", 0)
	d.SetAlias(id1, "a1")
	if got := d.NextAutoAlias(); got != "a2" {
		tt.Errorf("after a1: got %q, want 'a2'", got)
	}

	// Add a2, next should be a3.
	id2, _ := d.AddNode(id1, typ.RoleAssistant, textContent("reply"), "", "", 0)
	d.SetAlias(id2, "a2")
	if got := d.NextAutoAlias(); got != "a3" {
		tt.Errorf("after a2: got %q, want 'a3'", got)
	}
}

func TestNextAutoAlias_IgnoresNonAutoAliases(tt *testing.T) {
	d := newTestDAG(tt)
	id1, _ := d.AddNode("", typ.RoleUser, textContent("msg"), "", "", 0)

	// Non-auto aliases should not affect the counter.
	d.SetAlias(id1, "start")
	d.SetAlias(id1, "bookmark")

	if got := d.NextAutoAlias(); got != "a1" {
		tt.Errorf("NextAutoAlias with only non-auto aliases: got %q, want 'a1'", got)
	}
}

// ---- 9. Branch / NewBranch / SwitchBranch / ListBranches ----

func TestBranch_ForksFromNode(tt *testing.T) {
	d := newTestDAG(tt)

	id1, _ := d.AddNode("", typ.RoleUser, textContent("root"), "", "", 0)
	id2, _ := d.AddNode(id1, typ.RoleAssistant, textContent("reply"), "", "", 0)

	brID, err := d.Branch(id2, "experiment")
	if err != nil {
		tt.Fatalf("Branch: %v", err)
	}
	if !strings.HasPrefix(brID, "br_") {
		tt.Errorf("branch ID should start with 'br_', got %q", brID)
	}

	// Active branch should now be the new one.
	if d.CurrentBranchID() != brID {
		tt.Errorf("expected active branch %q, got %q", brID, d.CurrentBranchID())
	}

	// Head of new branch should be the fork point.
	head, _ := d.GetHead()
	if head != id2 {
		tt.Errorf("branch head: got %q, want %q", head, id2)
	}

	// ListBranches should show 2 branches.
	branches, _ := d.ListBranches()
	if len(branches) != 2 {
		tt.Fatalf("expected 2 branches, got %d", len(branches))
	}

	// The new branch should record forked_from.
	var forkedBranch *BranchInfo
	for i := range branches {
		if branches[i].ID == brID {
			forkedBranch = &branches[i]
			break
		}
	}
	if forkedBranch == nil {
		tt.Fatal("new branch not found in list")
	}
	if forkedBranch.ForkedFrom != id2 {
		tt.Errorf("forked_from: got %q, want %q", forkedBranch.ForkedFrom, id2)
	}
}

func TestNewBranch_EmptyHead(tt *testing.T) {
	d := newTestDAG(tt)

	// Add a node so main has a head.
	id1, _ := d.AddNode("", typ.RoleUser, textContent("root"), "", "", 0)

	brID, err := d.NewBranch("fresh")
	if err != nil {
		tt.Fatalf("NewBranch: %v", err)
	}
	if d.CurrentBranchID() != brID {
		tt.Errorf("active branch mismatch")
	}

	// Head should be empty.
	head, _ := d.GetHead()
	if head != "" {
		tt.Errorf("new branch head should be empty, got %q", head)
	}

	// forked_from should be the old head.
	branches, _ := d.ListBranches()
	for _, b := range branches {
		if b.ID == brID {
			if b.ForkedFrom != id1 {
				tt.Errorf("forked_from: got %q, want %q", b.ForkedFrom, id1)
			}
			break
		}
	}
}

func TestSwitchBranch(tt *testing.T) {
	d := newTestDAG(tt)

	mainBranch := d.CurrentBranchID()
	d.AddNode("", typ.RoleUser, textContent("root"), "", "", 0)

	brID, _ := d.NewBranch("other")
	if d.CurrentBranchID() != brID {
		tt.Fatal("should be on new branch")
	}

	// Switch back to main.
	if err := d.SwitchBranch(mainBranch); err != nil {
		tt.Fatalf("SwitchBranch: %v", err)
	}
	if d.CurrentBranchID() != mainBranch {
		tt.Errorf("expected branch %q, got %q", mainBranch, d.CurrentBranchID())
	}
}

func TestSwitchBranch_NotFound(tt *testing.T) {
	d := newTestDAG(tt)
	err := d.SwitchBranch("br_nonexistent")
	if err == nil {
		tt.Error("expected error switching to non-existent branch")
	}
}

func TestListBranches_MultiplePresent(tt *testing.T) {
	d := newTestDAG(tt)

	d.NewBranch("b1")
	d.NewBranch("b2")

	branches, err := d.ListBranches()
	if err != nil {
		tt.Fatalf("ListBranches: %v", err)
	}
	if len(branches) != 3 { // main + b1 + b2
		tt.Errorf("expected 3 branches, got %d", len(branches))
	}
	names := map[string]bool{}
	for _, b := range branches {
		names[b.Name] = true
	}
	for _, want := range []string{"main", "b1", "b2"} {
		if !names[want] {
			tt.Errorf("missing branch %q", want)
		}
	}
}

// ---- 10. CurrentBranchInfo ----

func TestCurrentBranchInfo_Basic(tt *testing.T) {
	d := newTestDAG(tt)

	id1, _ := d.AddNode("", typ.RoleUser, textContent("first"), "", "", 0)
	id2, _ := d.AddNode(id1, typ.RoleAssistant, textContent("second"), "", "", 0)
	d.AddNode(id2, typ.RoleUser, textContent("third"), "", "", 0)

	branchID, branchName, headNode, msgCount, err := d.CurrentBranchInfo()
	if err != nil {
		tt.Fatalf("CurrentBranchInfo: %v", err)
	}
	if branchID != d.CurrentBranchID() {
		tt.Errorf("branchID mismatch")
	}
	if branchName != "main" {
		tt.Errorf("branchName: got %q, want 'main'", branchName)
	}
	if headNode == "" {
		tt.Error("headNode should not be empty")
	}
	if msgCount != 3 {
		tt.Errorf("msgCount: got %d, want 3", msgCount)
	}
}

func TestCurrentBranchInfo_EmptyBranch(tt *testing.T) {
	d := newTestDAG(tt)

	branchID, branchName, headNode, msgCount, err := d.CurrentBranchInfo()
	if err != nil {
		tt.Fatalf("CurrentBranchInfo: %v", err)
	}
	if branchID == "" || branchName != "main" {
		tt.Errorf("unexpected branch info: id=%q name=%q", branchID, branchName)
	}
	if headNode != "" {
		tt.Errorf("headNode should be empty on fresh branch, got %q", headNode)
	}
	if msgCount != 0 {
		tt.Errorf("msgCount should be 0 on fresh branch, got %d", msgCount)
	}
}

func TestCurrentBranchInfo_BadBranch(tt *testing.T) {
	d := newTestDAG(tt)
	// Force a bad branch ID.
	d.branchID = "br_bogus"
	_, _, _, _, err := d.CurrentBranchInfo()
	if err == nil {
		tt.Error("expected error for non-existent branch")
	}
}

// ---- 11. forked_from migration ----

func TestForkedFromMigration_ExistingDB(tt *testing.T) {
	// Simulate a DB that was created without the forked_from column:
	// create schema without forked_from, then open with NewDAG which should migrate.
	dir := tt.TempDir()
	dbPath := filepath.Join(dir, "legacy.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		tt.Fatalf("open: %v", err)
	}
	// Create schema without forked_from column.
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS nodes (
			id TEXT PRIMARY KEY,
			parent_id TEXT,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			model TEXT DEFAULT '',
			provider TEXT DEFAULT '',
			timestamp INTEGER NOT NULL,
			token_count INTEGER DEFAULT 0
		);
		CREATE INDEX IF NOT EXISTS idx_nodes_parent ON nodes(parent_id);
		CREATE TABLE IF NOT EXISTS branches (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			head_node_id TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS node_aliases (
			alias TEXT PRIMARY KEY,
			node_id TEXT NOT NULL
		);
	`)
	if err != nil {
		tt.Fatalf("legacy schema: %v", err)
	}
	// Insert a branch without forked_from.
	_, err = db.Exec("INSERT INTO branches (id, name, head_node_id) VALUES ('br_legacy', 'main', '')")
	if err != nil {
		tt.Fatalf("insert legacy branch: %v", err)
	}
	db.Close()

	// Now open with NewDAG -- migration should add forked_from.
	d, err := NewDAG(dbPath)
	if err != nil {
		tt.Fatalf("NewDAG on legacy db: %v", err)
	}
	defer d.Close()

	// Verify the column exists by creating a branch with forked_from.
	brID, err := d.Branch("", "migrated-branch")
	if err != nil {
		tt.Fatalf("Branch after migration: %v", err)
	}

	branches, _ := d.ListBranches()
	for _, b := range branches {
		if b.ID == brID {
			// forked_from should be empty string (the fromNodeID we passed).
			if b.Name != "migrated-branch" {
				tt.Errorf("branch name: got %q, want 'migrated-branch'", b.Name)
			}
			return
		}
	}
	tt.Error("migrated branch not found in list")
}

// ---- Additional coverage ----

func TestResetHead(tt *testing.T) {
	d := newTestDAG(tt)
	d.AddNode("", typ.RoleUser, textContent("msg"), "", "", 0)

	if err := d.ResetHead(); err != nil {
		tt.Fatalf("ResetHead: %v", err)
	}
	head, _ := d.GetHead()
	if head != "" {
		tt.Errorf("head after reset: got %q, want empty", head)
	}
}

func TestPromptFrom(tt *testing.T) {
	d := newTestDAG(tt)

	id1, _ := d.AddNode("", typ.RoleUser, textContent("hello"), "", "", 0)
	id2, _ := d.AddNode(id1, typ.RoleAssistant, textContent("hi"), "", "", 0)

	msgs, err := d.PromptFrom(id2)
	if err != nil {
		tt.Fatalf("PromptFrom: %v", err)
	}
	if len(msgs) != 2 {
		tt.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != typ.RoleUser {
		tt.Errorf("msg[0] role: got %q, want 'user'", msgs[0].Role)
	}
	if msgs[1].Role != typ.RoleAssistant {
		tt.Errorf("msg[1] role: got %q, want 'assistant'", msgs[1].Role)
	}
}

func TestResolveNodeOrAlias_ByAlias(tt *testing.T) {
	d := newTestDAG(tt)
	id1, _ := d.AddNode("", typ.RoleUser, textContent("msg"), "", "", 0)
	d.SetAlias(id1, "mark")

	resolved, err := d.ResolveNodeOrAlias("mark")
	if err != nil {
		tt.Fatalf("ResolveNodeOrAlias by alias: %v", err)
	}
	if resolved != id1 {
		tt.Errorf("got %q, want %q", resolved, id1)
	}
}

func TestResolveNodeOrAlias_ByNodeID(tt *testing.T) {
	d := newTestDAG(tt)
	id1, _ := d.AddNode("", typ.RoleUser, textContent("msg"), "", "", 0)

	resolved, err := d.ResolveNodeOrAlias(id1)
	if err != nil {
		tt.Fatalf("ResolveNodeOrAlias by ID: %v", err)
	}
	if resolved != id1 {
		tt.Errorf("got %q, want %q", resolved, id1)
	}
}

func TestResolveNodeOrAlias_NotFound(tt *testing.T) {
	d := newTestDAG(tt)
	_, err := d.ResolveNodeOrAlias("unknown_thing")
	if err == nil {
		tt.Error("expected error for non-existent alias/node")
	}
}

func TestSearchAll_FindsMatches(tt *testing.T) {
	d := newTestDAG(tt)
	d.AddNode("", typ.RoleUser, textContent("the quick brown fox"), "", "", 0)
	d.AddNode("", typ.RoleUser, textContent("lazy dog"), "", "", 0)

	result, err := d.SearchAll("fox", 5)
	if err != nil {
		tt.Fatalf("SearchAll: %v", err)
	}
	if !strings.Contains(result, "fox") {
		tt.Errorf("SearchAll should contain 'fox', got %q", result)
	}
	if strings.Contains(result, "dog") {
		tt.Errorf("SearchAll should not match 'dog' when searching 'fox'")
	}
}

func TestSearchAll_NoResults(tt *testing.T) {
	d := newTestDAG(tt)
	d.AddNode("", typ.RoleUser, textContent("hello"), "", "", 0)

	result, err := d.SearchAll("nonexistent_term", 5)
	if err != nil {
		tt.Fatalf("SearchAll: %v", err)
	}
	if result != "" {
		tt.Errorf("expected empty result, got %q", result)
	}
}

func TestGetSubtree(tt *testing.T) {
	d := newTestDAG(tt)

	root, _ := d.AddNode("", typ.RoleUser, textContent("root"), "", "", 0)
	child1, _ := d.AddNode(root, typ.RoleAssistant, textContent("child1"), "", "", 0)
	child2, _ := d.AddNode(root, typ.RoleUser, textContent("child2"), "", "", 0)
	grandchild, _ := d.AddNode(child1, typ.RoleUser, textContent("grandchild"), "", "", 0)

	subtree, err := d.GetSubtree(root)
	if err != nil {
		tt.Fatalf("GetSubtree: %v", err)
	}
	if len(subtree) != 3 { // child1, child2, grandchild (not root itself)
		tt.Fatalf("expected 3 descendants, got %d", len(subtree))
	}

	ids := map[string]bool{}
	for _, n := range subtree {
		ids[n.ID] = true
	}
	if !ids[child1] || !ids[child2] || !ids[grandchild] {
		tt.Errorf("missing expected node IDs in subtree: %v", ids)
	}
	if ids[root] {
		tt.Error("subtree should not include the root node itself")
	}
}

// ---- Fork ----

func TestFork_IndependentBranch(tt *testing.T) {
	d := newTestDAG(tt)

	// Add a node on main branch
	mainNodeID, _ := d.AddNode("", "user", textContent("hello"), "", "", 0)

	// Create a sub-branch and fork
	subBranchID, err := d.Branch(mainNodeID, "sub")
	if err != nil {
		tt.Fatalf("Branch: %v", err)
	}

	// Fork shares DB but has its own branchID
	forked := d.Fork(subBranchID)

	// Parent DAG is now on sub branch (Branch switches it); switch back
	d.SwitchBranch(d.CurrentBranchID()) // stays on sub, that's fine

	// Forked DAG should be on sub branch
	if forked.CurrentBranchID() != subBranchID {
		tt.Errorf("forked branch = %s, want %s", forked.CurrentBranchID(), subBranchID)
	}

	// Append on forked DAG should not affect parent's branchID
	originalBranch := d.CurrentBranchID()
	forked.AddNode("", "assistant", textContent("world"), "", "", 0)

	if d.CurrentBranchID() != originalBranch {
		tt.Errorf("parent branchID changed: got %s, want %s", d.CurrentBranchID(), originalBranch)
	}

	// Both should see the same branches (shared DB)
	parentBranches, _ := d.ListBranches()
	forkedBranches, _ := forked.ListBranches()
	if len(parentBranches) != len(forkedBranches) {
		tt.Errorf("branch counts differ: parent=%d, forked=%d", len(parentBranches), len(forkedBranches))
	}
}
