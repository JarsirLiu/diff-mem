// Package store implements a SQLite-backed memory store.
// It uses the pure-Go driver (modernc.org/sqlite), so cross-compilation
// works without a C toolchain.
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"github.com/diff-mem/diff-mem/internal/model"
)

const sqliteSchema = `
CREATE TABLE IF NOT EXISTS nodes (
	path   TEXT PRIMARY KEY,
	status TEXT NOT NULL DEFAULT 'active',
	data   BLOB NOT NULL
);
`

// SQLiteStore persists nodes as JSON rows in a SQLite database.
type SQLiteStore struct {
	db *sql.DB
	mu sync.RWMutex
}

func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(sqliteSchema); err != nil {
		db.Close()
		return nil, err
	}
	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) GetNode(path string) (*model.Node, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getNode(path)
}

func (s *SQLiteStore) getNode(path string) (*model.Node, bool) {
	var data []byte
	err := s.db.QueryRow(`SELECT data FROM nodes WHERE path = ?`, path).Scan(&data)
	if err != nil {
		return nil, false
	}
	var node model.Node
	if err := json.Unmarshal(data, &node); err != nil {
		return nil, false
	}
	return &node, true
}

func (s *SQLiteStore) PutNode(node *model.Node) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	node.Header.UpdatedAt = time.Now()
	data, err := json.Marshal(node)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
		INSERT INTO nodes (path, status, data) VALUES (?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET status = excluded.status, data = excluded.data`,
		node.Header.Path, string(node.Header.Status), data)
	return err
}

func (s *SQLiteStore) DeleteNode(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM nodes WHERE path = ?`, path)
	return err
}

func (s *SQLiteStore) Exists(path string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var one int
	err := s.db.QueryRow(`SELECT 1 FROM nodes WHERE path = ?`, path).Scan(&one)
	return err == nil
}

func (s *SQLiteStore) BuildTagIndex() map[string][]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	idx := make(map[string][]string)
	rows, err := s.db.Query(`SELECT path, data FROM nodes WHERE status != 'archived'`)
	if err != nil {
		return idx
	}
	defer rows.Close()
	for rows.Next() {
		var path string
		var data []byte
		if rows.Scan(&path, &data) != nil {
			continue
		}
		var node model.Node
		if json.Unmarshal(data, &node) != nil {
			continue
		}
		for _, tag := range node.Header.Tags {
			idx[tag] = append(idx[tag], path)
		}
	}
	return idx
}

func (s *SQLiteStore) BuildKeywordIndex() map[string][]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	idx := make(map[string][]string)
	rows, err := s.db.Query(`SELECT path, data FROM nodes WHERE status != 'archived'`)
	if err != nil {
		return idx
	}
	defer rows.Close()
	for rows.Next() {
		var path string
		var data []byte
		if rows.Scan(&path, &data) != nil {
			continue
		}
		var node model.Node
		if json.Unmarshal(data, &node) != nil {
			continue
		}
		for _, word := range tokenize(path, node.Header.Title, node.Header.Summary) {
			idx[word] = append(idx[word], path)
		}
	}
	return idx
}

func (s *SQLiteStore) AllNodes() []*model.Node {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var nodes []*model.Node
	rows, err := s.db.Query(`SELECT data FROM nodes`)
	if err != nil {
		return nodes
	}
	defer rows.Close()
	for rows.Next() {
		var data []byte
		if rows.Scan(&data) != nil {
			continue
		}
		var node model.Node
		if json.Unmarshal(data, &node) != nil {
			continue
		}
		nodes = append(nodes, &node)
	}
	return nodes
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
