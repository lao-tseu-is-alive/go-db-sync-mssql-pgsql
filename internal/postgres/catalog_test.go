package postgres

import (
	"strings"
	"testing"

	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/schema"
)

func testTable() schema.Table {
	return schema.Table{
		SourceSchema: "dbo", SourceName: "Thing", TargetSchema: "public", TargetName: "thing",
		Columns: []schema.Column{
			{SourceName: "ID", TargetName: "id", PostgreSQLType: "integer", PKOrdinal: 1},
			{SourceName: "Name", TargetName: "name", PostgreSQLType: "text", Nullable: true},
		},
	}
}

func TestValidateTarget(t *testing.T) {
	t.Parallel()
	table := testTable()
	target := targetTable{exists: true, columns: []targetColumn{
		{name: "id", typeName: "integer", notNull: true},
		{name: "name", typeName: "text", notNull: false},
	}, primaryKey: []string{"id"}}
	if err := validateTarget(table, target); err != nil {
		t.Fatal(err)
	}
	target.columns[1].typeName = "bigint"
	if err := validateTarget(table, target); err == nil || !strings.Contains(err.Error(), "expected \"text\"") {
		t.Fatalf("incompatible type error = %v", err)
	}
}

func TestCreateTablePreservesColumnsAndKeyOrder(t *testing.T) {
	t.Parallel()
	table := testTable()
	actual := createTableSQL(table)
	expected := `CREATE TABLE "public"."thing" ("id" integer NOT NULL, "name" text, PRIMARY KEY ("id"))`
	if actual != expected {
		t.Fatalf("create SQL = %q, want %q", actual, expected)
	}
}

func TestPublicationUsesAtomicUpsertAndDelete(t *testing.T) {
	t.Parallel()
	actual := strings.Join(publicationSQL(testTable(), "_dbsync_stage"), "; ")
	for _, required := range []string{"ON CONFLICT", "DO UPDATE", "DELETE FROM", "NOT EXISTS"} {
		if !strings.Contains(actual, required) {
			t.Fatalf("publication SQL %q lacks %q", actual, required)
		}
	}
}

func TestPublicationWithoutKeyReplacesAllRows(t *testing.T) {
	t.Parallel()
	table := testTable()
	table.Columns[0].PKOrdinal = 0
	actual := strings.Join(publicationSQL(table, "_dbsync_stage"), "; ")
	if !strings.HasPrefix(actual, `DELETE FROM "public"."thing"; INSERT INTO`) {
		t.Fatalf("publication SQL = %q", actual)
	}
}
