package multisync

import (
	"reflect"
	"strings"
	"testing"
)

func TestBuildPlanSortsAndExcludesExactOriginalNames(t *testing.T) {
	plan, err := BuildPlan("dbo", "public", []string{"XMLDocument", "Document", "document"}, []string{"XMLDocument"})
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(plan.Tables))
	for index, table := range plan.Tables {
		got[index] = table.SourceName
	}
	if want := []string{"Document", "XMLDocument", "document"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("source order = %v, want %v", got, want)
	}
	if !plan.Tables[1].Excluded || plan.Tables[0].Excluded || plan.Tables[2].Excluded {
		t.Fatalf("exclusions = %#v", plan.Tables)
	}
	if len(plan.Collisions) != 1 || plan.Collisions[0].TargetName != "document" {
		t.Fatalf("collisions = %#v", plan.Collisions)
	}
}

func TestBuildPlanRejectsUnknownExactExclusion(t *testing.T) {
	_, err := BuildPlan("dbo", "public", []string{"Document"}, []string{"document"})
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("BuildPlan error = %v", err)
	}
}

func TestExcludedSourceDoesNotCauseCollision(t *testing.T) {
	plan, err := BuildPlan("dbo", "public", []string{"Document", "document"}, []string{"document"})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Collisions) != 0 || plan.Tables[0].Collision {
		t.Fatalf("plan = %#v", plan)
	}
}
