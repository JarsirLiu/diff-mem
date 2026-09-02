// Write operations: Create, Append, UpdateField, UpdateSummary.
package engine

import (
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/diff-mem/diff-mem/internal/model"
	"github.com/diff-mem/diff-mem/internal/validator"
)

func (e *Engine) Create(opts model.CreateOptions) *model.ToolResponse {
	if err := validator.ValidateCreate(opts); err != "" {
		return fail("VALIDATION_FAILED", err)
	}
	if e.store.Exists(opts.Path) {
		return fail("PATH_EXISTS", "path already exists: "+opts.Path, "use diff_mem_show to inspect existing node")
	}
	// Content-link gate: every [[path]] in body events must resolve (zero side effects on failure).
	if resp := e.validateContentLinks(opts.Path, opts.InitialEvents...); resp != nil {
		return resp
	}
	for _, p := range collectParents(opts.Path) {
		if !e.store.Exists(p) {
			autoCreateParent(e, p)
		}
	}

	node := &model.Node{
		Header: model.Header{
			Path:       opts.Path,
			Title:      opts.Title,
			Status:     model.StatusActive,
			Tags:       opts.Tags,
			Summary:    opts.Summary,
			Fields:     make(map[string]string),
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
			EventCount: len(opts.InitialEvents),
		},
		Events: make([]model.Event, 0, len(opts.InitialEvents)+1),
	}
	node.Events = append(node.Events, model.Event{
		ID: uuid.NewString(), Type: "create",
		Content: opts.Title + ": " + opts.Summary,
		Meta: map[string]string{"reason": opts.Reason}, Timestamp: time.Now(),
	})
	for _, ev := range opts.InitialEvents {
		node.Events = append(node.Events, model.Event{
			ID: uuid.NewString(), Type: "user",
			Content: ev, Timestamp: time.Now(),
		})
	}
	e.store.PutNode(node)
	e.links.addLinks(opts.Path, nodeContentLinks(node))
	return success(node.Header)
}

func (e *Engine) Append(opts model.AppendOptions) *model.ToolResponse {
	if err := validator.ValidateAppend(opts); err != "" {
		return fail("VALIDATION_FAILED", err)
	}
	node, ok := e.store.GetNode(opts.Path)
	if !ok {
		return fail("PATH_NOT_FOUND", "path not found: "+opts.Path)
	}
	if node.Header.Status == model.StatusArchived {
		return fail("NODE_ARCHIVED", "node is archived, cannot append events")
	}
	// Content-link gate: every [[path]] in the event must resolve (zero side effects on failure).
	if resp := e.validateContentLinks(opts.Path, opts.Event); resp != nil {
		return resp
	}
	event := model.Event{
		ID: uuid.NewString(), Type: "user",
		Content: opts.Event,
		Meta: map[string]string{"reason": opts.Reason}, Timestamp: time.Now(),
	}
	node.Events = append(node.Events, event)
	node.Header.EventCount = len(node.Events)
	node.Header.UpdatedAt = time.Now()
	e.store.PutNode(node)
	e.links.addLinks(opts.Path, ExtractContentLinks(opts.Event))
	return success(event)
}

// Update mutates a node's Header in one call: batch fields and/or summary
// refresh. Summary is pre-validated (mismatch + drift) before any write so
// the call stays all-or-nothing for its common failure modes.
func (e *Engine) Update(opts model.UpdateOptions) *model.ToolResponse {
	node, ok := e.store.GetNode(opts.Path)
	if !ok {
		return fail("PATH_NOT_FOUND", "path not found: "+opts.Path)
	}
	if node.Header.Status == model.StatusArchived {
		return fail("NODE_ARCHIVED", "node is archived, cannot update")
	}
	if len(opts.Fields) == 0 && opts.NewSummary == "" {
		return fail("VALIDATION_FAILED", "update requires fields and/or summary")
	}
	// Pre-validate summary before applying anything.
	if opts.NewSummary != "" {
		if node.Header.Summary != opts.OldSummary {
			return fail("SUMMARY_MISMATCH", "old_summary does not match current summary")
		}
		disappeared := DisappearedEntities(opts.OldSummary, opts.NewSummary)
		needsReason, _ := validator.ValidateSummaryDrift(disappeared, opts.SummaryReason)
		if needsReason {
			return &model.ToolResponse{
				Success: false,
				Error: &model.ErrorInfo{
					Code:       "SUMMARY_DRIFT_DETECTED",
					Message:    "entities disappeared from summary: " + strings.Join(disappeared, ", "),
					Suggestion: "请补充 reason 解释为什么这些实体被移除",
				},
			}
		}
	}
	// Apply fields in stable order.
	keys := make([]string, 0, len(opts.Fields))
	for k := range opts.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		resp := e.UpdateField(model.UpdateFieldOptions{
			Path: opts.Path, Field: k, Value: opts.Fields[k], Reason: opts.FieldReason,
		})
		if !resp.Success {
			return resp
		}
	}
	if opts.NewSummary != "" {
		return e.UpdateSummary(model.UpdateSummaryOptions{
			Path: opts.Path, OldSummary: opts.OldSummary,
			NewSummary: opts.NewSummary, Reason: opts.SummaryReason,
		})
	}
	updated, _ := e.store.GetNode(opts.Path)
	return success(updated.Header)
}

func (e *Engine) UpdateField(opts model.UpdateFieldOptions) *model.ToolResponse {
	if err := validator.ValidateUpdateField(opts); err != "" {
		return fail("VALIDATION_FAILED", err)
	}
	node, ok := e.store.GetNode(opts.Path)
	if !ok {
		return fail("PATH_NOT_FOUND", "path not found: "+opts.Path)
	}
	if node.Header.Status == model.StatusArchived {
		return fail("NODE_ARCHIVED", "node is archived, cannot update fields")
	}
	oldValue := node.Header.Fields[opts.Field]
	node.Header.Fields[opts.Field] = opts.Value
	node.Header.UpdatedAt = time.Now()
	node.Events = append(node.Events, model.Event{
		ID: uuid.NewString(), Type: "field_change",
		Content: "field " + opts.Field + " updated",
		Meta: map[string]string{"field": opts.Field, "old": oldValue, "new": opts.Value, "reason": opts.Reason},
		Timestamp: time.Now(),
	})
	node.Header.EventCount = len(node.Events)
	e.store.PutNode(node)
	return success(node.Header.Fields)
}

func (e *Engine) UpdateSummary(opts model.UpdateSummaryOptions) *model.ToolResponse {
	if err := validator.ValidateUpdateSummary(opts); err != "" {
		return fail("VALIDATION_FAILED", err)
	}
	node, ok := e.store.GetNode(opts.Path)
	if !ok {
		return fail("PATH_NOT_FOUND", "path not found: "+opts.Path)
	}
	if node.Header.Status == model.StatusArchived {
		return fail("NODE_ARCHIVED", "node is archived, cannot update summary")
	}
	if node.Header.Summary != opts.OldSummary {
		return fail("SUMMARY_MISMATCH", "old_summary does not match current summary")
	}

	disappeared := DisappearedEntities(opts.OldSummary, opts.NewSummary)
	needsReason, _ := validator.ValidateSummaryDrift(disappeared, opts.Reason)
	if needsReason {
		return &model.ToolResponse{
			Success: false,
			Error: &model.ErrorInfo{
				Code:       "SUMMARY_DRIFT_DETECTED",
				Message:    "entities disappeared from summary: " + strings.Join(disappeared, ", "),
				Suggestion: "请补充 reason 解释为什么这些实体被移除",
			},
		}
	}

	node.Header.Summary = opts.NewSummary
	node.Header.UpdatedAt = time.Now()
	node.Events = append(node.Events, model.Event{
		ID: uuid.NewString(), Type: "summary_update",
		Content: "summary updated",
		Meta: map[string]string{"old": opts.OldSummary, "new": opts.NewSummary,
			"removed_entities": strings.Join(disappeared, ","), "reason": opts.Reason},
		Timestamp: time.Now(),
	})
	node.Header.EventCount = len(node.Events)
	e.store.PutNode(node)
	return success(node.Header)
}
