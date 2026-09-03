// Package engine is the central orchestrator for Diff-Mem operations.
package engine

import (
	"github.com/diff-mem/diff-mem/internal/model"
	"github.com/diff-mem/diff-mem/internal/store"
)

type Engine struct {
	store store.Store
	links *linkIndex
}

func New(s store.Store) *Engine {
	e := &Engine{store: s, links: newLinkIndex()}
	for _, node := range s.AllNodes() {
		e.links.addLinks(node.Header.Path, nodeContentLinks(node))
	}
	return e
}

// Dispatch routes a tool name + params to the appropriate handler.
// Tool surface: create, append, update, lifecycle, list, search, show.
func (e *Engine) Dispatch(name string, params map[string]interface{}) *model.ToolResponse {
	switch name {
	case "create":
		return e.Create(extractCreate(params))
	case "append":
		return e.Append(extractAppend(params))
	case "update":
		return e.Update(extractUpdate(params))
	case "lifecycle":
		action, opts := extractLifecycle(params)
		switch action {
		case "delete":
			return e.Delete(opts)
		case "archive":
			return e.Archive(opts)
		case "restore":
			return e.Restore(opts)
		default:
			return fail("VALIDATION_FAILED", "action must be one of: delete, archive, restore")
		}
	case "list":
		path, _ := params["path"].(string)
		inc, _ := params["include_archived"].(bool)
		return e.List(path, inc)
	case "search":
		return e.Search(extractSearch(params))
	case "show":
		path, _ := params["path"].(string)
		window, _ := params["window"].(string)
		if window == "" {
			return e.Show(path)
		}
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
			field := params["field"].(string)
			value := params["value"].(string)
			params["fields"] = map[string]interface{}{field: value}
			resp = e.Update(extractUpdate(params))
		case "UPDATE_SUMMARY":
			params["summary"] = map[string]interface{}{
				"old":    params["old_summary"],
				"new":    params["new_summary"],
				"reason": params["reason"],
			}
			resp = e.Update(extractUpdate(params))
		case "DELETE":
			params["action"] = "delete"
			resp = e.Delete(extractArchive(params))
		case "ARCHIVE":
			params["action"] = "archive"
			resp = e.Archive(extractArchive(params))
		case "RESTORE":
			params["action"] = "restore"
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
