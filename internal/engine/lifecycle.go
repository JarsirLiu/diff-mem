// Lifecycle operations: Delete, Archive, Restore.
package engine

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/diff-mem/diff-mem/internal/model"
	"github.com/diff-mem/diff-mem/internal/validator"
)

func (e *Engine) Delete(opts model.ArchiveOptions) *model.ToolResponse {
	if err := validator.ValidateArchive(opts); err != "" {
		return fail("VALIDATION_FAILED", err)
	}
	if !e.store.Exists(opts.Path) {
		return fail("PATH_NOT_FOUND", "path not found: "+opts.Path)
	}
	inbound := e.graph.InboundReachable(opts.Path, 1)
	if len(inbound) > 0 {
		paths := make([]string, len(inbound))
		for i, n := range inbound {
			paths[i] = n.Path
		}
		return fail("ACTIVE_DEPENDENCIES", "node is referenced by active nodes: "+strings.Join(paths, ", "))
	}
	e.store.DeleteNode(opts.Path)
	return success(map[string]string{"deleted": opts.Path, "reason": opts.Reason})
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

	impacted := e.graph.OutboundReachable(opts.Path, 3)
	var impactedNodes []model.ImpactedNode
	for _, r := range impacted {
		impactedNodes = append(impactedNodes, model.ImpactedNode{
			Path: r.Path, Type: r.Type, Distance: r.Distance,
		})
	}

	node.Header.Status = model.StatusArchived
	node.Header.UpdatedAt = time.Now()
	node.Events = append(node.Events, model.Event{
		ID: uuid.NewString(), Type: "archived",
		Content: "node archived",
		Meta: map[string]string{"reason": opts.Reason}, Timestamp: time.Now(),
	})
	node.Header.EventCount = len(node.Events)
	e.store.PutNode(node)

	resp := success(node.Header)
	resp.Impacted = impactedNodes
	if len(impactedNodes) > 0 {
		resp.Warning = "归档此节点会影响 " + idx(len(impactedNodes)) + " 个下游节点"
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
		Meta: map[string]string{"reason": opts.Reason}, Timestamp: time.Now(),
	})
	node.Header.EventCount = len(node.Events)
	e.store.PutNode(node)
	return success(node.Header)
}
