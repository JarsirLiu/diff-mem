// Package store implements a disk-backed memory store using BadgerDB.
package store

import (
	"encoding/json"
	"sync"

	"github.com/dgraph-io/badger/v4"
	"github.com/diff-mem/diff-mem/internal/model"
)

const (
	nodeKey    = "node:"
	outEdgeKey = "out:"
	inEdgeKey  = "in:"
)

type FileStore struct {
	db       *badger.DB
	mu       sync.RWMutex
	outbound map[string][]model.Edge
	inbound  map[string][]model.Edge
}

func NewFileStore(dir string) (*FileStore, error) {
	db, err := badger.Open(badger.DefaultOptions(dir).WithInMemory(false))
	if err != nil {
		return nil, err
	}
	s := &FileStore{
		db:       db,
		outbound: make(map[string][]model.Edge),
		inbound:  make(map[string][]model.Edge),
	}
	s.loadEdges()
	return s, nil
}

func (s *FileStore) loadEdges() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := []byte(outEdgeKey)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			var edge model.Edge
			item := it.Item()
			item.Value(func(val []byte) error {
				return json.Unmarshal(val, &edge)
			})
			s.outbound[edge.From] = append(s.outbound[edge.From], edge)
			s.inbound[edge.To] = append(s.inbound[edge.To], edge)
		}
		return nil
	})
}

func (s *FileStore) persistNode(node *model.Node) error {
	val, _ := json.Marshal(node)
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(nodeKey+node.Header.Path), val)
	})
}

func (s *FileStore) loadNode(path string) (*model.Node, bool) {
	var node model.Node
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(nodeKey + path))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &node)
		})
	})
	if err != nil {
		return nil, false
	}
	return &node, true
}

func (s *FileStore) deleteNode(path string) error {
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(nodeKey + path))
	})
}

func (s *FileStore) persistEdge(edge model.Edge) error {
	val, _ := json.Marshal(edge)
	return s.db.Update(func(txn *badger.Txn) error {
		err1 := txn.Set([]byte(outEdgeKey+edge.From+"|"+edge.To), val)
		err2 := txn.Set([]byte(inEdgeKey+edge.To+"|"+edge.From), val)
		if err1 != nil {
			return err1
		}
		return err2
	})
}

func (s *FileStore) deleteEdge(from, to string) error {
	return s.db.Update(func(txn *badger.Txn) error {
		err1 := txn.Delete([]byte(outEdgeKey + from + "|" + to))
		err2 := txn.Delete([]byte(inEdgeKey + to + "|" + from))
		if err1 != nil {
			return err1
		}
		return err2
	})
}

// Store interface
func (s *FileStore) GetNode(path string) (*model.Node, bool) {
	return s.loadNode(path)
}

func (s *FileStore) PutNode(node *model.Node) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.persistNode(node)
}

func (s *FileStore) DeleteNode(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.outbound[path] {
		s.deleteEdge(e.From, e.To)
	}
	s.outbound[path] = nil
	s.inbound[path] = nil
	return s.deleteNode(path)
}

func (s *FileStore) Exists(path string) bool {
	_, ok := s.loadNode(path)
	return ok
}

func (s *FileStore) AddEdge(edge model.Edge) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outbound[edge.From] = append(s.outbound[edge.From], edge)
	s.inbound[edge.To] = append(s.inbound[edge.To], edge)
	return s.persistEdge(edge)
}

func (s *FileStore) RemoveEdge(from, to string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeOutboundEdge(from, to)
	s.removeInboundEdge(to, from)
	return s.deleteEdge(from, to)
}

func (s *FileStore) removeOutboundEdge(from, to string) {
	edges := s.outbound[from]
	for i, e := range edges {
		if e.To == to {
			s.outbound[from] = append(edges[:i], edges[i+1:]...)
			return
		}
	}
}

func (s *FileStore) removeInboundEdge(to, from string) {
	edges := s.inbound[to]
	for i, e := range edges {
		if e.From == from {
			s.inbound[to] = append(edges[:i], edges[i+1:]...)
			return
		}
	}
}

func (s *FileStore) GetOutboundEdges(path string) []model.Edge {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.outbound[path]
}

func (s *FileStore) GetInboundEdges(path string) []model.Edge {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.inbound[path]
}

func (s *FileStore) HasEdge(from, to string) bool {
	err := s.db.View(func(txn *badger.Txn) error {
		_, err := txn.Get([]byte(outEdgeKey + from + "|" + to))
		return err
	})
	return err == nil
}

func (s *FileStore) BuildTagIndex() map[string][]string {
	idx := make(map[string][]string)
	s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := []byte(nodeKey)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			key := string(item.Key())
			path := key[len(nodeKey):]
			var node model.Node
			if err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, &node)
			}); err != nil {
				continue
			}
			if node.Header.Status == model.StatusArchived {
				continue
			}
			for _, tag := range node.Header.Tags {
				idx[tag] = append(idx[tag], path)
			}
		}
		return nil
	})
	return idx
}

func (s *FileStore) BuildKeywordIndex() map[string][]string {
	idx := make(map[string][]string)
	s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := []byte(nodeKey)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			key := string(item.Key())
			path := key[len(nodeKey):]
			var node model.Node
			if err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, &node)
			}); err != nil {
				continue
			}
			if node.Header.Status == model.StatusArchived {
				continue
			}
			for _, word := range tokenize(path, node.Header.Title, node.Header.Summary) {
				idx[word] = append(idx[word], path)
			}
		}
		return nil
	})
	return idx
}

func (s *FileStore) AllNodes() []*model.Node {
	var nodes []*model.Node
	s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := []byte(nodeKey)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			var node model.Node
			item := it.Item()
			if err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, &node)
			}); err != nil {
				continue
			}
			nodes = append(nodes, &node)
		}
		return nil
	})
	return nodes
}

func (s *FileStore) Close() error {
	return s.db.Close()
}

func tokenize(fields ...string) []string {
	var words []string
	for _, f := range fields {
		word := ""
		for _, r := range []rune(f) {
			if r == '/' || r == '-' || r == '_' || r == ' ' || r == '\n' || r == ',' {
				if word != "" {
					words = append(words, word)
					word = ""
				}
			} else {
				word += string(r)
			}
		}
		if word != "" {
			words = append(words, word)
		}
	}
	return words
}
