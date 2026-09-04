// Package model defines the core data structures for Diff-Mem.
package model

import (
	"time"
)

// Status represents the lifecycle state of a node.
type Status string

const (
	StatusActive   Status = "active"
	StatusArchived Status = "archived"
)

// Header is the lightweight index of a node — metadata + summary + fields.
type Header struct {
	Path         string            `json:"path"`
	Title        string            `json:"title"`
	Status       Status            `json:"status"`
	Tags         []string          `json:"tags"`
	Summary      string            `json:"summary"`
	Fields       map[string]string `json:"fields"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
	EventCount   int               `json:"event_count"`
	LastAccessed *time.Time        `json:"last_accessed,omitempty"`
}

// Event is an immutable record appended to a node's body.
type Event struct {
	ID        string            `json:"id"`
	Type      string            `json:"type"` // "user", "system", "field_change", "summary_update", "archived", "restored"
	Content   string            `json:"content"`
	Meta      map[string]string `json:"meta,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
}

// Node is the complete memory entity — path + header + event stream.
type Node struct {
	Header Header
	Events []Event
}

// ToolRequest is the shape of an incoming tool call.
type ToolRequest struct {
	ToolName string                 `json:"tool"`
	Params   map[string]interface{} `json:"params"`
}

// ToolResponse is the uniform response returned to the agent.
type ToolResponse struct {
	Success bool        `json:"success"`
	Result  interface{} `json:"result,omitempty"`
	Warning string      `json:"warning,omitempty"`
	Error   *ErrorInfo  `json:"error,omitempty"`
}

// ErrorInfo carries structured error details.
type ErrorInfo struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

// SearchResultEntry is one hit from a search or list operation.
type SearchResultEntry struct {
	Path    string   `json:"path"`
	Type    string   `json:"type"` // "node" or "group"
	Summary string   `json:"summary,omitempty"`
	Status  Status   `json:"status,omitempty"`
	Tags    []string `json:"tags,omitempty"`
	Count   int      `json:"count,omitempty"`
}

// ShowResult is the response from showing a node: header + content links + backlinks.
type ShowResult struct {
	Header    Header   `json:"header"`
	Links     []string `json:"links,omitempty"`
	Backlinks []string `json:"backlinks,omitempty"`
}

// DeepLoadResult is the response from deep loading a node's body.
type DeepLoadResult struct {
	Path    string   `json:"path"`
	Events  []Event  `json:"events"`
	Total   int      `json:"total"`
	HasMore bool     `json:"has_more"`
	Links   []string `json:"links,omitempty"`
}

// SearchOptions holds parameters for search/list queries.
type SearchOptions struct {
	Tags            []string
	Keywords        string
	Limit           int
	IncludeArchived bool
}

// CreateOptions holds parameters for node creation.
type CreateOptions struct {
	Path          string
	Title         string
	Summary       string
	Tags          []string
	InitialEvents []string
	Reason        string
}

// UpdateSummaryOptions holds parameters for summary updates.
type UpdateSummaryOptions struct {
	Path       string
	OldSummary string
	NewSummary string
	Reason     string
}

// ArchiveOptions holds parameters for archive/restore.
type ArchiveOptions struct {
	Path   string
	Reason string
}

// UpdateFieldOptions holds parameters for field updates.
type UpdateFieldOptions struct {
	Path   string
	Field  string
	Value  string
	Reason string
}

// UpdateOptions holds parameters for the combined update tool:
// batch Header fields and/or a summary refresh in one call.
type UpdateOptions struct {
	Path          string
	Fields        map[string]string
	FieldReason   string
	OldSummary    string
	NewSummary    string
	SummaryReason string
}

// AppendOptions holds parameters for appending events.
type AppendOptions struct {
	Path   string
	Event  string
	Reason string
}
