package naming

import "testing"

func TestTableNameLegacyExamples(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"Employe":           "employe",
		"SomeTable":         "some_table",
		"ABCDefGHI":         "a_bcdef_ghi",
		"already_snake":     "already_snake",
		"lowerUPPEREnd":     "lower_upperend",
		"ActeurAdresseCCJP": "acteur_adresse_cc_jp",
		"İName":             "i̇_name",
	}
	for input, expected := range tests {
		input, expected := input, expected
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			if actual := TableName(input); actual != expected {
				t.Fatalf("TableName(%q) = %q, want %q", input, actual, expected)
			}
		})
	}
}

func TestStandardTableNameEveryLegacyAcronym(t *testing.T) {
	t.Parallel()
	expectedNames := map[string]string{
		"AGF": "agf", "EGID": "egid", "ARCH": "arch", "ASST": "asst", "CID": "cid",
		"CIL": "cil", "ESTRID": "estrid", "ES": "es", "GC": "gc", "GEO": "geo",
		"CCJP": "cc_jp", "CH": "ch", "CN": "cn", "RCB": "rcb", "DMZ": "dmz",
		"ISO": "iso", "NT": "nt", "OPC": "opc", "POLC": "polc", "QSE": "qse",
		"RM": "rm", "SAP": "sap", "SCC": "scc", "SPD": "spd", "URB": "urb",
	}
	for _, replacement := range acronymReplacements {
		replacement := replacement
		t.Run(replacement.old, func(t *testing.T) {
			t.Parallel()
			input := "x" + replacement.old + "x"
			expected := "x" + replacement.new + "x"
			if actual := StandardTableName(input); actual != expected {
				t.Fatalf("StandardTableName(%q) = %q, want %q", input, actual, expected)
			}
			expectedTable := "x_" + expectedNames[replacement.old] + "x"
			if actual := TableName(input); actual != expectedTable {
				t.Fatalf("TableName(%q) = %q, want %q", input, actual, expectedTable)
			}
		})
	}
}

func TestReplacementOrderMatchesLegacy(t *testing.T) {
	t.Parallel()
	if actual := StandardTableName("ESTRIDES"); actual != "EstridEs" {
		t.Fatalf("ordered replacements returned %q", actual)
	}
}
