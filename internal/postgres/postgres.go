// Package postgres validates and atomically publishes PostgreSQL table refreshes.
package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/schema"
	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/stream"
	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/syncmode"
)

// Status describes target validation without changing PostgreSQL.
type Status struct {
	// Exists reports whether the target table already exists.
	Exists bool
	// Compatible reports whether an existing table exactly matches the Phase 1 contract.
	Compatible bool
}

// Result reports verified publication counts.
type Result struct {
	// Mode is the publication strategy used for this result.
	Mode syncmode.Mode
	// CopiedRows is the number accepted by direct or staged COPY.
	CopiedRows int64
	// PublishedRows is the final live-table count checked before commit.
	PublishedRows int64
	// CreatedTarget reports whether this transaction created the live target.
	CreatedTarget bool
	// StageDuration is the elapsed time for merge staging and verification; replace reports zero.
	StageDuration time.Duration
	// PublishDuration is the elapsed time mutating and verifying the live target.
	// In replace mode it includes the direct COPY.
	PublishDuration time.Duration
	// Performance contains detailed phase timings collected without changing publication semantics.
	Performance Performance
}

// Performance contains PostgreSQL and optional source-stream timing evidence.
type Performance struct {
	// TargetPreparationDuration covers schema checks, target inspection and optional creation.
	TargetPreparationDuration time.Duration
	// TruncateDuration covers replace-mode TRUNCATE TABLE with RESTRICT.
	TruncateDuration time.Duration
	// StageCreateDuration covers temporary staging table creation.
	StageCreateDuration time.Duration
	// CopyFromDuration is total pgx.CopyFrom time, including pulling source rows.
	CopyFromDuration time.Duration
	// StageVerifyDuration covers the staging COUNT verification.
	StageVerifyDuration time.Duration
	// LiveInsertDuration covers direct COPY or insert/upsert into the live target.
	LiveInsertDuration time.Duration
	// LiveDeleteDuration covers deletion of rows absent from the refreshed source.
	LiveDeleteDuration time.Duration
	// FinalVerifyDuration covers the live target COUNT verification.
	FinalVerifyDuration time.Duration
	// CommitDuration covers the PostgreSQL transaction commit.
	CommitDuration time.Duration
	// Source contains opt-in MSSQL row-stream timings.
	Source stream.Metrics
}

// CopyPipelineRemainder returns COPY time not accounted for by measured source work.
// It includes pgx encoding, I/O, PostgreSQL insertion and unmeasured adapter overhead.
func (performance Performance) CopyPipelineRemainder() time.Duration {
	remainder := performance.CopyFromDuration - performance.Source.TotalDuration()
	if remainder < 0 {
		return 0
	}
	return remainder
}

// Client owns the PostgreSQL connection pool used for validation and publication.
type Client struct {
	pool *pgxpool.Pool
}

// Open connects to PostgreSQL using the supplied DSN and verifies connectivity.
func Open(ctx context.Context, dsn string) (*Client, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse PostgreSQL connection config: %w", err)
	}
	// A client is owned by one bounded worker. Keep one physical connection per
	// client so --workers is also the target-connection upper bound.
	config.MaxConns = 1
	config.MinConns = 0
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect to PostgreSQL: %w", err)
	}
	return &Client{pool: pool}, nil
}

// New returns a client around an existing pool, primarily for integration tests.
func New(pool *pgxpool.Pool) *Client {
	return &Client{pool: pool}
}

// Close releases the PostgreSQL connection pool.
func (client *Client) Close() {
	client.pool.Close()
}

// Inspect checks target schema existence and exact compatibility without writes.
func (client *Client) Inspect(ctx context.Context, table schema.Table) (Status, error) {
	if err := table.Validate(); err != nil {
		return Status{}, fmt.Errorf("validate table contract: %w", err)
	}
	if err := requireSchema(ctx, client.pool, table.TargetSchema); err != nil {
		return Status{}, err
	}
	target, err := inspectTarget(ctx, client.pool, table.TargetSchema, table.TargetName)
	if err != nil {
		return Status{}, err
	}
	if !target.exists {
		return Status{Exists: false, Compatible: true}, nil
	}
	if err := validateTarget(table, target); err != nil {
		return Status{Exists: true, Compatible: false}, err
	}
	return Status{Exists: true, Compatible: true}, nil
}

// Publish refreshes the live table using replace or merge in one transaction.
func (client *Client) Publish(
	ctx context.Context,
	table schema.Table,
	source stream.Source,
	expectedRows int64,
	mode syncmode.Mode,
) (result Result, returnErr error) {
	if err := table.Validate(); err != nil {
		_ = source.Close()
		return Result{}, fmt.Errorf("validate table contract: %w", err)
	}
	if _, err := syncmode.Parse(string(mode)); err != nil {
		_ = source.Close()
		return Result{}, err
	}
	result.Mode = mode
	sourceClosed := false
	defer func() {
		if !sourceClosed {
			_ = source.Close()
		}
	}()

	tx, err := client.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Result{}, fmt.Errorf("begin PostgreSQL publication: %w", err)
	}
	defer func() {
		if returnErr != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	if err := prepareTarget(ctx, tx, table, &result); err != nil {
		return Result{}, err
	}
	closeSource := func() error {
		if err := source.Close(); err != nil {
			return fmt.Errorf("close SQL Server row stream: %w", err)
		}
		sourceClosed = true
		if provider, ok := source.(stream.MetricsProvider); ok {
			result.Performance.Source = provider.StreamMetrics()
		}
		return nil
	}
	if mode == syncmode.Replace {
		err = publishReplace(ctx, tx, table, source, expectedRows, closeSource, &result)
	} else {
		err = publishMerge(ctx, tx, table, source, expectedRows, closeSource, &result)
	}
	if err != nil {
		return Result{}, err
	}
	commitStarted := time.Now()
	if err := tx.Commit(ctx); err != nil {
		return Result{}, fmt.Errorf("commit PostgreSQL publication: %w", err)
	}
	result.Performance.CommitDuration = time.Since(commitStarted)
	return result, nil
}

func prepareTarget(ctx context.Context, tx pgx.Tx, table schema.Table, result *Result) error {
	started := time.Now()
	defer func() { result.Performance.TargetPreparationDuration = time.Since(started) }()
	if err := requireSchema(ctx, tx, table.TargetSchema); err != nil {
		return err
	}
	target, err := inspectTarget(ctx, tx, table.TargetSchema, table.TargetName)
	if err != nil {
		return err
	}
	if target.exists {
		return validateTarget(table, target)
	}
	if _, err := tx.Exec(ctx, createTableSQL(table)); err != nil {
		return fmt.Errorf("create target table: %w", err)
	}
	result.CreatedTarget = true
	return nil
}

func publishReplace(
	ctx context.Context,
	tx pgx.Tx,
	table schema.Table,
	source stream.Source,
	expectedRows int64,
	closeSource func() error,
	result *Result,
) error {
	publishStarted := time.Now()
	if !result.CreatedTarget {
		truncateStarted := time.Now()
		_, err := tx.Exec(ctx, truncateTableSQL(table))
		result.Performance.TruncateDuration = time.Since(truncateStarted)
		if err != nil {
			return fmt.Errorf("replace mode cannot truncate target with RESTRICT; check TRUNCATE privilege and foreign-key dependencies or use --sync-mode=merge: %w", err)
		}
	}
	copyStarted := time.Now()
	copied, err := tx.CopyFrom(ctx, pgx.Identifier{table.TargetSchema, table.TargetName}, table.TargetColumnNames(), source)
	result.Performance.CopyFromDuration = time.Since(copyStarted)
	result.Performance.LiveInsertDuration = result.Performance.CopyFromDuration
	if err != nil {
		return fmt.Errorf("copy source rows directly into target: %w", err)
	}
	result.CopiedRows = copied
	if err := closeSource(); err != nil {
		return err
	}
	if copied != expectedRows {
		return fmt.Errorf("COPY reported %d rows but source count was %d", copied, expectedRows)
	}
	if err := verifyPublishedCount(ctx, tx, table, expectedRows, result); err != nil {
		return err
	}
	result.PublishDuration = time.Since(publishStarted)
	return nil
}

func publishMerge(
	ctx context.Context,
	tx pgx.Tx,
	table schema.Table,
	source stream.Source,
	expectedRows int64,
	closeSource func() error,
	result *Result,
) error {
	const stageName = "_dbsync_stage"
	stageStarted := time.Now()
	stageCreateStarted := time.Now()
	if _, err := tx.Exec(ctx, createStageSQL(table, stageName)); err != nil {
		return fmt.Errorf("create temporary staging table: %w", err)
	}
	result.Performance.StageCreateDuration = time.Since(stageCreateStarted)
	copyStarted := time.Now()
	copied, err := tx.CopyFrom(ctx, pgx.Identifier{stageName}, table.TargetColumnNames(), source)
	result.Performance.CopyFromDuration = time.Since(copyStarted)
	if err != nil {
		return fmt.Errorf("copy source rows into staging: %w", err)
	}
	result.CopiedRows = copied
	if err := closeSource(); err != nil {
		return err
	}
	stageVerifyStarted := time.Now()
	if err := verifyCount(ctx, tx, quote(stageName), expectedRows, "staging"); err != nil {
		return err
	}
	result.Performance.StageVerifyDuration = time.Since(stageVerifyStarted)
	if copied != expectedRows {
		return fmt.Errorf("COPY reported %d rows but source count was %d", copied, expectedRows)
	}
	result.StageDuration = time.Since(stageStarted)

	publishStarted := time.Now()
	for index, statement := range publicationSQL(table, stageName) {
		statementStarted := time.Now()
		if _, err := tx.Exec(ctx, statement); err != nil {
			return fmt.Errorf("publish staged rows: %w", err)
		}
		duration := time.Since(statementStarted)
		if publicationStatementIsDelete(table, index) {
			result.Performance.LiveDeleteDuration += duration
		} else {
			result.Performance.LiveInsertDuration += duration
		}
	}
	if err := verifyPublishedCount(ctx, tx, table, expectedRows, result); err != nil {
		return err
	}
	result.PublishDuration = time.Since(publishStarted)
	return nil
}

func verifyPublishedCount(ctx context.Context, tx pgx.Tx, table schema.Table, expectedRows int64, result *Result) error {
	started := time.Now()
	err := verifyCount(ctx, tx, qualifiedName(table.TargetSchema, table.TargetName), expectedRows, "published target")
	result.Performance.FinalVerifyDuration = time.Since(started)
	if err != nil {
		return err
	}
	result.PublishedRows = expectedRows
	return nil
}

func publicationStatementIsDelete(table schema.Table, index int) bool {
	if len(table.PrimaryKeyNames()) == 0 {
		return index == 0
	}
	return index == 1
}
