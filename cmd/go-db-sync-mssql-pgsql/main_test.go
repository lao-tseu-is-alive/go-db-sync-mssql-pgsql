package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/config"
	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/multisync"
	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/postgres"
	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/stream"
	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/syncer"
)

func TestVersionDoesNotNeedConnections(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"--version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run returned %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "v0.3.0") {
		t.Fatalf("version output = %q", stdout.String())
	}
}

func TestCPUProfileRefusesToOverwrite(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "existing.pprof")
	if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := startCPUProfile(path); err == nil {
		t.Fatal("startCPUProfile unexpectedly accepted an existing file")
	}
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != "keep" {
		t.Fatalf("existing profile changed: %q, %v", contents, err)
	}
}

func TestCPUProfileWritesNewFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.pprof")
	stop, err := startCPUProfile(path)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Millisecond)
	var work uint64
	for time.Now().Before(deadline) {
		work = work*1664525 + 1013904223
	}
	benchmarkCPUSink = work
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Fatal("CPU profile is empty")
	}
}

var benchmarkCPUSink uint64

func TestPerformancePrinterSeparatesSourceAndCopyPipeline(t *testing.T) {
	t.Parallel()
	report := syncer.Report{
		Applied: true,
		Performance: syncer.Performance{Publication: postgres.Performance{
			CopyFromDuration: 10 * time.Second, TruncateDuration: 2 * time.Second,
			Source: stream.Metrics{
				Rows: 3, NextDuration: 4 * time.Second, ScanDuration: time.Second, ConversionDuration: 2 * time.Second,
			},
		}},
	}
	var output bytes.Buffer
	printPerformance(&output, report)
	for _, expected := range []string{"rows=3", "next=4s", "conversion=2s", "pipeline_remainder=3s", "truncate=2s"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("performance output %q lacks %q", output.String(), expected)
		}
	}
}

func TestProgressPrinterReportsCountPercentAndRate(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	printProgress := progressPrinter(&output)
	printProgress(syncer.Progress{Phase: "staging", Rows: 50, TotalRows: 100, Elapsed: 2 * time.Second})
	for _, expected := range []string{"rows=50/100", "percent=50.0", "elapsed=2s", "rows_per_second=25.0"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("progress output %q lacks %q", output.String(), expected)
		}
	}
}

func TestProgressPrinterPreservesCopyPhase(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	progressPrinter(&output)(syncer.Progress{Phase: "copying", Rows: 1, TotalRows: 2, Elapsed: time.Second})
	if !strings.Contains(output.String(), "phase=copying") {
		t.Fatalf("progress output = %q", output.String())
	}
}

func TestNoModeFailsBeforeConnections(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"--source-table", "Thing"}, &stdout, &stderr); code != 2 {
		t.Fatalf("run returned %d", code)
	}
	if !strings.Contains(stderr.String(), "exactly one") {
		t.Fatalf("error output = %q", stderr.String())
	}
}

func TestWriteTableNamesUsesExactDeterministicOrder(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	if err := writeTableNames(&output, []string{"XMLDocument", "Document", "document"}); err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), "Document\nXMLDocument\ndocument\n"; got != want {
		t.Fatalf("table list = %q, want %q", got, want)
	}
}

func TestApplyPlanSuppressesExcludedDetailsUnlessRequested(t *testing.T) {
	t.Parallel()
	plan := multisync.Plan{SourceSchema: "dbo", TargetSchema: "public", Tables: []multisync.TablePlan{
		{SourceName: "Selected", TargetName: "selected"},
		{SourceName: "Excluded", TargetName: "excluded", Excluded: true},
	}}
	prepared := []preparedTable{
		{plan: plan.Tables[0], report: syncer.Report{SourceRows: 2}},
		{plan: plan.Tables[1]},
	}
	var compact bytes.Buffer
	printPlan(&compact, config.Options{Apply: true}, plan, prepared, time.Second)
	if strings.Contains(compact.String(), "source=dbo.Excluded") || !strings.Contains(compact.String(), "excluded=1 excluded_details=false") {
		t.Fatalf("compact apply plan = %q", compact.String())
	}
	var detailed bytes.Buffer
	printPlan(&detailed, config.Options{Apply: true, ShowExcluded: true}, plan, prepared, time.Second)
	if !strings.Contains(detailed.String(), "source=dbo.Excluded") || !strings.Contains(detailed.String(), "excluded_details=true") {
		t.Fatalf("detailed apply plan = %q", detailed.String())
	}
	var dryRun bytes.Buffer
	printPlan(&dryRun, config.Options{DryRun: true}, plan, prepared, time.Second)
	if strings.Contains(dryRun.String(), "source=dbo.Excluded") || !strings.Contains(dryRun.String(), "excluded_details=false") {
		t.Fatalf("dry-run plan = %q", dryRun.String())
	}
}
