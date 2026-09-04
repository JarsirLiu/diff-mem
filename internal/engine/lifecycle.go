// Lifecycle operations: Delete, Archive, Restore.
package engine

import (
	"strings"
	"time"

	"github.com/diff-mem/diff-mem/internal/model"
	"github.com/diff-mem/diff-mem/internal/validator"
	"github.com/google/uuid"
)

func (e *Engine) Delete(opts model.ArchiveOptions) *model.ToolResponse {
	if err := validator.ValidateArchive(opts); err != "" {
		return fail("VALIDATION_FAILED", err)
	}
	if !e.store.Exists(opts.Path) {
		return fail("PATH_NOT_FOUND", "path not found: "+opts.Path)
	}
	// Cascade: the node and all its descendants are removed (docs/01 §3.4).
	prefix := opts.Path + "/"
	victims := []string{opts.Path}
	for _, n := range e.store.AllNodes() {
		if strings.HasPrefix(n.Header.Path, prefix) {
			victims = append(victims, n.Header.Path)
		}
	}
	// Gate: active nodes OUTSIDE the victim set linking to any victim would
	// leave dangling links — reject and point at the referrers.
	victimSet := make(map[string]bool, len(victims))
	for _, v := range victims {
		victimSet[v] = true
	}
	var referrers []string
	seenRef := make(map[string]bool)
	for _, v := range victims {
		for _, r := range e.findReferrers(v, v) {
			if victimSet[r] || seenRef[r] {
				continue
			}
			seenRef[r] = true
			referrers = append(referrers, r)
		}
	}
	if len(referrers) > 0 {
		return fail("LINKED_BY_OTHERS",
			"node body is linked by active nodes: "+strings.Join(referrers, ", "),
			"先更新这些节点的 Body，移除或修正 [[链接]] 后再删除")
	}
	for _, v := range victims {
		e.store.DeleteNode(v)
		e.links.removeNode(v)
	}
	return success(map[string]interface{}{"deleted": opts.Path, "removed": victims, "reason": opts.Reason})
}

func (e *Engine) Archive(opts model.ArchiveOptions) *model.ToolResponse {
	if err := validator.ValidateArchive(opts); err != "" {
		return fail("VALIDATION_FAILED", err)
	}
	node, ok := e.store.GetNode(opts.Path)
	if !ok {
		return fail("PATH_NOT_FOUND", "path not found: "+opts.Path)
	}
	if node.Header.Status == model.StatusArchived {
		return fail("NODE_ALREADY_ARCHIVED", "node is already archived")
	}

	node.Header.Status = model.StatusArchived
	node.Header.UpdatedAt = time.Now()
	node.Events = append(node.Events, model.Event{
		ID: uuid.NewString(), Type: "archived",
		Content: "node archived",
		Meta:    map[string]string{"reason": opts.Reason}, Timestamp: time.Now(),
	})
	node.Header.EventCount = len(node.Events)
	e.store.PutNode(node)

	resp := success(node.Header)
	// Warning: inbound links will dangle while archived. Give the AI explicit
	// options for handling them (modeled after deepseek-harness archive rules).
	if referrers := e.findReferrers(opts.Path, opts.Path); len(referrers) > 0 {
		resp.Warning = "以下活跃节点的 Body 链接了此节点，归档期间这些链接不可解析：" + strings.Join(referrers, ", ") +
			"。请处置这些 [[链接]]：1) 若有新的承接节点，将链接改指到它；" +
			"2) 若有意引用历史快照，可保留链接（归档节点仍是有效链接目标）；" +
			"3) 若链接已无意义，更新对应节点的 Body 移除该链接"
	}
	return resp
}

func (e *Engine) Restore(opts model.ArchiveOptions) *model.ToolResponse {
	if err := validator.ValidateArchive(opts); err != "" {
		return fail("VALIDATION_FAILED", err)
	}
	node, ok := e.store.GetNode(opts.Path)
	if !ok {
		return fail("PATH_NOT_FOUND", "path not found: "+opts.Path)
	}
	if node.Header.Status != model.StatusArchived {
		return fail("NODE_NOT_ARCHIVED", "node is not archived, cannot restore")
	}

	node.Header.Status = model.StatusActive
	node.Header.UpdatedAt = time.Now()
	node.Events = append(node.Events, model.Event{
		ID: uuid.NewString(), Type: "restored",
		Content: "node restored",
		Meta:    map[string]string{"reason": opts.Reason}, Timestamp: time.Now(),
	})
	node.Header.EventCount = len(node.Events)
	e.store.PutNode(node)

	resp := success(node.Header)
	// Warning: outbound [[links]] in the body may dangle if their targets were
	// deleted (or archived) while this node was archived.
	if dangling := e.danglingOutboundLinks(node); len(dangling) > 0 {
		resp.Warning = "本节点的 Body 中以下 [[链接]] 目标当前不存在：" + strings.Join(dangling, ", ") +
			"。请更新 Body 修正或移除这些链接"
	}
	return resp
}

// danglingOutboundLinks returns the node's content links whose targets no
// longer exist or are archived (unresolvable while archived).
func (e *Engine) danglingOutboundLinks(node *model.Node) []string {
	var dangling []string
	for _, target := range nodeContentLinks(node) {
		if target == node.Header.Path {
			continue
		}
		t, ok := e.store.GetNode(target)
		if !ok || t.Header.Status != model.StatusActive {
			dangling = append(dangling, target)
		}
	}
	return dangling
}
