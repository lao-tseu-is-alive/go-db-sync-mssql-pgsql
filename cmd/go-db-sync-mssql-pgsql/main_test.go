package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/syncer"
)

func TestVersionDoesNotNeedConnections(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"--version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run returned %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "v0.1.0") {
		t.Fatalf("version output = %q", stdout.String())
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
