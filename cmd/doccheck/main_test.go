package main

import (
	"strings"
	"testing"
)

func TestAtlasCheckerRejectsMissingDuplicateAndStalePaths(t *testing.T) {
	t.Parallel()
	atlas := "Tracked version: **v0.1.0**.\n\n- `a` — A.\n- `a` — Duplicate.\n- `stale` — Stale.\n"
	failures := strings.Join(checkAtlas([]string{"a", "missing"}, atlas, "0.1.0"), "\n")
	for _, expected := range []string{"missing missing", "duplicates a", "stale path stale"} {
		if !strings.Contains(failures, expected) {
			t.Fatalf("failures %q do not contain %q", failures, expected)
		}
	}
}

func TestGoCheckerRejectsMissingExportedComment(t *testing.T) {
	t.Parallel()
	source := []byte("// Package sample demonstrates checks.\npackage sample\n\ntype Exported struct{}\n")
	failures, _, documented := checkGoSource("sample.go", source)
	if !documented || len(failures) != 1 || !strings.Contains(failures[0], "Exported") {
		t.Fatalf("documented=%t failures=%v", documented, failures)
	}
}

func TestGoCheckerAcceptsDocumentedAPI(t *testing.T) {
	t.Parallel()
	source := []byte("// Package sample demonstrates checks.\npackage sample\n\n// Exported is documented.\ntype Exported struct {\n// Value is documented.\nValue string\n}\n")
	failures, _, documented := checkGoSource("sample.go", source)
	if !documented || len(failures) != 0 {
		t.Fatalf("documented=%t failures=%v", documented, failures)
	}
}
