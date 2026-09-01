// Package graph implements the memory graph operations on top of the store.
package graph

import (
	"strings"

	"github.com/diff-mem/diff-mem/internal/model"
	"github.com/diff-mem/diff-mem/internal/store"
)

// Graph provides graph algorithms over the memory node relationship network.
type Graph struct {
	store store.Store
}

func New(s store.Store) *Graph {
	return &Graph{store: s}
}

// BFSResult holds one node found during traversal.
type BFSResult struct {
	Path     string
	Type     model.EdgeType
	Distance int
}

// --- Reachability (outbound BFS) ---

// OutboundReachable returns all active nodes reachable from path via outbound edges.
func (g *Graph) OutboundReachable(path string, maxDist int) []BFSResult {
	results := []BFSResult{}
	visited := map[string]bool{path: true}
	queue := []struct{ path, type_ string; dist int }{{path, "", 0}}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if current.dist >= maxDist {
			continue
		}

		edges := g.store.GetOutboundEdges(current.path)
		for _, e := range edges {
			if visited[e.To] {
				continue
			}
			visited[e.To] = true
			_, exists := g.store.GetNode(e.To)
			if !exists {
				continue
			}
			results = append(results, BFSResult{
				Path:     e.To,
				Type:     e.Type,
				Distance: current.dist + 1,
			})
			queue = append(queue, struct{ path, type_ string; dist int }{e.To, "", current.dist + 1})
		}
	}
	return results
}

// --- Inbound Reachability (reverse BFS) ---

// InboundReachable returns all active nodes that can reach path via their outbound edges.
func (g *Graph) InboundReachable(path string, maxDist int) []BFSResult {
	results := []BFSResult{}
	visited := map[string]bool{path: true}
	queue := []struct{ path string; dist int }{{path, 0}}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if current.dist >= maxDist {
			continue
		}

		edges := g.store.GetInboundEdges(current.path)
		for _, e := range edges {
			if visited[e.From] {
				continue
			}
			visited[e.From] = true
			_, exists := g.store.GetNode(e.From)
			if !exists {
				continue
			}
			results = append(results, BFSResult{
				Path:     e.From,
				Type:     e.Type,
				Distance: current.dist + 1,
			})
			queue = append(queue, struct{ path string; dist int }{e.From, current.dist + 1})
		}
	}
	return results
}

// --- Cycle Detection ---

// DetectCycle checks if adding edge (from → to) would create a cycle.
// A cycle exists iff `from` is reachable from `to` via existing edges
// (i.e., there's already a path to→...→from, and the new edge from→to closes it).
func (g *Graph) DetectCycle(from, to string) (bool, []string) {
	if from == to {
		return true, []string{from}
	}

	// BFS from `to`, looking for `from`
	visited := map[string]bool{to: true}
	parent := map[string]string{}
	queue := []string{to}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		edges := g.store.GetOutboundEdges(current)
		for _, e := range edges {
			if e.To == from {
				// from is reachable from to → cycle
				cycle := buildPath(parent, from, to, current)
				return true, cycle
			}
			if !visited[e.To] {
				visited[e.To] = true
				parent[e.To] = current
				queue = append(queue, e.To)
			}
		}
	}
	return false, nil
}

func buildPath(parent map[string]string, from, to, last string) []string {
	// Build path: to → ... → last → from → to (the new edge)
	path := []string{to}
	node := last
	for node != to {
		path = append(path, node)
		if parent[node] == "" {
			break
		}
		node = parent[node]
	}
	path = append(path, from)
	// Reverse
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}

// --- Union-Find for Connected Components ---

// UnionFind implements union-find with path compression and union by rank.
type UnionFind struct {
	parent map[string]string
	rank   map[string]int
}

func NewUnionFind(paths []string) *UnionFind {
	uf := &UnionFind{
		parent: make(map[string]string),
		rank:   make(map[string]int),
	}
	for _, p := range paths {
		uf.parent[p] = p
		uf.rank[p] = 0
	}
	return uf
}

func (uf *UnionFind) Find(x string) string {
	if uf.parent[x] != x {
		uf.parent[x] = uf.Find(uf.parent[x])
	}
	return uf.parent[x]
}

func (uf *UnionFind) Union(x, y string) {
	rootX := uf.Find(x)
	rootY := uf.Find(y)
	if rootX == rootY {
		return
	}
	if uf.rank[rootX] < uf.rank[rootY] {
		uf.parent[rootX] = rootY
	} else if uf.rank[rootX] > uf.rank[rootY] {
		uf.parent[rootY] = rootX
	} else {
		uf.parent[rootY] = rootX
		uf.rank[rootX]++
	}
}

// ConnectedComponents returns all connected components as groups of paths.
func (g *Graph) ConnectedComponents() [][]string {
	allPaths := []string{}
	for _, node := range g.store.AllNodes() {
		if node.Header.Status == model.StatusArchived {
			continue
		}
		allPaths = append(allPaths, node.Header.Path)
	}

	uf := NewUnionFind(allPaths)

	// Union all connected pairs
	for _, node := range g.store.AllNodes() {
		if node.Header.Status == model.StatusArchived {
			continue
		}
		for _, e := range g.store.GetOutboundEdges(node.Header.Path) {
			uf.Union(node.Header.Path, e.To)
		}
	}

	// Group by root
	groups := map[string][]string{}
	for _, p := range allPaths {
		root := uf.Find(p)
		groups[root] = append(groups[root], p)
	}

	result := make([][]string, 0, len(groups))
	for _, g := range groups {
		result = append(result, g)
	}
	return result
}

// Orphans returns nodes with no inbound or outbound edges.
func (g *Graph) Orphans() []string {
	var orphans []string
	for _, node := range g.store.AllNodes() {
		if node.Header.Status == model.StatusArchived {
			continue
		}
		if len(g.store.GetOutboundEdges(node.Header.Path)) == 0 &&
			len(g.store.GetInboundEdges(node.Header.Path)) == 0 {
			orphans = append(orphans, node.Header.Path)
		}
	}
	return orphans
}

// --- Summary entity extraction (for drift detection) ---

// ExtractEntities extracts key entities from a summary string.
// Returns names, numbers, dates, and proper nouns.
func ExtractEntities(summary string) []string {
	entities := []string{}
	// Extract numbers
	fieldScan(summary, func(r rune) bool { return r >= '0' && r <= '9' }, func(s string) {
		entities = append(entities, s)
	})
	// Extract Chinese names (2-4 Chinese chars)
	fieldScan(summary, func(r rune) bool { return r >= 0x4e00 && r <= 0x9fff }, func(s string) {
		if len([]rune(s)) >= 2 && len([]rune(s)) <= 4 {
			entities = append(entities, s)
		}
	})
	// Extract date-like patterns (YYYY-MM-DD or M月D日)
	fieldScan(summary, func(r rune) bool {
		return (r >= '0' && r <= '9') || r == '-' || r == '/' || r == '月' || r == '日'
	}, func(s string) {
		if len(s) >= 3 {
			entities = append(entities, s)
		}
	})
	// Deduplicate
	seen := map[string]bool{}
	deduped := []string{}
	for _, e := range entities {
		if !seen[e] {
			seen[e] = true
			deduped = append(deduped, e)
		}
	}
	return deduped
}

// fieldScan scans a string collecting contiguous runs of runes matching predicate.
func fieldScan(s string, predicate func(rune) bool, onField func(string)) {
	runes := []rune(s)
	field := ""
	for _, r := range runes {
		if predicate(r) {
			field += string(r)
		} else {
			if field != "" {
				onField(field)
				field = ""
			}
		}
	}
	if field != "" {
		onField(field)
	}
}

// DisappearedEntities returns entities in old that are not in new.
func DisappearedEntities(oldSummary, newSummary string) []string {
	oldEntities := ExtractEntities(oldSummary)
	newEntities := ExtractEntities(newSummary)
	newSet := map[string]bool{}
	for _, e := range newEntities {
		newSet[e] = true
	}
	var disappeared []string
	for _, e := range oldEntities {
		if !newSet[e] && !strings.Contains(newSummary, e) {
			disappeared = append(disappeared, e)
		}
	}
	return disappeared
}
