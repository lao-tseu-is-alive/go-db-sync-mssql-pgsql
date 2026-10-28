package mssql

import (
	"database/sql"
	"fmt"

	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/schema"
)

// RowSource adapts database/sql rows to pgx.CopyFromSource without buffering the table.
type RowSource struct {
	rows    *sql.Rows
	columns []schema.Column
	values  []any
	err     error
}

// Next advances to the next SQL Server row and converts its typed values.
func (source *RowSource) Next() bool {
	if source.err != nil || !source.rows.Next() {
		return false
	}
	raw := make([]any, len(source.columns))
	destinations := make([]any, len(raw))
	for index := range raw {
		destinations[index] = &raw[index]
	}
	if err := source.rows.Scan(destinations...); err != nil {
		source.err = fmt.Errorf("scan SQL Server row: %w", err)
		return false
	}
	source.values = make([]any, len(raw))
	for index, value := range raw {
		converted, err := schema.ConvertValue(source.columns[index], value)
		if err != nil {
			source.err = fmt.Errorf("convert column %q: %w", source.columns[index].SourceName, err)
			return false
		}
		source.values[index] = converted
	}
	return true
}

// Values returns the current typed row for pgx COPY.
func (source *RowSource) Values() ([]any, error) {
	return source.values, source.err
}

// Err returns conversion, scan or SQL Server iteration errors.
func (source *RowSource) Err() error {
	if source.err != nil {
		return source.err
	}
	return source.rows.Err()
}

// Close releases the SQL Server result set.
func (source *RowSource) Close() error {
	return source.rows.Close()
}
