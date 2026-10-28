package ident

import (
	"strings"
	"testing"
)

func TestIdentifierQuoting(t *testing.T) {
	t.Parallel()
	if actual := MSSQL("a]b"); actual != "[a]]b]" {
		t.Fatalf("MSSQL quote = %q", actual)
	}
	if actual := PostgreSQL(`a"b`); actual != `"a""b"` {
		t.Fatalf("PostgreSQL quote = %q", actual)
	}
}

func TestIdentifierValidation(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"", "bad\x00name", "bad\nname"} {
		if err := ValidateSource(value); err == nil {
			t.Fatalf("ValidateSource(%q) unexpectedly succeeded", value)
		}
	}
	if err := ValidateTarget(strings.Repeat("é", 32)); err == nil {
		t.Fatal("ValidateTarget accepted more than 63 UTF-8 bytes")
	}
	if err := ValidateSource("name with spaces"); err != nil {
		t.Fatalf("quoted identifier should permit spaces: %v", err)
	}
}
