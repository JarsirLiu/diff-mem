// Package agent provides an interactive CLI agent that talks to Diff-Mem over HTTP.
// It auto-maintains its own session memory nodes and implements the retrieve-store cycle.
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/diff-mem/diff-mem/internal/model"
)

const (
	// DefaultServerURL is the default Diff-Mem HTTP server address.
	DefaultServerURL = "http://localhost:8080"
)

// Agent is a CLI agent that interacts with a Diff-Mem HTTP server.
// It auto-maintains its own session nodes and interaction history.
type Agent struct {
	client      *http.Client
	baseURL     string
	sessionPath string // e.g. /agent/session-2026-09-01
	profilePath string // e.g. /agent/profile
}

// RecallResult holds one result from the recall pipeline.
type RecallResult struct {
	Path    string
	Title   string
	Status  string
	Summary string
	Tags    []string
	Events  []EventSummary
}

// EventSummary is a lightweight event representation for recall results.
type EventSummary struct {
	Type      string
	Content   string
	Timestamp string
}

// AgentStatus holds the current agent state.
type AgentStatus struct {
	SessionPath     string
	ProfilePath     string
	ServerURL       string
	SessionExists   bool
	ProfileExists   bool
	InteractionCount int
	LastInteraction *string
}

// New creates a new Agent.
func New(baseURL string) *Agent {
	today := time.Now().Format("2006-01-02")
	return &Agent{
		client: &http.Client{Timeout: 30 * time.Second},
		baseURL: strings.TrimRight(baseURL, "/"),
		sessionPath: fmt.Sprintf("/agent/session-%s", today),
		profilePath: "/agent/profile",
	}
}

// --- Core methods ---

// Init ensures the session and profile nodes exist.
func (a *Agent) Init(ctx context.Context) error {
	if err := a.ensureNode(ctx, a.profilePath, "Agent Profile", "Agent 的配置和元信息", []string{}, "初始化 agent profile"); err != nil {
		return err
	}
	if err := a.ensureNode(ctx, a.sessionPath,
		"Session "+a.sessionPath[strings.LastIndex(a.sessionPath, "/")+1:],
		fmt.Sprintf("Agent 会话记录 — %s", time.Now().Format("2006-01-02")),
		[]string{}, "初始化 agent 会话"); err != nil {
		return err
	}
	return nil
}

// Recall retrieves relevant memories using the search → show → deep_load pipeline.
func (a *Agent) Recall(ctx context.Context, query string) ([]*RecallResult, error) {
	// Step 1: search for candidates
	searchResp, err := a.callTool(ctx, "search", map[string]interface{}{
		"keywords": query,
		"limit":    10,
	})
	if err != nil {
		return nil, err
	}
	if !searchResp.Success {
		return nil, fmt.Errorf("search failed: %s", searchResp.Error.Message)
	}

	// Extract candidate paths
	candidates, err := extractPaths(searchResp)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	// Step 2: show top candidates (up to 3)
	var results []*RecallResult
	for _, path := range candidates[:min(len(candidates), 3)] {
		showResp, err := a.callTool(ctx, "show", map[string]interface{}{"path": path})
		if err != nil {
			continue
		}
		if !showResp.Success {
			continue
		}

		result := &RecallResult{Path: path}
		if r := showResp.Result; r != nil {
			header, err := extractHeader(r)
			if err == nil {
				result.Title = header.Title
				result.Status = string(header.Status)
				result.Summary = header.Summary
				result.Tags = header.Tags
			}
		}
		results = append(results, result)
	}

	// Step 3: deep_load events for each result (top 2)
	for _, r := range results[:min(len(results), 2)] {
		deepResp, err := a.callTool(ctx, "deep_load", map[string]interface{}{
			"path":   r.Path,
			"window": "recent",
		})
		if err != nil || !deepResp.Success {
			continue
		}
		if dr := deepResp.Result; dr != nil {
			if events, err := extractEvents(dr); err == nil {
				r.Events = events
			}
		}
	}

	if len(results) == 0 {
		return nil, nil
	}
	return results, nil
}

// Store appends a memory event to the current session node.
func (a *Agent) Store(ctx context.Context, memory string, reason string) error {
	if strings.TrimSpace(reason) == "" {
		reason = "agent 自动记录"
	}
	resp, err := a.callTool(ctx, "append", map[string]interface{}{
		"path":  a.sessionPath,
		"event": memory,
		"reason": reason,
	})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("store failed: %s", resp.Error.Message)
	}
	return nil
}

// RecordInteraction stores a conversation turn (role + content) into the session.
func (a *Agent) RecordInteraction(ctx context.Context, role, content string) error {
	entry := fmt.Sprintf("[%s] %s", role, content)
	return a.Store(ctx, entry, fmt.Sprintf("记录 %s 轮次", role))
}

// Status returns the current agent state.
func (a *Agent) Status(ctx context.Context) (*AgentStatus, error) {
	status := &AgentStatus{
		SessionPath: a.sessionPath,
		ProfilePath: a.profilePath,
		ServerURL:   a.baseURL,
	}

	// Check session node
	resp, err := a.callTool(ctx, "show", map[string]interface{}{"path": a.sessionPath})
	if err == nil && resp.Success {
		status.SessionExists = true
		if r := resp.Result; r != nil {
			header, hErr := extractHeader(r)
			if hErr == nil {
				status.InteractionCount = header.EventCount
				if header.LastAccessed != nil {
					s := header.LastAccessed.Format(time.RFC3339)
					status.LastInteraction = &s
				}
			}
		}
	}

	// Check profile node
	resp, err = a.callTool(ctx, "show", map[string]interface{}{"path": a.profilePath})
	if err == nil && resp.Success {
		status.ProfileExists = true
	}

	return status, nil
}

// Show retrieves a node's full header by path.
func (a *Agent) Show(ctx context.Context, path string) (*model.Header, error) {
	resp, err := a.callTool(ctx, "show", map[string]interface{}{"path": path})
	if err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf("show failed: %s", resp.Error.Message)
	}
	return extractHeader(resp.Result)
}

// ForgetPath archives a node (soft delete).
func (a *Agent) ForgetPath(ctx context.Context, path string, reason string) error {
	if strings.TrimSpace(reason) == "" {
		reason = "agent 清理旧记忆"
	}
	resp, err := a.callTool(ctx, "archive", map[string]interface{}{
		"path":   path,
		"reason": reason,
	})
	if err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("forget failed: %s", resp.Error.Message)
	}
	return nil
}

// --- Internal helpers ---

// callTool sends a POST to /tools/{name} with the given params.
func (a *Agent) callTool(ctx context.Context, name string, params map[string]interface{}) (*model.ToolResponse, error) {
	body, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("marshal params: %w", err)
	}

	url := fmt.Sprintf("%s/tools/%s", a.baseURL, name)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request to %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, string(b))
	}

	var result model.ToolResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}

// ensureNode creates a node if it doesn't already exist (idempotent).
func (a *Agent) ensureNode(ctx context.Context, path, title, summary string, tags []string, reason string) error {
	exists := a.nodeExists(ctx, path)
	if exists {
		return nil
	}

	var tagArgs []interface{}
	for _, t := range tags {
		tagArgs = append(tagArgs, t)
	}
	_, err := a.callTool(ctx, "create", map[string]interface{}{
		"path":    path,
		"title":   title,
		"summary": summary,
		"tags":    tagArgs,
		"reason":  reason,
	})
	return err
}

func (a *Agent) nodeExists(ctx context.Context, path string) bool {
	resp, err := a.callTool(ctx, "show", map[string]interface{}{"path": path})
	return err == nil && resp.Success
}

// --- Response extractors ---

func extractPaths(resp *model.ToolResponse) ([]string, error) {
	if resp.Result == nil {
		return nil, nil
	}
	b, err := json.Marshal(resp.Result)
	if err != nil {
		return nil, err
	}

	// Try []SearchResultEntry
	var entries []model.SearchResultEntry
	if err := json.Unmarshal(b, &entries); err == nil {
		paths := make([]string, len(entries))
		for i, e := range entries {
			paths[i] = e.Path
		}
		return paths, nil
	}

	// Try map with "results" key
	var wrapper map[string]interface{}
	if err := json.Unmarshal(b, &wrapper); err == nil {
		if results, ok := wrapper["results"]; ok {
			rb, _ := json.Marshal(results)
			var entries []model.SearchResultEntry
			if err := json.Unmarshal(rb, &entries); err == nil {
				paths := make([]string, len(entries))
				for i, e := range entries {
					paths[i] = e.Path
				}
				return paths, nil
			}
		}
	}
	return nil, fmt.Errorf("cannot extract paths from search result")
}

func extractHeader(result interface{}) (*model.Header, error) {
	b, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	var header model.Header
	if err := json.Unmarshal(b, &header); err != nil {
		return nil, err
	}
	return &header, nil
}

func extractEvents(result interface{}) ([]EventSummary, error) {
	b, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}

	var deep model.DeepLoadResult
	if err := json.Unmarshal(b, &deep); err == nil {
		evtSummaries := make([]EventSummary, 0, len(deep.Events))
		for _, e := range deep.Events {
			evtSummaries = append(evtSummaries, EventSummary{
				Type:      e.Type,
				Content:   e.Content,
				Timestamp: e.Timestamp.Format(time.RFC3339),
			})
		}
		return evtSummaries, nil
	}

	// Try raw events array
	var rawEvents []model.Event
	if err := json.Unmarshal(b, &rawEvents); err == nil {
		evtSummaries := make([]EventSummary, 0, len(rawEvents))
		for _, e := range rawEvents {
			evtSummaries = append(evtSummaries, EventSummary{
				Type:      e.Type,
				Content:   e.Content,
				Timestamp: e.Timestamp.Format(time.RFC3339),
			})
		}
		return evtSummaries, nil
	}
	return nil, fmt.Errorf("cannot extract events from deep_load result")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
