package schema

import (
	"bytes"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestDirectValuesRemainExact(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 28, 12, 34, 56, 123000000, time.UTC)
	values := []any{nil, `\N`, "unicode é漢字", "line one\nline two", "back\\slash", "control\x1f", true, false, []byte{0, 1, 255}, now}
	column := Column{SourceName: "value", Conversion: ConversionDirect}
	for _, expected := range values {
		actual, err := ConvertValue(column, expected)
		if err != nil {
			t.Fatal(err)
		}
		expectedBytes, expectedIsBytes := expected.([]byte)
		actualBytes, actualIsBytes := actual.([]byte)
		if expectedIsBytes && actualIsBytes {
			if !bytes.Equal(expectedBytes, actualBytes) {
				t.Fatalf("binary changed: %v != %v", actualBytes, expectedBytes)
			}
			continue
		}
		if actual != expected {
			t.Fatalf("ConvertValue(%#v) = %#v", expected, actual)
		}
	}
}

func TestMoneyPreservesDecimal(t *testing.T) {
	t.Parallel()
	actual, err := ConvertValue(Column{Conversion: ConversionMoney}, []byte("-123456789012345.6789"))
	if err != nil {
		t.Fatal(err)
	}
	numeric := actual.(pgtype.Numeric)
	value, err := numeric.Value()
	if err != nil {
		t.Fatal(err)
	}
	if value != "-123456789012345.6789" {
		t.Fatalf("numeric value = %#v", value)
	}
}

func TestUUIDConversion(t *testing.T) {
	t.Parallel()
	expected := uuid.MustParse("6f9619ff-8b86-d011-b42d-00c04fc964ff")
	actual, err := ConvertValue(Column{Conversion: ConversionUUID}, expected.String())
	if err != nil {
		t.Fatal(err)
	}
	if actual != expected {
		t.Fatalf("UUID = %v, want %v", actual, expected)
	}
}

func TestRowVersionBigEndianAndRange(t *testing.T) {
	t.Parallel()
	actual, err := ConvertValue(Column{Conversion: ConversionRowVersion}, []byte{0, 0, 0, 0, 0, 0, 7, 0xd3})
	if err != nil {
		t.Fatal(err)
	}
	if actual != int64(2003) {
		t.Fatalf("rowversion = %v, want 2003", actual)
	}
	max := []byte{0x7f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	actual, err = ConvertValue(Column{Conversion: ConversionRowVersion}, max)
	if err != nil || actual != int64(math.MaxInt64) {
		t.Fatalf("max signed rowversion = %v, %v", actual, err)
	}
	if _, err := ConvertValue(Column{Conversion: ConversionRowVersion}, []byte{0x80, 0, 0, 0, 0, 0, 0, 0}); err == nil {
		t.Fatal("rowversion above signed bigint unexpectedly succeeded")
	}
}
