package schema

import "testing"

func TestMapColumnSupportedTypes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		source string
		length int
		target string
	}{
		{"tinyint", 1, "integer"}, {"smallint", 2, "integer"}, {"int", 4, "integer"},
		{"bigint", 8, "bigint"}, {"bit", 1, "boolean"}, {"varchar", 20, "text"},
		{"nvarchar", 40, "text"}, {"nchar", 20, "text"}, {"text", -1, "text"},
		{"ntext", -1, "text"}, {"char", 7, "char(7)"}, {"binary", 8, "bytea"},
		{"varbinary", -1, "bytea"}, {"smalldatetime", 4, "timestamp without time zone"},
		{"datetime", 8, "timestamp without time zone"}, {"money", 8, "numeric(20,4)"},
		{"smallmoney", 4, "numeric(20,4)"}, {"uniqueidentifier", 16, "uuid"},
		{"timestamp", 8, "bigint"}, {"rowversion", 8, "bigint"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.source, func(t *testing.T) {
			t.Parallel()
			column, err := MapColumn(SourceColumn{Name: "Value", TypeName: test.source, MaxLength: test.length})
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
	for _, typeName := range []string{"decimal", "numeric", "datetime2", "datetimeoffset", "date", "time", "float", "real", "image", "xml", "sql_variant"} {
		if _, err := MapColumn(SourceColumn{Name: "value", TypeName: typeName}); err == nil {
			t.Errorf("MapColumn accepted unsupported %q", typeName)
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
