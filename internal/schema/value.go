package schema

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// ConvertValue converts one non-NULL SQL Server driver value for pgx COPY.
func ConvertValue(column Column, value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	switch column.Conversion {
	case ConversionDirect:
		return value, nil
	case ConversionMoney:
		return convertMoney(value)
	case ConversionUUID:
		return convertUUID(value)
	case ConversionRowVersion:
		return convertRowVersion(value)
	default:
		return nil, fmt.Errorf("column %q has unknown conversion %d", column.SourceName, column.Conversion)
	}
}

func convertMoney(value any) (pgtype.Numeric, error) {
	var text string
	switch typed := value.(type) {
	case []byte:
		text = string(typed)
	case string:
		text = typed
	default:
		return pgtype.Numeric{}, fmt.Errorf("money value has driver type %T, want exact decimal bytes", value)
	}
	var numeric pgtype.Numeric
	if err := numeric.Scan(text); err != nil {
		return pgtype.Numeric{}, fmt.Errorf("parse money value: %w", err)
	}
	return numeric, nil
}

func convertUUID(value any) (uuid.UUID, error) {
	switch typed := value.(type) {
	case string:
		parsed, err := uuid.Parse(typed)
		if err != nil {
			return uuid.Nil, fmt.Errorf("parse uniqueidentifier: %w", err)
		}
		return parsed, nil
	case uuid.UUID:
		return typed, nil
	default:
		return uuid.Nil, fmt.Errorf("uniqueidentifier has driver type %T, want canonical string projection", value)
	}
}

func convertRowVersion(value any) (int64, error) {
	bytes, ok := value.([]byte)
	if !ok {
		return 0, fmt.Errorf("rowversion has driver type %T, want []byte", value)
	}
	if len(bytes) != 8 {
		return 0, fmt.Errorf("rowversion has %d bytes, want 8", len(bytes))
	}
	unsigned := binary.BigEndian.Uint64(bytes)
	if unsigned > math.MaxInt64 {
		return 0, fmt.Errorf("rowversion %d exceeds PostgreSQL signed bigint range", unsigned)
	}
	return int64(unsigned), nil
}
