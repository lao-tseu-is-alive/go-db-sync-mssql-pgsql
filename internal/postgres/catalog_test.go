package postgres

import (
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/schema"
	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/stream"
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

func TestCopyPipelineRemainderSubtractsMeasuredSourceWork(t *testing.T) {
	t.Parallel()
	performance := Performance{
		CopyFromDuration: 10 * time.Second,
		Source: stream.Metrics{
			NextDuration: 4 * time.Second, ScanDuration: time.Second, ConversionDuration: 2 * time.Second,
		},
	}
	if actual := performance.CopyPipelineRemainder(); actual != 3*time.Second {
		t.Fatalf("CopyPipelineRemainder = %s", actual)
	}
}

func TestPublicationStatementClassification(t *testing.T) {
	t.Parallel()
	withKey := testTable()
	if publicationStatementIsDelete(withKey, 0) || !publicationStatementIsDelete(withKey, 1) {
		t.Fatal("primary-key publication statement classification is wrong")
	}
	withoutKey := testTable()
	withoutKey.Columns[0].PKOrdinal = 0
	if !publicationStatementIsDelete(withoutKey, 0) || publicationStatementIsDelete(withoutKey, 1) {
		t.Fatal("keyless publication statement classification is wrong")
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

func TestTargetCatalogMarkersAcceptBinaryQChar(t *testing.T) {
	t.Parallel()
	var column targetColumn
	codec := pgtype.QCharCodec{}
	for name, destination := range map[string]*byte{
		"identity":  &column.identity,
		"generated": &column.generated,
	} {
		plan := codec.PlanScan(nil, pgtype.QCharOID, pgtype.BinaryFormatCode, destination)
		if plan == nil {
			t.Fatalf("%s marker does not accept PostgreSQL binary qchar", name)
		}
		if err := plan.Scan([]byte{'a'}, destination); err != nil {
			t.Fatalf("scan %s marker: %v", name, err)
		}
		if *destination != 'a' {
			t.Fatalf("%s marker = %q, want %q", name, *destination, byte('a'))
		}
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

func TestTruncateUsesQualifiedIdentifierAndRestrict(t *testing.T) {
	t.Parallel()
	actual := truncateTableSQL(testTable())
	if actual != `TRUNCATE TABLE "public"."thing" RESTRICT` {
		t.Fatalf("truncate SQL = %q", actual)
	}
	if strings.Contains(actual, "CASCADE") {
		t.Fatalf("truncate SQL must never cascade: %q", actual)
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
