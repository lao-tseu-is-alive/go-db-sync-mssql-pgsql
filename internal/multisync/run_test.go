package multisync

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lao-tseu-is-alive/go-db-sync-mssql-pgsql/internal/syncer"
)

type workerFunc func(context.Context, syncer.Request, syncer.Report) (syncer.Report, error)

func (function workerFunc) ApplyPrepared(ctx context.Context, request syncer.Request, report syncer.Report) (syncer.Report, error) {
	return function(ctx, request, report)
}

func TestRunBoundsWorkersAndPreservesResultOrder(t *testing.T) {
	var active atomic.Int64
	var maximum atomic.Int64
	var created atomic.Int64
	factory := func(context.Context, int) (Worker, func(), error) {
		created.Add(1)
		return workerFunc(func(_ context.Context, request syncer.Request, report syncer.Report) (syncer.Report, error) {
			current := active.Add(1)
			defer active.Add(-1)
			for {
				previous := maximum.Load()
				if current <= previous || maximum.CompareAndSwap(previous, current) {
					break
				}
			}
			time.Sleep(5 * time.Millisecond)
			report.Duration = time.Millisecond
			return report, nil
		}), func() {}, nil
	}
	jobs := testJobs(8)
	summary, err := Run(context.Background(), jobs, 2, factory)
	if err != nil {
		t.Fatal(err)
	}
	if created.Load() != 2 || maximum.Load() > 2 {
		t.Fatalf("created=%d maximum=%d", created.Load(), maximum.Load())
	}
	if summary.Succeeded != len(jobs) || summary.Failed != 0 || summary.Cancelled != 0 {
		t.Fatalf("summary = %#v", summary)
	}
	for index, result := range summary.Results {
		if result.SourceName != fmt.Sprintf("Table%02d", index) {
			t.Fatalf("result[%d] = %q", index, result.SourceName)
		}
	}
}

func TestRunFailureCancelsRemainingQueue(t *testing.T) {
	factory := func(context.Context, int) (Worker, func(), error) {
		return workerFunc(func(_ context.Context, _ syncer.Request, _ syncer.Report) (syncer.Report, error) {
			return syncer.Report{}, errors.New("copy failed")
		}), func() {}, nil
	}
	summary, err := Run(context.Background(), testJobs(3), 1, factory)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Failed != 1 || summary.Cancelled != 2 || summary.Succeeded != 0 {
		t.Fatalf("summary = %#v", summary)
	}
	if summary.Results[0].Status != StatusFailed {
		t.Fatalf("first result = %#v", summary.Results[0])
	}
	for _, result := range summary.Results[1:] {
		if result.Status != StatusCancelled || result.Err == nil {
			t.Fatalf("queued result = %#v", result)
		}
	}
}

func TestRunPropagatesExternalCancellation(t *testing.T) {
	started := make(chan struct{}, 1)
	factory := func(context.Context, int) (Worker, func(), error) {
		return workerFunc(func(ctx context.Context, _ syncer.Request, _ syncer.Report) (syncer.Report, error) {
			started <- struct{}{}
			<-ctx.Done()
			return syncer.Report{}, ctx.Err()
		}), func() {}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var summary Summary
	var err error
	go func() {
		summary, err = Run(ctx, testJobs(2), 1, factory)
		close(done)
	}()
	<-started
	cancel()
	<-done
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v", err)
	}
	if summary.Cancelled != 2 {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestRunRejectsInvalidWorkerCountBeforeFactory(t *testing.T) {
	called := false
	_, err := Run(context.Background(), testJobs(1), 0, func(context.Context, int) (Worker, func(), error) {
		called = true
		return nil, nil, nil
	})
	if err == nil || called {
		t.Fatalf("Run error=%v called=%t", err, called)
	}
}

func testJobs(count int) []Job {
	jobs := make([]Job, count)
	for index := range jobs {
		name := fmt.Sprintf("Table%02d", index)
		jobs[index] = Job{
			Request:  syncer.Request{SourceTable: name, TargetTable: "table" + fmt.Sprintf("%02d", index)},
			Prepared: syncer.Report{SourceRows: int64(index + 1)},
		}
	}
	return jobs
}
