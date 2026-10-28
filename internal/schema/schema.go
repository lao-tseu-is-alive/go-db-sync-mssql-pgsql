// Package schema maps SQL Server table metadata and typed values to PostgreSQL contracts.
package schema

import (
	"fmt"
	"sort"

	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/ident"
)

// Conversion identifies value handling needed before pgx COPY encoding.
type Conversion uint8

const (
	// ConversionDirect passes the typed database/sql value to pgx unchanged.
	ConversionDirect Conversion = iota
	// ConversionMoney parses the driver's exact decimal bytes as pgtype.Numeric.
	ConversionMoney
	// ConversionUUID normalizes the driver's 16-byte UUID representation.
	ConversionUUID
	// ConversionRowVersion converts an eight-byte big-endian rowversion to signed bigint.
	ConversionRowVersion
)

// SourceColumn is the SQL Server metadata required by Phase 1.
type SourceColumn struct {
	// Name is the source column identifier.
	Name string
	// TypeName is the lower-case SQL Server system type name.
	TypeName string
	// MaxLength is sys.columns.max_length in bytes.
	MaxLength int
	// Precision is the SQL Server numeric precision.
	Precision int
	// Scale is the SQL Server numeric scale.
	Scale int
	// Nullable reports whether SQL NULL is allowed.
	Nullable bool
	// PKOrdinal is the one-based primary-key position, or zero when not in the key.
	PKOrdinal int
}

// Column is one validated source-to-target column mapping.
type Column struct {
	// SourceName is the quoted-only SQL Server column identifier.
	SourceName string
	// TargetName is the legacy-compatible lower-case PostgreSQL identifier.
	TargetName string
	// SourceType is the lower-case SQL Server system type name.
	SourceType string
	// PostgreSQLType is the canonical DDL type emitted for a new target.
	PostgreSQLType string
	// Nullable reports whether SQL NULL is allowed.
	Nullable bool
	// PKOrdinal is the one-based primary-key position, or zero when not in the key.
	PKOrdinal int
	// Conversion selects the lossless value conversion for this column.
	Conversion Conversion
}

// Table is the complete Phase 1 contract for one explicitly selected table.
type Table struct {
	// SourceSchema is the SQL Server schema identifier.
	SourceSchema string
	// SourceName is the SQL Server table identifier.
	SourceName string
	// TargetSchema is the PostgreSQL schema identifier.
	TargetSchema string
	// TargetName is the PostgreSQL table identifier.
	TargetName string
	// Columns preserves SQL Server column order.
	Columns []Column
}

// PrimaryKeyNames returns target column names in SQL Server key order.
func (table Table) PrimaryKeyNames() []string {
	keys := make([]Column, 0)
	for _, column := range table.Columns {
		if column.PKOrdinal > 0 {
			keys = append(keys, column)
		}
	}
	sort.Slice(keys, func(left, right int) bool {
		return keys[left].PKOrdinal < keys[right].PKOrdinal
	})
	names := make([]string, len(keys))
	for index, key := range keys {
		names[index] = key.TargetName
	}
	return names
}

// TargetColumnNames returns target columns in source column order.
func (table Table) TargetColumnNames() []string {
	names := make([]string, len(table.Columns))
	for index, column := range table.Columns {
		names[index] = column.TargetName
	}
	return names
}

// Validate rejects unsafe identifiers and ambiguous column or key mappings.
func (table Table) Validate() error {
	for label, value := range map[string]string{
		"source schema": table.SourceSchema,
		"source table":  table.SourceName,
	} {
		if err := ident.ValidateSource(value); err != nil {
			return fmt.Errorf("%s %q: %w", label, value, err)
		}
	}
	for label, value := range map[string]string{
		"target schema": table.TargetSchema,
		"target table":  table.TargetName,
	} {
		if err := ident.ValidateTarget(value); err != nil {
			return fmt.Errorf("%s %q: %w", label, value, err)
		}
	}
	if len(table.Columns) == 0 {
		return fmt.Errorf("source table has no columns")
	}

	targetNames := make(map[string]string, len(table.Columns))
	keyOrdinals := make(map[int]string)
	for _, column := range table.Columns {
		if err := ident.ValidateSource(column.SourceName); err != nil {
			return fmt.Errorf("source column %q: %w", column.SourceName, err)
		}
		if err := ident.ValidateTarget(column.TargetName); err != nil {
			return fmt.Errorf("target column %q: %w", column.TargetName, err)
		}
		folded := lowerIdentifier.String(column.TargetName)
		if previous, exists := targetNames[folded]; exists {
			return fmt.Errorf("columns %q and %q map ambiguously to %q", previous, column.SourceName, folded)
		}
		targetNames[folded] = column.SourceName
		if column.PKOrdinal > 0 {
			if previous, exists := keyOrdinals[column.PKOrdinal]; exists {
				return fmt.Errorf("primary-key ordinal %d is shared by %q and %q", column.PKOrdinal, previous, column.SourceName)
			}
			keyOrdinals[column.PKOrdinal] = column.SourceName
		}
	}
	for ordinal := 1; ordinal <= len(keyOrdinals); ordinal++ {
		if _, exists := keyOrdinals[ordinal]; !exists {
			return fmt.Errorf("primary-key ordinal %d is missing", ordinal)
		}
	}
	return nil
}
