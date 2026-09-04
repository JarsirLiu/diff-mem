// Read operations: List, Search, Show, DeepLoad.
package engine

import (
	"sort"
	"strings"
	"time"

	"github.com/diff-mem/diff-mem/internal/model"
)

// Memory root directories (docs/02 §四). They are virtual: no node records
// exist for them, listing and links resolve them by prefix.
var memoryRoots = []string{"/short-term", "/long-term"}

func (e *Engine) List(path string, includeArchived bool) *model.ToolResponse {
	if path == "" || path == "/" {
		return e.listRoot(includeArchived)
	}
	return e.listChildren(path, includeArchived)
}

func (e *Engine) listRoot(includeArchived bool) *model.ToolResponse {
	// The two memory areas are always visible, even when empty.
	entries := make([]model.SearchResultEntry, 0, len(memoryRoots)+2)
	for _, root := range memoryRoots {
		entries = append(entries, model.SearchResultEntry{Path: root, Type: "group"})
	}
	for _, node := range e.store.AllNodes() {
		if node.Header.Status == model.StatusArchived && !includeArchived {
			continue
		}
		segs := strings.Split(strings.TrimPrefix(node.Header.Path, "/"), "/")
		// Depth-1 real nodes would shadow the memory roots — can only come
		// from legacy data created before the two-root rule; still list them.
		if len(segs) == 1 {
			entries = append(entries, model.SearchResultEntry{Path: node.Header.Path, Type: "node"})
		}
	}
	return success(map[string]interface{}{"path": "/", "children": entries, "has_more": false})
}

// listChildren lists the direct children of path. Works for both real nodes
// and virtual directories: children are found by prefix scan, and intermediate
// segments that only exist as prefixes are listed as virtual directories
// (type "group") — no parent node needs to exist.
func (e *Engine) listChildren(path string, includeArchived bool) *model.ToolResponse {
	prefix := strings.TrimSuffix(path, "/") + "/"
	base := strings.TrimSuffix(path, "/")

	seen := map[string]bool{}
	var entries []model.SearchResultEntry
	for _, node := range e.store.AllNodes() {
		if node.Header.Status == model.StatusArchived && !includeArchived {
			continue
		}
		if !strings.HasPrefix(node.Header.Path, prefix) {
			continue
		}
		rel := node.Header.Path[len(prefix):]
		if rel == "" {
			continue
		}
		if i := strings.Index(rel, "/"); i >= 0 {
			// Node sits deeper: its first segment is a virtual child directory.
			dirPath := base + "/" + rel[:i]
			if !seen[dirPath] {
				seen[dirPath] = true
				entries = append(entries, model.SearchResultEntry{Path: dirPath, Type: "group"})
			}
			continue
		}
		if !seen[node.Header.Path] {
			seen[node.Header.Path] = true
			entries = append(entries, model.SearchResultEntry{Path: node.Header.Path, Type: "node"})
		}
	}
	if len(entries) == 0 {
		if !e.store.Exists(path) {
			return fail("PATH_NOT_FOUND", "path not found: "+path, e.didYouMean(path))
		}
		// Real node with no children — an empty listing is correct.
		return success(map[string]interface{}{"path": path, "children": entries, "has_more": false})
	}
	// Deterministic output.
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	if len(entries) > 50 {
		return aggregateList(base, entries, func(e model.SearchResultEntry) string {
			rel := strings.TrimPrefix(e.Path, prefix)
			return firstRunes(rel, 1)
		})
	}
	return success(map[string]interface{}{"path": path, "children": entries, "has_more": false})
}

func aggregateList(base string, entries []model.SearchResultEntry, keyFn func(model.SearchResultEntry) string) *model.ToolResponse {
	counts := map[string]int{}
	for _, e := range entries {
		counts[keyFn(e)]++
	}
	aggregated := make([]model.SearchResultEntry, 0, len(counts))
	for prefix, count := range counts {
		label := prefix + "-*"
		if base != "" {
			label = base + "/" + label
		}
		aggregated = append(aggregated, model.SearchResultEntry{Path: label, Type: "group", Count: count})
	}
	// Deterministic output: groups sorted by label.
	sort.Slice(aggregated, func(i, j int) bool { return aggregated[i].Path < aggregated[j].Path })
	return success(map[string]interface{}{"path": base, "children": aggregated, "has_more": true})
}

// firstRunes returns the first n runes of s (UTF-8 safe byte slicing).
func firstRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func (e *Engine) Search(opts model.SearchOptions) *model.ToolResponse {
	if opts.Limit <= 0 {
		opts.Limit = 10
	}
	if opts.Limit > 50 {
		opts.Limit = 50
	}

	tagIdx := e.store.BuildTagIndex()
	results := map[string]*model.Node{}

	if len(opts.Tags) > 0 {
		for _, path := range tagIdx[opts.Tags[0]] {
			node, ok := e.store.GetNode(path)
			if !ok || node.Header.Status != model.StatusActive {
				continue
			}
			if tagMatch(node.Header.Tags, opts.Tags) {
				results[path] = node
			}
		}
	}

	if opts.Keywords != "" {
		kwIdx := e.store.BuildKeywordIndex()
		for _, kw := range strings.Fields(opts.Keywords) {
			for _, path := range kwIdx[kw] {
				if _, exists := results[path]; exists {
					continue
				}
				node, ok := e.store.GetNode(path)
				if ok && node.Header.Status == model.StatusActive {
					results[path] = node
				}
			}
		}
	}

	// Relevance scoring (docs/01 §2.2: "结果按 relevance 排序"):
	// exact tag match > keyword in path > keyword in title > keyword in summary.
	// Ties broken by path for deterministic output.
	type scored struct {
		node  *model.Node
		score int
	}
	list := make([]scored, 0, len(results))
	for _, node := range results {
		s := 0
		if len(opts.Tags) > 0 {
			s += 100
		}
		pathLower := strings.ToLower(node.Header.Path)
		titleLower := strings.ToLower(node.Header.Title)
		summaryLower := strings.ToLower(node.Header.Summary)
		for _, kw := range strings.Fields(opts.Keywords) {
			kw = strings.ToLower(kw)
			if strings.Contains(pathLower, kw) {
				s += 10
			}
			if strings.Contains(titleLower, kw) {
				s += 7
			}
			if strings.Contains(summaryLower, kw) {
				s += 5
			}
		}
		list = append(list, scored{node: node, score: s})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].score != list[j].score {
			return list[i].score > list[j].score
		}
		return list[i].node.Header.Path < list[j].node.Header.Path
	})

	total := len(list)
	if total > opts.Limit {
		list = list[:opts.Limit]
	}
	entries := make([]model.SearchResultEntry, 0, len(list))
	for _, it := range list {
		entries = append(entries, model.SearchResultEntry{
			Path: it.node.Header.Path, Type: "node",
			Summary: it.node.Header.Summary, Status: it.node.Header.Status, Tags: it.node.Header.Tags,
		})
	}
	return success(map[string]interface{}{"results": entries, "total": total})
}

func (e *Engine) Show(path string) *model.ToolResponse {
	node, ok := e.store.GetNode(path)
	if !ok {
		return fail("PATH_NOT_FOUND", "path not found: "+path)
	}
	// Track access for the agent's "last interaction" signal. This is the ONLY
	// field a read may touch — UpdatedAt and the event stream stay untouched.
	now := time.Now()
	node.Header.LastAccessed = &now
	e.store.PutNode(node)
	// Content links + backlinks: AI sees related entries and decides whether to look.
	result := model.ShowResult{Header: node.Header}
	result.Links = nodeContentLinks(node)
	result.Backlinks = e.findReferrers(path, path)
	return success(result)
}

func (e *Engine) DeepLoad(path string, window string) *model.ToolResponse {
	node, ok := e.store.GetNode(path)
	if !ok {
		return fail("PATH_NOT_FOUND", "path not found: "+path)
	}
	var events []model.Event
	switch window {
	case "recent":
		events = takeLast(node.Events, 5)
	case "last_10":
		events = takeLast(node.Events, 10)
	case "last_50":
		events = takeLast(node.Events, 50)
	case "last_100":
		events = takeLast(node.Events, 100)
	case "all":
		events = node.Events
	default:
		return fail("VALIDATION_FAILED", "invalid window: "+window)
	}
	return success(model.DeepLoadResult{
		Path: path, Events: events,
		Total: node.Header.EventCount, HasMore: node.Header.EventCount > len(events),
		Links: nodeContentLinks(node),
	})
}

func tagMatch(nodeTags, required []string) bool {
	for _, req := range required {
		found := false
		for _, t := range nodeTags {
			if t == req {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
