// Package store defines the storage interface and in-memory implementation.
package store

import (
	"sync"
	"time"

	"github.com/diff-mem/diff-mem/internal/model"
)

// Store is the persistence layer for all memory data.
type Store interface {
	// Node operations
	GetNode(path string) (*model.Node, bool)
	PutNode(node *model.Node) error
	DeleteNode(path string) error
	Exists(path string) bool

	// Edge operations
	AddEdge(edge model.Edge) error
	RemoveEdge(from, to string) error
	GetOutboundEdges(path string) []model.Edge
	GetInboundEdges(path string) []model.Edge
	HasEdge(from, to string) bool

	// Index operations
	BuildTagIndex() map[string][]string
	BuildKeywordIndex() map[string][]string

	// Bulk
	AllNodes() []*model.Node
}

// MemoryStore is the in-memory implementation.
type MemoryStore struct {
	mu      sync.RWMutex
	nodes   map[string]*model.Node
	outbound map[string][]model.Edge
	inbound  map[string][]model.Edge
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		nodes:    make(map[string]*model.Node),
		outbound: make(map[string][]model.Edge),
		inbound:  make(map[string][]model.Edge),
	}
}

func (s *MemoryStore) GetNode(path string) (*model.Node, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n, ok := s.nodes[path]
	return n, ok
}

func (s *MemoryStore) PutNode(node *model.Node) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	node.Header.UpdatedAt = time.Now()
	s.nodes[node.Header.Path] = node
	return nil
}

func (s *MemoryStore) DeleteNode(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.nodes, path)
	// Clean up edges
	for _, e := range s.outbound[path] {
		s.removeInboundEdge(e.To, path)
	}
	delete(s.outbound, path)
	delete(s.inbound, path)
	return nil
}

func (s *MemoryStore) Exists(path string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.nodes[path]
	return ok
}

func (s *MemoryStore) AddEdge(edge model.Edge) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outbound[edge.From] = append(s.outbound[edge.From], edge)
	s.inbound[edge.To] = append(s.inbound[edge.To], edge)
	return nil
}

func (s *MemoryStore) RemoveEdge(from, to string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeOutboundEdge(from, to)
	s.removeInboundEdge(to, from)
	return nil
}

func (s *MemoryStore) removeOutboundEdge(from, to string) {
	edges := s.outbound[from]
	for i, e := range edges {
		if e.To == to {
			s.outbound[from] = append(edges[:i], edges[i+1:]...)
			return
		}
	}
}

func (s *MemoryStore) removeInboundEdge(to, from string) {
	edges := s.inbound[to]
	for i, e := range edges {
		if e.From == from {
			s.inbound[to] = append(edges[:i], edges[i+1:]...)
			return
		}
	}
}

func (s *MemoryStore) GetOutboundEdges(path string) []model.Edge {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.outbound[path]
}

func (s *MemoryStore) GetInboundEdges(path string) []model.Edge {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.inbound[path]
}

func (s *MemoryStore) HasEdge(from, to string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, e := range s.outbound[from] {
		if e.To == to {
			return true
		}
	}
	return false
}

func (s *MemoryStore) BuildTagIndex() map[string][]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	idx := make(map[string][]string)
	for path, node := range s.nodes {
		if node.Header.Status == model.StatusArchived {
			continue
		}
		for _, tag := range node.Header.Tags {
			idx[tag] = append(idx[tag], path)
		}
	}
	return idx
}

func (s *MemoryStore) BuildKeywordIndex() map[string][]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	idx := make(map[string][]string)
	for path, node := range s.nodes {
		if node.Header.Status == model.StatusArchived {
			continue
		}
		// Index path segments
		// Index summary
		// Index title
		fields := []string{path, node.Header.Title, node.Header.Summary}
		for _, f := range fields {
			// Simple word-based indexing (split on common delimiters)
			runes := []rune(f)
			word := ""
			for _, r := range runes {
				if r == '/' || r == '-' || r == '_' || r == ' ' || r == '\n' || r == ',' || r == '，' {
					if word != "" {
						idx[word] = append(idx[word], path)
						word = ""
					}
				} else {
					word += string(r)
				}
			}
			if word != "" {
				idx[word] = append(idx[word], path)
			}
		}
	}
	return idx
}

func (s *MemoryStore) AllNodes() []*model.Node {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*model.Node, 0, len(s.nodes))
	for _, n := range s.nodes {
		result = append(result, n)
	}
	return result
}
