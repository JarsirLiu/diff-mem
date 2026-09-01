// Graph operations: Link, Unlink.
package engine

import (
	"github.com/diff-mem/diff-mem/internal/model"
	"github.com/diff-mem/diff-mem/internal/validator"
)

func (e *Engine) Link(opts model.LinkOptions) *model.ToolResponse {
	if err := validator.ValidateLink(opts); err != "" {
		return fail("VALIDATION_FAILED", err)
	}
	if !e.store.Exists(opts.From) {
		return fail("PATH_NOT_FOUND", "from path not found: "+opts.From)
	}
	if !e.store.Exists(opts.To) {
		return fail("PATH_NOT_FOUND", "to path not found: "+opts.To)
	}
	if e.store.HasEdge(opts.From, opts.To) {
		return fail("EDGE_EXISTS", "edge already exists: "+opts.From+" -> "+opts.To)
	}

	if opts.Type == model.EdgeDependsOn || opts.Type == model.EdgeSupersedes {
		hasCycle, cycle := e.graph.DetectCycle(opts.From, opts.To)
		if hasCycle {
			return &model.ToolResponse{
				Success: false,
				Error: &model.ErrorInfo{
					Code:    "CYCLE_DETECTED",
					Message: "establishing this link would create a cycle",
					Cycle:   cycle,
				},
			}
		}
	}

	edge := model.Edge{From: opts.From, To: opts.To, Type: opts.Type}
	e.store.AddEdge(edge)
	return success(edge)
}

func (e *Engine) Unlink(from, to string) *model.ToolResponse {
	if !e.store.HasEdge(from, to) {
		return fail("EDGE_NOT_FOUND", "edge not found: "+from+" -> "+to)
	}
	e.store.RemoveEdge(from, to)
	return success(map[string]string{"removed": from + " -> " + to})
}
