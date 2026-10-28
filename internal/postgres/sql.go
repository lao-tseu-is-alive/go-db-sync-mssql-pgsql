package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/ident"
	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/schema"
)

func quote(value string) string {
	return ident.PostgreSQL(value)
}

func qualifiedName(schemaName, tableName string) string {
	return ident.PostgreSQLQualified(schemaName, tableName)
}

func createTableSQL(table schema.Table) string {
	definitions := make([]string, 0, len(table.Columns)+1)
	for _, column := range table.Columns {
		definition := quote(column.TargetName) + " " + column.PostgreSQLType
		if !column.Nullable {
			definition += " NOT NULL"
		}
		definitions = append(definitions, definition)
	}
	if keys := table.PrimaryKeyNames(); len(keys) > 0 {
		definitions = append(definitions, "PRIMARY KEY ("+quotedList(keys)+")")
	}
	return "CREATE TABLE " + qualifiedName(table.TargetSchema, table.TargetName) + " (" + strings.Join(definitions, ", ") + ")"
}

func createStageSQL(table schema.Table, stageName string) string {
	return "CREATE TEMP TABLE " + quote(stageName) + " (LIKE " + qualifiedName(table.TargetSchema, table.TargetName) + ") ON COMMIT DROP"
}

func publicationSQL(table schema.Table, stageName string) []string {
	target := qualifiedName(table.TargetSchema, table.TargetName)
	columns := quotedList(table.TargetColumnNames())
	stage := quote(stageName)
	keys := table.PrimaryKeyNames()
	if len(keys) == 0 {
		return []string{
			"DELETE FROM " + target,
			"INSERT INTO " + target + " (" + columns + ") SELECT " + columns + " FROM " + stage,
		}
	}

	keySet := make(map[string]bool, len(keys))
	for _, key := range keys {
		keySet[key] = true
	}
	updates := make([]string, 0, len(table.Columns)-len(keys))
	for _, column := range table.Columns {
		if !keySet[column.TargetName] {
			updates = append(updates, quote(column.TargetName)+" = EXCLUDED."+quote(column.TargetName))
		}
	}
	conflictAction := "DO NOTHING"
	if len(updates) > 0 {
		conflictAction = "DO UPDATE SET " + strings.Join(updates, ", ")
	}
	comparisons := make([]string, len(keys))
	for index, key := range keys {
		comparisons[index] = "live." + quote(key) + " = staged." + quote(key)
	}
	upsert := "INSERT INTO " + target + " AS live (" + columns + ") SELECT " + columns + " FROM " + stage +
		" ON CONFLICT (" + quotedList(keys) + ") " + conflictAction
	deleteMissing := "DELETE FROM " + target + " AS live WHERE NOT EXISTS (SELECT 1 FROM " + stage + " AS staged WHERE " + strings.Join(comparisons, " AND ") + ")"
	return []string{upsert, deleteMissing}
}

func quotedList(values []string) string {
	quoted := make([]string, len(values))
	for index, value := range values {
		quoted[index] = quote(value)
	}
	return strings.Join(quoted, ", ")
}

func verifyCount(ctx context.Context, query queryer, relation string, expected int64, label string) error {
	var actual int64
	if err := query.QueryRow(ctx, "SELECT COUNT(*) FROM "+relation).Scan(&actual); err != nil {
		return fmt.Errorf("count %s rows: %w", label, err)
	}
	if actual != expected {
		return fmt.Errorf("%s row count is %d, expected %d", label, actual, expected)
	}
	return nil
}
