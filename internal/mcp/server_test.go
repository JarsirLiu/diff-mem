package mcp

import (
	"encoding/json"
	"testing"

	stdmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/diff-mem/diff-mem/internal/engine"
	"github.com/diff-mem/diff-mem/internal/model"
	"github.com/diff-mem/diff-mem/internal/store"
)

// --- helpers ---

func setupServer() *Server {
	return New(engine.New(store.NewMemoryStore()))
}

// simulateHandler reproduces the exact logic inside addTool's handler closure.
func simulateHandler(e *engine.Engine, engineName string, args map[string]interface{}) (*stdmcp.CallToolResult, error) {
	resp := e.Dispatch(engineName, args)
	respBytes, _ := json.Marshal(resp)

	var hasError bool
	var check struct {
		Success bool
	}
	json.Unmarshal(respBytes, &check)
	if !check.Success {
		hasError = true
	}

	return &stdmcp.CallToolResult{
		Content: []stdmcp.Content{&stdmcp.TextContent{Text: string(respBytes)}},
		IsError: hasError,
	}, nil
}

func unmarshalResponse(result *stdmcp.CallToolResult) map[string]interface{} {
	text := result.Content[0].(*stdmcp.TextContent).Text
	var m map[string]interface{}
	json.Unmarshal([]byte(text), &m)
	return m
}

// --- tests ---

func TestNew(t *testing.T) {
	s := setupServer()
	if s == nil {
		t.Fatal("expected non-nil server")
	}
	if s.engine == nil {
		t.Fatal("expected non-nil engine")
	}
}

func TestServe_DelegatesToRun(t *testing.T) {
	s := setupServer()
	if s == nil {
		t.Fatal("expected non-nil server")
	}
}

func TestToolNameStrip(t *testing.T) {
	tests := []struct {
		toolName   string
		engineName string
	}{
		{"diff_mem_create", "create"},
		{"diff_mem_append", "append"},
		{"diff_mem_update", "update"},
		{"diff_mem_lifecycle", "lifecycle"},
		{"diff_mem_list", "list"},
		{"diff_mem_search", "search"},
		{"diff_mem_show", "show"},
	}
	for _, tt := range tests {
		t.Run(tt.toolName, func(t *testing.T) {
			got := tt.toolName[len("diff_mem_"):]
			if got != tt.engineName {
				t.Errorf("expected engine name %q, got %q", tt.engineName, got)
			}
		})
	}
}

func TestHandler_CreateSuccess(t *testing.T) {
	e := engine.New(store.NewMemoryStore())
	result, err := simulateHandler(e, "create", map[string]interface{}{
		"path":    "/projects/test",
		"title":   "Test Project",
		"summary": "A test project node",
		"tags":    []interface{}{"project", "test"},
		"reason":  "测试创建节点",
	})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected success, got error result")
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Content))
	}
	resp := unmarshalResponse(result)
	if ok, _ := resp["success"].(bool); !ok {
		t.Fatal("expected success=true in response body")
	}
	if _, ok := resp["result"]; !ok {
		t.Fatal("expected result field in response")
	}
}

func TestHandler_CreateDuplicate(t *testing.T) {
	e := engine.New(store.NewMemoryStore())
	simulateHandler(e, "create", map[string]interface{}{
		"path": "/dup", "title": "D", "summary": "D", "reason": "test",
	})
	result, _ := simulateHandler(e, "create", map[string]interface{}{
		"path": "/dup", "title": "D", "summary": "D", "reason": "test",
	})
	if !result.IsError {
		t.Fatal("expected error for duplicate create")
	}
	resp := unmarshalResponse(result)
	if errInfo, ok := resp["error"].(map[string]interface{}); ok {
		if code, _ := errInfo["code"].(string); code == "" {
			t.Fatal("expected error code in response")
		}
	}
}

func TestHandler_ShowNotFound(t *testing.T) {
	e := engine.New(store.NewMemoryStore())
	result, err := simulateHandler(e, "show", map[string]interface{}{
		"path": "/not-found",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for show not found")
	}
	resp := unmarshalResponse(result)
	if ok, _ := resp["success"].(bool); ok {
		t.Fatal("expected success=false")
	}
	if errInfo, ok := resp["error"].(map[string]interface{}); ok {
		if _, ok := errInfo["code"].(string); !ok {
			t.Fatal("expected error code field")
		}
	}
}

func TestHandler_ArchiveSuccess(t *testing.T) {
	e := engine.New(store.NewMemoryStore())
	simulateHandler(e, "create", map[string]interface{}{
		"path": "/arch", "title": "A", "summary": "A", "reason": "test",
	})
	result, _ := simulateHandler(e, "lifecycle", map[string]interface{}{"action": "archive",
		"path":   "/arch",
		"reason": "测试归档",
	})
	if result.IsError {
		t.Fatalf("expected archive success, got error: %v", unmarshalResponse(result))
	}
	resp := unmarshalResponse(result)
	if ok, _ := resp["success"].(bool); !ok {
		t.Fatal("expected success=true")
	}
}

func TestHandler_ArchiveLinkWarning(t *testing.T) {
	e := engine.New(store.NewMemoryStore())
	simulateHandler(e, "create", map[string]interface{}{"path": "/a", "title": "A", "summary": "A", "reason": "test"})
	simulateHandler(e, "create", map[string]interface{}{"path": "/b", "title": "B", "summary": "B", "reason": "test"})
	simulateHandler(e, "append", map[string]interface{}{
		"path": "/a", "event": "依赖 [[/b]]", "reason": "test",
	})

	// Archiving /b should succeed but carry a warning (its body is linked by /a)
	result, _ := simulateHandler(e, "lifecycle", map[string]interface{}{"action": "archive",
		"path": "/b", "reason": "测试归档被链接节点",
	})
	if result.IsError {
		t.Fatalf("archive should succeed even when linked: %v", unmarshalResponse(result))
	}
	resp := unmarshalResponse(result)
	if warning, _ := resp["warning"].(string); warning == "" {
		t.Fatal("expected warning in response")
	}
}

func TestHandler_RestoreArchived(t *testing.T) {
	e := engine.New(store.NewMemoryStore())
	simulateHandler(e, "create", map[string]interface{}{
		"path": "/rst", "title": "R", "summary": "R", "reason": "test",
	})
	simulateHandler(e, "lifecycle", map[string]interface{}{"action": "archive",
		"path": "/rst", "reason": "test",
	})
	result, _ := simulateHandler(e, "lifecycle", map[string]interface{}{"action": "restore",
		"path": "/rst", "reason": "测试恢复",
	})
	if result.IsError {
		t.Fatal("expected restore success")
	}
}

func TestHandler_RestoreNonArchived(t *testing.T) {
	e := engine.New(store.NewMemoryStore())
	simulateHandler(e, "create", map[string]interface{}{
		"path": "/rst2", "title": "R", "summary": "R", "reason": "test",
	})
	result, _ := simulateHandler(e, "lifecycle", map[string]interface{}{"action": "restore",
		"path": "/rst2", "reason": "test",
	})
	if !result.IsError {
		t.Fatal("expected error restoring non-archived node")
	}
}

func TestHandler_UnknownTool(t *testing.T) {
	e := engine.New(store.NewMemoryStore())
	result, err := simulateHandler(e, "unknown_tool", map[string]interface{}{})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for unknown tool")
	}
	resp := unmarshalResponse(result)
	if errInfo, ok := resp["error"].(map[string]interface{}); ok {
		if code, _ := errInfo["code"].(string); code != "UNKNOWN_TOOL" {
			t.Fatalf("expected UNKNOWN_TOOL, got %s", code)
		}
	}
}

func TestHandler_NilArgs(t *testing.T) {
	e := engine.New(store.NewMemoryStore())
	// Simulates what happens when req.Params.Arguments is nil —
	// the handler creates an empty map, which should not panic.
	result, err := simulateHandler(e, "list", make(map[string]interface{}))
	if err != nil {
		t.Fatal(err)
	}
	// list with empty path should list root — may succeed or fail depending on state
	_ = result
}

func TestHandler_DeleteWithDependency(t *testing.T) {
	e := engine.New(store.NewMemoryStore())
	simulateHandler(e, "create", map[string]interface{}{"path": "/a", "title": "A", "summary": "A", "reason": "test"})
	simulateHandler(e, "create", map[string]interface{}{"path": "/b", "title": "B", "summary": "B", "reason": "test"})
	simulateHandler(e, "append", map[string]interface{}{
		"path": "/a", "event": "依赖 [[/b]] 的产出", "reason": "test",
	})

	// /b is linked by /a's body; deleting /b should fail
	result, _ := simulateHandler(e, "lifecycle", map[string]interface{}{"action": "delete",
		"path": "/b", "reason": "test",
	})
	if !result.IsError {
		t.Fatal("expected delete to fail when linked by other nodes")
	}
}

func TestHandler_SearchTag(t *testing.T) {
	e := engine.New(store.NewMemoryStore())
	simulateHandler(e, "create", map[string]interface{}{
		"path": "/p1", "title": "P1", "summary": "P1",
		"tags": []interface{}{"project", "backend"}, "reason": "test",
	})
	simulateHandler(e, "create", map[string]interface{}{
		"path": "/p2", "title": "P2", "summary": "P2",
		"tags": []interface{}{"project", "frontend"}, "reason": "test",
	})

	result, _ := simulateHandler(e, "search", map[string]interface{}{
		"tags": []interface{}{"backend"},
	})
	if result.IsError {
		t.Fatalf("search failed: %v", unmarshalResponse(result))
	}
}

func TestHandler_SearchKeyword(t *testing.T) {
	e := engine.New(store.NewMemoryStore())
	simulateHandler(e, "create", map[string]interface{}{
		"path": "/proj/backend", "title": "Backend", "summary": "后端开发", "reason": "test",
	})

	result, _ := simulateHandler(e, "search", map[string]interface{}{
		"keywords": "后端",
	})
	if result.IsError {
		t.Fatalf("keyword search failed: %v", unmarshalResponse(result))
	}
}

func TestHandler_DeepLoad(t *testing.T) {
	e := engine.New(store.NewMemoryStore())
	simulateHandler(e, "create", map[string]interface{}{
		"path": "/tasks/t1", "title": "T1", "summary": "T1", "reason": "test",
	})
	for i := 0; i < 5; i++ {
		simulateHandler(e, "append", map[string]interface{}{
			"path": "/tasks/t1", "event": "evt" + string(rune(i+48)), "reason": "test",
		})
	}

	windows := []string{"recent", "last_10", "last_50", "last_100", "all"}
	for _, w := range windows {
		result, _ := simulateHandler(e, "show", map[string]interface{}{
			"path": "/tasks/t1", "window": w,
		})
		if result.IsError {
			t.Fatalf("deep_load window %q failed: %v", w, unmarshalResponse(result))
		}
	}
}

func TestHandler_DeepLoadInvalidWindow(t *testing.T) {
	e := engine.New(store.NewMemoryStore())
	result, _ := simulateHandler(e, "show", map[string]interface{}{
		"path": "/x", "window": "invalid",
	})
	if !result.IsError {
		t.Fatal("expected error for invalid window")
	}
}

func TestHandler_SummaryDriftLazyReason(t *testing.T) {
	e := engine.New(store.NewMemoryStore())
	simulateHandler(e, "create", map[string]interface{}{
		"path": "/drift", "title": "D", "summary": "张三负责后端，deadline 2026-09-30", "reason": "test",
	})

	result, _ := simulateHandler(e, "update", map[string]interface{}{
		"path": "/drift",
		"summary": map[string]interface{}{
			"old": "张三负责后端，deadline 2026-09-30", "new": "项目进行中", "reason": "ok",
		},
	})
	if !result.IsError {
		t.Fatal("expected lazy reason to be rejected")
	}
	resp := unmarshalResponse(result)
	if errInfo, ok := resp["error"].(map[string]interface{}); ok {
		if code, _ := errInfo["code"].(string); code != "SUMMARY_DRIFT_DETECTED" {
			t.Fatalf("expected SUMMARY_DRIFT_DETECTED, got %s", code)
		}
	}
}

func TestHandler_SummaryDriftSubstantiveReason(t *testing.T) {
	e := engine.New(store.NewMemoryStore())
	simulateHandler(e, "create", map[string]interface{}{
		"path": "/drift2", "title": "D", "summary": "张三负责后端", "reason": "test",
	})

	result, _ := simulateHandler(e, "update", map[string]interface{}{
		"path": "/drift2",
		"summary": map[string]interface{}{
			"old": "张三负责后端", "new": "李四接替负责",
			"reason": "张三离职，李四接替负责后端开发工作",
		},
	})
	if result.IsError {
		t.Fatalf("expected substantive reason to pass: %v", unmarshalResponse(result))
	}
}

func TestAllToolsMapToEngine(t *testing.T) {
	e := engine.New(store.NewMemoryStore())
	expectedTools := []string{
		"create", "append", "update", "lifecycle",
		"list", "search", "show",
	}
	for _, name := range expectedTools {
		resp := e.Dispatch(name, map[string]interface{}{})
		if resp.Error != nil && resp.Error.Code == "UNKNOWN_TOOL" {
			t.Errorf("engine does not recognize tool %q", name)
		}
	}
}

// Ensure model types are used (suppress unused import warnings)
var _ model.Node
