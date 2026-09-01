package graph_test

import (
	"testing"

	"github.com/diff-mem/diff-mem/internal/graph"
	"github.com/diff-mem/diff-mem/internal/model"
	"github.com/diff-mem/diff-mem/internal/store"
)

// helper creates active nodes in the store.
func addNodes(s *store.MemoryStore, paths []string) {
	for _, p := range paths {
		s.PutNode(&model.Node{
			Header: model.Header{Path: p, Status: model.StatusActive, Fields: map[string]string{}},
		})
	}
}

// --- Outbound Reachability ---

func TestOutboundReachable_Basic(t *testing.T) {
	s := store.NewMemoryStore()
	addNodes(s, []string{"/a", "/b", "/c"})
	s.AddEdge(model.Edge{From: "/a", To: "/b", Type: model.EdgeDependsOn})
	s.AddEdge(model.Edge{From: "/b", To: "/c", Type: model.EdgeReferences})

	g := graph.New(s)
	results := g.OutboundReachable("/a", 3)

	if len(results) != 2 {
		t.Fatalf("expected 2 reachable nodes, got %d", len(results))
	}
}

func TestOutboundReachable_MultiHop(t *testing.T) {
	s := store.NewMemoryStore()
	addNodes(s, []string{"/a", "/b", "/c", "/d"})
	s.AddEdge(model.Edge{From: "/a", To: "/b", Type: model.EdgeDependsOn})
	s.AddEdge(model.Edge{From: "/b", To: "/c", Type: model.EdgeDependsOn})
	s.AddEdge(model.Edge{From: "/c", To: "/d", Type: model.EdgeDependsOn})

	g := graph.New(s)
	results := g.OutboundReachable("/a", 3)

	distMap := map[string]int{}
	for _, r := range results {
		distMap[r.Path] = r.Distance
	}
	if distMap["/b"] != 1 || distMap["/c"] != 2 || distMap["/d"] != 3 {
		t.Fatalf("wrong distances: %v", distMap)
	}
}

func TestOutboundReachable_DistanceLimit(t *testing.T) {
	s := store.NewMemoryStore()
	addNodes(s, []string{"/a", "/b", "/c"})
	s.AddEdge(model.Edge{From: "/a", To: "/b", Type: model.EdgeDependsOn})
	s.AddEdge(model.Edge{From: "/b", To: "/c", Type: model.EdgeDependsOn})

	g := graph.New(s)
	results := g.OutboundReachable("/a", 1)

	if len(results) != 1 {
		t.Fatalf("expected 1 node within distance 1, got %d", len(results))
	}
	if results[0].Path != "/b" {
		t.Fatalf("expected /b, got %s", results[0].Path)
	}
}

func TestOutboundReachable_NoEdges(t *testing.T) {
	s := store.NewMemoryStore()
	addNodes(s, []string{"/a"})

	g := graph.New(s)
	results := g.OutboundReachable("/a", 3)

	if len(results) != 0 {
		t.Fatalf("expected 0 reachable nodes, got %d", len(results))
	}
}

func TestOutboundReachable_Disconnected(t *testing.T) {
	s := store.NewMemoryStore()
	addNodes(s, []string{"/a", "/b"})
	// No edge between /a and /b

	g := graph.New(s)
	results := g.OutboundReachable("/a", 3)

	if len(results) != 0 {
		t.Fatalf("expected 0 reachable nodes (no edge), got %d", len(results))
	}
}

func TestOutboundReachable_Directed(t *testing.T) {
	s := store.NewMemoryStore()
	addNodes(s, []string{"/a", "/b"})
	s.AddEdge(model.Edge{From: "/b", To: "/a", Type: model.EdgeDependsOn})

	g := graph.New(s)
	results := g.OutboundReachable("/a", 3)

	if len(results) != 0 {
		t.Fatalf("expected 0 (edge is reversed), got %d", len(results))
	}
}

// --- Inbound Reachability ---

func TestInboundReachable_Basic(t *testing.T) {
	s := store.NewMemoryStore()
	addNodes(s, []string{"/a", "/b", "/c"})
	s.AddEdge(model.Edge{From: "/a", To: "/b", Type: model.EdgeDependsOn})
	s.AddEdge(model.Edge{From: "/c", To: "/b", Type: model.EdgeReferences})

	g := graph.New(s)
	results := g.InboundReachable("/b", 3)

	if len(results) != 2 {
		t.Fatalf("expected 2 inbound nodes, got %d", len(results))
	}
}

func TestInboundReachable_MultiHop(t *testing.T) {
	s := store.NewMemoryStore()
	addNodes(s, []string{"/a", "/b", "/c"})
	s.AddEdge(model.Edge{From: "/a", To: "/b", Type: model.EdgeDependsOn})
	s.AddEdge(model.Edge{From: "/b", To: "/c", Type: model.EdgeDependsOn})

	g := graph.New(s)
	results := g.InboundReachable("/c", 3)

	distMap := map[string]int{}
	for _, r := range results {
		distMap[r.Path] = r.Distance
	}
	if distMap["/b"] != 1 || distMap["/a"] != 2 {
		t.Fatalf("wrong inbound distances: %v", distMap)
	}
}

// --- Cycle Detection ---

func TestDetectCycle_NoCycle(t *testing.T) {
	s := store.NewMemoryStore()
	addNodes(s, []string{"/a", "/b", "/c"})
	s.AddEdge(model.Edge{From: "/a", To: "/b", Type: model.EdgeDependsOn})
	s.AddEdge(model.Edge{From: "/b", To: "/c", Type: model.EdgeDependsOn})

	g := graph.New(s)
	// Adding c→b: existing edges are a→b, b→c. c→b creates no cycle
	// (there's no path from b back to c... wait, there is: b→c. So c→b would be a cycle b→c→b)
	// Let's test: adding a→c when edges are a→b, b→c. a→c creates no cycle.
	hasCycle, _ := g.DetectCycle("/a", "/c")
	if hasCycle {
		t.Fatal("a→c should not create cycle (no path from c back to a)")
	}
}

func TestDetectCycle_CycleDetected(t *testing.T) {
	s := store.NewMemoryStore()
	addNodes(s, []string{"/a", "/b", "/c"})
	s.AddEdge(model.Edge{From: "/a", To: "/b", Type: model.EdgeDependsOn})
	s.AddEdge(model.Edge{From: "/b", To: "/c", Type: model.EdgeDependsOn})
	// Adding c→a would create a cycle: a→b→c→a

	g := graph.New(s)
	hasCycle, cycle := g.DetectCycle("/c", "/a")
	if !hasCycle {
		t.Fatal("expected cycle detection")
	}
	if len(cycle) < 3 {
		t.Fatalf("expected cycle of length >= 3, got %v", cycle)
	}
}

func TestDetectCycle_SelfCycle(t *testing.T) {
	s := store.NewMemoryStore()
	addNodes(s, []string{"/a"})

	g := graph.New(s)
	hasCycle, cycle := g.DetectCycle("/a", "/a")
	if !hasCycle {
		t.Fatal("self-cycle should be detected")
	}
	if len(cycle) != 1 || cycle[0] != "/a" {
		t.Fatalf("expected self-cycle [\"/a\"], got %v", cycle)
	}
}

func TestDetectCycle_DirectReverse(t *testing.T) {
	s := store.NewMemoryStore()
	addNodes(s, []string{"/a", "/b"})
	s.AddEdge(model.Edge{From: "/a", To: "/b", Type: model.EdgeDependsOn})
	// Adding b→a would create cycle: a→b→a

	g := graph.New(s)
	hasCycle, _ := g.DetectCycle("/b", "/a")
	if !hasCycle {
		t.Fatal("expected cycle for reverse edge")
	}
}

func TestDetectCycle_NoCycle_Disconnected(t *testing.T) {
	s := store.NewMemoryStore()
	addNodes(s, []string{"/a", "/b"})
	// No edges at all

	g := graph.New(s)
	hasCycle, _ := g.DetectCycle("/a", "/b")
	if hasCycle {
		t.Fatal("no edges → no cycle possible")
	}
}

// --- Connected Components ---

func TestConnectedComponents_Connected(t *testing.T) {
	s := store.NewMemoryStore()
	addNodes(s, []string{"/a", "/b", "/c"})
	s.AddEdge(model.Edge{From: "/a", To: "/b", Type: model.EdgeDependsOn})
	s.AddEdge(model.Edge{From: "/b", To: "/c", Type: model.EdgeDependsOn})

	g := graph.New(s)
	components := g.ConnectedComponents()

	if len(components) != 1 {
		t.Fatalf("expected 1 component, got %d", len(components))
	}
	if len(components[0]) != 3 {
		t.Fatalf("expected component size 3, got %d", len(components[0]))
	}
}

func TestConnectedComponents_Disconnected(t *testing.T) {
	s := store.NewMemoryStore()
	addNodes(s, []string{"/a", "/b", "/c", "/d"})
	s.AddEdge(model.Edge{From: "/a", To: "/b", Type: model.EdgeDependsOn})
	// /c and /d are isolated

	g := graph.New(s)
	components := g.ConnectedComponents()

	// Should have 3 components: {a,b}, {c}, {d}
	componentSizes := make([]int, len(components))
	for i, c := range components {
		componentSizes[i] = len(c)
	}

	found := map[int]int{}
	for _, sz := range componentSizes {
		found[sz]++
	}
	if found[2] != 1 || found[1] != 2 {
		t.Fatalf("expected sizes {2, 1, 1}, got %v", found)
	}
}

func TestConnectedComponents_ExcludesArchived(t *testing.T) {
	s := store.NewMemoryStore()
	addNodes(s, []string{"/a", "/b"})
	s.AddEdge(model.Edge{From: "/a", To: "/b", Type: model.EdgeDependsOn})
	// Archive /a
	node, _ := s.GetNode("/a")
	node.Header.Status = model.StatusArchived
	s.PutNode(node)

	g := graph.New(s)
	components := g.ConnectedComponents()

	foundA := false
	for _, c := range components {
		for _, p := range c {
			if p == "/a" {
				foundA = true
			}
		}
	}
	if foundA {
		t.Fatal("archived node should be excluded from components")
	}
}

// --- Orphans ---

func TestOrphans_Empty(t *testing.T) {
	s := store.NewMemoryStore()
	addNodes(s, []string{"/a", "/b"})
	s.AddEdge(model.Edge{From: "/a", To: "/b", Type: model.EdgeDependsOn})

	g := graph.New(s)
	orphans := g.Orphans()

	if len(orphans) != 0 {
		t.Fatalf("expected 0 orphans, got %v", orphans)
	}
}

func TestOrphans_HasOrphans(t *testing.T) {
	s := store.NewMemoryStore()
	addNodes(s, []string{"/a", "/b", "/c"})
	s.AddEdge(model.Edge{From: "/a", To: "/b", Type: model.EdgeDependsOn})
	// /c has no edges

	g := graph.New(s)
	orphans := g.Orphans()

	if len(orphans) != 1 || orphans[0] != "/c" {
		t.Fatalf("expected orphan /c, got %v", orphans)
	}
}

func TestOrphans_AllOrphans(t *testing.T) {
	s := store.NewMemoryStore()
	addNodes(s, []string{"/a", "/b"})
	// No edges at all

	g := graph.New(s)
	orphans := g.Orphans()

	if len(orphans) != 2 {
		t.Fatalf("expected 2 orphans, got %d", len(orphans))
	}
}

// --- Entity Extraction ---

func TestExtractEntities_Numbers(t *testing.T) {
	entities := graph.ExtractEntities("2026-09-30")
	found := false
	for _, e := range entities {
		if e == "2026-09-30" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected '2026-09-30' in entities, got %v", entities)
	}
}

func TestExtractEntities_ChineseNames(t *testing.T) {
	entities := graph.ExtractEntities("张三")
	found := false
	for _, e := range entities {
		if e == "张三" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected '张三' in entities, got %v", entities)
	}
}

func TestExtractEntities_ChineseNamesWithDelimiters(t *testing.T) {
	entities := graph.ExtractEntities("负责人：张三")
	found := false
	for _, e := range entities {
		if e == "张三" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected '张三' in entities, got %v", entities)
	}
}

func TestExtractEntities_NoEntities(t *testing.T) {
	entities := graph.ExtractEntities("")
	if len(entities) != 0 {
		t.Fatalf("empty string should produce no entities, got %v", entities)
	}
}

func TestExtractEntities_Deduplication(t *testing.T) {
	entities := graph.ExtractEntities("2026 2026 2026")
	count := 0
	for _, e := range entities {
		if e == "2026" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected deduplicated '2026', got %d occurrences", count)
	}
}

func TestDisappearedEntities(t *testing.T) {
	old := "张三负责，deadline 2026-09-30"
	new := "李四负责，deadline 2026-10-15"

	disappeared := graph.DisappearedEntities(old, new)
	disMap := map[string]bool{}
	for _, e := range disappeared {
		disMap[e] = true
	}
	if !disMap["2026-09-30"] {
		t.Fatalf("expected '2026-09-30' disappeared, got %v", disappeared)
	}
	if disMap["2026-10-15"] {
		t.Fatalf("'2026-10-15' should not be disappeared (it's in new), got %v", disappeared)
	}
}

func TestDisappearedEntities_None(t *testing.T) {
	old := "张三负责"
	new := "张三负责，项目完成"

	disappeared := graph.DisappearedEntities(old, new)
	if len(disappeared) != 0 {
		t.Fatalf("expected no disappeared entities, got %v", disappeared)
	}
}

func TestDisappearedEntities_AllGone(t *testing.T) {
	old := "2026-09-30 项目"
	new := "项目继续"

	disappeared := graph.DisappearedEntities(old, new)
	disMap := map[string]bool{}
	for _, e := range disappeared {
		disMap[e] = true
	}
	if !disMap["2026-09-30"] {
		t.Fatalf("expected '2026-09-30' disappeared, got %v", disappeared)
	}
}
