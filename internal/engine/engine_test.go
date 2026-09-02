package engine_test

import (
	"strings"
	"testing"

	"github.com/diff-mem/diff-mem/internal/engine"
	"github.com/diff-mem/diff-mem/internal/model"
	"github.com/diff-mem/diff-mem/internal/store"
)

func setupEngine() *engine.Engine {
	return engine.New(store.NewMemoryStore())
}

func setupEngineWithStore(s store.Store) *engine.Engine {
	return engine.New(s)
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

	resp := e.Dispatch("lifecycle", map[string]interface{}{"action": "archive",
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
	resp = e.Dispatch("lifecycle", map[string]interface{}{"action": "restore",
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

func TestContentLinkGate_Append(t *testing.T) {
	e := setupEngine()
	e.Dispatch("create", map[string]interface{}{"path": "/a", "title": "A", "summary": "A", "reason": "test"})
	e.Dispatch("create", map[string]interface{}{"path": "/b", "title": "B", "summary": "B", "reason": "test"})

	// Dangling link → rejected
	resp := e.Dispatch("append", map[string]interface{}{
		"path": "/a", "event": "参见 [[/b]] 和 [[/missing/node]]", "reason": "test",
	})
	if resp.Success {
		t.Fatal("expected dangling content link to be rejected")
	}
	if resp.Error.Code != "LINK_TARGET_NOT_FOUND" {
		t.Fatalf("expected LINK_TARGET_NOT_FOUND, got %s", resp.Error.Code)
	}

	// Valid links → accepted
	resp = e.Dispatch("append", map[string]interface{}{
		"path": "/a", "event": "参见 [[/b]] 和 [[/a]] 自引用", "reason": "test",
	})
	if !resp.Success {
		t.Fatalf("expected valid content links to pass: %v", resp.Error)
	}
}

func TestContentLinkGate_Create(t *testing.T) {
	e := setupEngine()
	e.Dispatch("create", map[string]interface{}{"path": "/a", "title": "A", "summary": "A", "reason": "test"})

	// Dangling link in initial events → rejected, zero side effects
	resp := e.Create(model.CreateOptions{
		Path: "/x/y", Title: "X", Summary: "X", Reason: "test",
		InitialEvents: []string{"引用 [[/missing/node]]"},
	})
	if resp.Success {
		t.Fatal("expected dangling content link in create to be rejected")
	}
	if resp.Error.Code != "LINK_TARGET_NOT_FOUND" {
		t.Fatalf("expected LINK_TARGET_NOT_FOUND, got %s", resp.Error.Code)
	}
	if e.Dispatch("show", map[string]interface{}{"path": "/x/y"}).Success {
		t.Fatal("create should have zero side effects on gate failure")
	}

	// Self and ancestor links are allowed (parents auto-created)
	resp = e.Create(model.CreateOptions{
		Path: "/p/child", Title: "C", Summary: "C", Reason: "test",
		InitialEvents: []string{"关联 [[/p]] 与 [[/p/child]]"},
	})
	if !resp.Success {
		t.Fatalf("self/ancestor links should pass: %v", resp.Error)
	}
}

func TestShowLinksAndBacklinks(t *testing.T) {
	e := setupEngine()
	e.Dispatch("create", map[string]interface{}{"path": "/a", "title": "A", "summary": "A", "reason": "test"})
	e.Dispatch("create", map[string]interface{}{"path": "/b", "title": "B", "summary": "B", "reason": "test"})
	e.Dispatch("append", map[string]interface{}{
		"path": "/a", "event": "工作依赖 [[/b]] 的产出", "reason": "test",
	})

	// /a: links → [/b], backlinks → []
	resp := e.Dispatch("show", map[string]interface{}{"path": "/a"})
	if !resp.Success {
		t.Fatalf("show failed: %v", resp.Error)
	}
	result, ok := resp.Result.(model.ShowResult)
	if !ok {
		t.Fatalf("expected model.ShowResult, got %T", resp.Result)
	}
	if len(result.Links) != 1 || result.Links[0] != "/b" {
		t.Fatalf("expected links [/b], got %v", result.Links)
	}
	if len(result.Backlinks) != 0 {
		t.Fatalf("expected no backlinks for /a, got %v", result.Backlinks)
	}

	// /b: backlinks → [/a]
	resp = e.Dispatch("show", map[string]interface{}{"path": "/b"})
	result, ok = resp.Result.(model.ShowResult)
	if !ok {
		t.Fatalf("expected model.ShowResult, got %T", resp.Result)
	}
	if len(result.Backlinks) != 1 || result.Backlinks[0] != "/a" {
		t.Fatalf("expected backlinks [/a], got %v", result.Backlinks)
	}
}

func TestLinkIndexRebuildOnRestart(t *testing.T) {
	s := store.NewMemoryStore()
	e := setupEngineWithStore(s)
	e.Dispatch("create", map[string]interface{}{"path": "/a", "title": "A", "summary": "A", "reason": "test"})
	e.Dispatch("create", map[string]interface{}{"path": "/b", "title": "B", "summary": "B", "reason": "test"})
	e.Dispatch("append", map[string]interface{}{
		"path": "/a", "event": "依赖 [[/b]] 的产出", "reason": "test",
	})

	// New engine over the same store: index must be rebuilt from persisted data.
	e2 := engine.New(s)
	resp := e2.Dispatch("show", map[string]interface{}{"path": "/b"})
	result, ok := resp.Result.(model.ShowResult)
	if !ok {
		t.Fatalf("expected model.ShowResult, got %T", resp.Result)
	}
	if len(result.Backlinks) != 1 || result.Backlinks[0] != "/a" {
		t.Fatalf("expected backlinks [/a] after restart, got %v", result.Backlinks)
	}

	// Gate still enforced on the rebuilt engine.
	resp = e2.Dispatch("lifecycle", map[string]interface{}{"action": "delete", "path": "/b", "reason": "test"})
	if resp.Success || resp.Error.Code != "LINKED_BY_OTHERS" {
		t.Fatalf("expected LINKED_BY_OTHERS after restart, got %+v", resp.Error)
	}

	// Delete clears the index: gate no longer fires afterwards.
	e2.Dispatch("lifecycle", map[string]interface{}{"action": "delete", "path": "/a", "reason": "test"})
	resp = e2.Dispatch("lifecycle", map[string]interface{}{"action": "delete", "path": "/b", "reason": "test"})
	if !resp.Success {
		t.Fatalf("delete of unlinked node should pass after referrer deleted: %v", resp.Error)
	}
}

func TestContentLinkEdgeCases(t *testing.T) {
	e := setupEngine()
	e.Dispatch("create", map[string]interface{}{"path": "/a", "title": "A", "summary": "A", "reason": "test"})
	e.Dispatch("create", map[string]interface{}{"path": "/b", "title": "B", "summary": "B", "reason": "test"})

	// Invalid link format (not starting with /) → LINK_TARGET_INVALID
	resp := e.Dispatch("append", map[string]interface{}{
		"path": "/a", "event": "坏链接 [[b-no-slash]]", "reason": "test",
	})
	if resp.Success || resp.Error.Code != "LINK_TARGET_INVALID" {
		t.Fatalf("expected LINK_TARGET_INVALID, got %+v", resp.Error)
	}

	// Links accumulate across multiple appends (union of all events).
	e.Dispatch("append", map[string]interface{}{"path": "/a", "event": "先链 [[/b]]", "reason": "test"})
	e.Dispatch("create", map[string]interface{}{"path": "/c", "title": "C", "summary": "C", "reason": "test"})
	e.Dispatch("append", map[string]interface{}{"path": "/a", "event": "再链 [[/c]] 与自引用 [[/a]]", "reason": "test"})

	resp = e.Dispatch("show", map[string]interface{}{"path": "/a"})
	result, ok := resp.Result.(model.ShowResult)
	if !ok {
		t.Fatalf("expected model.ShowResult, got %T", resp.Result)
	}
	if len(result.Links) != 3 {
		t.Fatalf("expected links union [/a /b /c], got %v", result.Links)
	}
	// Self-reference must not appear as backlink of /a.
	if len(result.Backlinks) != 0 {
		t.Fatalf("self-reference should not be a backlink, got %v", result.Backlinks)
	}

	// Active referrer: /b sees backlink [/a]; delete gated.
	resp = e.Dispatch("show", map[string]interface{}{"path": "/b"})
	result, _ = resp.Result.(model.ShowResult)
	if len(result.Backlinks) != 1 || result.Backlinks[0] != "/a" {
		t.Fatalf("expected backlinks [/a], got %v", result.Backlinks)
	}
	resp = e.Dispatch("lifecycle", map[string]interface{}{"action": "delete","path": "/b", "reason": "test"})
	if resp.Success || resp.Error.Code != "LINKED_BY_OTHERS" {
		t.Fatalf("expected LINKED_BY_OTHERS while referrer active, got %+v", resp.Error)
	}

	// Archive the referrer /a: excluded from /b backlinks, delete gate lifted.
	e.Dispatch("lifecycle", map[string]interface{}{"action": "archive","path": "/a", "reason": "test"})
	resp = e.Dispatch("show", map[string]interface{}{"path": "/b"})
	result, _ = resp.Result.(model.ShowResult)
	if len(result.Backlinks) != 0 {
		t.Fatalf("archived referrer /a should be excluded from /b backlinks, got %v", result.Backlinks)
	}

	// Restore /a: backlink returns and the delete gate fires again.
	e.Dispatch("lifecycle", map[string]interface{}{"action": "restore","path": "/a", "reason": "test"})
	resp = e.Dispatch("show", map[string]interface{}{"path": "/b"})
	result, _ = resp.Result.(model.ShowResult)
	if len(result.Backlinks) != 1 || result.Backlinks[0] != "/a" {
		t.Fatalf("expected backlinks [/a] after restore, got %v", result.Backlinks)
	}
	resp = e.Dispatch("lifecycle", map[string]interface{}{"action": "delete","path": "/b", "reason": "test"})
	if resp.Success || resp.Error.Code != "LINKED_BY_OTHERS" {
		t.Fatalf("expected LINKED_BY_OTHERS after referrer restored, got %+v", resp.Error)
	}
}

func TestArchivedReferrerDoesNotBlockDelete(t *testing.T) {
	e := setupEngine()
	e.Dispatch("create", map[string]interface{}{"path": "/x", "title": "X", "summary": "X", "reason": "test"})
	e.Dispatch("create", map[string]interface{}{"path": "/y", "title": "Y", "summary": "Y", "reason": "test"})
	e.Dispatch("append", map[string]interface{}{"path": "/x", "event": "依赖 [[/y]]", "reason": "test"})

	// With /x archived, /y is deletable despite /x's body still linking it.
	e.Dispatch("lifecycle", map[string]interface{}{"action": "archive","path": "/x", "reason": "test"})
	resp := e.Dispatch("lifecycle", map[string]interface{}{"action": "delete","path": "/y", "reason": "test"})
	if !resp.Success {
		t.Fatalf("delete should pass when only referrer is archived: %v", resp.Error)
	}
}

func TestArchiveWarningGuidance(t *testing.T) {
	e := setupEngine()
	e.Dispatch("create", map[string]interface{}{"path": "/a", "title": "A", "summary": "A", "reason": "test"})
	e.Dispatch("create", map[string]interface{}{"path": "/b", "title": "B", "summary": "B", "reason": "test"})
	e.Dispatch("append", map[string]interface{}{"path": "/a", "event": "依赖 [[/b]]", "reason": "test"})

	// Archive /b: warning lists referrer and gives explicit disposal options.
	resp := e.Dispatch("lifecycle", map[string]interface{}{"action": "archive","path": "/b", "reason": "test"})
	if !resp.Success {
		t.Fatalf("archive failed: %v", resp.Error)
	}
	if resp.Warning == "" || !strings.Contains(resp.Warning, "/a") ||
		!strings.Contains(resp.Warning, "改指") || !strings.Contains(resp.Warning, "移除") {
		t.Fatalf("expected archive warning with referrers and disposal options, got %q", resp.Warning)
	}

	// No referrers → no warning.
	e.Dispatch("create", map[string]interface{}{"path": "/c", "title": "C", "summary": "C", "reason": "test"})
	resp = e.Dispatch("lifecycle", map[string]interface{}{"action": "archive","path": "/c", "reason": "test"})
	if resp.Success && resp.Warning != "" {
		t.Fatalf("expected no warning without referrers, got %q", resp.Warning)
	}
}

func TestRestoreDanglingOutboundWarning(t *testing.T) {
	e := setupEngine()
	e.Dispatch("create", map[string]interface{}{"path": "/a", "title": "A", "summary": "A", "reason": "test"})
	e.Dispatch("create", map[string]interface{}{"path": "/b", "title": "B", "summary": "B", "reason": "test"})
	e.Dispatch("append", map[string]interface{}{"path": "/a", "event": "依赖 [[/b]] 与自引用 [[/a]]", "reason": "test"})
	e.Dispatch("lifecycle", map[string]interface{}{"action": "archive","path": "/a", "reason": "test"})

	// Delete /b while /a is archived → restore should warn about the dangling link.
	e.Dispatch("lifecycle", map[string]interface{}{"action": "delete","path": "/b", "reason": "test"})
	resp := e.Dispatch("lifecycle", map[string]interface{}{"action": "restore","path": "/a", "reason": "test"})
	if !resp.Success {
		t.Fatalf("restore failed: %v", resp.Error)
	}
	if resp.Warning == "" || !strings.Contains(resp.Warning, "/b") {
		t.Fatalf("expected dangling-link warning on restore, got %q", resp.Warning)
	}

	// All targets intact → no warning on restore.
	e.Dispatch("create", map[string]interface{}{"path": "/x", "title": "X", "summary": "X", "reason": "test"})
	e.Dispatch("append", map[string]interface{}{"path": "/x", "event": "链接 [[/a]]", "reason": "test"})
	e.Dispatch("lifecycle", map[string]interface{}{"action": "archive","path": "/x", "reason": "test"})
	resp = e.Dispatch("lifecycle", map[string]interface{}{"action": "restore","path": "/x", "reason": "test"})
	if resp.Success && resp.Warning != "" {
		t.Fatalf("expected no warning when targets intact, got %q", resp.Warning)
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
	resp := e.Dispatch("update", map[string]interface{}{
		"path": "/projects/alpha",
		"summary": map[string]interface{}{
			"old": "张三负责后端开发，deadline 2026-09-30", "new": "项目进行中", "reason": "ok",
		},
	})
	if resp.Success {
		t.Fatal("expected lazy reason to be rejected")
	}
	if resp.Error.Code != "SUMMARY_DRIFT_DETECTED" {
		t.Fatalf("expected SUMMARY_DRIFT_DETECTED, got %s", resp.Error.Code)
	}

	// Now with a substantive reason
	resp = e.Dispatch("update", map[string]interface{}{
		"path": "/projects/alpha",
		"summary": map[string]interface{}{
			"old": "张三负责后端开发，deadline 2026-09-30",
			"new": "李四接替负责，deadline 延至 2026-10-15",
			"reason": "张三离职，李四接替，deadline 顺延两周",
		},
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
	e.Dispatch("append", map[string]interface{}{
		"path": "/a", "event": "依赖 [[/b]] 的产出", "reason": "test",
	})

	// /b is linked by /a's body, deleting /b should fail
	resp := e.Dispatch("lifecycle", map[string]interface{}{"action": "delete",
		"path": "/b", "reason": "test",
	})
	if resp.Success {
		t.Fatal("expected delete of linked node to fail")
	}
	if resp.Error.Code != "LINKED_BY_OTHERS" {
		t.Fatalf("expected LINKED_BY_OTHERS, got %s", resp.Error.Code)
	}

	// Delete /a first is fine (its own links don't block its deletion)
	resp = e.Dispatch("lifecycle", map[string]interface{}{"action": "delete",
		"path": "/a", "reason": "test",
	})
	if !resp.Success {
		t.Fatalf("delete of node with outbound links should pass: %v", resp.Error)
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

	resp := e.Dispatch("show", map[string]interface{}{
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

func TestArchive_LinkWarning(t *testing.T) {
	e := setupEngine()
	e.Dispatch("create", map[string]interface{}{"path": "/a", "title": "A", "summary": "A", "reason": "test"})
	e.Dispatch("create", map[string]interface{}{"path": "/b", "title": "B", "summary": "B", "reason": "test"})
	e.Dispatch("append", map[string]interface{}{
		"path": "/a", "event": "参考 [[/b]] 的结论", "reason": "test",
	})

	// Archiving /b succeeds but warns about dangling links
	resp := e.Dispatch("lifecycle", map[string]interface{}{"action": "archive",
		"path": "/b", "reason": "done",
	})
	if !resp.Success {
		t.Fatalf("archive failed: %v", resp.Error)
	}
	if resp.Warning == "" {
		t.Fatal("expected warning about referrers when archiving linked node")
	}
}

func TestUpdateField_Basic(t *testing.T) {
	e := setupEngine()
	e.Dispatch("create", map[string]interface{}{
		"path": "/projects/alpha", "title": "A", "summary": "A", "reason": "test",
	})
	resp := e.Dispatch("update", map[string]interface{}{
		"path": "/projects/alpha", "fields": map[string]interface{}{"status": "active"}, "reason": "init",
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
	e.Dispatch("lifecycle", map[string]interface{}{"action": "archive",
		"path": "/projects/alpha", "reason": "test",
	})
	resp := e.Dispatch("update", map[string]interface{}{
		"path": "/projects/alpha", "fields": map[string]interface{}{"status": "active"}, "reason": "test",
	})
	if resp.Success {
		t.Fatal("update_field on archived node should fail")
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
		resp := e.Dispatch("show", map[string]interface{}{"path": "/tasks/t1", "window": w})
		if !resp.Success {
			t.Fatalf("deep_load window %q failed: %v", w, resp.Error)
		}
	}
}

func TestDeepLoad_InvalidWindow(t *testing.T) {
	e := setupEngine()
	resp := e.Dispatch("show", map[string]interface{}{"path": "/a", "window": "invalid"})
	if resp.Success {
		t.Fatal("invalid window should fail")
	}
}

func TestDeepLoad_NotFound(t *testing.T) {
	e := setupEngine()
	resp := e.Dispatch("show", map[string]interface{}{"path": "/not-found", "window": "recent"})
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

func TestArchive_NoWarningWithoutReferrers(t *testing.T) {
	e := setupEngine()
	e.Dispatch("create", map[string]interface{}{"path": "/a", "title": "A", "summary": "A", "reason": "test"})
	e.Dispatch("create", map[string]interface{}{"path": "/b", "title": "B", "summary": "B", "reason": "test"})
	// /a links to /b, archiving /a (the referrer) must not warn about /b
	e.Dispatch("append", map[string]interface{}{
		"path": "/a", "event": "参考 [[/b]]", "reason": "test",
	})

	resp := e.Dispatch("lifecycle", map[string]interface{}{"action": "archive",
		"path": "/a", "reason": "done",
	})
	if !resp.Success {
		t.Fatalf("archive failed: %v", resp.Error)
	}
	if resp.Warning != "" {
		t.Fatalf("expected no warning (no one links to /a), got %q", resp.Warning)
	}
}

func TestDuplicateArchive(t *testing.T) {
	e := setupEngine()
	e.Dispatch("create", map[string]interface{}{
		"path": "/projects/alpha", "title": "A", "summary": "A", "reason": "test",
	})
	e.Dispatch("lifecycle", map[string]interface{}{"action": "archive",
		"path": "/projects/alpha", "reason": "done",
	})
	resp := e.Dispatch("lifecycle", map[string]interface{}{"action": "archive",
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
	resp := e.Dispatch("lifecycle", map[string]interface{}{"action": "restore",
		"path": "/projects/alpha", "reason": "test",
	})
	if resp.Success {
		t.Fatal("restore on non-archived node should fail")
	}
}

func TestDelete_NotFound(t *testing.T) {
	e := setupEngine()
	resp := e.Dispatch("lifecycle", map[string]interface{}{"action": "delete",
		"path": "/not-found", "reason": "test",
	})
	if resp.Success {
		t.Fatal("delete on non-existent path should fail")
	}
}

