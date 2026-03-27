package core

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	t "torus_go_agent/internal/types"
	_ "modernc.org/sqlite"
)

type Node struct {
	ID         string           `json:"id"`
	ParentID   string           `json:"parent_id"`
	Role       string           `json:"role"`
	Content    []t.ContentBlock `json:"content"`
	Model      string           `json:"model"`
	Provider   string           `json:"provider"`
	Timestamp  int64            `json:"timestamp"`
	TokenCount int              `json:"token_count"`
}

type BranchInfo struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	HeadNodeID string `json:"head_node_id"`
	ForkedFrom string `json:"forked_from,omitempty"` // node ID this branch was forked from
}

const dagBusyTimeoutMs = 5000

type DAG struct {
	db       *sql.DB
	mu       sync.RWMutex
	branchID string
	hooks    *HookRegistry // optional, set via SetHooks
}

// SetHooks attaches a hook registry to the DAG for mutation events.
func (d *DAG) SetHooks(h *HookRegistry) { d.hooks = h }

// Fork returns a new DAG that shares the same database but has its own
// independent branchID. Safe for concurrent use by sub-agents.
func (d *DAG) Fork(branchID string) *DAG {
	return &DAG{db: d.db, branchID: branchID, hooks: d.hooks}
}

func genID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

const dagSchema = `
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
	head_node_id TEXT NOT NULL,
	forked_from TEXT DEFAULT ''
);
CREATE TABLE IF NOT EXISTS node_aliases (
	alias TEXT PRIMARY KEY,
	node_id TEXT NOT NULL
);
`

func NewDAG(dbPath string) (*DAG, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	for _, p := range []string{"PRAGMA journal_mode=WAL", fmt.Sprintf("PRAGMA busy_timeout=%d", dagBusyTimeoutMs), "PRAGMA synchronous=NORMAL"} {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("pragma: %w", err)
		}
	}
	if _, err := db.Exec(dagSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("schema: %w", err)
	}
	// Migration: add forked_from column if missing (existing databases).
	var colCount int
	db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('branches') WHERE name='forked_from'").Scan(&colCount)
	if colCount == 0 {
		if _, err := db.Exec("ALTER TABLE branches ADD COLUMN forked_from TEXT DEFAULT ''"); err != nil {
			db.Close()
			return nil, fmt.Errorf("migration forked_from: %w", err)
		}
	}


	d := &DAG{db: db}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM branches").Scan(&count); err != nil {
		db.Close()
		return nil, fmt.Errorf("count branches: %w", err)
	}
	if count == 0 {
		id := "br_" + genID()
		if _, err := db.Exec("INSERT INTO branches (id, name, head_node_id) VALUES (?, ?, ?)", id, "main", ""); err != nil {
			db.Close()
			return nil, fmt.Errorf("create main branch: %w", err)
		}
		d.branchID = id
	} else {
		// Pick the most recently created branch by finding the newest head node timestamp.
		// Branches with empty heads (from /new) get timestamp 0, so they sort last unless they're the only option.
		if err := db.QueryRow(`
			SELECT b.id FROM branches b
			LEFT JOIN nodes n ON n.id = b.head_node_id
			ORDER BY COALESCE(n.timestamp, 0) DESC
			LIMIT 1
		`).Scan(&d.branchID); err != nil {
			db.Close()
			return nil, fmt.Errorf("select branch: %w", err)
		}
	}
	return d, nil
}

func (d *DAG) AddNode(parentID string, role t.Role, content []t.ContentBlock, model, provider string, tokenCount int) (string, error) {
	id := "nd_" + genID()
	cj, err := json.Marshal(content)
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}
	ts := time.Now().UnixMilli()
	_, err = d.db.Exec(
		"INSERT INTO nodes (id, parent_id, role, content, model, provider, timestamp, token_count) VALUES (?,?,?,?,?,?,?,?)",
		id, parentID, string(role), string(cj), model, provider, ts, tokenCount,
	)
	if err != nil {
		return "", fmt.Errorf("insert: %w", err)
	}
	if _, err := d.db.Exec("UPDATE branches SET head_node_id = ? WHERE id = ?", id, d.branchID); err != nil {
		return "", fmt.Errorf("update head: %w", err)
	}
	if d.hooks != nil {
		d.hooks.Fire(context.Background(), HookOnNodeAdded, &HookData{
			AgentID: "main",
			Meta:    map[string]any{"node_id": id, "parent_id": parentID, "role": string(role), "branch": d.branchID},
		})
	}
	return id, nil
}

func (d *DAG) GetNode(id string) (*Node, error) {
	row := d.db.QueryRow("SELECT id, parent_id, role, content, model, provider, timestamp, token_count FROM nodes WHERE id = ?", id)
	return scanNode(row)
}

func scanNode(row *sql.Row) (*Node, error) {
	var n Node
	var pid sql.NullString
	var cj string
	err := row.Scan(&n.ID, &pid, &n.Role, &cj, &n.Model, &n.Provider, &n.Timestamp, &n.TokenCount)
	if err != nil {
		return nil, err
	}
	if pid.Valid {
		n.ParentID = pid.String
	}
	if err := json.Unmarshal([]byte(cj), &n.Content); err != nil {
		return nil, fmt.Errorf("unmarshal node content: %w", err)
	}
	return &n, nil
}

func (d *DAG) GetAncestors(nodeID string) ([]Node, error) {
	if nodeID == "" {
		return nil, nil
	}
	query := `WITH RECURSIVE chain(id, depth) AS (
		SELECT ?, 0
		UNION ALL
		SELECT n.parent_id, c.depth + 1 FROM nodes n JOIN chain c ON n.id = c.id WHERE n.parent_id IS NOT NULL AND n.parent_id != ''
	)
	SELECT n.id, n.parent_id, n.role, n.content, n.model, n.provider, n.timestamp, n.token_count
	FROM nodes n JOIN chain c ON n.id = c.id
	ORDER BY c.depth DESC`

	rows, err := d.db.Query(query, nodeID)
	if err != nil {
		return nil, fmt.Errorf("get ancestors: %w", err)
	}
	defer rows.Close()

	var ancestors []Node
	for rows.Next() {
		var n Node
		var pid sql.NullString
		var cj string
		if err := rows.Scan(&n.ID, &pid, &n.Role, &cj, &n.Model, &n.Provider, &n.Timestamp, &n.TokenCount); err != nil {
			return nil, fmt.Errorf("scan ancestor: %w", err)
		}
		if pid.Valid {
			n.ParentID = pid.String
		}
		if err := json.Unmarshal([]byte(cj), &n.Content); err != nil {
			return nil, fmt.Errorf("unmarshal ancestor content: %w", err)
		}
		ancestors = append(ancestors, n)
	}
	return ancestors, rows.Err()
}

func (d *DAG) PromptFrom(nodeID string) ([]t.Message, error) {
	anc, err := d.GetAncestors(nodeID)
	if err != nil {
		return nil, err
	}
	msgs := make([]t.Message, len(anc))
	for i, n := range anc {
		msgs[i] = t.Message{Role: t.Role(n.Role), Content: n.Content}
	}
	return msgs, nil
}

func (d *DAG) GetHead() (string, error) {
	var h string
	err := d.db.QueryRow("SELECT head_node_id FROM branches WHERE id = ?", d.branchID).Scan(&h)
	return h, err
}

// CurrentBranchInfo returns the active branch ID, name, head node, and ancestor count.
func (d *DAG) CurrentBranchInfo() (branchID, branchName, headNode string, msgCount int, err error) {
	branchID = d.branchID
	if err = d.db.QueryRow("SELECT name, head_node_id FROM branches WHERE id = ?", d.branchID).Scan(&branchName, &headNode); err != nil {
		return
	}
	if headNode != "" {
		cur := headNode
		for cur != "" {
			var pid sql.NullString
			if d.db.QueryRow("SELECT parent_id FROM nodes WHERE id = ?", cur).Scan(&pid) != nil {
				break
			}
			msgCount++
			if pid.Valid {
				cur = pid.String
			} else {
				cur = ""
			}
		}
	}
	return
}

// RemoveNode deletes a node from the DAG and rewinds the branch head to the node's parent.
// Used to roll back a dangling user node when an LLM call fails.
func (d *DAG) RemoveNode(nodeID string) error {
	var parentID sql.NullString
	if err := d.db.QueryRow("SELECT parent_id FROM nodes WHERE id = ?", nodeID).Scan(&parentID); err != nil {
		return fmt.Errorf("find node: %w", err)
	}
	newHead := ""
	if parentID.Valid {
		newHead = parentID.String
	}
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	if _, err := tx.Exec("DELETE FROM nodes WHERE id = ?", nodeID); err != nil {
		tx.Rollback()
		return fmt.Errorf("remove node: delete: %w", err)
	}
	if _, err := tx.Exec("UPDATE branches SET head_node_id = ? WHERE id = ?", newHead, d.branchID); err != nil {
		tx.Rollback()
		return fmt.Errorf("remove node: update head: %w", err)
	}
	if err := tx.Commit(); err != nil {
		tx.Rollback()
		return fmt.Errorf("remove node: %w", err)
	}
	return nil
}

// ResetHead clears the current branch's head, so the next message starts a fresh
// chain on the same branch. Existing nodes remain in the DB but won't be traversed.
func (d *DAG) ResetHead() error {
	_, err := d.db.Exec("UPDATE branches SET head_node_id = '' WHERE id = ?", d.branchID)
	return err
}

// Branch creates a new branch continuing from fromNodeID (prompt includes ancestor history).
func (d *DAG) Branch(fromNodeID, name string) (string, error) {
	id := "br_" + genID()
	_, err := d.db.Exec("INSERT INTO branches (id, name, head_node_id, forked_from) VALUES (?,?,?,?)", id, name, fromNodeID, fromNodeID)
	if err != nil {
		return "", err
	}
	d.mu.Lock()
	d.branchID = id
	d.mu.Unlock()
	return id, nil
}

// NewBranch creates a fresh branch with an empty head (prompt starts clean)
// but records forkedFrom so the lineage is preserved.
func (d *DAG) NewBranch(name string) (string, error) {
	// Capture the current head as the fork point before resetting.
	head, _ := d.GetHead()
	id := "br_" + genID()
	_, err := d.db.Exec("INSERT INTO branches (id, name, head_node_id, forked_from) VALUES (?,?,?,?)", id, name, "", head)
	if err != nil {
		return "", err
	}
	d.mu.Lock()
	d.branchID = id
	d.mu.Unlock()
	return id, nil
}

func (d *DAG) SwitchBranch(branchID string) error {
	var exists int
	d.db.QueryRow("SELECT COUNT(*) FROM branches WHERE id = ?", branchID).Scan(&exists)
	if exists == 0 {
		return fmt.Errorf("branch %s not found", branchID)
	}
	d.mu.Lock()
	oldBranch := d.branchID
	d.branchID = branchID
	d.mu.Unlock()
	if d.hooks != nil {
		d.hooks.Fire(context.Background(), HookOnBranchSwitch, &HookData{
			AgentID: "main",
			Meta:    map[string]any{"old_branch": oldBranch, "new_branch": branchID},
		})
	}
	return nil
}

func (d *DAG) ListBranches() ([]BranchInfo, error) {
	rows, err := d.db.Query("SELECT id, name, head_node_id, forked_from FROM branches")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var bs []BranchInfo
	for rows.Next() {
		var b BranchInfo
		if err := rows.Scan(&b.ID, &b.Name, &b.HeadNodeID, &b.ForkedFrom); err != nil {
			return nil, err
		}
		bs = append(bs, b)
	}
	return bs, nil
}

func (d *DAG) CurrentBranchID() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.branchID
}
func (d *DAG) Close() error            { return d.db.Close() }

// SearchAll searches all nodes across all branches for content matching the query.
// Returns formatted results with branch name and role, limited to maxResults.
func (d *DAG) SearchAll(query string, maxResults int) (string, error) {
	if maxResults <= 0 {
		maxResults = 5
	}
	rows, err := d.db.Query(`
		SELECT n.role, n.content, b.name
		FROM nodes n
		LEFT JOIN branches b ON b.head_node_id IN (
			WITH RECURSIVE ancestors(id) AS (
				SELECT b2.head_node_id FROM branches b2 WHERE b2.id = b.id
				UNION ALL
				SELECT n2.parent_id FROM nodes n2 JOIN ancestors a ON a.id = n2.id WHERE n2.parent_id != ''
			)
			SELECT id FROM ancestors
		)
		WHERE n.content LIKE '%' || ? || '%'
		ORDER BY n.timestamp DESC
		LIMIT ?
	`, query, maxResults)
	if err != nil {
		// Fallback to simpler query if recursive CTE fails
		rows, err = d.db.Query(
			"SELECT role, content, '' FROM nodes WHERE content LIKE '%' || ? || '%' ORDER BY timestamp DESC LIMIT ?",
			query, maxResults,
		)
		if err != nil {
			return "", err
		}
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var role, content, branch string
		if err := rows.Scan(&role, &content, &branch); err != nil {
			return "", err
		}
		// Truncate long content
		if len(content) > 300 {
			content = content[:300] + "..."
		}
		label := fmt.Sprintf("[%s", role)
		if branch != "" {
			label += " @ " + branch
		}
		label += "] "
		results = append(results, label+content)
	}
	if len(results) == 0 {
		return "", nil
	}
	return fmt.Sprintf("Found %d matches:\n\n%s", len(results), strings.Join(results, "\n\n")), nil
}

// --- Node Aliases ---

// SetAlias assigns a human-readable alias to a node. Overwrites if the alias already exists.
func (d *DAG) SetAlias(nodeID, alias string) error {
	_, err := d.db.Exec(
		"INSERT INTO node_aliases (alias, node_id) VALUES (?, ?) ON CONFLICT(alias) DO UPDATE SET node_id = ?",
		alias, nodeID, nodeID,
	)
	return err
}

// NextAutoAlias returns the next available auto-alias (a1, a2, ...) by finding the current max.
func (d *DAG) NextAutoAlias() string {
	var maxN int
	d.db.QueryRow("SELECT COALESCE(MAX(CAST(SUBSTR(alias, 2) AS INTEGER)), 0) FROM node_aliases WHERE alias GLOB 'a[0-9]*'").Scan(&maxN)
	return fmt.Sprintf("a%d", maxN+1)
}

// ResolveAlias returns the node ID for a given alias, or sql.ErrNoRows if not found.
func (d *DAG) ResolveAlias(alias string) (string, error) {
	var nodeID string
	err := d.db.QueryRow("SELECT node_id FROM node_aliases WHERE alias = ?", alias).Scan(&nodeID)
	return nodeID, err
}

// GetAliases returns all aliases for a given node.
func (d *DAG) GetAliases(nodeID string) ([]string, error) {
	rows, err := d.db.Query("SELECT alias FROM node_aliases WHERE node_id = ?", nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var aliases []string
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			return nil, err
		}
		aliases = append(aliases, a)
	}
	return aliases, nil
}

// DeleteAlias removes an alias.
func (d *DAG) DeleteAlias(alias string) error {
	_, err := d.db.Exec("DELETE FROM node_aliases WHERE alias = ?", alias)
	return err
}

// ResolveNodeOrAlias tries to resolve an input as an alias first, then falls back to treating it as a node ID.
func (d *DAG) ResolveNodeOrAlias(input string) (string, error) {
	nodeID, err := d.ResolveAlias(input)
	if err == nil {
		return nodeID, nil
	}
	// Check if it's a valid node ID directly.
	var exists int
	if d.db.QueryRow("SELECT COUNT(*) FROM nodes WHERE id = ?", input).Scan(&exists); exists > 0 {
		return input, nil
	}
	return "", fmt.Errorf("node or alias %q not found", input)
}

// --- GetSubtree ---

// GetSubtree returns all descendants of a node (children, grandchildren, etc.)
// using a recursive CTE. Results are ordered by timestamp ascending.
func (d *DAG) GetSubtree(nodeID string) ([]Node, error) {
	rows, err := d.db.Query(`
		WITH RECURSIVE subtree(id) AS (
			SELECT id FROM nodes WHERE parent_id = ?
			UNION ALL
			SELECT n.id FROM nodes n JOIN subtree s ON n.parent_id = s.id
		)
		SELECT n.id, n.parent_id, n.role, n.content, n.model, n.provider, n.timestamp, n.token_count
		FROM nodes n JOIN subtree s ON n.id = s.id
		ORDER BY n.timestamp ASC
	`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var nodes []Node
	for rows.Next() {
		var n Node
		var pid sql.NullString
		var cj string
		if err := rows.Scan(&n.ID, &pid, &n.Role, &cj, &n.Model, &n.Provider, &n.Timestamp, &n.TokenCount); err != nil {
			return nil, err
		}
		if pid.Valid {
			n.ParentID = pid.String
		}
		if err := json.Unmarshal([]byte(cj), &n.Content); err != nil {
			return nil, fmt.Errorf("unmarshal node content: %w", err)
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}


// ListBranchesWithCounts returns all branches along with a map of branch ID to
// ancestor (message) count. Uses a single recursive CTE to count ancestors for
// all branch heads at once, avoiding N+1 queries.
func (d *DAG) ListBranchesWithCounts() ([]BranchInfo, map[string]int, error) {
	branches, err := d.ListBranches()
	if err != nil {
		return nil, nil, err
	}

	countQuery := `WITH RECURSIVE chain(branch_id, id, depth) AS (
		SELECT b.id, b.head_node_id, 0 FROM branches b WHERE b.head_node_id != ''
		UNION ALL
		SELECT c.branch_id, n.parent_id, c.depth+1
		FROM nodes n JOIN chain c ON n.id = c.id
		WHERE n.parent_id IS NOT NULL AND n.parent_id != ''
	)
	SELECT branch_id, COUNT(*) FROM chain GROUP BY branch_id`

	rows, err := d.db.Query(countQuery)
	if err != nil {
		return branches, nil, fmt.Errorf("list branches with counts: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int, len(branches))
	for rows.Next() {
		var branchID string
		var count int
		if err := rows.Scan(&branchID, &count); err != nil {
			return branches, nil, fmt.Errorf("scan branch count: %w", err)
		}
		counts[branchID] = count
	}
	return branches, counts, rows.Err()
}
