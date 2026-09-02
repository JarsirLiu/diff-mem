package engine_test

import (
	"testing"

	"github.com/diff-mem/diff-mem/internal/engine"
)

func TestExtractEntities_Numbers(t *testing.T) {
	entities := engine.ExtractEntities("2026-09-30")
	found := false
	for _, e := range entities {
		if e == "2026-09-30" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected '2026-09-30' in entities, got %v", entities)
	}
}

func TestExtractEntities_ChineseNames(t *testing.T) {
	entities := engine.ExtractEntities("张三")
	found := false
	for _, e := range entities {
		if e == "张三" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected '张三' in entities, got %v", entities)
	}
}

func TestExtractEntities_ChineseNamesWithDelimiters(t *testing.T) {
	entities := engine.ExtractEntities("负责人：张三")
	found := false
	for _, e := range entities {
		if e == "张三" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected '张三' in entities, got %v", entities)
	}
}

func TestExtractEntities_NoEntities(t *testing.T) {
	entities := engine.ExtractEntities("")
	if len(entities) != 0 {
		t.Fatalf("empty string should produce no entities, got %v", entities)
	}
}

func TestExtractEntities_Deduplication(t *testing.T) {
	entities := engine.ExtractEntities("2026 2026 2026")
	count := 0
	for _, e := range entities {
		if e == "2026" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected deduplicated '2026', got %d occurrences", count)
	}
}

func TestDisappearedEntities(t *testing.T) {
	old := "张三负责，deadline 2026-09-30"
	new := "李四负责，deadline 2026-10-15"

	disappeared := engine.DisappearedEntities(old, new)
	disMap := map[string]bool{}
	for _, e := range disappeared {
		disMap[e] = true
	}
	if !disMap["2026-09-30"] {
		t.Fatalf("expected '2026-09-30' disappeared, got %v", disappeared)
	}
	if disMap["2026-10-15"] {
		t.Fatalf("'2026-10-15' should not be disappeared (it's in new), got %v", disappeared)
	}
}

func TestDisappearedEntities_None(t *testing.T) {
	old := "张三负责"
	new := "张三负责，项目完成"

	disappeared := engine.DisappearedEntities(old, new)
	if len(disappeared) != 0 {
		t.Fatalf("expected no disappeared entities, got %v", disappeared)
	}
}

func TestDisappearedEntities_AllGone(t *testing.T) {
	old := "2026-09-30 项目"
	new := "项目继续"

	disappeared := engine.DisappearedEntities(old, new)
	disMap := map[string]bool{}
	for _, e := range disappeared {
		disMap[e] = true
	}
	if !disMap["2026-09-30"] {
		t.Fatalf("expected '2026-09-30' disappeared, got %v", disappeared)
	}
}

func TestExtractContentLinks(t *testing.T) {
	links := engine.ExtractContentLinks("参见 [[/a/b]] 和 [[/c]]，重复 [[/a/b]] 无效文本 [[not-a-path]]")
	if len(links) != 3 {
		t.Fatalf("expected 3 links, got %v", links)
	}
	if links[0] != "/a/b" || links[1] != "/c" {
		t.Fatalf("unexpected links: %v", links)
	}
}

func TestExtractContentLinks_Empty(t *testing.T) {
	if links := engine.ExtractContentLinks("no links here"); len(links) != 0 {
		t.Fatalf("expected no links, got %v", links)
	}
}
