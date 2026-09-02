// Summary entity extraction (for drift detection).
package engine

import "strings"

// ExtractEntities extracts key entities from a summary string.
// Returns names, numbers, dates, and proper nouns.
func ExtractEntities(summary string) []string {
	entities := []string{}
	// Extract numbers
	fieldScan(summary, func(r rune) bool { return r >= '0' && r <= '9' }, func(s string) {
		entities = append(entities, s)
	})
	// Extract Chinese names (2-4 Chinese chars)
	fieldScan(summary, func(r rune) bool { return r >= 0x4e00 && r <= 0x9fff }, func(s string) {
		if len([]rune(s)) >= 2 && len([]rune(s)) <= 4 {
			entities = append(entities, s)
		}
	})
	// Extract date-like patterns (YYYY-MM-DD or M月D日)
	fieldScan(summary, func(r rune) bool {
		return (r >= '0' && r <= '9') || r == '-' || r == '/' || r == '月' || r == '日'
	}, func(s string) {
		if len(s) >= 3 {
			entities = append(entities, s)
		}
	})
	// Deduplicate
	seen := map[string]bool{}
	deduped := []string{}
	for _, e := range entities {
		if !seen[e] {
			seen[e] = true
			deduped = append(deduped, e)
		}
	}
	return deduped
}

// fieldScan scans a string collecting contiguous runs of runes matching predicate.
func fieldScan(s string, predicate func(rune) bool, onField func(string)) {
	runes := []rune(s)
	field := ""
	for _, r := range runes {
		if predicate(r) {
			field += string(r)
		} else {
			if field != "" {
				onField(field)
				field = ""
			}
		}
	}
	if field != "" {
		onField(field)
	}
}

// DisappearedEntities returns entities in old that are not in new.
func DisappearedEntities(oldSummary, newSummary string) []string {
	oldEntities := ExtractEntities(oldSummary)
	newEntities := ExtractEntities(newSummary)
	newSet := map[string]bool{}
	for _, e := range newEntities {
		newSet[e] = true
	}
	var disappeared []string
	for _, e := range oldEntities {
		if !newSet[e] && !strings.Contains(newSummary, e) {
			disappeared = append(disappeared, e)
		}
	}
	return disappeared
}
