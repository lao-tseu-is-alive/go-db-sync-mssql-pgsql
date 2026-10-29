package schema

import "testing"

func TestMapColumnSupportedTypes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		source    string
		length    int
		precision int
		scale     int
		target    string
	}{
		{"tinyint", 1, 0, 0, "integer"}, {"smallint", 2, 0, 0, "integer"}, {"int", 4, 0, 0, "integer"},
		{"bigint", 8, 0, 0, "bigint"}, {"bit", 1, 0, 0, "boolean"}, {"varchar", 20, 0, 0, "text"},
		{"nvarchar", 40, 0, 0, "text"}, {"nchar", 20, 0, 0, "text"}, {"text", -1, 0, 0, "text"},
		{"ntext", -1, 0, 0, "text"}, {"char", 7, 0, 0, "char(7)"}, {"binary", 8, 0, 0, "bytea"},
		{"varbinary", -1, 0, 0, "bytea"}, {"smalldatetime", 4, 0, 0, "timestamp without time zone"},
		{"datetime", 8, 0, 0, "timestamp without time zone"}, {"money", 8, 0, 0, "numeric(20,4)"},
		{"smallmoney", 4, 0, 0, "numeric(20,4)"}, {"decimal", 9, 18, 6, "numeric(18,6)"},
		{"numeric", 17, 38, 18, "numeric(38,18)"}, {"uniqueidentifier", 16, 0, 0, "uuid"},
		{"timestamp", 8, 0, 0, "bigint"}, {"rowversion", 8, 0, 0, "bigint"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.source, func(t *testing.T) {
			t.Parallel()
			column, err := MapColumn(SourceColumn{
				Name: "Value", TypeName: test.source, MaxLength: test.length,
				Precision: test.precision, Scale: test.scale,
			})
			if err != nil {
				t.Fatal(err)
			}
			if column.PostgreSQLType != test.target || column.TargetName != "value" {
				t.Fatalf("MapColumn = %#v, want type %q and lower-case name", column, test.target)
			}
		})
	}
}

func TestMapColumnRejectsAmbiguousTypes(t *testing.T) {
	t.Parallel()
	for _, typeName := range []string{"datetime2", "datetimeoffset", "date", "time", "float", "real", "image", "xml", "sql_variant"} {
		if _, err := MapColumn(SourceColumn{Name: "value", TypeName: typeName}); err == nil {
			t.Errorf("MapColumn accepted unsupported %q", typeName)
		}
	}
}

func TestMapColumnRejectsInvalidDecimalMetadata(t *testing.T) {
	t.Parallel()
	for _, source := range []SourceColumn{
		{Name: "value", TypeName: "decimal", Precision: 0, Scale: 0},
		{Name: "value", TypeName: "decimal", Precision: 39, Scale: 0},
		{Name: "value", TypeName: "numeric", Precision: 10, Scale: -1},
		{Name: "value", TypeName: "numeric", Precision: 10, Scale: 11},
	} {
		if _, err := MapColumn(source); err == nil {
			t.Fatalf("MapColumn accepted precision=%d scale=%d", source.Precision, source.Scale)
		}
	}
}

func TestCanonicalPostgreSQLNumericType(t *testing.T) {
	t.Parallel()
	for input, expected := range map[string]string{
		"numeric(38, 18)":  "numeric(38,18)",
		"DECIMAL( 18 , 6)": "numeric(18,6)",
	} {
		if actual := CanonicalPostgreSQLType(input); actual != expected {
			t.Fatalf("CanonicalPostgreSQLType(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestMapColumnUsesPythonUnicodeLowercase(t *testing.T) {
	t.Parallel()
	column, err := MapColumn(SourceColumn{Name: "İD", TypeName: "int"})
	if err != nil {
		t.Fatal(err)
	}
	if column.TargetName != "i̇d" {
		t.Fatalf("TargetName = %q", column.TargetName)
	}
}

func TestTableRejectsLowercaseCollision(t *testing.T) {
	t.Parallel()
	table := Table{
		SourceSchema: "dbo", SourceName: "Thing", TargetSchema: "public", TargetName: "thing",
		Columns: []Column{{SourceName: "ID", TargetName: "id"}, {SourceName: "Id", TargetName: "id"}},
	}
	if err := table.Validate(); err == nil {
		t.Fatal("Validate accepted an ambiguous lower-case mapping")
	}
}

func TestPrimaryKeyOrderIsMetadataOrder(t *testing.T) {
	t.Parallel()
	table := Table{Columns: []Column{
		{TargetName: "first_in_columns", PKOrdinal: 2},
		{TargetName: "second_in_columns", PKOrdinal: 1},
	}}
	keys := table.PrimaryKeyNames()
	if len(keys) != 2 || keys[0] != "second_in_columns" || keys[1] != "first_in_columns" {
		t.Fatalf("PrimaryKeyNames = %v", keys)
	}
}
