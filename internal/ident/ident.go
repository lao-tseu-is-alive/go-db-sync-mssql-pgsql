// Package ident validates and quotes database identifiers without treating them as SQL parameters.
package ident

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ValidateSource validates one SQL Server schema, table or column identifier.
func ValidateSource(value string) error {
	if err := validateCommon(value); err != nil {
		return err
	}
	if utf8.RuneCountInString(value) > 128 {
		return fmt.Errorf("SQL Server identifier exceeds 128 characters")
	}
	return nil
}

// ValidateTarget validates one PostgreSQL schema, table or column identifier.
func ValidateTarget(value string) error {
	if err := validateCommon(value); err != nil {
		return err
	}
	if len(value) > 63 {
		return fmt.Errorf("PostgreSQL identifier exceeds 63 UTF-8 bytes")
	}
	return nil
}

func validateCommon(value string) error {
	if value == "" {
		return fmt.Errorf("identifier is empty")
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("identifier is not valid UTF-8")
	}
	for _, character := range value {
		if character == 0 || unicode.IsControl(character) {
			return fmt.Errorf("identifier contains a control character")
		}
	}
	return nil
}

// MSSQL quotes a validated SQL Server identifier.
func MSSQL(value string) string {
	return "[" + strings.ReplaceAll(value, "]", "]]") + "]"
}

// PostgreSQL quotes a validated PostgreSQL identifier.
func PostgreSQL(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

// PostgreSQLQualified quotes a PostgreSQL schema and relation name.
func PostgreSQLQualified(schema, relation string) string {
	return PostgreSQL(schema) + "." + PostgreSQL(relation)
}
