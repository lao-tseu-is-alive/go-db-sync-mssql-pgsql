package schema

import (
	"fmt"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var lowerIdentifier = cases.Lower(language.Und)

// MapColumn maps one SQL Server metadata column using the Phase 1 compatibility table.
func MapColumn(source SourceColumn) (Column, error) {
	typeName := strings.ToLower(strings.TrimSpace(source.TypeName))
	targetType, conversion, err := mapType(typeName, source.MaxLength)
	if err != nil {
		return Column{}, fmt.Errorf("column %q type %q: %w", source.Name, source.TypeName, err)
	}
	return Column{
		SourceName:     source.Name,
		TargetName:     lowerIdentifier.String(source.Name),
		SourceType:     typeName,
		PostgreSQLType: targetType,
		Nullable:       source.Nullable,
		PKOrdinal:      source.PKOrdinal,
		Conversion:     conversion,
	}, nil
}

func mapType(typeName string, maxLength int) (string, Conversion, error) {
	switch typeName {
	case "tinyint", "smallint", "int", "integer":
		return "integer", ConversionDirect, nil
	case "bigint":
		return "bigint", ConversionDirect, nil
	case "bit":
		return "boolean", ConversionDirect, nil
	case "varchar", "nvarchar", "nchar", "text", "ntext":
		return "text", ConversionDirect, nil
	case "char":
		if maxLength < 1 {
			return "", 0, fmt.Errorf("invalid char length %d", maxLength)
		}
		return fmt.Sprintf("char(%d)", maxLength), ConversionDirect, nil
	case "binary", "varbinary":
		return "bytea", ConversionDirect, nil
	case "smalldatetime", "datetime":
		return "timestamp without time zone", ConversionDirect, nil
	case "money", "smallmoney":
		return "numeric(20,4)", ConversionMoney, nil
	case "uniqueidentifier":
		return "uuid", ConversionUUID, nil
	case "timestamp", "rowversion":
		return "bigint", ConversionRowVersion, nil
	default:
		return "", 0, fmt.Errorf("unsupported SQL Server type; add a documented mapping and exact-value tests")
	}
}

// CanonicalPostgreSQLType normalizes equivalent PostgreSQL catalog spellings.
func CanonicalPostgreSQLType(value string) string {
	value = strings.ToLower(strings.Join(strings.Fields(value), " "))
	value = strings.ReplaceAll(value, "character(", "char(")
	value = strings.ReplaceAll(value, "numeric(20, 4)", "numeric(20,4)")
	value = strings.ReplaceAll(value, "decimal(20,4)", "numeric(20,4)")
	if value == "timestamp" {
		return "timestamp without time zone"
	}
	return value
}
