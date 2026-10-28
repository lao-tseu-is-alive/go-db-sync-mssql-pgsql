package config

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

func testEnv(name string) string {
	return map[string]string{"MSSQL_DSN": "sqlserver://user:secret@source/db", "PGSQL_DSN": "postgres://user:secret@target/db"}[name]
}

func TestParseRequiresExplicitMode(t *testing.T) {
	t.Parallel()
	for _, arguments := range [][]string{{"--source-table", "Thing"}, {"--source-table", "Thing", "--apply", "--dry-run"}} {
		if _, err := Parse(arguments, testEnv, &bytes.Buffer{}); err == nil {
			t.Fatalf("Parse(%v) unexpectedly succeeded", arguments)
		}
	}
}

func TestParseAppliesLegacyTargetName(t *testing.T) {
	t.Parallel()
	options, err := Parse([]string{"--source-table", "AFF_OPCThing", "--dry-run"}, testEnv, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if options.TargetTable != "a_ff__opc_thing" {
		t.Fatalf("TargetTable = %q", options.TargetTable)
	}
	if options.ProgressInterval != 5*time.Second {
		t.Fatalf("ProgressInterval = %s", options.ProgressInterval)
	}
}

func TestParseRejectsNegativeProgressInterval(t *testing.T) {
	t.Parallel()
	_, err := Parse([]string{"--source-table", "Thing", "--dry-run", "--progress-interval=-1s"}, testEnv, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "cannot be negative") {
		t.Fatalf("Parse error = %v", err)
	}
}

func TestVersionNeedsNoSecrets(t *testing.T) {
	t.Parallel()
	options, err := Parse([]string{"--version"}, func(string) string { return "" }, &bytes.Buffer{})
	if err != nil || !options.Version {
		t.Fatalf("version parse = %#v, %v", options, err)
	}
}

func TestRedactError(t *testing.T) {
	t.Parallel()
	err := errors.New("connect postgres://alice:secret@example/db password=hunter2")
	redacted := RedactError(err).Error()
	for _, secret := range []string{"alice", "secret", "hunter2"} {
		if strings.Contains(redacted, secret) {
			t.Fatalf("redacted error still contains %q: %s", secret, redacted)
		}
	}
}
