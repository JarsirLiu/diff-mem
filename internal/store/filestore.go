// Package store implements a disk-backed memory store using BadgerDB.
package store

import (
	"encoding/json"
	"sync"

	"github.com/dgraph-io/badger/v4"
	"github.com/diff-mem/diff-mem/internal/model"
)

const nodeKey = "node:"

type FileStore struct {
	db *badger.DB
	mu sync.RWMutex
}

func NewFileStore(dir string) (*FileStore, error) {
	db, err := badger.Open(badger.DefaultOptions(dir).WithInMemory(false))
	if err != nil {
		return nil, err
	}
	return &FileStore{db: db}, nil
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

// Store interface — UpdatedAt is stamped by the caller (engine), keeping all
// three store implementations on the same contract.
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
	return s.deleteNode(path)
}

func (s *FileStore) Exists(path string) bool {
	_, ok := s.loadNode(path)
	return ok
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

// tokenize splits fields into indexable words on common ASCII and CJK
// delimiters. Single implementation shared by all store backends so keyword
// search behaves identically regardless of backend.
func tokenize(fields ...string) []string {
	var words []string
	for _, f := range fields {
		word := ""
		for _, r := range []rune(f) {
			switch r {
			case '/', '-', '_', ' ', '\t', '\n', '\r', ',',
				'，', '、', '；', '：', '.', '。', '·', '（', '）', '(', ')':
				if word != "" {
					words = append(words, word)
					word = ""
				}
			default:
				word += string(r)
			}
		}
		if word != "" {
			words = append(words, word)
		}
	}
	return words
}
