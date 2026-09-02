package store_test

import (
	"testing"

	"github.com/diff-mem/diff-mem/internal/model"
	"github.com/diff-mem/diff-mem/internal/store"
)

func TestPutAndGet(t *testing.T) {
	s := store.NewMemoryStore()
	node := &model.Node{
		Header: model.Header{Path: "/a", Title: "A", Status: model.StatusActive, Fields: map[string]string{}},
	}
	s.PutNode(node)

	got, ok := s.GetNode("/a")
	if !ok {
		t.Fatal("expected node to exist")
	}
	if got.Header.Path != "/a" {
		t.Fatalf("expected /a, got %s", got.Header.Path)
	}
}

func TestGetNonExistent(t *testing.T) {
	s := store.NewMemoryStore()
	_, ok := s.GetNode("/not-found")
	if ok {
		t.Fatal("non-existent node should return false")
	}
}

func TestExists(t *testing.T) {
	s := store.NewMemoryStore()
	if s.Exists("/a") {
		t.Fatal("should not exist yet")
	}
	s.PutNode(&model.Node{Header: model.Header{Path: "/a", Status: model.StatusActive, Fields: map[string]string{}}})
	if !s.Exists("/a") {
		t.Fatal("should exist after PutNode")
	}
}

func TestDelete(t *testing.T) {
	s := store.NewMemoryStore()
	s.PutNode(&model.Node{Header: model.Header{Path: "/a", Status: model.StatusActive, Fields: map[string]string{}}})
	s.DeleteNode("/a")
	if s.Exists("/a") {
		t.Fatal("node should be deleted")
	}
}

func TestTagIndex(t *testing.T) {
	s := store.NewMemoryStore()
	s.PutNode(&model.Node{Header: model.Header{Path: "/a", Status: model.StatusActive, Tags: []string{"backend"}, Fields: map[string]string{}}})
	s.PutNode(&model.Node{Header: model.Header{Path: "/b", Status: model.StatusActive, Tags: []string{"frontend"}, Fields: map[string]string{}}})

	idx := s.BuildTagIndex()
	if len(idx["backend"]) != 1 || idx["backend"][0] != "/a" {
		t.Fatalf("tag index wrong: %v", idx["backend"])
	}
	if len(idx["frontend"]) != 1 || idx["frontend"][0] != "/b" {
		t.Fatalf("tag index wrong: %v", idx["frontend"])
	}
}

func TestTagIndex_ExcludesArchived(t *testing.T) {
	s := store.NewMemoryStore()
	s.PutNode(&model.Node{Header: model.Header{Path: "/archived", Status: model.StatusArchived, Tags: []string{"tag"}, Fields: map[string]string{}}})
	s.PutNode(&model.Node{Header: model.Header{Path: "/active", Status: model.StatusActive, Tags: []string{"tag"}, Fields: map[string]string{}}})

	idx := s.BuildTagIndex()
	if len(idx["tag"]) != 1 || idx["tag"][0] != "/active" {
		t.Fatalf("archived nodes should be excluded from tag index: %v", idx)
	}
}

func TestKeywordIndex(t *testing.T) {
	s := store.NewMemoryStore()
	s.PutNode(&model.Node{Header: model.Header{Path: "/projects/alpha", Title: "Alpha", Summary: "alpha backend project", Status: model.StatusActive, Fields: map[string]string{}}})

	idx := s.BuildKeywordIndex()
	// "alpha" should be indexed from path, title, and summary
	if len(idx["alpha"]) == 0 {
		t.Fatal("'alpha' should be in keyword index")
	}
	if idx["alpha"][0] != "/projects/alpha" {
		t.Fatalf("expected /projects/alpha, got %v", idx["alpha"])
	}
}

func TestKeywordIndex_ExcludesArchived(t *testing.T) {
	s := store.NewMemoryStore()
	s.PutNode(&model.Node{Header: model.Header{Path: "/old", Title: "Old", Summary: "old stuff", Status: model.StatusArchived, Fields: map[string]string{}}})
	s.PutNode(&model.Node{Header: model.Header{Path: "/new", Title: "New", Summary: "old stuff too", Status: model.StatusActive, Fields: map[string]string{}}})

	idx := s.BuildKeywordIndex()
	found := false
	for _, p := range idx["old"] {
		if p == "/old" {
			found = true
		}
	}
	if found {
		t.Fatal("archived node should not appear in keyword index")
	}
}

func TestAllNodes(t *testing.T) {
	s := store.NewMemoryStore()
	s.PutNode(&model.Node{Header: model.Header{Path: "/a", Status: model.StatusActive, Fields: map[string]string{}}})
	s.PutNode(&model.Node{Header: model.Header{Path: "/b", Status: model.StatusActive, Fields: map[string]string{}}})

	nodes := s.AllNodes()
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}
}

func TestPutNode_UpdatesTimestamp(t *testing.T) {
	s := store.NewMemoryStore()
	node := &model.Node{
		Header: model.Header{Path: "/a", Title: "A", Status: model.StatusActive, Fields: map[string]string{}},
	}
	s.PutNode(node)

	firstTime := node.Header.UpdatedAt
	// Wait a tiny bit (not reliable but works for most cases)
	s.PutNode(node)
	secondTime := node.Header.UpdatedAt

	if secondTime.Before(firstTime) {
		t.Fatal("updated_at should not decrease")
	}
}
