// Package engine is the central orchestrator for Diff-Mem operations.
package engine

import (
	"encoding/json"
	"strconv"

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

// AllNodesCount exposes the stored node count for tests and diagnostics.
func (e *Engine) AllNodesCount() []*model.Node {
	return e.store.AllNodes()
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
		return e.List(NormalizePath(path), inc)
	case "search":
		return e.Search(extractSearch(params))
	case "show":
		path, _ := params["path"].(string)
		window, _ := params["window"].(string)
		if window == "" {
			return e.Show(NormalizePath(path))
		}
		return e.DeepLoad(NormalizePath(path), window)
	default:
		return fail("UNKNOWN_TOOL", "unknown tool: "+name)
	}
}

// Exec atomically executes multiple memory operations.
// All operations succeed or all rollback: a full-state snapshot is taken
// before execution and restored if any operation fails.
func (e *Engine) Exec(operations []map[string]interface{}) *model.ToolResponse {
	if len(operations) == 0 || len(operations) > 20 {
		return fail("VALIDATION_FAILED", "operations must be 1-20 items")
	}
	for i, op := range operations {
		opName, ok := op["op"].(string)
		if !ok {
			return fail("VALIDATION_FAILED", "operation ["+strconv.Itoa(i)+"] op must be string")
		}
		switch opName {
		case "SHOW", "LIST", "SEARCH", "DEEP_LOAD":
			return fail("VALIDATION_FAILED", "read operation "+opName+" not allowed in transaction")
		}
	}

	// Snapshot full engine state for rollback. Bounded by the 20-op limit and
	// the memory-tree data scale (agent memory, not big data), a full snapshot
	// is simple and correct.
	snap, err := e.snapshot()
	if err != nil {
		return fail("TRANSACTION_FAILED", "snapshot before transaction failed: "+err.Error())
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
			e.restore(snap)
			return fail("TRANSACTION_FAILED",
				"operation ["+strconv.Itoa(i)+"] failed: "+resp.Error.Message+"; transaction rolled back")
		}
	}

	return success(map[string]string{"committed": "true", "operations": strconv.Itoa(len(operations))})
}

// engineSnapshot is a deep copy of all nodes, taken before a transaction.
type engineSnapshot struct {
	nodes []*model.Node
}

func cloneNode(n *model.Node) (*model.Node, error) {
	b, err := json.Marshal(n)
	if err != nil {
		return nil, err
	}
	var c model.Node
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func (e *Engine) snapshot() (*engineSnapshot, error) {
	nodes := e.store.AllNodes()
	snap := make([]*model.Node, 0, len(nodes))
	for _, n := range nodes {
		c, err := cloneNode(n)
		if err != nil {
			return nil, err
		}
		snap = append(snap, c)
	}
	return &engineSnapshot{nodes: snap}, nil
}

// restore puts the store and the link index back to the snapshot state.
func (e *Engine) restore(s *engineSnapshot) {
	keep := make(map[string]bool, len(s.nodes))
	for _, n := range s.nodes {
		keep[n.Header.Path] = true
	}
	for _, n := range e.store.AllNodes() {
		if !keep[n.Header.Path] {
			e.store.DeleteNode(n.Header.Path)
		}
	}
	for _, n := range s.nodes {
		c, err := cloneNode(n)
		if err != nil {
			continue
		}
		e.store.PutNode(c)
	}
	// Rebuild the in-memory link index from the restored state.
	e.links.reset()
	for _, n := range s.nodes {
		e.links.addLinks(n.Header.Path, nodeContentLinks(n))
	}
}
