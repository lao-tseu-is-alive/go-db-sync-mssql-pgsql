package multisync

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/syncer"
)

// Status is the terminal state of one selected table job.
type Status string

const (
	// StatusSucceeded means the table transaction committed successfully.
	StatusSucceeded Status = "succeeded"
	// StatusFailed means the table encountered the error that failed the run.
	StatusFailed Status = "failed"
	// StatusCancelled means cancellation stopped an in-flight or queued table.
	StatusCancelled Status = "cancelled"
)

// Job contains a prevalidated table request and its immutable preparation report.
type Job struct {
	// Request identifies the source, target and publication options.
	Request syncer.Request
	// Prepared is the read-only discovery, compatibility and count result.
	Prepared syncer.Report
}

// Result is the deterministic terminal summary for one table job.
type Result struct {
	// SourceName is the original SQL Server table name.
	SourceName string
	// TargetName is the mapped PostgreSQL table name.
	TargetName string
	// Rows is the expected source row count measured during preparation.
	Rows int64
	// Duration is the table application duration, if the job started.
	Duration time.Duration
	// Status is succeeded, failed or cancelled.
	Status Status
	// Err is the table-specific failure or cancellation cause.
	Err error
	// Report contains application and performance evidence when the job started.
	Report syncer.Report
}

// Summary aggregates one bounded multi-table application run.
type Summary struct {
	// Results contains one result per job in original deterministic order.
	Results []Result
	// Succeeded is the number of committed table transactions.
	Succeeded int
	// Failed is the number of table failures unrelated to cancellation.
	Failed int
	// Cancelled is the number of in-flight or queued tables stopped by cancellation.
	Cancelled int
	// Rows is the sum of rows committed by successful jobs.
	Rows int64
	// Duration is wall-clock application time across all workers.
	Duration time.Duration
}

// Worker applies prepared jobs using its independently owned database clients.
type Worker interface {
	ApplyPrepared(context.Context, syncer.Request, syncer.Report) (syncer.Report, error)
}

// WorkerFactory creates one independently connected worker and its cleanup function.
type WorkerFactory func(context.Context, int) (Worker, func(), error)

type indexedJob struct {
	index int
	job   Job
}

type indexedResult struct {
	index  int
	result Result
}

// Run applies jobs with at most workers concurrent transactions and fails fast.
// The first non-cancellation failure cancels in-flight work and leaves queued jobs
// unstarted; already committed table transactions remain committed.
func Run(ctx context.Context, jobs []Job, workers int, factory WorkerFactory) (Summary, error) {
	started := time.Now()
	if workers < 1 {
		return Summary{}, fmt.Errorf("workers must be at least 1")
	}
	if len(jobs) == 0 {
		return Summary{Duration: time.Since(started)}, nil
	}
	workerCount := min(workers, len(jobs))
	workerSet := make([]Worker, 0, workerCount)
	cleanups := make([]func(), 0, workerCount)
	for index := 0; index < workerCount; index++ {
		worker, cleanup, err := factory(ctx, index)
		if err != nil {
			closeWorkers(cleanups)
			workerErr := fmt.Errorf("create worker %d: %w", index+1, err)
			results := make([]Result, len(jobs))
			for jobIndex, job := range jobs {
				results[jobIndex] = baseResult(job, StatusCancelled, fmt.Errorf("not started: %w", workerErr))
			}
			return summarize(results, time.Since(started)), workerErr
		}
		workerSet = append(workerSet, worker)
		cleanups = append(cleanups, cleanup)
	}
	defer closeWorkers(cleanups)

	runCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	jobQueue := make(chan indexedJob, len(jobs))
	resultQueue := make(chan indexedResult, len(jobs))
	for index, job := range jobs {
		jobQueue <- indexedJob{index: index, job: job}
	}
	close(jobQueue)
	var wait sync.WaitGroup
	for _, worker := range workerSet {
		wait.Add(1)
		go runWorker(runCtx, cancel, worker, jobQueue, resultQueue, &wait)
	}
	wait.Wait()
	close(resultQueue)

	results := make([]Result, len(jobs))
	received := make([]bool, len(jobs))
	for item := range resultQueue {
		results[item.index] = item.result
		received[item.index] = true
	}
	cause := context.Cause(runCtx)
	if cause == nil {
		cause = context.Canceled
	}
	for index, job := range jobs {
		if received[index] {
			continue
		}
		results[index] = baseResult(job, StatusCancelled, fmt.Errorf("not started: %w", cause))
	}
	summary := summarize(results, time.Since(started))
	if ctx.Err() != nil {
		return summary, ctx.Err()
	}
	return summary, nil
}

func runWorker(
	ctx context.Context,
	cancel context.CancelCauseFunc,
	worker Worker,
	jobs <-chan indexedJob,
	results chan<- indexedResult,
	wait *sync.WaitGroup,
) {
	defer wait.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case item, open := <-jobs:
			if !open {
				return
			}
			if ctx.Err() != nil {
				return
			}
			report, err := worker.ApplyPrepared(ctx, item.job.Request, item.job.Prepared)
			status := StatusSucceeded
			if err != nil {
				status = StatusFailed
				if ctx.Err() != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
					status = StatusCancelled
				} else {
					cancel(err)
				}
			}
			result := baseResult(item.job, status, err)
			result.Report = report
			result.Duration = report.Duration
			results <- indexedResult{index: item.index, result: result}
			if err != nil {
				return
			}
		}
	}
}

func baseResult(job Job, status Status, err error) Result {
	return Result{
		SourceName: job.Request.SourceTable,
		TargetName: job.Request.TargetTable,
		Rows:       job.Prepared.SourceRows,
		Status:     status,
		Err:        err,
	}
}

func summarize(results []Result, duration time.Duration) Summary {
	summary := Summary{Results: results, Duration: duration}
	for _, result := range results {
		switch result.Status {
		case StatusSucceeded:
			summary.Succeeded++
			summary.Rows += result.Rows
		case StatusFailed:
			summary.Failed++
		case StatusCancelled:
			summary.Cancelled++
		}
	}
	return summary
}

func closeWorkers(cleanups []func()) {
	for index := len(cleanups) - 1; index >= 0; index-- {
		if cleanups[index] != nil {
			cleanups[index]()
		}
	}
}
