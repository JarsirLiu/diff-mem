// Content links: [[/path]] references inside Body event text, with write-side gating.
//
// Links are content, not metadata: they live in event text, the engine validates
// them on write (dangling target → reject), and surfaces them on read.
//
// Backlinks are served by an in-memory inverted index (target → referrers),
// maintained on writes and rebuilt from storage on startup.
package engine

import (
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/diff-mem/diff-mem/internal/model"
)

// linkRe matches content links of the form [[/path/to/node]].
var linkRe = regexp.MustCompile(`\[\[([^\[\]\n]+)\]\]`)

// ExtractContentLinks returns deduplicated memory paths referenced via [[path]].
func ExtractContentLinks(content string) []string {
	matches := linkRe.FindAllStringSubmatch(content, -1)
	seen := map[string]bool{}
	var links []string
	for _, m := range matches {
		target := strings.TrimSpace(m[1])
		if target == "" || seen[target] {
			continue
		}
		seen[target] = true
		links = append(links, target)
	}
	return links
}

// nodeContentLinks returns the union of links across all events of a node.
func nodeContentLinks(node *model.Node) []string {
	seen := map[string]bool{}
	var links []string
	for _, ev := range node.Events {
		for _, l := range ExtractContentLinks(ev.Content) {
			if !seen[l] {
				seen[l] = true
				links = append(links, l)
			}
		}
	}
	return links
}

// isSelfOrAncestor reports whether target is the node itself or one of its
// ancestor paths (which get auto-created during Create).
func isSelfOrAncestor(target, self string) bool {
	return target == self || strings.HasPrefix(self, strings.TrimSuffix(target, "/")+"/")
}

// validateContentLinks is the write-side gate: every [[path]] in content must
// point to an existing node (or the node being created itself). Zero side effects on failure.
func (e *Engine) validateContentLinks(self string, contents ...string) *model.ToolResponse {
	seen := map[string]bool{}
	for _, content := range contents {
		for _, target := range ExtractContentLinks(content) {
			if seen[target] {
				continue
			}
			seen[target] = true

			if !strings.HasPrefix(target, "/") {
				return fail("LINK_TARGET_INVALID",
					"content link [["+target+"]] must be a memory path starting with /",
					"内容链接格式：[[/path/to/node]]")
			}
			if isSelfOrAncestor(target, self) || e.store.Exists(target) {
				continue
			}
			return fail("LINK_TARGET_NOT_FOUND",
				"content link target does not exist: "+target,
				e.didYouMean(target))
		}
	}
	return nil
}

// didYouMean suggests existing nodes similar to the given path (up to 3 hits,
// matched by shared last segment).
func (e *Engine) didYouMean(target string) string {
	lastSeg := target
	if i := strings.LastIndex(target, "/"); i >= 0 {
		lastSeg = target[i+1:]
	}
	if lastSeg == "" {
		return ""
	}
	var hits []string
	for _, node := range e.store.AllNodes() {
		if strings.Contains(node.Header.Path, lastSeg) {
			hits = append(hits, node.Header.Path)
			if len(hits) == 3 {
				break
			}
		}
	}
	if len(hits) == 0 {
		return ""
	}
	return "你可能想链接：" + strings.Join(hits, "、")
}

// linkIndex is the in-memory inverted index for content links:
// outbound[target] = set of nodes whose Body links to target,
// inbound[referrer] = set of targets the referrer links to.
type linkIndex struct {
	mu       sync.RWMutex
	outbound map[string]map[string]bool
	inbound  map[string]map[string]bool
}

func newLinkIndex() *linkIndex {
	return &linkIndex{
		outbound: make(map[string]map[string]bool),
		inbound:  make(map[string]map[string]bool),
	}
}

// addLinks registers all links of a node. Idempotent: safe to call again
// after appending more events.
func (idx *linkIndex) addLinks(referrer string, targets []string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	for _, t := range targets {
		if idx.outbound[referrer] == nil {
			idx.outbound[referrer] = make(map[string]bool)
		}
		if idx.inbound[t] == nil {
			idx.inbound[t] = make(map[string]bool)
		}
		idx.outbound[referrer][t] = true
		idx.inbound[t][referrer] = true
	}
}

// removeNode drops a node's outbound links and clears it as a link target.
func (idx *linkIndex) removeNode(path string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	for t := range idx.outbound[path] {
		delete(idx.inbound[t], path)
		if len(idx.inbound[t]) == 0 {
			delete(idx.inbound, t)
		}
	}
	delete(idx.outbound, path)
	delete(idx.inbound, path)
}

// referrersOf returns all nodes linking to target, sorted for stable output.
func (idx *linkIndex) referrersOf(target string) []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	refs := make([]string, 0, len(idx.inbound[target]))
	for r := range idx.inbound[target] {
		refs = append(refs, r)
	}
	sort.Strings(refs)
	return refs
}

// findReferrers returns active nodes whose Body links to target (excluding
// target itself), served by the inverted index.
func (e *Engine) findReferrers(target string, exclude string) []string {
	var referrers []string
	for _, ref := range e.links.referrersOf(target) {
		if ref == exclude {
			continue
		}
		if node, ok := e.store.GetNode(ref); ok && node.Header.Status == model.StatusActive {
			referrers = append(referrers, ref)
		}
	}
	return referrers
}
