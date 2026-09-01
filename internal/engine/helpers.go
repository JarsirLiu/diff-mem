// Helpers: response builders, path utilities, common utilities.
package engine

import (
	"strings"
	"time"

	"github.com/diff-mem/diff-mem/internal/model"
)

func success(result interface{}) *model.ToolResponse {
	return &model.ToolResponse{Success: true, Result: result}
}

func fail(code, message string, suggestion ...string) *model.ToolResponse {
	s := ""
	if len(suggestion) > 0 {
		s = suggestion[0]
	}
	return &model.ToolResponse{
		Success: false,
		Error: &model.ErrorInfo{Code: code, Message: message, Suggestion: s},
	}
}

// parentPath returns the immediate parent of a path, or "" for root-level.
func parentPath(path string) string {
	parts := strings.Split(strings.TrimSuffix(path, "/"), "/")
	if len(parts) <= 2 {
		return ""
	}
	return strings.Join(parts[:len(parts)-1], "/")
}

// collectParents returns all intermediate parent paths, shallowest first.
func collectParents(path string) []string {
	parts := strings.Split(strings.TrimSuffix(path, "/"), "/")
	if len(parts) <= 2 {
		return nil
	}
	var parents []string
	for i := 2; i < len(parts); i++ {
		parents = append(parents, "/"+strings.Join(parts[1:i], "/"))
	}
	return parents
}

// autoCreateParent creates a minimal parent node with auto-generated metadata.
func autoCreateParent(e *Engine, path string) {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	last := parts[len(parts)-1]
	title := last
	if len(title) > 0 {
		runes := []rune(title)
		runes[0] = rune(runes[0] - 32)
		title = string(runes)
	}
	e.store.PutNode(&model.Node{
		Header: model.Header{
			Path:      path,
			Title:     title,
			Status:    model.StatusActive,
			Tags:      []string{},
			Summary:   "",
			Fields:    make(map[string]string),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			EventCount: 0,
		},
	})
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func takeLast(events []model.Event, n int) []model.Event {
	if len(events) <= n {
		return events
	}
	return events[len(events)-n:]
}
