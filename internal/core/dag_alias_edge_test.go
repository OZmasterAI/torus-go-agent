package core

import (
	"database/sql"
	"path/filepath"
	"testing"

	t "torus_go_agent/internal/types"
)

// ---- Alias Edge Cases ----

// TestNextAutoAlias_SkipsManuallySetAliases verifies that NextAutoAlias ignores
// manually-set aliases when determining the next auto-alias number.
// This prevents collisions between auto-aliases (a1, a2, ...) and manual aliases.
func TestNextAutoAlias_SkipsManuallySetAliases(tt *testing.T) {
	d := newTestDAG(tt)
	id1, _ := d.AddNode("", t.RoleUser, textContent("node1"), "", "", 0)
	id2, _ := d.AddNode(id1, t.RoleAssistant, textContent("node2"), "", "", 0)
	id3, _ := d.AddNode(id2, t.RoleUser, textContent("node3"), "", "", 0)

	// Set manual aliases
	d.SetAlias(id1, "checkpoint")
	d.SetAlias(id2, "important")
	d.SetAlias(id3, "final")

	// NextAutoAlias should not be affected by manual aliases and should return a1
	if got := d.NextAutoAlias(); got != "a1" {
		tt.Errorf("NextAutoAlias with manual aliases: got %q, want 'a1'", got)
	}

	// Now set an auto-alias a1
	d.SetAlias(id1, "a1")
	if got := d.NextAutoAlias(); got != "a2" {
		tt.Errorf("after setting a1: got %q, want 'a2'", got)
	}

	// Set a3 directly (skipping a2)
	d.SetAlias(id2, "a3")
	if got := d.NextAutoAlias(); got != "a4" {
		tt.Errorf("after setting a3 (skipping a2): got %q, want 'a4'", got)
	}
}

// TestAliasPersistsAcrossReopen verifies that aliases survive a database close
// and reopen cycle, critical for long-running agents that may restart.
func TestAliasPersistsAcrossReopen(tt *testing.T) {
	dir := tt.TempDir()
	dbPath := filepath.Join(dir, "alias_persist.db")

	// First session: create nodes and set aliases
	d1, err := NewDAG(dbPath)
	if err != nil {
		tt.Fatalf("NewDAG 1: %v", err)
	}
	id1, _ := d1.AddNode("", t.RoleUser, textContent("first"), "", "", 0)
	id2, _ := d1.AddNode(id1, t.RoleAssistant, textContent("second"), "", "", 0)

	// Set various alias types
	d1.SetAlias(id1, "start")
	d1.SetAlias(id1, "a1")       // auto-alias
	d1.SetAlias(id2, "end")
	d1.SetAlias(id2, "a2")       // auto-alias
	d1.SetAlias(id2, "a_backup") // prefixed with 'a' but not auto-format

	d1.Close()

	// Second session: reopen and verify aliases
	d2, err := NewDAG(dbPath)
	if err != nil {
		tt.Fatalf("NewDAG 2: %v", err)
	}
	defer d2.Close()

	// Verify all aliases still resolve
	resolved1, err := d2.ResolveAlias("start")
	if err != nil || resolved1 != id1 {
		tt.Errorf("start alias lost: err=%v, resolved=%q, want=%q", err, resolved1, id1)
	}

	resolved2, err := d2.ResolveAlias("a1")
	if err != nil || resolved2 != id1 {
		tt.Errorf("a1 alias lost: err=%v, resolved=%q, want=%q", err, resolved2, id1)
	}

	resolved3, err := d2.ResolveAlias("end")
	if err != nil || resolved3 != id2 {
		tt.Errorf("end alias lost: err=%v, resolved=%q, want=%q", err, resolved3, id2)
	}

	resolved4, err := d2.ResolveAlias("a2")
	if err != nil || resolved4 != id2 {
		tt.Errorf("a2 alias lost: err=%v, resolved=%q, want=%q", err, resolved4, id2)
	}

	resolved5, err := d2.ResolveAlias("a_backup")
	if err != nil || resolved5 != id2 {
		tt.Errorf("a_backup alias lost: err=%v, resolved=%q, want=%q", err, resolved5, id2)
	}

	// Verify NextAutoAlias still works after reopen
	if got := d2.NextAutoAlias(); got != "a3" {
		tt.Errorf("NextAutoAlias after reopen: got %q, want 'a3'", got)
	}
}

// TestResolveNodeOrAlias_PrefersAlias verifies that when both an alias
// and a node ID could potentially match (e.g., if someone names a node
// with an alias-like ID), the alias takes precedence in resolution.
func TestResolveNodeOrAlias_PrefersAlias(tt *testing.T) {
	d := newTestDAG(tt)
	id1, _ := d.AddNode("", t.RoleUser, textContent("node1"), "", "", 0)
	id2, _ := d.AddNode(id1, t.RoleAssistant, textContent("node2"), "", "", 0)

	// Set an alias that happens to match a node ID substring pattern
	d.SetAlias(id1, "lookup")

	// Create a new node
	id3, _ := d.AddNode(id2, t.RoleUser, textContent("node3"), "", "", 0)

	// ResolveNodeOrAlias with the alias should return id1, not any node ID
	resolved, err := d.ResolveNodeOrAlias("lookup")
	if err != nil {
		tt.Fatalf("ResolveNodeOrAlias: %v", err)
	}
	if resolved != id1 {
		tt.Errorf("expected alias resolution to id1=%q, got %q", id1, resolved)
	}

	// ResolveNodeOrAlias with an actual node ID should return that node ID
	resolved, err = d.ResolveNodeOrAlias(id3)
	if err != nil {
		tt.Fatalf("ResolveNodeOrAlias by ID: %v", err)
	}
	if resolved != id3 {
		tt.Errorf("expected node ID resolution to id3=%q, got %q", id3, resolved)
	}

	// Edge case: alias "data" should resolve to the alias, not try to find a node
	d.SetAlias(id2, "data")
	resolved, err = d.ResolveNodeOrAlias("data")
	if err != nil {
		tt.Fatalf("ResolveNodeOrAlias for 'data': %v", err)
	}
	if resolved != id2 {
		tt.Errorf("alias 'data' should resolve to id2=%q, got %q", id2, resolved)
	}
}

// TestSetAlias_OverwritesWithONCONFLICT verifies that SetAlias uses SQLite's
// ON CONFLICT clause to overwrite existing aliases correctly, maintaining
// ACID properties even when overwriting.
func TestSetAlias_OverwritesWithONCONFLICT(tt *testing.T) {
	d := newTestDAG(tt)
	id1, _ := d.AddNode("", t.RoleUser, textContent("node1"), "", "", 0)
	id2, _ := d.AddNode(id1, t.RoleAssistant, textContent("node2"), "", "", 0)
	id3, _ := d.AddNode(id2, t.RoleUser, textContent("node3"), "", "", 0)

	// Initial assignment
	if err := d.SetAlias(id1, "tag"); err != nil {
		tt.Fatalf("SetAlias initial: %v", err)
	}
	resolved, _ := d.ResolveAlias("tag")
	if resolved != id1 {
		tt.Errorf("initial: expected %q, got %q", id1, resolved)
	}

	// Overwrite with different node
	if err := d.SetAlias(id2, "tag"); err != nil {
		tt.Fatalf("SetAlias overwrite: %v", err)
	}
	resolved, _ = d.ResolveAlias("tag")
	if resolved != id2 {
		tt.Errorf("after overwrite: expected %q, got %q", id2, resolved)
	}

	// Overwrite again
	if err := d.SetAlias(id3, "tag"); err != nil {
		tt.Fatalf("SetAlias overwrite 2: %v", err)
	}
	resolved, _ = d.ResolveAlias("tag")
	if resolved != id3 {
		tt.Errorf("after second overwrite: expected %q, got %q", id3, resolved)
	}

	// Verify only the final mapping exists (no duplicates)
	aliases, _ := d.GetAliases(id1)
	if len(aliases) != 0 {
		tt.Errorf("id1 should have no aliases after transfer, got %v", aliases)
	}
	aliases, _ = d.GetAliases(id2)
	if len(aliases) != 0 {
		tt.Errorf("id2 should have no aliases after transfer, got %v", aliases)
	}
	aliases, _ = d.GetAliases(id3)
	if len(aliases) != 1 || aliases[0] != "tag" {
		tt.Errorf("id3 should have 'tag' alias, got %v", aliases)
	}
}

// TestDeleteAlias_NonexistentIsNoop verifies that DeleteAlias on a
// nonexistent alias does not error and leaves the database in a consistent state.
// This is important for idempotent operations and cleanup scripts.
func TestDeleteAlias_NonexistentIsNoop(tt *testing.T) {
	d := newTestDAG(tt)
	id1, _ := d.AddNode("", t.RoleUser, textContent("node1"), "", "", 0)

	// Deleting a non-existent alias should not error
	if err := d.DeleteAlias("never_existed"); err != nil {
		tt.Errorf("DeleteAlias non-existent should not error, got %v", err)
	}

	// Database should still be usable
	if err := d.SetAlias(id1, "test"); err != nil {
		tt.Fatalf("SetAlias after delete non-existent: %v", err)
	}
	resolved, err := d.ResolveAlias("test")
	if err != nil || resolved != id1 {
		tt.Errorf("alias resolution after delete non-existent failed: err=%v, resolved=%q", err, resolved)
	}

	// Delete a real alias
	if err := d.DeleteAlias("test"); err != nil {
		tt.Fatalf("DeleteAlias existing: %v", err)
	}

	// Verify it's gone
	_, err = d.ResolveAlias("test")
	if err != sql.ErrNoRows {
		tt.Errorf("expected ErrNoRows after delete, got %v", err)
	}

	// Now delete it again (should be noop)
	if err := d.DeleteAlias("test"); err != nil {
		tt.Errorf("DeleteAlias after removal should not error, got %v", err)
	}
}

// TestSetAlias_MultipleAliasesPerNode verifies that a single node can have
// multiple aliases and all are maintained correctly through overwrites.
func TestSetAlias_MultipleAliasesPerNode(tt *testing.T) {
	d := newTestDAG(tt)
	id1, _ := d.AddNode("", t.RoleUser, textContent("node1"), "", "", 0)
	id2, _ := d.AddNode(id1, t.RoleAssistant, textContent("node2"), "", "", 0)

	// Assign multiple aliases to id1
	d.SetAlias(id1, "first")
	d.SetAlias(id1, "primary")
	d.SetAlias(id1, "a1")

	// Verify all resolve to id1
	for _, alias := range []string{"first", "primary", "a1"} {
		resolved, err := d.ResolveAlias(alias)
		if err != nil || resolved != id1 {
			tt.Errorf("alias %q: err=%v, resolved=%q, want %q", alias, err, resolved, id1)
		}
	}

	// GetAliases should return all
	aliases, _ := d.GetAliases(id1)
	if len(aliases) != 3 {
		tt.Fatalf("expected 3 aliases, got %d: %v", len(aliases), aliases)
	}

	// Now overwrite "primary" to point to id2
	d.SetAlias(id2, "primary")

	// Verify id1 still has "first" and "a1"
	aliases, _ = d.GetAliases(id1)
	if len(aliases) != 2 {
		tt.Fatalf("after transfer: expected 2 aliases for id1, got %d: %v", len(aliases), aliases)
	}

	// Verify id2 has "primary"
	aliases, _ = d.GetAliases(id2)
	if len(aliases) != 1 || aliases[0] != "primary" {
		tt.Errorf("id2 aliases: expected [primary], got %v", aliases)
	}

	// Verify "primary" resolves to id2 now
	resolved, _ := d.ResolveAlias("primary")
	if resolved != id2 {
		tt.Errorf("primary after transfer: expected %q, got %q", id2, resolved)
	}
}

// TestNextAutoAlias_SkipsGaps verifies that NextAutoAlias correctly handles
// gaps in the auto-alias sequence (e.g., if a1 and a3 exist but a2 doesn't,
// it should still return a4).
func TestNextAutoAlias_SkipsGaps(tt *testing.T) {
	d := newTestDAG(tt)
	id1, _ := d.AddNode("", t.RoleUser, textContent("node1"), "", "", 0)
	id2, _ := d.AddNode(id1, t.RoleAssistant, textContent("node2"), "", "", 0)
	id3, _ := d.AddNode(id2, t.RoleUser, textContent("node3"), "", "", 0)
	id4, _ := d.AddNode(id3, t.RoleAssistant, textContent("node4"), "", "", 0)

	// Set a1 and a3, deliberately skipping a2
	d.SetAlias(id1, "a1")
	d.SetAlias(id3, "a3")

	// NextAutoAlias should find the max (a3) and return a4
	if got := d.NextAutoAlias(); got != "a4" {
		tt.Errorf("with a1 and a3: got %q, want 'a4'", got)
	}

	// Set a4, should get a5
	d.SetAlias(id4, "a4")
	if got := d.NextAutoAlias(); got != "a5" {
		tt.Errorf("after a4: got %q, want 'a5'", got)
	}
}

// TestDeleteAlias_DoesNotAffectOtherAliases verifies that deleting one alias
// for a node does not affect its other aliases or the node itself.
func TestDeleteAlias_DoesNotAffectOtherAliases(tt *testing.T) {
	d := newTestDAG(tt)
	id1, _ := d.AddNode("", t.RoleUser, textContent("node1"), "", "", 0)

	// Set multiple aliases
	d.SetAlias(id1, "alias1")
	d.SetAlias(id1, "alias2")
	d.SetAlias(id1, "alias3")

	// Delete one
	d.DeleteAlias("alias2")

	// Verify alias2 is gone
	_, err := d.ResolveAlias("alias2")
	if err != sql.ErrNoRows {
		tt.Errorf("alias2 should be deleted, got err=%v", err)
	}

	// Verify alias1 and alias3 still work
	for _, alias := range []string{"alias1", "alias3"} {
		resolved, err := d.ResolveAlias(alias)
		if err != nil || resolved != id1 {
			tt.Errorf("alias %q: err=%v, resolved=%q", alias, err, resolved)
		}
	}

	// Verify the node still exists
	node, err := d.GetNode(id1)
	if err != nil || node.ID != id1 {
		tt.Errorf("node after alias deletion: err=%v, node=%+v", err, node)
	}

	// Verify node still has the remaining aliases
	aliases, _ := d.GetAliases(id1)
	if len(aliases) != 2 {
		tt.Errorf("expected 2 aliases, got %d: %v", len(aliases), aliases)
	}
}
