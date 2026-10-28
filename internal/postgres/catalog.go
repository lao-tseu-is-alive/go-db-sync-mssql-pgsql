package postgres

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/schema"
)

type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type targetColumn struct {
	name      string
	typeName  string
	notNull   bool
	identity  string
	generated string
}

type targetTable struct {
	exists     bool
	columns    []targetColumn
	primaryKey []string
}

func requireSchema(ctx context.Context, query queryer, name string) error {
	var exists bool
	if err := query.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = $1)`, name).Scan(&exists); err != nil {
		return fmt.Errorf("check target schema: %w", err)
	}
	if !exists {
		return fmt.Errorf("target schema %q does not exist; Phase 1 does not create schemas", name)
	}
	return nil
}

const targetColumnsQuery = `
SELECT a.attname, pg_catalog.format_type(a.atttypid, a.atttypmod), a.attnotnull, a.attidentity, a.attgenerated
FROM pg_catalog.pg_class AS c
JOIN pg_catalog.pg_namespace AS n ON n.oid = c.relnamespace
JOIN pg_catalog.pg_attribute AS a ON a.attrelid = c.oid
WHERE n.nspname = $1 AND c.relname = $2 AND c.relkind IN ('r', 'p')
  AND a.attnum > 0 AND NOT a.attisdropped
ORDER BY a.attnum`

const targetPrimaryKeyQuery = `
SELECT a.attname
FROM pg_catalog.pg_class AS c
JOIN pg_catalog.pg_namespace AS n ON n.oid = c.relnamespace
JOIN pg_catalog.pg_index AS i ON i.indrelid = c.oid AND i.indisprimary
JOIN LATERAL unnest(i.indkey) WITH ORDINALITY AS k(attnum, key_order) ON true
JOIN pg_catalog.pg_attribute AS a ON a.attrelid = c.oid AND a.attnum = k.attnum
WHERE n.nspname = $1 AND c.relname = $2
ORDER BY k.key_order`

func inspectTarget(ctx context.Context, query queryer, targetSchema, targetName string) (targetTable, error) {
	rows, err := query.Query(ctx, targetColumnsQuery, targetSchema, targetName)
	if err != nil {
		return targetTable{}, fmt.Errorf("inspect target columns: %w", err)
	}
	defer rows.Close()
	var target targetTable
	for rows.Next() {
		var column targetColumn
		if err := rows.Scan(&column.name, &column.typeName, &column.notNull, &column.identity, &column.generated); err != nil {
			return targetTable{}, fmt.Errorf("scan target column: %w", err)
		}
		target.columns = append(target.columns, column)
	}
	if err := rows.Err(); err != nil {
		return targetTable{}, fmt.Errorf("iterate target columns: %w", err)
	}
	target.exists = len(target.columns) > 0
	if !target.exists {
		return target, nil
	}
	keyRows, err := query.Query(ctx, targetPrimaryKeyQuery, targetSchema, targetName)
	if err != nil {
		return targetTable{}, fmt.Errorf("inspect target primary key: %w", err)
	}
	defer keyRows.Close()
	for keyRows.Next() {
		var name string
		if err := keyRows.Scan(&name); err != nil {
			return targetTable{}, fmt.Errorf("scan target primary key: %w", err)
		}
		target.primaryKey = append(target.primaryKey, name)
	}
	if err := keyRows.Err(); err != nil {
		return targetTable{}, fmt.Errorf("iterate target primary key: %w", err)
	}
	return target, nil
}

func validateTarget(expected schema.Table, actual targetTable) error {
	if len(actual.columns) != len(expected.Columns) {
		return fmt.Errorf("target schema incompatible: has %d columns, expected %d", len(actual.columns), len(expected.Columns))
	}
	for index, column := range expected.Columns {
		found := actual.columns[index]
		if found.name != column.TargetName {
			return fmt.Errorf("target schema incompatible at column %d: found %q, expected %q", index+1, found.name, column.TargetName)
		}
		if schema.CanonicalPostgreSQLType(found.typeName) != schema.CanonicalPostgreSQLType(column.PostgreSQLType) {
			return fmt.Errorf("target column %q has type %q, expected %q", found.name, found.typeName, column.PostgreSQLType)
		}
		if found.notNull != !column.Nullable {
			return fmt.Errorf("target column %q nullability is incompatible", found.name)
		}
		if found.identity != "" || found.generated != "" {
			return fmt.Errorf("target column %q is identity/generated; Phase 1 refuses implicit value changes", found.name)
		}
	}
	expectedKey := expected.PrimaryKeyNames()
	if !slices.Equal(actual.primaryKey, expectedKey) {
		return fmt.Errorf("target primary key %v does not match source key %v", actual.primaryKey, expectedKey)
	}
	return nil
}
