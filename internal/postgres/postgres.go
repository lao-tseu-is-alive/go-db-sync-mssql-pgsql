// Package postgres validates, stages and atomically publishes one PostgreSQL table refresh.
package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/schema"
	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/stream"
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
	// CopiedRows is the number accepted into the staging table.
	CopiedRows int64
	// PublishedRows is the final live-table count checked before commit.
	PublishedRows int64
	// CreatedTarget reports whether this transaction created the live target.
	CreatedTarget bool
	// StageDuration is the elapsed time for staging and its count verification.
	StageDuration time.Duration
	// PublishDuration is the elapsed time for live publication and its count verification.
	PublishDuration time.Duration
}

// Client owns the PostgreSQL connection pool used for validation and publication.
type Client struct {
	pool *pgxpool.Pool
}

// Open connects to PostgreSQL using the supplied DSN and verifies connectivity.
func Open(ctx context.Context, dsn string) (*Client, error) {
	pool, err := pgxpool.New(ctx, dsn)
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

// Publish copies into staging, verifies it, and refreshes the live table in one transaction.
func (client *Client) Publish(ctx context.Context, table schema.Table, source stream.Source, expectedRows int64) (result Result, returnErr error) {
	if err := table.Validate(); err != nil {
		_ = source.Close()
		return Result{}, fmt.Errorf("validate table contract: %w", err)
	}
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

	if err := requireSchema(ctx, tx, table.TargetSchema); err != nil {
		return Result{}, err
	}
	target, err := inspectTarget(ctx, tx, table.TargetSchema, table.TargetName)
	if err != nil {
		return Result{}, err
	}
	if target.exists {
		if err := validateTarget(table, target); err != nil {
			return Result{}, err
		}
	} else {
		if _, err := tx.Exec(ctx, createTableSQL(table)); err != nil {
			return Result{}, fmt.Errorf("create target table: %w", err)
		}
		result.CreatedTarget = true
	}

	const stageName = "_dbsync_stage"
	stageStarted := time.Now()
	if _, err := tx.Exec(ctx, createStageSQL(table, stageName)); err != nil {
		return Result{}, fmt.Errorf("create temporary staging table: %w", err)
	}
	copied, err := tx.CopyFrom(ctx, pgx.Identifier{stageName}, table.TargetColumnNames(), source)
	if err != nil {
		return Result{}, fmt.Errorf("copy source rows into staging: %w", err)
	}
	result.CopiedRows = copied
	if err := source.Close(); err != nil {
		return Result{}, fmt.Errorf("close SQL Server row stream: %w", err)
	}
	sourceClosed = true
	if err := verifyCount(ctx, tx, quote(stageName), expectedRows, "staging"); err != nil {
		return Result{}, err
	}
	if copied != expectedRows {
		return Result{}, fmt.Errorf("COPY reported %d rows but source count was %d", copied, expectedRows)
	}
	result.StageDuration = time.Since(stageStarted)

	publishStarted := time.Now()
	for _, statement := range publicationSQL(table, stageName) {
		if _, err := tx.Exec(ctx, statement); err != nil {
			return Result{}, fmt.Errorf("publish staged rows: %w", err)
		}
	}
	qualified := qualifiedName(table.TargetSchema, table.TargetName)
	if err := verifyCount(ctx, tx, qualified, expectedRows, "published target"); err != nil {
		return Result{}, err
	}
	result.PublishedRows = expectedRows
	result.PublishDuration = time.Since(publishStarted)
	if err := tx.Commit(ctx); err != nil {
		return Result{}, fmt.Errorf("commit PostgreSQL publication: %w", err)
	}
	return result, nil
}
