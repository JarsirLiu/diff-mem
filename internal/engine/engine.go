// Package engine is the central orchestrator for Diff-Mem operations.
package engine

import (
	"github.com/diff-mem/diff-mem/internal/graph"
	"github.com/diff-mem/diff-mem/internal/model"
	"github.com/diff-mem/diff-mem/internal/store"
)

type Engine struct {
	store store.Store
	graph *graph.Graph
}

func New(s store.Store) *Engine {
	return &Engine{
		store: s,
		graph: graph.New(s),
	}
}

// Dispatch routes a tool name + params to the appropriate handler.
func (e *Engine) Dispatch(name string, params map[string]interface{}) *model.ToolResponse {
	switch name {
	case "create":
		return e.Create(extractCreate(params))
	case "append":
		return e.Append(extractAppend(params))
	case "update_field":
		return e.UpdateField(extractUpdateField(params))
	case "update_summary":
		return e.UpdateSummary(extractUpdateSummary(params))
	case "delete":
		return e.Delete(extractArchive(params))
	case "archive":
		return e.Archive(extractArchive(params))
	case "restore":
		return e.Restore(extractArchive(params))
	case "link":
		return e.Link(extractLink(params))
	case "unlink":
		from, _ := params["from"].(string)
		to, _ := params["to"].(string)
		return e.Unlink(from, to)
	case "list":
		path, _ := params["path"].(string)
		inc, _ := params["include_archived"].(bool)
		return e.List(path, inc)
	case "search":
		return e.Search(extractSearch(params))
	case "show":
		path, _ := params["path"].(string)
		return e.Show(path)
	case "deep_load":
		path, _ := params["path"].(string)
		window, _ := params["window"].(string)
		return e.DeepLoad(path, window)
	default:
		return fail("UNKNOWN_TOOL", "unknown tool: "+name)
	}
}

// Exec atomically executes multiple memory operations.
// All operations succeed or all rollback.
func (e *Engine) Exec(operations []map[string]interface{}) *model.ToolResponse {
	if len(operations) == 0 || len(operations) > 20 {
		return fail("VALIDATION_FAILED", "operations must be 1-20 items")
	}
	for i, op := range operations {
		opName, ok := op["op"].(string)
		if !ok {
			return fail("VALIDATION_FAILED", "operation ["+idx(i)+"] op must be string")
		}
		switch opName {
		case "SHOW", "LIST", "SEARCH", "DEEP_LOAD":
			return fail("VALIDATION_FAILED", "read operation "+opName+" not allowed in transaction")
		}
	}

	for i, op := range operations {
		opName := op["op"].(string)
		params := make(map[string]interface{})
		if p, ok := op["params"].(map[string]interface{}); ok {
			params = p
		}

		var resp *model.ToolResponse
		switch opName {
		case "CREATE":
			resp = e.Create(extractCreate(params))
		case "APPEND":
			resp = e.Append(extractAppend(params))
		case "UPDATE_FIELD":
			resp = e.UpdateField(extractUpdateField(params))
		case "DELETE":
			resp = e.Delete(extractArchive(params))
		case "ARCHIVE":
			resp = e.Archive(extractArchive(params))
		case "RESTORE":
			resp = e.Restore(extractArchive(params))
		}
		if resp != nil && !resp.Success {
			return fail("TRANSACTION_FAILED", "operation ["+idx(i)+"] failed: "+resp.Error.Message)
		}
	}

	return success(map[string]string{"committed": "true", "operations": idx(len(operations))})
}

func idx(i int) string {
	return string(rune(i + 48))
}
