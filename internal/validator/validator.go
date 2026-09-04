// Package validator implements all semantic validation rules for Diff-Mem operations.
package validator

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/diff-mem/diff-mem/internal/model"
)

var (
	// fieldRe validates field names.
	fieldRe = regexp.MustCompile(`^[a-z0-9_]+$`)
)

// protectedFields must never be written via the generic fields mechanism:
// status has its own lifecycle tool, summary has dedicated drift-checked
// update semantics (docs/04-state-drift-defense.md).
var protectedFields = map[string]bool{
	"status":  true,
	"summary": true,
}

// memoryRoots are the two enforced top-level memory areas (docs/02 §四):
// short-term for recent events, long-term for durable knowledge.
var memoryRoots = map[string]bool{
	"short-term": true,
	"long-term":  true,
}

// ValidatePath checks path format rules.
func ValidatePath(path string) string {
	if path == "" {
		return "path is required"
	}
	if !strings.HasPrefix(path, "/") {
		return "path must start with /"
	}
	if strings.HasSuffix(path, "/") && path != "/" {
		return "path must not end with /"
	}
	if strings.Contains(path, "//") {
		return "path must not contain empty segments"
	}
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")
	// 5 usable levels below the mandatory memory root.
	if len(segments) > 6 {
		return "path must not exceed 5 levels below the memory root"
	}
	// Two-root structure: every path lives under short-term or long-term,
	// and the roots themselves are directories, not nodes.
	if !memoryRoots[segments[0]] {
		return "path must start with /short-term or /long-term, got /" + segments[0]
	}
	if len(segments) < 2 {
		return "path must have at least one level below /" + segments[0]
	}
	for _, seg := range segments {
		if len(seg) > 100 {
			return "path segment must not exceed 100 characters"
		}
		for _, r := range seg {
			if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || (r >= 0x4e00 && r <= 0x9fff)) {
				return "path contains invalid character: " + string(r)
			}
		}
	}
	return ""
}

// ValidateCreate checks CREATE parameters.
func ValidateCreate(opts model.CreateOptions) string {
	if err := ValidatePath(opts.Path); err != "" {
		return "invalid path: " + err
	}
	if strings.TrimSpace(opts.Title) == "" {
		return "title is required"
	}
	if strings.TrimSpace(opts.Summary) == "" {
		return "summary is required"
	}
	if len(opts.Summary) > 500 {
		return "summary must not exceed 500 characters"
	}
	if len(opts.Tags) > 20 {
		return "tags must not exceed 20 items"
	}
	if strings.TrimSpace(opts.Reason) == "" {
		return "reason is required"
	}
	return ""
}

// ValidateAppend checks APPEND parameters.
func ValidateAppend(opts model.AppendOptions) string {
	if err := ValidatePath(opts.Path); err != "" {
		return "invalid path: " + err
	}
	if strings.TrimSpace(opts.Event) == "" {
		return "event is required"
	}
	if len(opts.Event) > 2000 {
		return "event must not exceed 2000 characters"
	}
	if strings.TrimSpace(opts.Reason) == "" {
		return "reason is required"
	}
	return ""
}

// ValidateUpdateField checks UPDATE_FIELD parameters.
func ValidateUpdateField(opts model.UpdateFieldOptions) string {
	if err := ValidatePath(opts.Path); err != "" {
		return "invalid path: " + err
	}
	if strings.TrimSpace(opts.Field) == "" {
		return "field is required"
	}
	if !fieldRe.MatchString(opts.Field) {
		return "field must contain only a-z, 0-9, underscore"
	}
	if protectedFields[opts.Field] {
		return "field \"" + opts.Field + "\" is protected: use diff_mem_lifecycle for status or diff_mem_update summary for summary"
	}
	if len(opts.Value) > 5000 {
		return "value must not exceed 5000 characters"
	}
	if strings.TrimSpace(opts.Reason) == "" {
		return "reason is required"
	}
	return ""
}

// ValidateUpdateSummary checks UPDATE_SUMMARY parameters.
func ValidateUpdateSummary(opts model.UpdateSummaryOptions) string {
	if err := ValidatePath(opts.Path); err != "" {
		return "invalid path: " + err
	}
	if strings.TrimSpace(opts.NewSummary) == "" {
		return "new_summary is required"
	}
	if len(opts.NewSummary) > 500 {
		return "new_summary must not exceed 500 characters"
	}
	if strings.TrimSpace(opts.Reason) == "" {
		return "reason is required"
	}
	return ""
}

// ValidateArchive checks ARCHIVE parameters.
func ValidateArchive(opts model.ArchiveOptions) string {
	if err := ValidatePath(opts.Path); err != "" {
		return "invalid path: " + err
	}
	if strings.TrimSpace(opts.Reason) == "" {
		return "reason is required"
	}
	return ""
}

// ValidateSummaryDrift checks if a summary update has disappearing entities
// and whether the reason is substantive enough.
func ValidateSummaryDrift(disappeared []string, reason string) (bool, []string) {
	if len(disappeared) == 0 {
		return false, nil
	}
	// Check if reason is substantive
	trimmed := strings.TrimSpace(reason)
	if len(trimmed) < 10 {
		return true, disappeared
	}
	// Check for lazy/敷衍 answers
	lazyPatterns := []string{"不需要", "没用了", "none", "n/a", "na", "ok", "yes", "no", "无所谓"}
	for _, pattern := range lazyPatterns {
		if strings.ToLower(trimmed) == pattern {
			return true, disappeared
		}
	}
	return false, nil
}
