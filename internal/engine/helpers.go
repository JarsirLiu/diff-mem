// Helpers: response builders, path utilities, common utilities.
package engine

import (
	"strings"

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
		Error:   &model.ErrorInfo{Code: code, Message: message, Suggestion: s},
	}
}

// NormalizePath applies the engine's documented path normalization
// (docs/02-path-naming.md): trim spaces, lowercase, collapse duplicate
// separators, strip characters outside [a-z0-9_-] and CJK, collapse
// empty segments. The result is what gets validated and stored.
func NormalizePath(path string) string {
	p := strings.TrimSpace(path)
	var b strings.Builder
	b.Grow(len(p))
	prevSep := false
	for _, r := range p {
		switch {
		case r == '/':
			if !prevSep {
				b.WriteRune('/')
				prevSep = true
			}
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + 32)
			prevSep = false
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_':
			b.WriteRune(r)
			prevSep = false
		case r >= 0x4e00 && r <= 0x9fff:
			b.WriteRune(r)
			prevSep = false
		default:
			// Strip special characters (documented normalization step).
			// A dropped rune may separate two words; emit '-' so
			// "Alpha Beta" doesn't fuse into "alphabeta".
			if !prevSep {
				b.WriteRune('-')
				prevSep = true
			}
		}
	}
	return strings.TrimRight(b.String(), "-/")
}

// autoCreateParent was removed: directories are virtual (pure prefix
// addressing). Nodes may live under non-existent intermediate paths;
// listing works off prefix scans instead of physical parent nodes.

func takeLast(events []model.Event, n int) []model.Event {
	if len(events) <= n {
		return events
	}
	return events[len(events)-n:]
}
