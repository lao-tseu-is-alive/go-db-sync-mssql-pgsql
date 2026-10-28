//go:build integration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/schema"
)

type sliceSource struct {
	rows    [][]any
	index   int
	errorAt int
	err     error
}

func (source *sliceSource) Next() bool {
	if source.errorAt >= 0 && source.index == source.errorAt {
		source.err = errors.New("injected stream failure")
		return false
	}
	return source.index < len(source.rows)
}

func (source *sliceSource) Values() ([]any, error) {
	row := source.rows[source.index]
	source.index++
	return row, nil
}

func (source *sliceSource) Err() error   { return source.err }
func (source *sliceSource) Close() error { return nil }

func integrationClient(t *testing.T) (context.Context, *Client, string) {
	t.Helper()
	dsn := os.Getenv("DBSYNC_TEST_PGSQL_DSN")
	if dsn == "" {
		if os.Getenv("DBSYNC_REQUIRE_DB_TESTS") == "true" {
			t.Fatal("DBSYNC_TEST_PGSQL_DSN is required")
		}
		t.Skip("set DBSYNC_TEST_PGSQL_DSN to a disposable PostgreSQL database")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	client := New(pool)
	schemaName := "dbsync_test_" + uuid.NewString()[:8]
	if _, err := pool.Exec(ctx, "CREATE SCHEMA "+quote(schemaName)); err != nil {
		pool.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DROP SCHEMA "+quote(schemaName)+" CASCADE")
		pool.Close()
	})
	return ctx, client, schemaName
}

func integrationTable(schemaName, tableName string) schema.Table {
	return schema.Table{
		SourceSchema: "dbo", SourceName: "IntegrationThing", TargetSchema: schemaName, TargetName: tableName,
		Columns: []schema.Column{
			{SourceName: "ID", TargetName: "id", PostgreSQLType: "integer", PKOrdinal: 1},
			{SourceName: "TextValue", TargetName: "textvalue", PostgreSQLType: "text", Nullable: true},
			{SourceName: "RawValue", TargetName: "rawvalue", PostgreSQLType: "bytea", Nullable: true},
			{SourceName: "Enabled", TargetName: "enabled", PostgreSQLType: "boolean"},
			{SourceName: "Amount", TargetName: "amount", PostgreSQLType: "numeric(20,4)"},
			{SourceName: "Identifier", TargetName: "identifier", PostgreSQLType: "uuid"},
			{SourceName: "Happened", TargetName: "happened", PostgreSQLType: "timestamp without time zone"},
			{SourceName: "Version", TargetName: "version", PostgreSQLType: "bigint"},
		},
	}
}

func numeric(t *testing.T, value string) pgtype.Numeric {
	t.Helper()
	var result pgtype.Numeric
	if err := result.Scan(value); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestPublishCreatesMissingTargetAndPreservesExactValues(t *testing.T) {
	ctx, client, schemaName := integrationClient(t)
	table := integrationTable(schemaName, "exact_values")
	id := uuid.MustParse("6f9619ff-8b86-d011-b42d-00c04fc964ff")
	happened := time.Date(2026, 10, 28, 12, 34, 56, 123000000, time.UTC)
	rows := [][]any{
		{int64(1), nil, []byte{0, 1, 255}, true, numeric(t, "123456789012345.6789"), id, happened, int64(2003)},
		{int64(2), "literal \\N\nunicode é漢字\nback\\slash\x1f", nil, false, numeric(t, "-1.2300"), uuid.New(), happened, int64(2004)},
	}
	result, err := client.Publish(ctx, table, &sliceSource{rows: rows, errorAt: -1}, int64(len(rows)))
	if err != nil {
		t.Fatal(err)
	}
	if !result.CreatedTarget || result.CopiedRows != 2 || result.PublishedRows != 2 {
		t.Fatalf("result = %#v", result)
	}
	var nullText *string
	var raw []byte
	var amount string
	var foundID uuid.UUID
	if err := client.pool.QueryRow(ctx,
		"SELECT textvalue, rawvalue, amount::text, identifier FROM "+qualifiedName(schemaName, table.TargetName)+" WHERE id = 1").
		Scan(&nullText, &raw, &amount, &foundID); err != nil {
		t.Fatal(err)
	}
	if nullText != nil || fmt.Sprint(raw) != "[0 1 255]" || amount != "123456789012345.6789" || foundID != id {
		t.Fatalf("exact values changed: text=%v raw=%v amount=%q id=%v", nullText, raw, amount, foundID)
	}
	var literal string
	if err := client.pool.QueryRow(ctx, "SELECT textvalue FROM "+qualifiedName(schemaName, table.TargetName)+" WHERE id = 2").Scan(&literal); err != nil {
		t.Fatal(err)
	}
	if literal != rows[1][1] {
		t.Fatalf("text changed: %q != %q", literal, rows[1][1])
	}
}

func TestPublishEmptySourceEmptiesTargetAfterVerifiedStage(t *testing.T) {
	ctx, client, schemaName := integrationClient(t)
	table := integrationTable(schemaName, "empty_source")
	row := []any{int64(1), "old", nil, true, numeric(t, "1.0000"), uuid.New(), time.Now(), int64(1)}
	if _, err := client.Publish(ctx, table, &sliceSource{rows: [][]any{row}, errorAt: -1}, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Publish(ctx, table, &sliceSource{errorAt: -1}, 0); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := client.pool.QueryRow(ctx, "SELECT COUNT(*) FROM "+qualifiedName(schemaName, table.TargetName)).Scan(&count); err != nil || count != 0 {
		t.Fatalf("count = %d, err = %v", count, err)
	}
}

func TestCopyFailureRollsBackAndPreservesTarget(t *testing.T) {
	ctx, client, schemaName := integrationClient(t)
	table := integrationTable(schemaName, "copy_failure")
	oldRow := []any{int64(1), "old", nil, true, numeric(t, "1.0000"), uuid.New(), time.Now(), int64(1)}
	if _, err := client.Publish(ctx, table, &sliceSource{rows: [][]any{oldRow}, errorAt: -1}, 1); err != nil {
		t.Fatal(err)
	}
	newRow := []any{int64(1), "new", nil, true, numeric(t, "2.0000"), uuid.New(), time.Now(), int64(2)}
	_, err := client.Publish(ctx, table, &sliceSource{rows: [][]any{newRow, newRow}, errorAt: 1}, 2)
	if err == nil {
		t.Fatal("injected COPY failure unexpectedly succeeded")
	}
	var text string
	if err := client.pool.QueryRow(ctx, "SELECT textvalue FROM "+qualifiedName(schemaName, table.TargetName)+" WHERE id = 1").Scan(&text); err != nil {
		t.Fatal(err)
	}
	if text != "old" {
		t.Fatalf("live target changed after failed COPY: %q", text)
	}
}

func TestInspectRejectsIncompatibleTarget(t *testing.T) {
	ctx, client, schemaName := integrationClient(t)
	table := integrationTable(schemaName, "incompatible")
	if _, err := client.pool.Exec(ctx, "CREATE TABLE "+qualifiedName(schemaName, table.TargetName)+" (id bigint PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Inspect(ctx, table); err == nil {
		t.Fatal("Inspect accepted incompatible target schema")
	}
}
