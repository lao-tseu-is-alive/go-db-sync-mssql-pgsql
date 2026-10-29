package main

import (
	"context"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/config"
	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/mssql"
)

func runListTables(ctx context.Context, options config.Options, stdout, stderr io.Writer) int {
	connectCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	source, err := mssql.Open(connectCtx, options.MSSQLDSN)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", config.RedactError(err, options.MSSQLDSN))
		return 1
	}
	defer source.Close()
	tables, err := source.Tables(ctx, options.SourceSchema)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", config.RedactError(err, options.MSSQLDSN))
		return 1
	}
	if err := writeTableNames(stdout, tables); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

func writeTableNames(output io.Writer, tables []string) error {
	names := append([]string(nil), tables...)
	sort.Strings(names)
	for _, table := range names {
		if _, err := fmt.Fprintln(output, table); err != nil {
			return fmt.Errorf("write source table list: %w", err)
		}
	}
	return nil
}
