package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/diff-mem/diff-mem/internal/api"
	"github.com/diff-mem/diff-mem/internal/engine"
	"github.com/diff-mem/diff-mem/internal/store"
)

// testServer creates a mock Diff-Mem HTTP server with an in-memory store.
func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	s := store.NewMemoryStore()
	e := engine.New(s)
	srv := api.New(e)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)
	return httptest.NewServer(mux)
}

func TestAgent_Init(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	a := New(ts.URL)
	ctx := context.Background()
	err := a.Init(ctx)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Idempotent
	err = a.Init(ctx)
	if err != nil {
		t.Fatalf("second Init failed: %v", err)
	}
}

func TestAgent_Store(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	a := New(ts.URL)
	ctx := context.Background()
	if err := a.Init(ctx); err != nil {
		t.Fatal(err)
	}

	err := a.Store(ctx, "测试存储内容", "测试存储")
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}
}

func TestAgent_StoreThenRecall(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	a := New(ts.URL)
	ctx := context.Background()
	if err := a.Init(ctx); err != nil {
		t.Fatal(err)
	}

	// Store a fact about 张三
	if err := a.Store(ctx, "明天下午三点跟张三开会讨论 API 设计", "用户记录会议"); err != nil {
		t.Fatal(err)
	}

	// Update summary to include keywords (search indexes header, not body)
	showResp, err := a.callTool(ctx, "show", map[string]interface{}{"path": a.sessionPath})
	if err != nil || !showResp.Success {
		t.Fatalf("show failed: %v", err)
	}
	header, err := extractHeader(showResp.Result)
	if err != nil {
		t.Fatal(err)
	}

	_, err = a.callTool(ctx, "update", map[string]interface{}{
		"path": a.sessionPath,
		"summary": map[string]interface{}{
			"old": header.Summary, "new": header.Summary + " 张三 API 设计",
			"reason": "更新摘要包含关键词以便检索",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Now search should find it
	results, err := a.Recall(ctx, "张三")
	if err != nil {
		t.Fatalf("Recall failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected to find session node when searching 张三")
	}
}

func TestAgent_RecallNotFound(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	a := New(ts.URL)
	ctx := context.Background()
	if err := a.Init(ctx); err != nil {
		t.Fatal(err)
	}

	results, err := a.Recall(ctx, "完全不存在的关键词xyz123")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("expected no results, got %d", len(results))
	}
}

func TestAgent_RecordInteraction(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	a := New(ts.URL)
	ctx := context.Background()
	if err := a.Init(ctx); err != nil {
		t.Fatal(err)
	}

	err := a.RecordInteraction(ctx, "user", "你好")
	if err != nil {
		t.Fatal(err)
	}
	err = a.RecordInteraction(ctx, "agent", "你好！")
	if err != nil {
		t.Fatal(err)
	}
}

func TestAgent_Status(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	a := New(ts.URL)
	ctx := context.Background()
	if err := a.Init(ctx); err != nil {
		t.Fatal(err)
	}

	status, err := a.Status(ctx)
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if status.SessionPath == "" {
		t.Fatal("expected non-empty session path")
	}
	if !status.SessionExists {
		t.Fatal("expected session to exist")
	}
	if !status.ProfileExists {
		t.Fatal("expected profile to exist")
	}
}

func TestAgent_ForgetPath(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	a := New(ts.URL)
	ctx := context.Background()
	if err := a.Init(ctx); err != nil {
		t.Fatal(err)
	}

	// Archive the profile path
	err := a.ForgetPath(ctx, a.profilePath, "测试归档")
	if err != nil {
		t.Fatalf("ForgetPath failed: %v", err)
	}

	// Verify archived by checking status
	resp, err := a.callTool(ctx, "show", map[string]interface{}{
		"path": a.profilePath,
	})
	if err != nil || !resp.Success {
		t.Fatal("show on archived node should still work")
	}
	header, err := extractHeader(resp.Result)
	if err != nil {
		t.Fatal(err)
	}
	if header.Status != "archived" {
		t.Fatalf("expected status 'archived', got '%s'", header.Status)
	}
}

func TestAgent_Show(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	a := New(ts.URL)
	ctx := context.Background()
	if err := a.Init(ctx); err != nil {
		t.Fatal(err)
	}

	header, err := a.Show(ctx, a.sessionPath)
	if err != nil {
		t.Fatalf("Show failed: %v", err)
	}
	if header.Path != a.sessionPath {
		t.Fatalf("expected path %s, got %s", a.sessionPath, header.Path)
	}
	if header.Title == "" {
		t.Fatal("expected non-empty title")
	}
	if header.Status != "active" {
		t.Fatalf("expected status 'active', got '%s'", header.Status)
	}
}

func TestCallTool_Error(t *testing.T) {
	a := New("http://localhost:1")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := a.callTool(ctx, "show", map[string]interface{}{"path": "/long-term/x"})
	if err == nil {
		t.Fatal("expected error when server is unreachable")
	}
}

func TestRepl_Quit(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	a := New(ts.URL)
	ctx := context.Background()

	input := "hello\n.quit\n"
	var output strings.Builder

	err := RunREPL(ctx, a, strings.NewReader(input), &output)
	if err != nil {
		t.Fatalf("RunREPL failed: %v", err)
	}

	out := output.String()
	if !strings.Contains(out, "goodbye") {
		t.Fatalf("expected goodbye in output, got: %s", out)
	}
}

func TestRepl_Store(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	a := New(ts.URL)
	ctx := context.Background()

	input := "记住这件事\n.quit\n"
	var output strings.Builder

	err := RunREPL(ctx, a, strings.NewReader(input), &output)
	if err != nil {
		t.Fatalf("RunREPL failed: %v", err)
	}

	out := output.String()
	if !strings.Contains(out, "已记录") {
		t.Fatalf("expected 已记录 in output, got: %s", out)
	}
}

func TestRepl_Recall(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	a := New(ts.URL)
	ctx := context.Background()

	input := "今天做了什么？\n.quit\n"
	var output strings.Builder

	err := RunREPL(ctx, a, strings.NewReader(input), &output)
	if err != nil {
		t.Fatalf("RunREPL failed: %v", err)
	}

	out := output.String()
	if !strings.Contains(out, "检索") {
		t.Fatalf("expected 检索 in output, got: %s", out)
	}
}

func TestRepl_Help(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	a := New(ts.URL)
	ctx := context.Background()

	input := ".help\n.quit\n"
	var output strings.Builder

	err := RunREPL(ctx, a, strings.NewReader(input), &output)
	if err != nil {
		t.Fatalf("RunREPL failed: %v", err)
	}

	out := output.String()
	if !strings.Contains(out, ".recall") {
		t.Fatalf("expected .recall in help output, got: %s", out)
	}
	if !strings.Contains(out, ".store") {
		t.Fatalf("expected .store in help output, got: %s", out)
	}
}

func TestAgent_StoreWithEmptyReason(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	a := New(ts.URL)
	ctx := context.Background()
	if err := a.Init(ctx); err != nil {
		t.Fatal(err)
	}

	err := a.Store(ctx, "test content", "")
	if err != nil {
		t.Fatal(err)
	}
}

func TestAgent_MultipleSessions(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	a1 := New(ts.URL)
	a2 := New(ts.URL)
	ctx := context.Background()

	if err := a1.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a2.Init(ctx); err != nil {
		t.Fatal(err)
	}

	// Same day → same session path
	if a1.sessionPath != a2.sessionPath {
		t.Fatalf("expected same session path, got %s vs %s", a1.sessionPath, a2.sessionPath)
	}
}

func TestExtractEvents_DeepLoadResponse(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	a := New(ts.URL)
	ctx := context.Background()
	if err := a.Init(ctx); err != nil {
		t.Fatal(err)
	}

	// Store two events
	a.Store(ctx, "event one", "test")
	a.Store(ctx, "event two", "test")

	resp, err := a.callTool(ctx, "show", map[string]interface{}{
		"path":   a.sessionPath,
		"window": "recent",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Success {
		t.Fatal(resp.Error)
	}

	events, err := extractEvents(resp.Result)
	if err != nil {
		t.Fatalf("extractEvents failed: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected events")
	}

	// The first event in the node is always a "create" event from Init()
	// Followed by our stored "user" events
	if events[0].Type != "create" {
		t.Fatalf("expected first event type 'create', got '%s'", events[0].Type)
	}
	// Last events should be our stored "user" events
	foundUser := false
	for _, e := range events {
		if e.Type == "user" {
			foundUser = true
			break
		}
	}
	if !foundUser {
		t.Fatal("expected at least one 'user' type event")
	}
}

func TestAgent_FullLifecycle(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	a := New(ts.URL)
	ctx := context.Background()

	// 1. Init
	if err := a.Init(ctx); err != nil {
		t.Fatal(err)
	}

	// 2. Store
	if err := a.Store(ctx, "关键信息：项目A需要延期", "用户记录"); err != nil {
		t.Fatal(err)
	}

	// 3. Record interaction
	if err := a.RecordInteraction(ctx, "user", "项目A怎么样"); err != nil {
		t.Fatal(err)
	}

	// 4. Update summary to include keywords (search indexes header)
	showResp, err := a.callTool(ctx, "show", map[string]interface{}{
		"path": a.sessionPath,
	})
	if err != nil || !showResp.Success {
		t.Fatal("show failed")
	}
	header, err := extractHeader(showResp.Result)
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.callTool(ctx, "update", map[string]interface{}{
		"path": a.sessionPath,
		"summary": map[string]interface{}{
			"old": header.Summary, "new": header.Summary + " 项目A",
			"reason": "添加关键词以便检索测试",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// 5. Recall should find it
	results, err := a.Recall(ctx, "项目A")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected to find project A")
	}

	// 6. Status
	status, err := a.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.InteractionCount < 2 {
		t.Fatalf("expected >= 2 interactions, got %d", status.InteractionCount)
	}

	// 7. Forget
	if err := a.ForgetPath(ctx, a.profilePath, "清理"); err != nil {
		t.Fatal(err)
	}

	// 8. Verify profile is archived
	showResp, _ = a.callTool(ctx, "show", map[string]interface{}{
		"path": a.profilePath,
	})
	if showResp.Success {
		h, _ := extractHeader(showResp.Result)
		if h.Status != "archived" {
			t.Fatal("profile should be archived")
		}
	}
}

func TestCallTool_BadParams(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	a := New(ts.URL)
	ctx := context.Background()

	// Send malformed params that should trigger VALIDATION_FAILED
	resp, err := a.callTool(ctx, "create", map[string]interface{}{
		"path": "no-leading-slash", "title": "T", "summary": "S", "reason": "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Success {
		t.Fatal("expected validation failure for bad path")
	}
}

func TestNew_Defaults(t *testing.T) {
	a := New(DefaultServerURL)
	if a.baseURL != DefaultServerURL {
		t.Fatalf("expected baseURL %s, got %s", DefaultServerURL, a.baseURL)
	}
	if !strings.HasPrefix(a.sessionPath, "/short-term/agent/session-") {
		t.Fatalf("unexpected session path: %s", a.sessionPath)
	}
	if a.profilePath != "/long-term/agent/profile" {
		t.Fatalf("unexpected profile path: %s", a.profilePath)
	}
	if a.client == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestAgent_BaseURLTrimSlash(t *testing.T) {
	a := New("http://localhost:8080/")
	if a.baseURL != "http://localhost:8080" {
		t.Fatalf("expected trailing slash trimmed, got %s", a.baseURL)
	}
}

// Ensure agent compiles with its deps
var _ = sync.Mutex{}
var _ = time.Second
var _ = json.Marshal
