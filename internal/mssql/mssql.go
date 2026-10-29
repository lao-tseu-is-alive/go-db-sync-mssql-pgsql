// Package mssql discovers and streams one read-only SQL Server table.
package mssql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/ident"
	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/schema"
	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/stream"
	_ "github.com/microsoft/go-mssqldb"
)

// ErrTableNotFound reports that the explicitly selected source table is absent.
var ErrTableNotFound = errors.New("SQL Server source table not found")

// Client is a read-only SQL Server metadata and row client.
type Client struct {
	db                 *sql.DB
	performanceMetrics bool
}

// Open connects to SQL Server using the supplied DSN and verifies connectivity.
func Open(ctx context.Context, dsn string) (*Client, error) {
	db, err := sql.Open("sqlserver", dsn)
	if err != nil {
		return nil, fmt.Errorf("open SQL Server driver: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect to SQL Server: %w", err)
	}
	db.SetMaxOpenConns(2)
	return &Client{db: db}, nil
}

// New returns a client around an existing database handle, primarily for tests.
func New(db *sql.DB) *Client {
	return &Client{db: db}
}

// Close releases the SQL Server connection pool.
func (client *Client) Close() error {
	return client.db.Close()
}

// EnablePerformanceMetrics enables opt-in per-row read, scan and conversion timings.
func (client *Client) EnablePerformanceMetrics(enabled bool) {
	client.performanceMetrics = enabled
}

const metadataQuery = `
SELECT
    c.column_id,
    c.name,
    st.name,
    c.max_length,
    c.precision,
    c.scale,
    c.is_nullable,
    COALESCE(pk.key_ordinal, 0)
FROM sys.schemas AS s
JOIN sys.tables AS tb ON tb.schema_id = s.schema_id
JOIN sys.columns AS c ON c.object_id = tb.object_id
JOIN sys.types AS st ON st.user_type_id = c.user_type_id
LEFT JOIN (
    SELECT ic.object_id, ic.column_id, ic.key_ordinal
    FROM sys.indexes AS i
    JOIN sys.index_columns AS ic ON ic.object_id = i.object_id AND ic.index_id = i.index_id
    WHERE i.is_primary_key = 1
) AS pk ON pk.object_id = c.object_id AND pk.column_id = c.column_id
WHERE s.name = @p1 AND tb.name = @p2
ORDER BY c.column_id`

// Describe discovers ordered source columns and maps them to one target table contract.
func (client *Client) Describe(ctx context.Context, sourceSchema, sourceTable, targetSchema, targetTable string) (schema.Table, error) {
	if err := ident.ValidateSource(sourceSchema); err != nil {
		return schema.Table{}, fmt.Errorf("source schema: %w", err)
	}
	if err := ident.ValidateSource(sourceTable); err != nil {
		return schema.Table{}, fmt.Errorf("source table: %w", err)
	}
	rows, err := client.db.QueryContext(ctx, metadataQuery, sourceSchema, sourceTable)
	if err != nil {
		return schema.Table{}, fmt.Errorf("query SQL Server metadata: %w", err)
	}
	defer rows.Close()

	table := schema.Table{
		SourceSchema: sourceSchema,
		SourceName:   sourceTable,
		TargetSchema: targetSchema,
		TargetName:   targetTable,
	}
	for rows.Next() {
		var columnID int
		var source schema.SourceColumn
		if err := rows.Scan(&columnID, &source.Name, &source.TypeName, &source.MaxLength, &source.Precision, &source.Scale, &source.Nullable, &source.PKOrdinal); err != nil {
			return schema.Table{}, fmt.Errorf("scan SQL Server metadata: %w", err)
		}
		column, err := schema.MapColumn(source)
		if err != nil {
			return schema.Table{}, err
		}
		table.Columns = append(table.Columns, column)
	}
	if err := rows.Err(); err != nil {
		return schema.Table{}, fmt.Errorf("iterate SQL Server metadata: %w", err)
	}
	if len(table.Columns) == 0 {
		return schema.Table{}, fmt.Errorf("%w: %s.%s", ErrTableNotFound, sourceSchema, sourceTable)
	}
	if err := table.Validate(); err != nil {
		return schema.Table{}, fmt.Errorf("validate mapped table: %w", err)
	}
	return table, nil
}

// Count returns the source row count before streaming starts.
func (client *Client) Count(ctx context.Context, table schema.Table) (int64, error) {
	query := "SELECT COUNT_BIG(*) FROM " + ident.MSSQL(table.SourceSchema) + "." + ident.MSSQL(table.SourceName)
	var count int64
	if err := client.db.QueryRowContext(ctx, query).Scan(&count); err != nil {
		return 0, fmt.Errorf("count SQL Server source rows: %w", err)
	}
	return count, nil
}

// Rows starts a forward-only query over the selected source columns.
func (client *Client) Rows(ctx context.Context, table schema.Table) (stream.Source, error) {
	columns := make([]string, len(table.Columns))
	for index, column := range table.Columns {
		columns[index] = selectColumn(column)
	}
	query := "SELECT " + joinComma(columns) + " FROM " + ident.MSSQL(table.SourceSchema) + "." + ident.MSSQL(table.SourceName)
	rows, err := client.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query SQL Server source rows: %w", err)
	}
	return &RowSource{rows: rows, columns: table.Columns, measure: client.performanceMetrics}, nil
}

func selectColumn(column schema.Column) string {
	quoted := ident.MSSQL(column.SourceName)
	if column.SourceType == "uniqueidentifier" {
		return "CONVERT(char(36), " + quoted + ") AS " + quoted
	}
	return quoted
}

func joinComma(values []string) string {
	if len(values) == 0 {
		return ""
	}
	result := values[0]
	for _, value := range values[1:] {
		result += ", " + value
	}
	return result
}
