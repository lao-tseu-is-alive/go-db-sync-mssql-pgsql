// Package syncer coordinates one guarded source discovery and PostgreSQL refresh.
package syncer

import (
	"context"
	"fmt"
	"time"

	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/postgres"
	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/schema"
	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/stream"
	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/syncmode"
)

// Source discovers, counts and streams one SQL Server table.
type Source interface {
	Describe(context.Context, string, string, string, string) (schema.Table, error)
	Count(context.Context, schema.Table) (int64, error)
	Rows(context.Context, schema.Table) (stream.Source, error)
}

// Target validates and publishes one PostgreSQL table.
type Target interface {
	Inspect(context.Context, schema.Table) (postgres.Status, error)
	Publish(context.Context, schema.Table, stream.Source, int64, syncmode.Mode) (postgres.Result, error)
}

// Request identifies the only table and whether mutation is explicitly authorized.
type Request struct {
	// SourceSchema is the SQL Server schema.
	SourceSchema string
	// SourceTable is the SQL Server table.
	SourceTable string
	// TargetSchema is the PostgreSQL schema.
	TargetSchema string
	// TargetTable is the PostgreSQL table.
	TargetTable string
	// Apply authorizes publication; false is a read-only dry run.
	Apply bool
	// SyncMode selects direct replacement or staged reconciliation.
	SyncMode syncmode.Mode
	// ProgressInterval is the minimum interval between COPY row reports.
	ProgressInterval time.Duration
	// Progress receives phase and COPY-count updates; nil disables reporting.
	Progress func(Progress)
}

// Progress describes one non-sensitive synchronization progress event.
type Progress struct {
	// Phase identifies the current operation.
	Phase string
	// Rows is the number of source rows staged so far.
	Rows int64
	// TotalRows is the source count measured before streaming.
	TotalRows int64
	// Elapsed is the time spent streaming source rows.
	Elapsed time.Duration
}

// Report contains discovery, validation and optional publication evidence.
type Report struct {
	// Table is the mapped metadata contract.
	Table schema.Table
	// SourceRows is the pre-stream source count.
	SourceRows int64
	// TargetStatus is the dry-run-compatible target inspection.
	TargetStatus postgres.Status
	// Publication is populated only for an applied refresh.
	Publication postgres.Result
	// Applied reports whether PostgreSQL publication ran.
	Applied bool
	// Duration is the total orchestration duration.
	Duration time.Duration
	// Performance contains coarse orchestration and detailed publication timings.
	Performance Performance
}

// Performance contains timing evidence for source discovery, validation and transfer.
type Performance struct {
	// DiscoveryDuration covers SQL Server table metadata discovery and mapping.
	DiscoveryDuration time.Duration
	// TargetValidationDuration covers the read-only PostgreSQL compatibility check.
	TargetValidationDuration time.Duration
	// SourceCountDuration covers SQL Server COUNT_BIG execution.
	SourceCountDuration time.Duration
	// SourceQueryDuration covers opening the forward-only SQL Server row query.
	SourceQueryDuration time.Duration
	// Publication contains COPY and transactional PostgreSQL timings.
	Publication postgres.Performance
}

// Service coordinates the source and target adapters sequentially.
type Service struct {
	source Source
	target Target
}

// New constructs a one-table synchronization service.
func New(source Source, target Target) *Service {
	return &Service{source: source, target: target}
}

// Prepare discovers, validates and counts one table without opening its row stream.
func (service *Service) Prepare(ctx context.Context, request Request) (Report, error) {
	started := time.Now()
	emitProgress(request.Progress, Progress{Phase: "discovering"})
	phaseStarted := time.Now()
	table, err := service.source.Describe(ctx, request.SourceSchema, request.SourceTable, request.TargetSchema, request.TargetTable)
	if err != nil {
		return Report{}, fmt.Errorf("discover source table: %w", err)
	}
	performance := Performance{DiscoveryDuration: time.Since(phaseStarted)}
	emitProgress(request.Progress, Progress{Phase: "validating-target"})
	phaseStarted = time.Now()
	status, err := service.target.Inspect(ctx, table)
	if err != nil {
		return Report{}, fmt.Errorf("validate target table: %w", err)
	}
	performance.TargetValidationDuration = time.Since(phaseStarted)
	emitProgress(request.Progress, Progress{Phase: "counting-source"})
	phaseStarted = time.Now()
	count, err := service.source.Count(ctx, table)
	if err != nil {
		return Report{}, err
	}
	performance.SourceCountDuration = time.Since(phaseStarted)
	report := Report{Table: table, SourceRows: count, TargetStatus: status, Performance: performance}
	report.Duration = time.Since(started)
	return report, nil
}

// ApplyPrepared streams and publishes one table that was already prepared.
func (service *Service) ApplyPrepared(ctx context.Context, request Request, prepared Report) (Report, error) {
	started := time.Now()
	report := prepared
	report.Applied = false
	report.Publication = postgres.Result{}
	if !request.Apply {
		return report, fmt.Errorf("apply prepared table: request does not authorize --apply")
	}
	if err := validatePreparedRequest(request, prepared.Table); err != nil {
		return report, err
	}
	transferPhase := "staging"
	if request.SyncMode == syncmode.Replace {
		transferPhase = "copying"
	}
	emitProgress(request.Progress, Progress{Phase: transferPhase, TotalRows: report.SourceRows})
	phaseStarted := time.Now()
	rows, err := service.source.Rows(ctx, report.Table)
	if err != nil {
		report.Duration = time.Since(started)
		return report, err
	}
	report.Performance.SourceQueryDuration = time.Since(phaseStarted)
	if request.Progress != nil && request.ProgressInterval > 0 {
		rows = newProgressSource(rows, report.SourceRows, transferPhase, request.ProgressInterval, request.Progress)
	}
	result, err := service.target.Publish(ctx, report.Table, rows, report.SourceRows, request.SyncMode)
	if err != nil {
		report.Duration = time.Since(started)
		return report, err
	}
	report.Publication = result
	report.Performance.Publication = result.Performance
	report.Applied = true
	report.Duration = time.Since(started)
	return report, nil
}

// Run prepares one table, then publishes only when Apply is true.
func (service *Service) Run(ctx context.Context, request Request) (Report, error) {
	started := time.Now()
	report, err := service.Prepare(ctx, request)
	if err != nil {
		return Report{}, err
	}
	if !request.Apply {
		report.Duration = time.Since(started)
		return report, nil
	}
	report, err = service.ApplyPrepared(ctx, request, report)
	report.Duration = time.Since(started)
	return report, err
}

func validatePreparedRequest(request Request, table schema.Table) error {
	if request.SourceSchema != table.SourceSchema || request.SourceTable != table.SourceName ||
		request.TargetSchema != table.TargetSchema || request.TargetTable != table.TargetName {
		return fmt.Errorf("apply prepared table: request does not match prepared mapping %s.%s to %s.%s",
			table.SourceSchema, table.SourceName, table.TargetSchema, table.TargetName)
	}
	return nil
}

func emitProgress(callback func(Progress), progress Progress) {
	if callback != nil {
		callback(progress)
	}
}
