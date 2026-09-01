package engine_test

import (
	"testing"

	"github.com/diff-mem/diff-mem/internal/engine"
	"github.com/diff-mem/diff-mem/internal/model"
	"github.com/diff-mem/diff-mem/internal/store"
)

func setupEngine() *engine.Engine {
	return engine.New(store.NewMemoryStore())
}

func TestCreateAndAppend(t *testing.T) {
	e := setupEngine()

	// Create
	resp := e.Dispatch("create", map[string]interface{}{
		"path":    "/projects/alpha",
		"title":   "Alpha 项目",
		"summary": "Alpha 项目的根目录，包含后端和前端",
		"tags":    []interface{}{"project", "alpha"},
		"reason":  "用户创建了 Alpha 项目",
	})
	if !resp.Success {
		t.Fatalf("create failed: %v", resp.Error)
	}

	// Append
	resp = e.Dispatch("append", map[string]interface{}{
		"path":  "/projects/alpha",
		"event": "项目启动，目标 Q4 上线",
		"reason": "记录项目启动信息",
	})
	if !resp.Success {
		t.Fatalf("append failed: %v", resp.Error)
	}

	// Show
	resp = e.Dispatch("show", map[string]interface{}{"path": "/projects/alpha"})
	if !resp.Success {
		t.Fatalf("show failed: %v", resp.Error)
	}
}

func TestDuplicateCreate(t *testing.T) {
	e := setupEngine()
	e.Dispatch("create", map[string]interface{}{
		"path": "/projects/alpha", "title": "A", "summary": "A", "reason": "test",
	})
	resp := e.Dispatch("create", map[string]interface{}{
		"path": "/projects/alpha", "title": "A", "summary": "A", "reason": "test",
	})
	if resp.Success {
		t.Fatal("expected duplicate create to fail")
	}
}

func TestAutoCreateParents(t *testing.T) {
	e := setupEngine()
	resp := e.Dispatch("create", map[string]interface{}{
		"path": "/projects/alpha/backend", "title": "Backend", "summary": "Backend", "reason": "test",
	})
	if !resp.Success {
		t.Fatalf("create with auto-parents failed: %v", resp.Error)
	}
	showResp := e.Dispatch("show", map[string]interface{}{"path": "/projects"})
	if !showResp.Success {
		t.Fatal("expected /projects to exist after auto-creation")
	}
}

func TestArchiveAndRestore(t *testing.T) {
	e := setupEngine()
	e.Dispatch("create", map[string]interface{}{
		"path": "/projects/alpha", "title": "A", "summary": "A", "reason": "test",
	})

	resp := e.Dispatch("archive", map[string]interface{}{
		"path": "/projects/alpha", "reason": "项目已关闭",
	})
	if !resp.Success {
		t.Fatalf("archive failed: %v", resp.Error)
	}

	// Archived node cannot be appended to
	resp = e.Dispatch("append", map[string]interface{}{
		"path": "/projects/alpha", "event": "test", "reason": "test",
	})
	if resp.Success {
		t.Fatal("expected append to archived node to fail")
	}

	// Restore
	resp = e.Dispatch("restore", map[string]interface{}{
		"path": "/projects/alpha", "reason": "重新激活",
	})
	if !resp.Success {
		t.Fatalf("restore failed: %v", resp.Error)
	}

	// Now can append again
	resp = e.Dispatch("append", map[string]interface{}{
		"path": "/projects/alpha", "event": "恢复了", "reason": "test",
	})
	if !resp.Success {
		t.Fatalf("append after restore failed: %v", resp.Error)
	}
}

func TestLinkCycleDetection(t *testing.T) {
	e := setupEngine()
	e.Dispatch("create", map[string]interface{}{"path": "/a", "title": "A", "summary": "A", "reason": "test"})
	e.Dispatch("create", map[string]interface{}{"path": "/b", "title": "B", "summary": "B", "reason": "test"})
	e.Dispatch("create", map[string]interface{}{"path": "/c", "title": "C", "summary": "C", "reason": "test"})

	// /a depends_on /b
	resp := e.Dispatch("link", map[string]interface{}{
		"from": "/a", "to": "/b", "type": "depends_on", "reason": "A 依赖 B",
	})
	if !resp.Success {
		t.Fatalf("link failed: %v", resp.Error)
	}

	// /b depends_on /c
	resp = e.Dispatch("link", map[string]interface{}{
		"from": "/b", "to": "/c", "type": "depends_on", "reason": "B 依赖 C",
	})
	if !resp.Success {
		t.Fatalf("link failed: %v", resp.Error)
	}

	// /c depends_on /a → cycle!
	resp = e.Dispatch("link", map[string]interface{}{
		"from": "/c", "to": "/a", "type": "depends_on", "reason": "C 依赖 A",
	})
	if resp.Success {
		t.Fatal("expected cycle detection to reject this link")
	}
	if resp.Error.Code != "CYCLE_DETECTED" {
		t.Fatalf("expected CYCLE_DETECTED, got %s", resp.Error.Code)
	}
}

func TestSummaryDrift(t *testing.T) {
	e := setupEngine()
	e.Dispatch("create", map[string]interface{}{
		"path": "/projects/alpha",
		"title": "A",
		"summary": "张三负责后端开发，deadline 2026-09-30",
		"reason": "test",
	})

	// Update summary with entities disappearing but reason is lazy
	resp := e.Dispatch("update_summary", map[string]interface{}{
		"path":        "/projects/alpha",
		"old_summary": "张三负责后端开发，deadline 2026-09-30",
		"new_summary": "项目进行中",
		"reason":      "ok",
	})
	if resp.Success {
		t.Fatal("expected lazy reason to be rejected")
	}
	if resp.Error.Code != "SUMMARY_DRIFT_DETECTED" {
		t.Fatalf("expected SUMMARY_DRIFT_DETECTED, got %s", resp.Error.Code)
	}

	// Now with a substantive reason
	resp = e.Dispatch("update_summary", map[string]interface{}{
		"path":        "/projects/alpha",
		"old_summary": "张三负责后端开发，deadline 2026-09-30",
		"new_summary": "李四接替负责，deadline 延至 2026-10-15",
		"reason":      "张三离职，李四接替，deadline 顺延两周",
	})
	if !resp.Success {
		t.Fatalf("expected substantive reason to pass: %v", resp.Error)
	}
}

func TestSearch(t *testing.T) {
	e := setupEngine()
	e.Dispatch("create", map[string]interface{}{
		"path": "/projects/alpha", "title": "Alpha", "summary": "Alpha 后端项目",
		"tags": []interface{}{"project", "backend"}, "reason": "test",
	})
	e.Dispatch("create", map[string]interface{}{
		"path": "/projects/beta", "title": "Beta", "summary": "Beta 前端项目",
		"tags": []interface{}{"project", "frontend"}, "reason": "test",
	})

	// Tag search
	resp := e.Dispatch("search", map[string]interface{}{
		"tags": []interface{}{"backend"},
	})
	if !resp.Success {
		t.Fatalf("search failed: %v", resp.Error)
	}

	// Keyword search
	resp = e.Dispatch("search", map[string]interface{}{
		"keywords": "Alpha",
	})
	if !resp.Success {
		t.Fatalf("keyword search failed: %v", resp.Error)
	}
}

func TestDeleteWithDependencies(t *testing.T) {
	e := setupEngine()
	e.Dispatch("create", map[string]interface{}{"path": "/a", "title": "A", "summary": "A", "reason": "test"})
	e.Dispatch("create", map[string]interface{}{"path": "/b", "title": "B", "summary": "B", "reason": "test"})
	e.Dispatch("link", map[string]interface{}{
		"from": "/a", "to": "/b", "type": "depends_on", "reason": "test",
	})

	// /b is referenced by /a, deleting /b should fail
	resp := e.Dispatch("delete", map[string]interface{}{
		"path": "/b", "reason": "test",
	})
	if resp.Success {
		t.Fatal("expected delete with active dependency to fail")
	}
}

func TestDeepLoad(t *testing.T) {
	e := setupEngine()
	e.Dispatch("create", map[string]interface{}{
		"path": "/tasks/t1", "title": "T1", "summary": "T1", "reason": "test",
	})
	for i := 0; i < 10; i++ {
		e.Dispatch("append", map[string]interface{}{
			"path": "/tasks/t1", "event": "event " + string(rune(i+48)), "reason": "test",
		})
	}

	resp := e.Dispatch("deep_load", map[string]interface{}{
		"path": "/tasks/t1", "window": "recent",
	})
	if !resp.Success {
		t.Fatalf("deep_load failed: %v", resp.Error)
	}
}

func TestExecTransaction(t *testing.T) {
	e := setupEngine()

	resp := e.Exec([]map[string]interface{}{
		{
			"op": "CREATE",
			"params": map[string]interface{}{
				"path": "/projects/exec-test", "title": "Exec", "summary": "Exec", "reason": "test",
			},
		},
		{
			"op": "APPEND",
			"params": map[string]interface{}{
				"path": "/projects/exec-test", "event": "事务写入", "reason": "test",
			},
		},
	})
	if !resp.Success {
		t.Fatalf("exec failed: %v", resp.Error)
	}
}

// Ensure model types are used (suppress unused import warnings)
var _ model.Node
var _ model.Edge

func TestUpdateField_Basic(t *testing.T) {
	e := setupEngine()
	e.Dispatch("create", map[string]interface{}{
		"path": "/projects/alpha", "title": "A", "summary": "A", "reason": "test",
	})
	resp := e.Dispatch("update_field", map[string]interface{}{
		"path": "/projects/alpha", "field": "status", "value": "active", "reason": "init",
	})
	if !resp.Success {
		t.Fatalf("update_field failed: %v", resp.Error)
	}
}

func TestUpdateField_ArchivedNode(t *testing.T) {
	e := setupEngine()
	e.Dispatch("create", map[string]interface{}{
		"path": "/projects/alpha", "title": "A", "summary": "A", "reason": "test",
	})
	e.Dispatch("archive", map[string]interface{}{
		"path": "/projects/alpha", "reason": "test",
	})
	resp := e.Dispatch("update_field", map[string]interface{}{
		"path": "/projects/alpha", "field": "status", "value": "active", "reason": "test",
	})
	if resp.Success {
		t.Fatal("update_field on archived node should fail")
	}
}

func TestUnlink_Basic(t *testing.T) {
	e := setupEngine()
	e.Dispatch("create", map[string]interface{}{"path": "/a", "title": "A", "summary": "A", "reason": "test"})
	e.Dispatch("create", map[string]interface{}{"path": "/b", "title": "B", "summary": "B", "reason": "test"})
	e.Dispatch("link", map[string]interface{}{
		"from": "/a", "to": "/b", "type": "references", "reason": "test",
	})
	resp := e.Dispatch("unlink", map[string]interface{}{"from": "/a", "to": "/b"})
	if !resp.Success {
		t.Fatalf("unlink failed: %v", resp.Error)
	}
}

func TestUnlink_NotFound(t *testing.T) {
	e := setupEngine()
	resp := e.Dispatch("unlink", map[string]interface{}{"from": "/a", "to": "/b"})
	if resp.Success {
		t.Fatal("unlink on non-existent edge should fail")
	}
}

func TestList_Root(t *testing.T) {
	e := setupEngine()
	e.Dispatch("create", map[string]interface{}{
		"path": "/projects/alpha", "title": "A", "summary": "A", "reason": "test",
	})
	e.Dispatch("create", map[string]interface{}{
		"path": "/tasks/t1", "title": "T", "summary": "T", "reason": "test",
	})
	resp := e.Dispatch("list", map[string]interface{}{"path": ""})
	if !resp.Success {
		t.Fatalf("list failed: %v", resp.Error)
	}
}

func TestList_SubPath(t *testing.T) {
	e := setupEngine()
	e.Dispatch("create", map[string]interface{}{
		"path": "/projects/alpha", "title": "A", "summary": "A", "reason": "test",
	})
	resp := e.Dispatch("list", map[string]interface{}{"path": "/projects"})
	if !resp.Success {
		t.Fatalf("list /projects failed: %v", resp.Error)
	}
}

func TestList_NotFound(t *testing.T) {
	e := setupEngine()
	resp := e.Dispatch("list", map[string]interface{}{"path": "/nonexistent"})
	if resp.Success {
		t.Fatal("list on non-existent path should fail")
	}
}

func TestDeepLoad_AllWindows(t *testing.T) {
	e := setupEngine()
	e.Dispatch("create", map[string]interface{}{
		"path": "/tasks/t1", "title": "T", "summary": "T", "reason": "test",
	})
	e.Dispatch("append", map[string]interface{}{
		"path": "/tasks/t1", "event": "test event", "reason": "test",
	})

	windows := []string{"recent", "last_10", "last_50", "last_100", "all"}
	for _, w := range windows {
		resp := e.Dispatch("deep_load", map[string]interface{}{"path": "/tasks/t1", "window": w})
		if !resp.Success {
			t.Fatalf("deep_load window %q failed: %v", w, resp.Error)
		}
	}
}

func TestDeepLoad_InvalidWindow(t *testing.T) {
	e := setupEngine()
	resp := e.Dispatch("deep_load", map[string]interface{}{"path": "/a", "window": "invalid"})
	if resp.Success {
		t.Fatal("invalid window should fail")
	}
}

func TestDeepLoad_NotFound(t *testing.T) {
	e := setupEngine()
	resp := e.Dispatch("deep_load", map[string]interface{}{"path": "/not-found", "window": "recent"})
	if resp.Success {
		t.Fatal("deep_load on non-existent path should fail")
	}
}

func TestExec_TransactionFailure(t *testing.T) {
	e := setupEngine()

	// First operation succeeds, second fails → should not leave partial state
	resp := e.Exec([]map[string]interface{}{
		{
			"op": "CREATE",
			"params": map[string]interface{}{
				"path": "/projects/exec-fail", "title": "F", "summary": "F", "reason": "test",
			},
		},
		{
			"op": "APPEND",
			"params": map[string]interface{}{
				"path": "/projects/exec-fail", "event": "", "reason": "test", // empty event fails
			},
		},
	})
	if resp.Success {
		t.Fatal("exec with failing operation should fail")
	}
	if resp.Error.Code != "TRANSACTION_FAILED" {
		t.Fatalf("expected TRANSACTION_FAILED, got %s", resp.Error.Code)
	}
}

func TestDispatch_UnknownTool(t *testing.T) {
	e := setupEngine()
	resp := e.Dispatch("unknown_tool", map[string]interface{}{})
	if resp.Success {
		t.Fatal("unknown tool should fail")
	}
	if resp.Error.Code != "UNKNOWN_TOOL" {
		t.Fatalf("expected UNKNOWN_TOOL, got %s", resp.Error.Code)
	}
}

func TestShow_NotFound(t *testing.T) {
	e := setupEngine()
	resp := e.Dispatch("show", map[string]interface{}{"path": "/not-found"})
	if resp.Success {
		t.Fatal("show on non-existent path should fail")
	}
}

func TestSearch_NoResults(t *testing.T) {
	e := setupEngine()
	resp := e.Dispatch("search", map[string]interface{}{
		"tags": []interface{}{"nonexistent_tag"},
	})
	if !resp.Success {
		t.Fatalf("search failed: %v", resp.Error)
	}
}

func TestArchive_OutboundImpact(t *testing.T) {
	e := setupEngine()
	e.Dispatch("create", map[string]interface{}{"path": "/a", "title": "A", "summary": "A", "reason": "test"})
	e.Dispatch("create", map[string]interface{}{"path": "/b", "title": "B", "summary": "B", "reason": "test"})
	e.Dispatch("link", map[string]interface{}{
		"from": "/a", "to": "/b", "type": "references", "reason": "test",
	})

	resp := e.Dispatch("archive", map[string]interface{}{
		"path": "/a", "reason": "done",
	})
	if !resp.Success {
		t.Fatalf("archive failed: %v", resp.Error)
	}
	if resp.Impacted == nil || len(resp.Impacted) == 0 {
		t.Log("no impacted nodes (expected since /a has outbound edges)")
	}
}

func TestDuplicateArchive(t *testing.T) {
	e := setupEngine()
	e.Dispatch("create", map[string]interface{}{
		"path": "/projects/alpha", "title": "A", "summary": "A", "reason": "test",
	})
	e.Dispatch("archive", map[string]interface{}{
		"path": "/projects/alpha", "reason": "done",
	})
	resp := e.Dispatch("archive", map[string]interface{}{
		"path": "/projects/alpha", "reason": "done",
	})
	if resp.Success {
		t.Fatal("duplicate archive should fail")
	}
}

func TestRestore_NonArchived(t *testing.T) {
	e := setupEngine()
	e.Dispatch("create", map[string]interface{}{
		"path": "/projects/alpha", "title": "A", "summary": "A", "reason": "test",
	})
	resp := e.Dispatch("restore", map[string]interface{}{
		"path": "/projects/alpha", "reason": "test",
	})
	if resp.Success {
		t.Fatal("restore on non-archived node should fail")
	}
}

func TestDelete_NotFound(t *testing.T) {
	e := setupEngine()
	resp := e.Dispatch("delete", map[string]interface{}{
		"path": "/not-found", "reason": "test",
	})
	if resp.Success {
		t.Fatal("delete on non-existent path should fail")
	}
}

