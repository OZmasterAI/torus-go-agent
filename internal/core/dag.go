package core

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	_ "modernc.org/sqlite"
)

type Node struct {
	ID         string         `json:"id"`
	ParentID   string         `json:"parent_id"`
	Role       string         `json:"role"`
	Content    []ContentBlock `json:"content"`
	Model      string         `json:"model"`
	Provider   string         `json:"provider"`
	Timestamp  int64          `json:"timestamp"`
	TokenCount int            `json:"token_count"`
}

type BranchInfo struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	HeadNodeID string `json:"head_node_id"`
}

type DAG struct {
	db       *sql.DB
	branchID string
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
	head_node_id TEXT NOT NULL
);
`

func NewDAG(dbPath string) (*DAG, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	for _, p := range []string{"PRAGMA journal_mode=WAL", "PRAGMA busy_timeout=5000", "PRAGMA synchronous=NORMAL"} {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("pragma: %w", err)
		}
	}
	if _, err := db.Exec(dagSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("schema: %w", err)
	}

	d := &DAG{db: db}
	var count int
	db.QueryRow("SELECT COUNT(*) FROM branches").Scan(&count)
	if count == 0 {
		id := "br_" + genID()
		db.Exec("INSERT INTO branches (id, name, head_node_id) VALUES (?, ?, ?)", id, "main", "")
		d.branchID = id
	} else {
		db.QueryRow("SELECT id FROM branches LIMIT 1").Scan(&d.branchID)
	}
	return d, nil
}

func (d *DAG) AddNode(parentID string, role Role, content []ContentBlock, model, provider string, tokenCount int) (string, error) {
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
	d.db.Exec("UPDATE branches SET head_node_id = ? WHERE id = ?", id, d.branchID)
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
	json.Unmarshal([]byte(cj), &n.Content)
	return &n, nil
}

func (d *DAG) GetAncestors(nodeID string) ([]Node, error) {
	var ancestors []Node
	cur := nodeID
	for cur != "" {
		row := d.db.QueryRow("SELECT id, parent_id, role, content, model, provider, timestamp, token_count FROM nodes WHERE id = ?", cur)
		n, err := scanNode(row)
		if err != nil {
			if err == sql.ErrNoRows {
				break
			}
			return nil, err
		}
		ancestors = append(ancestors, *n)
		cur = n.ParentID
	}
	slices.Reverse(ancestors)
	return ancestors, nil
}

func (d *DAG) PromptFrom(nodeID string) ([]Message, error) {
	anc, err := d.GetAncestors(nodeID)
	if err != nil {
		return nil, err
	}
	msgs := make([]Message, len(anc))
	for i, n := range anc {
		msgs[i] = Message{Role: Role(n.Role), Content: n.Content}
	}
	return msgs, nil
}

func (d *DAG) GetHead() (string, error) {
	var h string
	err := d.db.QueryRow("SELECT head_node_id FROM branches WHERE id = ?", d.branchID).Scan(&h)
	return h, err
}

func (d *DAG) Branch(fromNodeID, name string) (string, error) {
	id := "br_" + genID()
	_, err := d.db.Exec("INSERT INTO branches (id, name, head_node_id) VALUES (?,?,?)", id, name, fromNodeID)
	if err != nil {
		return "", err
	}
	d.branchID = id
	return id, nil
}

func (d *DAG) SwitchBranch(branchID string) error {
	var exists int
	d.db.QueryRow("SELECT COUNT(*) FROM branches WHERE id = ?", branchID).Scan(&exists)
	if exists == 0 {
		return fmt.Errorf("branch %s not found", branchID)
	}
	d.branchID = branchID
	return nil
}

func (d *DAG) ListBranches() ([]BranchInfo, error) {
	rows, err := d.db.Query("SELECT id, name, head_node_id FROM branches")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var bs []BranchInfo
	for rows.Next() {
		var b BranchInfo
		rows.Scan(&b.ID, &b.Name, &b.HeadNodeID)
		bs = append(bs, b)
	}
	return bs, nil
}

func (d *DAG) CurrentBranchID() string { return d.branchID }
func (d *DAG) Close() error            { return d.db.Close() }
