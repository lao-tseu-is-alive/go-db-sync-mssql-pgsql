package mssql

import (
	"testing"

	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/schema"
)

func TestSelectColumnProjectsUUIDAsCanonicalText(t *testing.T) {
	t.Parallel()
	column := schema.Column{SourceName: "ExternalID", SourceType: "uniqueidentifier"}
	actual := selectColumn(column)
	expected := "CONVERT(char(36), [ExternalID]) AS [ExternalID]"
	if actual != expected {
		t.Fatalf("selectColumn = %q, want %q", actual, expected)
	}
}

func TestSelectColumnLeavesTypedValuesUntouched(t *testing.T) {
	t.Parallel()
	column := schema.Column{SourceName: "Raw]Value", SourceType: "varbinary"}
	if actual := selectColumn(column); actual != "[Raw]]Value]" {
		t.Fatalf("selectColumn = %q", actual)
	}
}
