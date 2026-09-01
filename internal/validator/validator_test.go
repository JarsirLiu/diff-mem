package validator_test

import (
	"strings"
	"testing"

	"github.com/diff-mem/diff-mem/internal/model"
	"github.com/diff-mem/diff-mem/internal/validator"
)

func TestValidatePath_Valid(t *testing.T) {
	cases := []string{
		"/projects",
		"/projects/alpha",
		"/projects/alpha/backend-api",
		"/tasks/修复登录模块",
		"/a/b/c/d/e",
		"/a_b",
		"/a-b",
	}
	for _, p := range cases {
		if err := validator.ValidatePath(p); err != "" {
			t.Errorf("ValidatePath(%q) should be valid, got: %s", p, err)
		}
	}
}

func TestValidatePath_Empty(t *testing.T) {
	err := validator.ValidatePath("")
	if err == "" {
		t.Fatal("empty path should be rejected")
	}
}

func TestValidatePath_NoLeadingSlash(t *testing.T) {
	err := validator.ValidatePath("projects/alpha")
	if err == "" {
		t.Fatal("path without leading / should be rejected")
	}
}

func TestValidatePath_TrailingSlash(t *testing.T) {
	err := validator.ValidatePath("/projects/")
	if err == "" {
		t.Fatal("path with trailing / should be rejected")
	}
}

func TestValidatePath_DoubleSlash(t *testing.T) {
	err := validator.ValidatePath("/projects//alpha")
	if err == "" {
		t.Fatal("path with // should be rejected")
	}
}

func TestValidatePath_TooDeep(t *testing.T) {
	err := validator.ValidatePath("/a/b/c/d/e/f")
	if err == "" {
		t.Fatal("path with 6 levels should be rejected")
	}
}

func TestValidatePath_InvalidCharacter(t *testing.T) {
	cases := []string{"/projects/alpha!", "/projects/alpha beta", "/projects/alpha#1"}
	for _, p := range cases {
		err := validator.ValidatePath(p)
		if err == "" {
			t.Errorf("ValidatePath(%q) should be rejected", p)
		}
	}
}

func TestValidatePath_SegmentTooLong(t *testing.T) {
	long := strings.Repeat("a", 101)
	err := validator.ValidatePath("/" + long)
	if err == "" {
		t.Fatal("segment >100 chars should be rejected")
	}
}

func TestValidateCreate_AllValid(t *testing.T) {
	opts := model.CreateOptions{
		Path:      "/projects/alpha",
		Title:     "Alpha",
		Summary:   "A project",
		Tags:      []string{"tag1"},
		Reason:    "test reason",
	}
	if err := validator.ValidateCreate(opts); err != "" {
		t.Fatalf("valid create should pass: %s", err)
	}
}

func TestValidateCreate_EmptyTitle(t *testing.T) {
	opts := model.CreateOptions{Path: "/a", Title: "", Summary: "s", Reason: "r"}
	err := validator.ValidateCreate(opts)
	if err == "" {
		t.Fatal("empty title should be rejected")
	}
}

func TestValidateCreate_EmptySummary(t *testing.T) {
	opts := model.CreateOptions{Path: "/a", Title: "t", Summary: "", Reason: "r"}
	err := validator.ValidateCreate(opts)
	if err == "" {
		t.Fatal("empty summary should be rejected")
	}
}

func TestValidateCreate_SummaryTooLong(t *testing.T) {
	opts := model.CreateOptions{Path: "/a", Title: "t", Summary: strings.Repeat("a", 501), Reason: "r"}
	err := validator.ValidateCreate(opts)
	if err == "" {
		t.Fatal("summary >500 chars should be rejected")
	}
}

func TestValidateCreate_TagsTooMany(t *testing.T) {
	tags := make([]string, 21)
	for i := range tags {
		tags[i] = "tag"
	}
	opts := model.CreateOptions{Path: "/a", Title: "t", Summary: "s", Tags: tags, Reason: "r"}
	err := validator.ValidateCreate(opts)
	if err == "" {
		t.Fatal(">20 tags should be rejected")
	}
}

func TestValidateCreate_EmptyReason(t *testing.T) {
	opts := model.CreateOptions{Path: "/a", Title: "t", Summary: "s", Reason: ""}
	err := validator.ValidateCreate(opts)
	if err == "" {
		t.Fatal("empty reason should be rejected")
	}
}

func TestValidateAppend_AllValid(t *testing.T) {
	opts := model.AppendOptions{Path: "/a", Event: "test event", Reason: "r"}
	if err := validator.ValidateAppend(opts); err != "" {
		t.Fatalf("valid append should pass: %s", err)
	}
}

func TestValidateAppend_EmptyEvent(t *testing.T) {
	opts := model.AppendOptions{Path: "/a", Event: "", Reason: "r"}
	if err := validator.ValidateAppend(opts); err == "" {
		t.Fatal("empty event should be rejected")
	}
}

func TestValidateAppend_EventTooLong(t *testing.T) {
	opts := model.AppendOptions{Path: "/a", Event: strings.Repeat("x", 2001), Reason: "r"}
	if err := validator.ValidateAppend(opts); err == "" {
		t.Fatal("event >2000 chars should be rejected")
	}
}

func TestValidateUpdateField_AllValid(t *testing.T) {
	opts := model.UpdateFieldOptions{Path: "/a", Field: "status", Value: "active", Reason: "r"}
	if err := validator.ValidateUpdateField(opts); err != "" {
		t.Fatalf("valid update_field should pass: %s", err)
	}
}

func TestValidateUpdateField_InvalidField(t *testing.T) {
	opts := model.UpdateFieldOptions{Path: "/a", Field: "Status!", Value: "v", Reason: "r"}
	err := validator.ValidateUpdateField(opts)
	if err == "" {
		t.Fatal("invalid field name should be rejected")
	}
}

func TestValidateUpdateField_ValueTooLong(t *testing.T) {
	opts := model.UpdateFieldOptions{Path: "/a", Field: "f", Value: strings.Repeat("x", 5001), Reason: "r"}
	if err := validator.ValidateUpdateField(opts); err == "" {
		t.Fatal("value >5000 chars should be rejected")
	}
}

func TestValidateUpdateSummary_AllValid(t *testing.T) {
	opts := model.UpdateSummaryOptions{Path: "/a", NewSummary: "new summary", Reason: "r"}
	if err := validator.ValidateUpdateSummary(opts); err != "" {
		t.Fatalf("valid update_summary should pass: %s", err)
	}
}

func TestValidateUpdateSummary_EmptyNew(t *testing.T) {
	opts := model.UpdateSummaryOptions{Path: "/a", NewSummary: "", Reason: "r"}
	if err := validator.ValidateUpdateSummary(opts); err == "" {
		t.Fatal("empty new_summary should be rejected")
	}
}

func TestValidateUpdateSummary_TooLong(t *testing.T) {
	opts := model.UpdateSummaryOptions{Path: "/a", NewSummary: strings.Repeat("x", 501), Reason: "r"}
	if err := validator.ValidateUpdateSummary(opts); err == "" {
		t.Fatal("new_summary >500 chars should be rejected")
	}
}

func TestValidateArchive_Valid(t *testing.T) {
	opts := model.ArchiveOptions{Path: "/a", Reason: "done"}
	if err := validator.ValidateArchive(opts); err != "" {
		t.Fatalf("valid archive should pass: %s", err)
	}
}

func TestValidateArchive_EmptyReason(t *testing.T) {
	opts := model.ArchiveOptions{Path: "/a", Reason: ""}
	if err := validator.ValidateArchive(opts); err == "" {
		t.Fatal("empty reason should be rejected")
	}
}

func TestValidateLink_AllValid(t *testing.T) {
	opts := model.LinkOptions{From: "/a", To: "/b", Type: model.EdgeDependsOn, Reason: "r"}
	if err := validator.ValidateLink(opts); err != "" {
		t.Fatalf("valid link should pass: %s", err)
	}
}

func TestValidateLink_SelfReference(t *testing.T) {
	opts := model.LinkOptions{From: "/a", To: "/a", Type: model.EdgeDependsOn, Reason: "r"}
	if err := validator.ValidateLink(opts); err == "" {
		t.Fatal("self-reference should be rejected")
	}
}

func TestValidateLink_InvalidType(t *testing.T) {
	opts := model.LinkOptions{From: "/a", To: "/b", Type: "invalid_type", Reason: "r"}
	if err := validator.ValidateLink(opts); err == "" {
		t.Fatal("invalid edge type should be rejected")
	}
}

func TestValidateLink_AllEdgeTypes(t *testing.T) {
	types := []model.EdgeType{model.EdgeDependsOn, model.EdgeAlternative, model.EdgeSupersedes, model.EdgeReferences}
	for _, tp := range types {
		opts := model.LinkOptions{From: "/a", To: "/b", Type: tp, Reason: "r"}
		if err := validator.ValidateLink(opts); err != "" {
			t.Errorf("edge type %q should be valid: %s", tp, err)
		}
	}
}

func TestValidateSummaryDrift_NoDisappeared(t *testing.T) {
	needs, _ := validator.ValidateSummaryDrift(nil, "")
	if needs {
		t.Fatal("no disappeared entities should not require reason")
	}
}

func TestValidateSummaryDrift_EmptyReason(t *testing.T) {
	needs, disappeared := validator.ValidateSummaryDrift([]string{"张三", "9/30"}, "")
	if !needs {
		t.Fatal("empty reason with disappeared entities should be rejected")
	}
	if len(disappeared) != 2 {
		t.Fatalf("expected 2 disappeared entities, got %d", len(disappeared))
	}
}

func TestValidateSummaryDrift_ShortReason(t *testing.T) {
	needs, _ := validator.ValidateSummaryDrift([]string{"张三"}, "ok")
	if !needs {
		t.Fatal("short reason should be rejected")
	}
}

func TestValidateSummaryDrift_LazyPattern(t *testing.T) {
	lazy := []string{"不需要", "没用了", "none", "n/a", "na", "ok", "yes", "no", "无所谓"}
	for _, l := range lazy {
		needs, _ := validator.ValidateSummaryDrift([]string{"张三"}, l)
		if !needs {
			t.Errorf("lazy pattern %q should be rejected", l)
		}
	}
}

func TestValidateSummaryDrift_Substantive(t *testing.T) {
	needs, _ := validator.ValidateSummaryDrift([]string{"张三", "9/30"}, "张三离职，李四接替，deadline 顺延")
	if needs {
		t.Fatal("substantive reason should pass")
	}
}

// --- Path-error branches inside per-operation validators ---

func TestValidateCreate_InvalidPath(t *testing.T) {
	opts := model.CreateOptions{Path: "invalid", Title: "t", Summary: "s", Reason: "r"}
	err := validator.ValidateCreate(opts)
	if err == "" {
		t.Fatal("invalid path in Create should be rejected")
	}
}

func TestValidateAppend_InvalidPath(t *testing.T) {
	opts := model.AppendOptions{Path: "", Event: "e", Reason: "r"}
	err := validator.ValidateAppend(opts)
	if err == "" {
		t.Fatal("invalid path in Append should be rejected")
	}
}

func TestValidateAppend_EmptyReason(t *testing.T) {
	opts := model.AppendOptions{Path: "/a", Event: "e", Reason: ""}
	err := validator.ValidateAppend(opts)
	if err == "" {
		t.Fatal("empty reason in Append should be rejected")
	}
}

func TestValidateUpdateField_InvalidPath(t *testing.T) {
	opts := model.UpdateFieldOptions{Path: "", Field: "f", Value: "v", Reason: "r"}
	err := validator.ValidateUpdateField(opts)
	if err == "" {
		t.Fatal("invalid path in UpdateField should be rejected")
	}
}

func TestValidateUpdateField_EmptyField(t *testing.T) {
	opts := model.UpdateFieldOptions{Path: "/a", Field: "", Value: "v", Reason: "r"}
	err := validator.ValidateUpdateField(opts)
	if err == "" {
		t.Fatal("empty field should be rejected")
	}
}

func TestValidateUpdateField_EmptyReason(t *testing.T) {
	opts := model.UpdateFieldOptions{Path: "/a", Field: "f", Value: "v", Reason: ""}
	if err := validator.ValidateUpdateField(opts); err == "" {
		t.Fatal("empty reason in UpdateField should be rejected")
	}
}

func TestValidateUpdateSummary_InvalidPath(t *testing.T) {
	opts := model.UpdateSummaryOptions{Path: "", NewSummary: "s", Reason: "r"}
	err := validator.ValidateUpdateSummary(opts)
	if err == "" {
		t.Fatal("invalid path in UpdateSummary should be rejected")
	}
}

func TestValidateUpdateSummary_EmptyReason(t *testing.T) {
	opts := model.UpdateSummaryOptions{Path: "/a", NewSummary: "s", Reason: ""}
	if err := validator.ValidateUpdateSummary(opts); err == "" {
		t.Fatal("empty reason in UpdateSummary should be rejected")
	}
}

func TestValidateArchive_InvalidPath(t *testing.T) {
	opts := model.ArchiveOptions{Path: "", Reason: "r"}
	err := validator.ValidateArchive(opts)
	if err == "" {
		t.Fatal("invalid path in Archive should be rejected")
	}
}

func TestValidateLink_InvalidFrom(t *testing.T) {
	opts := model.LinkOptions{From: "", To: "/b", Type: model.EdgeDependsOn, Reason: "r"}
	err := validator.ValidateLink(opts)
	if err == "" {
		t.Fatal("invalid from path should be rejected")
	}
}

func TestValidateLink_InvalidTo(t *testing.T) {
	opts := model.LinkOptions{From: "/a", To: "", Type: model.EdgeDependsOn, Reason: "r"}
	err := validator.ValidateLink(opts)
	if err == "" {
		t.Fatal("invalid to path should be rejected")
	}
}

func TestValidateLink_EmptyReason(t *testing.T) {
	opts := model.LinkOptions{From: "/a", To: "/b", Type: model.EdgeDependsOn, Reason: ""}
	if err := validator.ValidateLink(opts); err == "" {
		t.Fatal("empty reason in Link should be rejected")
	}
}
