package config

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/syncmode"
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

func TestParseRequiresExactlyOneTableSelection(t *testing.T) {
	t.Parallel()
	for _, arguments := range [][]string{
		{"--dry-run"},
		{"--source-table", "Thing", "--all-tables", "--dry-run"},
	} {
		if _, err := Parse(arguments, testEnv, &bytes.Buffer{}); err == nil {
			t.Fatalf("Parse(%v) unexpectedly succeeded", arguments)
		}
	}
}

func TestParseAllTablesOptions(t *testing.T) {
	t.Parallel()
	options, err := Parse([]string{
		"--all-tables", "--dry-run", "--tables-config=config/tables.yaml", "--workers=3", "--show-excluded",
	}, testEnv, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !options.AllTables || options.TablesConfig != "config/tables.yaml" || options.Workers != 3 || !options.ShowExcluded {
		t.Fatalf("multi-table options = %#v", options)
	}
	if options.TargetTable != "" {
		t.Fatalf("TargetTable = %q, want empty", options.TargetTable)
	}
}

func TestParseListTablesNeedsOnlyMSSQL(t *testing.T) {
	t.Parallel()
	options, err := Parse([]string{"--list-tables", "--source-schema=archive"}, func(name string) string {
		if name == "MSSQL_DSN" {
			return "sqlserver://source"
		}
		return ""
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !options.ListTables || options.SourceSchema != "archive" || options.PGSQLDSN != "" {
		t.Fatalf("list options = %#v", options)
	}
}

func TestParseListTablesRejectsSynchronizationFlags(t *testing.T) {
	t.Parallel()
	for _, arguments := range [][]string{
		{"--list-tables", "--source-table=Thing"},
		{"--list-tables", "--all-tables"},
		{"--list-tables", "--dry-run"},
		{"--list-tables", "--apply"},
		{"--list-tables", "--tables-config=config/tables.yaml"},
		{"--list-tables", "--target-table=thing"},
		{"--list-tables", "--show-excluded"},
	} {
		if _, err := Parse(arguments, testEnv, &bytes.Buffer{}); err == nil {
			t.Fatalf("Parse(%v) unexpectedly succeeded", arguments)
		}
	}
}

func TestParseListTablesRequiresMSSQL(t *testing.T) {
	t.Parallel()
	_, err := Parse([]string{"--list-tables"}, func(string) string { return "" }, &bytes.Buffer{})
	if !errors.Is(err, ErrMissingMSSQLDSN) {
		t.Fatalf("Parse error = %v", err)
	}
}

func TestParseRejectsInvalidMultiTableCombinations(t *testing.T) {
	t.Parallel()
	tests := [][]string{
		{"--source-table", "Thing", "--dry-run", "--tables-config=config/tables.yaml"},
		{"--source-table", "Thing", "--dry-run", "--show-excluded"},
		{"--all-tables", "--dry-run", "--target-table=thing"},
		{"--all-tables", "--dry-run", "--workers=0"},
	}
	for _, arguments := range tests {
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
	if options.SyncMode != syncmode.Replace {
		t.Fatalf("SyncMode = %q", options.SyncMode)
	}
	if options.Workers != 2 {
		t.Fatalf("Workers = %d", options.Workers)
	}
}

func TestParseSyncMode(t *testing.T) {
	t.Parallel()
	options, err := Parse([]string{"--source-table", "Thing", "--apply", "--sync-mode=merge"}, testEnv, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if options.SyncMode != syncmode.Merge {
		t.Fatalf("SyncMode = %q", options.SyncMode)
	}
	_, err = Parse([]string{"--source-table", "Thing", "--apply", "--sync-mode=append"}, testEnv, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "choose replace or merge") {
		t.Fatalf("unsupported mode error = %v", err)
	}
}

func TestParseRejectsNegativeProgressInterval(t *testing.T) {
	t.Parallel()
	_, err := Parse([]string{"--source-table", "Thing", "--dry-run", "--progress-interval=-1s"}, testEnv, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "cannot be negative") {
		t.Fatalf("Parse error = %v", err)
	}
}

func TestParsePerformanceOptions(t *testing.T) {
	t.Parallel()
	options, err := Parse([]string{
		"--source-table", "Thing", "--dry-run", "--perf-metrics", "--cpu-profile=/tmp/test.cpu.pprof",
	}, testEnv, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if !options.PerformanceMetrics || options.CPUProfile != "/tmp/test.cpu.pprof" {
		t.Fatalf("performance options = %#v", options)
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
