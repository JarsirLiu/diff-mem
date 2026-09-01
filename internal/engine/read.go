// Read operations: List, Search, Show, DeepLoad.
package engine

import (
	"strings"

	"github.com/diff-mem/diff-mem/internal/model"
)

func (e *Engine) List(path string, includeArchived bool) *model.ToolResponse {
	if path == "" || path == "/" {
		return e.listRoot(includeArchived)
	}
	if !e.store.Exists(path) {
		return fail("PATH_NOT_FOUND", "path not found: "+path)
	}
	return e.listChildren(path, includeArchived)
}

func (e *Engine) listRoot(includeArchived bool) *model.ToolResponse {
	var entries []model.SearchResultEntry
	for _, node := range e.store.AllNodes() {
		if node.Header.Status == model.StatusArchived && !includeArchived {
			continue
		}
		segs := strings.Split(strings.TrimPrefix(node.Header.Path, "/"), "/")
		if len(segs) > 1 {
			continue
		}
		entries = append(entries, model.SearchResultEntry{Path: node.Header.Path + "/", Type: "node"})
	}
	if len(entries) > 50 {
		return aggregateList("", entries, func(e model.SearchResultEntry) string { return e.Path[:2] })
	}
	return success(map[string]interface{}{"path": "/", "children": entries, "has_more": false})
}

func (e *Engine) listChildren(path string, includeArchived bool) *model.ToolResponse {
	var entries []model.SearchResultEntry
	prefix := path + "/"
	for _, node := range e.store.AllNodes() {
		if node.Header.Status == model.StatusArchived && !includeArchived {
			continue
		}
		if !strings.HasPrefix(node.Header.Path, prefix) {
			continue
		}
		childPath := node.Header.Path[len(prefix):]
		if strings.Contains(childPath, "/") {
			continue
		}
		entries = append(entries, model.SearchResultEntry{Path: node.Header.Path, Type: "node"})
	}
	if len(entries) > 50 {
		return aggregateList(path, entries, func(e model.SearchResultEntry) string {
			rel := strings.TrimPrefix(e.Path, prefix)
			return rel[:1]
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
	return success(map[string]interface{}{"path": base, "children": aggregated, "has_more": true})
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

	entries := make([]model.SearchResultEntry, 0, len(results))
	for _, node := range results {
		entries = append(entries, model.SearchResultEntry{
			Path: node.Header.Path, Type: "node",
			Summary: node.Header.Summary, Status: node.Header.Status, Tags: node.Header.Tags,
		})
	}
	if len(entries) > opts.Limit {
		entries = entries[:opts.Limit]
	}
	return success(map[string]interface{}{"results": entries, "total": len(entries)})
}

func (e *Engine) Show(path string) *model.ToolResponse {
	node, ok := e.store.GetNode(path)
	if !ok {
		return fail("PATH_NOT_FOUND", "path not found: "+path)
	}
	e.store.PutNode(node)
	return success(node.Header)
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
